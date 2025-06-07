package kupmios

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/Amount"
	"github.com/Salvionied/apollo/serialization/Asset"
	"github.com/Salvionied/apollo/serialization/AssetName"
	"github.com/Salvionied/apollo/serialization/MultiAsset"
	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Policy"
	"github.com/Salvionied/apollo/serialization/Redeemer"
	"github.com/Salvionied/apollo/serialization/TransactionInput"
	"github.com/Salvionied/apollo/serialization/TransactionOutput"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/serialization/Value"
	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
	"github.com/Salvionied/cbor/v2"
	"github.com/SundaeSwap-finance/kugo"
	"github.com/SundaeSwap-finance/ogmigo/v6"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/chainsync/num"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/shared"
)

// parseKupoScript converts Kupo's script response to Apollo's ScriptRef
func parseKupoScript(kupoScript *kugo.Script) (*PlutusData.ScriptRef, error) {
	outerCborBytes, err := hex.DecodeString(kupoScript.Script)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to decode outer script hex '%s': %w",
			kupoScript.Script,
			err,
		)
	}

	scriptRefContent := PlutusData.ScriptRef(outerCborBytes)

	return &scriptRefContent, nil
}

// adaptKupoMatchToApolloUTxO converts a kugo.Match to UTxO.UTxO
func adaptKupoMatchToApolloUTxO(
	ctx context.Context,
	kugoClient *kugo.Client,
	match kugo.Match,
	addr string,
) (UTxO.UTxO, error) {
	txHashBytes, err := hex.DecodeString(match.TransactionID)
	if err != nil {
		return UTxO.UTxO{}, fmt.Errorf(
			"invalid tx_hash hex from Kupo '%s': %w",
			match.TransactionID,
			err,
		)
	}
	txIn := TransactionInput.TransactionInput{
		TransactionId: txHashBytes,
		Index:         match.OutputIndex,
	}

	// Convert kugo.Value to Apollo Value.Value
	var lovelaceAmt int64
	multiAssets := make(MultiAsset.MultiAsset[int64])

	for policyHex, assets := range match.Value {
		for assetNameHex, quantityObj := range assets {
			quantity := quantityObj.Int64()
			if policyHex == "ada" && assetNameHex == "lovelace" {
				lovelaceAmt = quantity
			} else {
				an := AssetName.NewAssetNameFromHexString(assetNameHex)
				polID := Policy.PolicyId{Value: policyHex}
				if _, ok := multiAssets[polID]; !ok {
					multiAssets[polID] = make(Asset.Asset[int64])
				}
				multiAssets[polID][*an] = quantity
			}
		}
	}
	apolloValue := Value.Value{}
	if len(multiAssets) > 0 {
		apolloValue.Am = Amount.Amount{Coin: lovelaceAmt, Value: multiAssets}
		apolloValue.HasAssets = true
	} else {
		apolloValue.Coin = lovelaceAmt
		apolloValue.HasAssets = false
	}

	txOut := TransactionOutput.TransactionOutput{IsPostAlonzo: true}
	decodedAddr, err := Address.DecodeAddress(addr)
	if err != nil {
		return UTxO.UTxO{}, fmt.Errorf("invalid address: %w", err)
	}
	txOut.PostAlonzo.Address = decodedAddr
	txOut.PostAlonzo.Amount = apolloValue.ToAlonzoValue()

	// Handle Datum
	if match.DatumHash != "" {
		var datumOpt PlutusData.DatumOption
		if match.DatumType == "inline" ||
			match.DatumType == "datum" { // "datum" means Kupo has the CBOR
			datumCBORHex, datumErr := kugoClient.Datum(ctx, match.DatumHash)
			if datumErr == nil && datumCBORHex != "" {
				datumBytes, hexErr := hex.DecodeString(datumCBORHex)
				if hexErr == nil {
					var pd PlutusData.PlutusData
					if cbor.Unmarshal(datumBytes, &pd) == nil {
						datumOpt = PlutusData.DatumOptionInline(&pd)
						txOut.PostAlonzo.Datum = &datumOpt
					}
				}
			}
		}
		// If still no inline datum but hash exists, set hash
		if txOut.PostAlonzo.Datum == nil {
			dhBytes, _ := hex.DecodeString(match.DatumHash)
			datumOpt = PlutusData.DatumOptionHash(dhBytes)
			txOut.PostAlonzo.Datum = &datumOpt
		}
	}

	if match.ScriptHash != "" {
		script, scriptErr := kugoClient.Script(ctx, match.ScriptHash)
		if scriptErr == nil && script != nil {
			scriptRef, parseErr := parseKupoScript(script)
			if parseErr == nil {
				txOut.PostAlonzo.ScriptRef = scriptRef
			}
		}
	}

	return UTxO.UTxO{Input: txIn, Output: txOut}, nil
}

