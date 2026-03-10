package maestro

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Salvionied/apollo/serialization/TransactionInput"
	"github.com/Salvionied/apollo/serialization/TransactionOutput"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
	"github.com/Salvionied/cbor/v2"
	"github.com/maestro-org/go-sdk/models"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

func parseMaestroFloat(floatString string) float32 {
	if floatString == "" {
		return 0
	}
	splitString := strings.Split(floatString, "/")
	top := splitString[0]
	bottom := splitString[1]
	topFloat, _ := strconv.ParseFloat(top, 32)
	bottomFloat, _ := strconv.ParseFloat(bottom, 32)
	return float32(topFloat / bottomFloat)
}

// adaptMaestroProtocolParams converts Maestro ProtocolParameters to Base.ProtocolParameters
// This function maps the actual Maestro SDK protocol parameters structure based on the user's provided code
func adaptMaestroProtocolParams(
	p models.ProtocolParams,
) (Base.ProtocolParameters, error) {
	protocolParams := Base.ProtocolParameters{}

	// Map ALL the fields
	protocolParams.MinFeeConstant = int(
		p.MinFeeConstant.LovelaceAmount.Lovelace,
	)
	protocolParams.MinFeeCoefficient = int(p.MinFeeCoefficient)
	protocolParams.MaxTxSize = int(p.MaxTransactionSize.Bytes)
	protocolParams.MaxBlockSize = int(p.MaxBlockBodySize.Bytes)
	protocolParams.MaxBlockHeaderSize = int(p.MaxBlockHeaderSize.Bytes)
	protocolParams.KeyDeposits = strconv.FormatInt(
		p.StakeCredentialDeposit.LovelaceAmount.Lovelace,
		10,
	)
	protocolParams.PoolDeposits = strconv.FormatInt(
		p.StakePoolDeposit.LovelaceAmount.Lovelace,
		10,
	)
	parsedPoolInfl, _ := strconv.ParseFloat(p.StakePoolPledgeInfluence, 32)
	protocolParams.PooolInfluence = float32(parsedPoolInfl)
	monExp, _ := strconv.ParseFloat(p.MonetaryExpansion, 32)
	protocolParams.MonetaryExpansion = float32(monExp)
	tresExp, _ := strconv.ParseFloat(p.TreasuryExpansion, 32)
	protocolParams.TreasuryExpansion = float32(tresExp)
	protocolParams.DecentralizationParam = 0
	protocolParams.ExtraEntropy = ""
	protocolParams.ProtocolMajorVersion = int(p.ProtocolVersion.Major)
	protocolParams.ProtocolMinorVersion = int(p.ProtocolVersion.Minor)
	protocolParams.MinPoolCost = strconv.FormatInt(
		p.MinStakePoolCost.LovelaceAmount.Lovelace,
		10,
	)
	protocolParams.PriceMem = parseMaestroFloat(p.ScriptExecutionPrices.Memory)
	protocolParams.PriceStep = parseMaestroFloat(p.ScriptExecutionPrices.Steps)
	protocolParams.MaxTxExMem = strconv.FormatInt(
		p.MaxExecutionUnitsPerTransaction.Memory,
		10,
	)
	protocolParams.MaxTxExSteps = strconv.FormatInt(
		p.MaxExecutionUnitsPerTransaction.Steps,
		10,
	)
	protocolParams.MaxBlockExMem = strconv.FormatInt(
		p.MaxExecutionUnitsPerBlock.Memory,
		10,
	)
	protocolParams.MaxBlockExSteps = strconv.FormatInt(
		p.MaxExecutionUnitsPerBlock.Steps,
		10,
	)
	protocolParams.MaxValSize = strconv.FormatInt(p.MaxValueSize.Bytes, 10)
	protocolParams.CollateralPercent = int(p.CollateralPercentage)
	protocolParams.MaxCollateralInuts = int(p.MaxCollateralInputs)
	protocolParams.CoinsPerUtxoByte = strconv.FormatInt(
		p.MinUtxoDepositCoefficient,
		10,
	)
	protocolParams.CoinsPerUtxoWord = "0"

	costModels, err := normalizeMaestroCostModels(p.PlutusCostModels)
	if err != nil {
		return Base.ProtocolParameters{}, err
	}
	protocolParams.CostModels = costModels
	protocolParams.MaximumReferenceScriptsSize = 0
	protocolParams.MinFeeReferenceScriptsRange = 0
	protocolParams.MinFeeReferenceScriptsBase = 0
	protocolParams.MinFeeReferenceScriptsMultiplier = 0

	return protocolParams, nil
}