func adaptApolloUtxoToOgmigo(u UTxO.UTxO) shared.Utxo {
	txID := hex.EncodeToString(u.Input.TransactionId)

	addressStr := u.Output.PostAlonzo.Address.String()

	value := value_ApolloToOgmigo(u.Output.PostAlonzo.Amount)

	var datumStr, datumHashStr string

	if datumHash := u.Output.GetDatumHash(); datumHash != nil &&
		datumHash.Payload != nil &&
		len(datumHash.Payload) > 0 {
		datumHashStr = hex.EncodeToString(datumHash.Payload)
	}

	if u.Output.IsPostAlonzo && u.Output.PostAlonzo.Datum != nil {
		if inlineDatum := u.Output.GetDatum(); inlineDatum != nil {
			datumCbor, err := cbor.Marshal(inlineDatum)
			if err == nil {
				datumStr = hex.EncodeToString(datumCbor)
				datumHashStr = ""
			}
		}
	}

	var scriptJson json.RawMessage
	if scriptRef := u.Output.GetScriptRef(); scriptRef != nil {
		scriptCbor, err := cbor.Marshal(scriptRef)
		if err == nil {
			scriptHex := hex.EncodeToString(scriptCbor)

			// TODO: Determine script language
			language := "plutus:v2"

			ogmiosScript := OgmiosScript{
				Language: language,
				CBOR:     scriptHex,
			}

			jsonBytes, err := json.Marshal(ogmiosScript)
			if err == nil {
				scriptJson = json.RawMessage(jsonBytes)
			}
		}
	}

	return shared.Utxo{
		Transaction: shared.UtxoTxID{ID: txID},
		Index:       uint32(u.Input.Index),
		Address:     addressStr,
		Value:       value,
		DatumHash:   datumHashStr,
		Datum:       datumStr,
		Script:      scriptJson,
	}
}

func value_ApolloToOgmigo(v Value.AlonzoValue) shared.Value {
	result := make(map[string]map[string]num.Int)

	adaAmount := v.Coin
	if v.HasAssets {
		adaAmount = v.Am.Coin
	}

	adaBigInt := big.NewInt(adaAmount)
	result["ada"] = map[string]num.Int{
		"lovelace": num.Int(*adaBigInt),
	}

	if v.HasAssets {
		for policyID, assets := range v.Am.Value {
			policyStr := policyID.Value
			if result[policyStr] == nil {
				result[policyStr] = make(map[string]num.Int)
			}
			for assetName, quantity := range assets {
				assetNameHex := assetName.HexString()
				quantityBigInt := big.NewInt(quantity)
				result[policyStr][assetNameHex] = num.Int(*quantityBigInt)
			}
		}
	}

	return shared.Value(result)
}

func ratio(s string) float32 {
	n, d, ok := strings.Cut(s, "/")
	if !ok {
		return 0
	}
	num, err := strconv.Atoi(n)
	if err != nil {
		return 0
	}
	den, err := strconv.Atoi(d)
	if err != nil {
		return 0
	}
	return float32(num) / float32(den)
}