func normalizeMaestroCostModels(raw any) (map[string][]int64, error) {
	if raw == nil {
		return nil, errors.New("maestro: protocol parameters are missing plutus_cost_models")
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("maestro: marshal plutus_cost_models: %w", err)
	}

	var decoded map[string][]int64
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("maestro: decode plutus_cost_models: %w", err)
	}

	keyAliases := map[string]string{
		"plutus_v1": "PlutusV1",
		"plutus_v2": "PlutusV2",
		"plutus_v3": "PlutusV3",
		"PlutusV1":  "PlutusV1",
		"PlutusV2":  "PlutusV2",
		"PlutusV3":  "PlutusV3",
	}

	result := make(map[string][]int64, 3)
	for sourceKey, targetKey := range keyAliases {
		values, ok := decoded[sourceKey]
		if !ok {
			continue
		}
		copied := make([]int64, len(values))
		copy(copied, values)
		result[targetKey] = copied
	}

	if len(result) == 0 {
		return nil, errors.New("maestro: protocol parameters contain no supported cost models")
	}

	return result, nil
}

// utxoCacheKey builds the cache key for a UTxO's raw CBOR.
func utxoCacheKey(txHash string, index int) string {
	return txHash + "#" + strconv.Itoa(index)
}

// adaptMaestroUtxoToApolloUtxo converts a Maestro UTxO to Apollo UTxO.
// It returns the decoded Apollo UTxO and the original txout_cbor hex
// so callers can cache it for later re-use.
func adaptMaestroUtxoToApolloUtxo(mUtxo models.Utxo) (UTxO.UTxO, string, error) {
	utxo := UTxO.UTxO{}
	decodedHash, _ := hex.DecodeString(mUtxo.TxHash)
	utxo.Input = TransactionInput.TransactionInput{
		TransactionId: decodedHash,
		Index:         int(mUtxo.Index),
	}
	output := TransactionOutput.TransactionOutput{}
	decodedCbor, _ := hex.DecodeString(mUtxo.TxOutCbor)
	err := cbor.Unmarshal(decodedCbor, &output)
	if err != nil {
		return UTxO.UTxO{}, "", err
	}
	utxo.Output = output

	return utxo, mUtxo.TxOutCbor, nil
}

// adaptMaestroDelegation converts Maestro account info to connector delegation
func adaptMaestroDelegation(
	acc models.AccountInformation,
	epoch int,
) connector.Delegation {
	return connector.Delegation{
		Active:  acc.Registered,
		Rewards: uint64(acc.RewardsAvailable),
		PoolId:  acc.DelegatedPool,
		Epoch:   epoch,
	}
}

// adaptApolloUtxosToMaestro converts Apollo UTxOs to Maestro format for evaluation.
// lookupRawCbor, when non-nil, is called with the cache key ("txhash#index") and
// should return the original txout_cbor hex if available. Using the original bytes
// avoids CBOR re-encoding mismatches that Maestro rejects as "Malformed additional UTxO".
func adaptApolloUtxosToMaestro(
	apollo []UTxO.UTxO,
	lookupRawCbor func(key string) (string, bool),
) ([]models.AdditionalUtxo, error) {
	out := make([]models.AdditionalUtxo, 0, len(apollo))
	for _, u := range apollo {
		txHash := hex.EncodeToString(u.Input.TransactionId)
		idx := u.Input.Index
		key := utxoCacheKey(txHash, idx)

		var txoutCbor string
		if lookupRawCbor != nil {
			if cached, ok := lookupRawCbor(key); ok {
				txoutCbor = cached
			}
		}
		if txoutCbor == "" {
			// Fallback: re-marshal through Apollo (may produce different CBOR)
			raw, err := cbor.Marshal(u.Output)
			if err != nil {
				return nil, fmt.Errorf(
					"cbor-marshal output for %s#%d: %w",
					txHash,
					idx,
					err,
				)
			}
			txoutCbor = hex.EncodeToString(raw)
		}

		out = append(out, models.AdditionalUtxo{
			TxHash:    txHash,
			Index:     idx,
			TxoutCbor: txoutCbor,
		})
	}
	return out, nil
}