func adaptOgmigoProtocolParamsToConnectorParams(
	ogmiosParams OgmiosProtocolParameters,
) Base.ProtocolParameters {
	return Base.ProtocolParameters{
		MinFeeConstant:     int(ogmiosParams.MinFeeConstant.Ada.Lovelace),
		MinFeeCoefficient:  int(ogmiosParams.MinFeeCoefficient),
		MaxBlockSize:       int(ogmiosParams.MaxBlockBodySize.Bytes),
		MaxTxSize:          int(ogmiosParams.MaxTransactionSize.Bytes),
		MaxBlockHeaderSize: int(ogmiosParams.MaxBlockHeaderSize.Bytes),
		KeyDeposits: strconv.FormatUint(
			ogmiosParams.StakeCredentialDeposit.Ada.Lovelace,
			10,
		),
		PoolDeposits: strconv.FormatUint(
			ogmiosParams.StakePoolDeposit.Ada.Lovelace,
			10,
		),
		PooolInfluence:    ratio(ogmiosParams.StakePoolPledgeInfluence),
		MonetaryExpansion: ratio(ogmiosParams.MonetaryExpansion),
		TreasuryExpansion: ratio(ogmiosParams.TreasuryExpansion),
		// Unsure if ogmios reports this, but it's 0 on mainnet and preview
		DecentralizationParam: 0,
		ExtraEntropy:          "",
		MinUtxo: strconv.FormatUint(
			ogmiosParams.MinUtxoDepositConstant.Ada.Lovelace,
			10,
		),
		ProtocolMajorVersion: int(ogmiosParams.Version.Major),
		ProtocolMinorVersion: int(ogmiosParams.Version.Minor),
		MinPoolCost: strconv.FormatUint(
			ogmiosParams.MinStakePoolCost.Ada.Lovelace,
			10,
		),
		PriceMem:  ratio(ogmiosParams.ScriptExecutionPrices.Memory),
		PriceStep: ratio(ogmiosParams.ScriptExecutionPrices.Cpu),
		MaxTxExMem: strconv.FormatUint(
			ogmiosParams.MaxExecutionUnitsPerTransaction.Memory,
			10,
		),
		MaxTxExSteps: strconv.FormatUint(
			ogmiosParams.MaxExecutionUnitsPerTransaction.Cpu,
			10,
		),
		MaxBlockExMem: strconv.FormatUint(
			ogmiosParams.MaxExecutionUnitsPerBlock.Memory,
			10,
		),
		MaxBlockExSteps: strconv.FormatUint(
			ogmiosParams.MaxExecutionUnitsPerBlock.Cpu,
			10,
		),
		MaxValSize: strconv.FormatUint(
			ogmiosParams.MaxValueSize.Bytes,
			10,
		),
		CollateralPercent: int(
			ogmiosParams.CollateralPercentage,
		),
		MaxCollateralInuts: int(ogmiosParams.MaxCollateralInputs),
		CoinsPerUtxoByte: strconv.FormatUint(
			ogmiosParams.MinUtxoDepositCoefficient,
			10,
		),
		CoinsPerUtxoWord: strconv.FormatUint(
			ogmiosParams.MinUtxoDepositCoefficient,
			10,
		),
		MaximumReferenceScriptsSize: int(
			ogmiosParams.MaxReferenceScriptsSize.Bytes,
		),
		MinFeeReferenceScriptsRange: int(
			ogmiosParams.MinFeeReferenceScripts.Range,
		),
		MinFeeReferenceScriptsBase: int(
			ogmiosParams.MinFeeReferenceScripts.Base,
		),
		MinFeeReferenceScriptsMultiplier: int(
			ogmiosParams.MinFeeReferenceScripts.Multiplier,
		),
		CostModels: ogmiosParams.PlutusCostModels,
	}
}

func multiAsset_OgmigoToApollo(
	m map[string]map[string]num.Int,
) MultiAsset.MultiAsset[int64] {
	if len(m) == 0 {
		return nil
	}
	assetMap := make(map[Policy.PolicyId]Asset.Asset[int64])
	for policy, tokens := range m {
		tokensMap := make(map[AssetName.AssetName]int64)
		for token, amt := range tokens {
			tok := *AssetName.NewAssetNameFromHexString(token)
			tokensMap[tok] = amt.Int64()
		}
		pol := Policy.PolicyId{
			Value: policy,
		}
		assetMap[pol] = make(map[AssetName.AssetName]int64)
		assetMap[pol] = tokensMap
	}
	return assetMap
}

func value_OgmigoToApollo(v shared.Value) Value.AlonzoValue {
	ass := multiAsset_OgmigoToApollo(v.AssetsExceptAda())
	if ass == nil {
		return Value.AlonzoValue{
			Am:        Amount.AlonzoAmount{},
			Coin:      v.AdaLovelace().Int64(),
			HasAssets: false,
		}
	}
	return Value.AlonzoValue{
		Am: Amount.AlonzoAmount{
			Coin:  v.AdaLovelace().Int64(),
			Value: ass,
		},
		Coin:      0,
		HasAssets: true,
	}
}

func datum_OgmigoToApollo(d string, dh string) *PlutusData.DatumOption {
	if d != "" {
		datumBytes, err := hex.DecodeString(d)
		if err != nil {
			log.Fatal(
				err,
				"OgmiosChainContext: Failed to decode datum from hex: %v",
				d,
			)
		}
		var pd PlutusData.PlutusData
		err = cbor.Unmarshal(datumBytes, &pd)
		if err != nil {
			log.Fatal(
				err,
				"OgmiosChainContext: datum is not valid plutus data: %v",
				d,
			)
		}
		res := PlutusData.DatumOptionInline(&pd)
		return &res
	}
	if dh != "" {
		datumHashBytes, err := hex.DecodeString(dh)
		if err != nil {
			log.Fatal(
				err,
				"OgmiosChainContext: Failed to decode datum hash from hex: %v",
				dh,
			)
		}
		res := PlutusData.DatumOptionHash(datumHashBytes)
		return &res
	}
	return nil
}

func scriptRef_OgmigoToApollo(
	script json.RawMessage,
) (*PlutusData.ScriptRef, error) {
	if len(script) == 0 {
		return nil, nil
	}

	var ogmiosScript OgmiosScript
	if err := json.Unmarshal(script, &ogmiosScript); err != nil {
		return nil, err
	}

	scriptBytes, err := hex.DecodeString(ogmiosScript.CBOR)
	if err != nil {
		return nil, fmt.Errorf("failed to decode script CBOR hex: %w", err)
	}

	scriptRef := PlutusData.ScriptRef(scriptBytes)
	return &scriptRef, nil
}

func adaptOgmigoEvalResult(
	ogmigoEval *ogmigo.EvaluateTxResponse,
) (map[string]Redeemer.ExecutionUnits, error) {
	if ogmigoEval.Error != nil {
		fmt.Printf(
			"Ogmios evaluation error (code %d): %s\n",
			ogmigoEval.Error.Code,
			ogmigoEval.Error.Message,
		)
		return nil, fmt.Errorf(
			"ogmios evaluation error (code %d): %s",
			ogmigoEval.Error.Code,
			ogmigoEval.Error.Message,
		)
	}

	if len(ogmigoEval.ExUnits) == 0 {
		return nil, errors.New(
			"ogmios evaluation error: No execution units returned",
		)
	}

	results := make(map[string]Redeemer.ExecutionUnits)
	for _, e := range ogmigoEval.ExUnits {
		purpose := strings.ToLower(e.Validator.Purpose)
		switch purpose {
		case "spend", "mint":
			// These are already in the correct format
		case "certificate", "cert":
			purpose = "certificate"
		case "withdrawal", "reward":
			purpose = "withdrawal"
		default:
			continue
		}

		results[fmt.Sprintf("%s:%d", purpose, e.Validator.Index)] = Redeemer.ExecutionUnits{
			Mem:   int64(e.Budget.Memory),
			Steps: int64(e.Budget.Cpu),
		}
	}
	return results, nil
}

func adaptOgmigoUtxoToApollo(u shared.Utxo) UTxO.UTxO {
	txHashRaw, _ := hex.DecodeString(u.Transaction.ID)
	addr, _ := Address.DecodeAddress(u.Address)

	datum := datum_OgmigoToApollo(u.Datum, u.DatumHash)
	v := value_OgmigoToApollo(u.Value)
	scriptRef, _ := scriptRef_OgmigoToApollo(u.Script)

	return UTxO.UTxO{
		Input: TransactionInput.TransactionInput{
			TransactionId: txHashRaw,
			Index:         int(u.Index),
		},
		Output: TransactionOutput.TransactionOutput{
			PostAlonzo: TransactionOutput.TransactionOutputAlonzo{
				Address:   addr,
				Amount:    v,
				Datum:     datum,
				ScriptRef: scriptRef,
			},
			PreAlonzo:    TransactionOutput.TransactionOutputShelley{},
			IsPostAlonzo: true,
		},
	}
}

func adaptShelleyGenesisToConnectorParams(
	shelley ShelleyGenesisParams,
) Base.GenesisParameters {
	// Convert startTime to Unix timestamp
	var systemStart int
	if startTime, err := time.Parse("2006-01-02T15:04:05Z", shelley.StartTime); err == nil {
		systemStart = int(startTime.Unix())
	}

	// Parse active slots coefficient fraction (e.g., "1/20" -> 0.05)
	activeSlotsCoeff := parseFraction(shelley.ActiveSlotsCoefficient)

	// Convert slot length from milliseconds to seconds
	slotLength := shelley.SlotLength.Milliseconds / 1000

	return Base.GenesisParameters{
		ActiveSlotsCoefficient: activeSlotsCoeff,
		UpdateQuorum:           shelley.UpdateQuorum,
		MaxLovelaceSupply: strconv.FormatInt(
			shelley.MaxLovelaceSupply,
			10,
		),
		NetworkMagic:      shelley.NetworkMagic,
		EpochLength:       shelley.EpochLength,
		SystemStart:       systemStart,
		SlotsPerKesPeriod: shelley.SlotsPerKesPeriod,
		SlotLength:        slotLength,
		MaxKesEvolutions:  shelley.MaxKesEvolutions,
		SecurityParam:     shelley.SecurityParameter,
	}
}

func parseFraction(fraction string) float32 {
	if fraction == "" {
		return 0
	}

	parts := strings.Split(fraction, "/")
	if len(parts) != 2 {
		return 0
	}

	numerator, err1 := strconv.ParseFloat(parts[0], 32)
	denominator, err2 := strconv.ParseFloat(parts[1], 32)

	if err1 != nil || err2 != nil || denominator == 0 {
		return 0
	}

	return float32(numerator / denominator)
}
