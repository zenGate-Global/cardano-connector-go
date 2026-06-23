package maestro

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/Salvionied/apollo/v2/backend"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/maestro-org/go-sdk/models"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

// adaptMaestroProtocolParams converts Maestro protocol parameters into the
// apollo v2 backend.ProtocolParameters shape.
func adaptMaestroProtocolParams(
	data models.ProtocolParams,
) (backend.ProtocolParameters, error) {
	// Script execution prices, as fraction strings (e.g. "577/10000" and
	// "721/10000000"). The SDK exposes the step price under .Cpu (the JSON key
	// Maestro actually returns).
	priceMem, err := backend.ParseFraction(data.ScriptExecutionPrices.Memory)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf("invalid memory price: %w", err)
	}
	priceStep, err := backend.ParseFraction(data.ScriptExecutionPrices.Cpu)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf("invalid step price: %w", err)
	}

	// These ratio/version fields are supplied live by Maestro (as fraction
	// strings and a {major,minor} object) and must be mapped here rather than
	// left zero for the preset to fill.
	poolInfluence, err := backend.ParseFraction(data.StakePoolPledgeInfluence)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf("invalid pool pledge influence: %w", err)
	}
	monetaryExpansion, err := backend.ParseFraction(data.MonetaryExpansion)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf("invalid monetary expansion: %w", err)
	}
	treasuryExpansion, err := backend.ParseFraction(data.TreasuryExpansion)
	if err != nil {
		return backend.ProtocolParameters{}, fmt.Errorf("invalid treasury expansion: %w", err)
	}
	protocolMajor, err := backend.BoundedInt(data.ProtocolVersion.Major, "protocol major version")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	protocolMinor, err := backend.BoundedInt(data.ProtocolVersion.Minor, "protocol minor version")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}

	maxBlockSize, err := backend.BoundedInt(data.MaxBlockBodySize.Bytes, "max block body size")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	maxTxSize, err := backend.BoundedInt(data.MaxTransactionSize.Bytes, "max transaction size")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	maxBlockHeaderSize, err := backend.BoundedInt(data.MaxBlockHeaderSize.Bytes, "max block header size")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	collateralPercent, err := backend.BoundedInt(data.CollateralPercentage, "collateral percentage")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}
	maxCollateralInputs, err := backend.BoundedInt(data.MaxCollateralInputs, "max collateral inputs")
	if err != nil {
		return backend.ProtocolParameters{}, err
	}

	pp := backend.ProtocolParameters{
		MinFeeCoefficient:    data.MinFeeCoefficient,
		MinFeeConstant:       data.MinFeeConstant.LovelaceAmount.Lovelace,
		MaxBlockSize:         maxBlockSize,
		MaxTxSize:            maxTxSize,
		MaxBlockHeaderSize:   maxBlockHeaderSize,
		KeyDeposits:          strconv.FormatInt(data.StakeCredentialDeposit.LovelaceAmount.Lovelace, 10),
		PoolDeposits:         strconv.FormatInt(data.StakePoolDeposit.LovelaceAmount.Lovelace, 10),
		MinPoolCost:          strconv.FormatInt(data.MinStakePoolCost.LovelaceAmount.Lovelace, 10),
		MaxTxExMem:           strconv.FormatInt(data.MaxExecutionUnitsPerTransaction.Memory, 10),
		MaxTxExSteps:         strconv.FormatInt(data.MaxExecutionUnitsPerTransaction.Steps, 10),
		MaxBlockExMem:        strconv.FormatInt(data.MaxExecutionUnitsPerBlock.Memory, 10),
		MaxBlockExSteps:      strconv.FormatInt(data.MaxExecutionUnitsPerBlock.Steps, 10),
		MaxValSize:           strconv.FormatInt(data.MaxValueSize.Bytes, 10),
		CollateralPercent:    collateralPercent,
		MaxCollateralInputs:  maxCollateralInputs,
		CoinsPerUtxoByte:     strconv.FormatInt(data.MinUtxoDepositCoefficient, 10),
		PriceMem:             priceMem,
		PriceStep:            priceStep,
		PoolInfluence:        poolInfluence,
		MonetaryExpansion:    monetaryExpansion,
		TreasuryExpansion:    treasuryExpansion,
		ProtocolMajorVersion: protocolMajor,
		ProtocolMinorVersion: protocolMinor,
	}

	// Parse cost models from Maestro response.
	// PlutusCostModels is typed as `any`; when unmarshaled from JSON it is
	// map[string]interface{} with keys like "plutus:v1", "plutus:v2", "plutus:v3"
	// and values that are []interface{} of float64.
	// ComputeScriptDataHash expects keys "PlutusV1", "PlutusV2", "PlutusV3".
	if rawModels, ok := data.PlutusCostModels.(map[string]any); ok {
		pp.CostModels = make(map[string][]int64, len(rawModels))
		for key, val := range rawModels {
			costs, ok := val.([]any)
			if !ok {
				return backend.ProtocolParameters{}, fmt.Errorf("unexpected cost model format for %s: expected []any, got %T", key, val)
			}
			int64Costs := make([]int64, 0, len(costs))
			for i, c := range costs {
				f, ok := c.(float64)
				if !ok {
					return backend.ProtocolParameters{}, fmt.Errorf("cost model %q element %d: expected float64, got %T", key, i, c)
				}
				// Reject non-integral or out-of-int64-range values rather than
				// silently truncating (out-of-range float-to-int conversion is
				// implementation-defined in Go).
				if f != math.Trunc(f) || f < math.MinInt64 || f >= math.MaxInt64 {
					return backend.ProtocolParameters{}, fmt.Errorf("cost model %q element %d: value %v is not a valid int64", key, i, f)
				}
				int64Costs = append(int64Costs, int64(f))
			}
			pp.CostModels[maestroCostModelKey(key)] = int64Costs
		}
	}

	return pp, nil
}

// maestroCostModelKey translates Maestro cost model keys to the canonical form
// expected by ComputeScriptDataHash ("PlutusV1", "PlutusV2", "PlutusV3").
func maestroCostModelKey(key string) string {
	switch key {
	case "plutus:v1", "plutus_v1":
		return "PlutusV1"
	case "plutus:v2", "plutus_v2":
		return "PlutusV2"
	case "plutus:v3", "plutus_v3":
		return "PlutusV3"
	default:
		return key
	}
}

// maestroUtxoToCommon converts a Maestro UTxO to a gouroboros common.Utxo.
func maestroUtxoToCommon(raw models.Utxo, address common.Address) (common.Utxo, error) {
	hashBytes, err := hex.DecodeString(raw.TxHash)
	if err != nil {
		return common.Utxo{}, err
	}
	if len(hashBytes) != common.Blake2b256Size {
		return common.Utxo{}, fmt.Errorf("invalid tx hash length: expected %d bytes, got %d", common.Blake2b256Size, len(hashBytes))
	}
	var txId common.Blake2b256
	copy(txId[:], hashBytes)

	if raw.Index < 0 {
		return common.Utxo{}, fmt.Errorf("negative output index: %d", raw.Index)
	}
	if raw.Index > math.MaxUint32 {
		return common.Utxo{}, fmt.Errorf("output index %d exceeds uint32 range", raw.Index)
	}
	input := shelley.ShelleyTransactionInput{
		TxId:        txId,
		OutputIndex: uint32(raw.Index),
	}

	// Prefer the resolved output CBOR when Maestro supplies it (requested via
	// params.WithCbor()). Decoding the on-chain output bytes directly preserves
	// inline datums and reference scripts exactly (era-generic), avoiding the
	// lossy JSON-field reconstruction below. Fall back to the JSON fields when
	// txout_cbor is absent.
	if raw.TxOutCbor != "" {
		outputBytes, err := hex.DecodeString(raw.TxOutCbor)
		if err != nil {
			return common.Utxo{}, fmt.Errorf("invalid txout_cbor hex: %w", err)
		}
		output, err := ledger.NewTransactionOutputFromCbor(outputBytes)
		if err != nil {
			return common.Utxo{}, fmt.Errorf("failed to decode txout_cbor: %w", err)
		}
		return common.Utxo{Id: input, Output: output}, nil
	}

	var lovelace uint64
	assetData := make(map[common.Blake2b224]map[cbor.ByteString]*big.Int)

	for _, asset := range raw.Assets {
		if asset.Unit == "lovelace" {
			if asset.Amount < 0 {
				return common.Utxo{}, fmt.Errorf("negative lovelace amount: %d", asset.Amount)
			}
			lovelace = uint64(asset.Amount) //nolint:gosec // validated non-negative above
		} else {
			if asset.Amount < 0 {
				return common.Utxo{}, fmt.Errorf("negative asset amount %d for unit %s", asset.Amount, asset.Unit)
			}
			policyId, assetName, err := backend.ParseAssetUnit(asset.Unit)
			if err != nil {
				return common.Utxo{}, fmt.Errorf("invalid asset unit %q: %w", asset.Unit, err)
			}

			if _, ok := assetData[policyId]; !ok {
				assetData[policyId] = make(map[cbor.ByteString]*big.Int)
			}
			assetData[policyId][assetName] = big.NewInt(asset.Amount)
		}
	}

	var assets *common.MultiAsset[common.MultiAssetTypeOutput]
	if len(assetData) > 0 {
		ma := common.NewMultiAsset[common.MultiAssetTypeOutput](assetData)
		assets = &ma
	}

	output := babbage.BabbageTransactionOutput{
		OutputAddress: address,
		OutputAmount: mary.MaryTransactionOutputValue{
			Amount: lovelace,
			Assets: assets,
		},
	}

	// Map datum to output's DatumOption.
	// Maestro returns the datum field as a JSON object with keys "type", "hash",
	// "bytes", "json". When unmarshaled into `any` it becomes map[string]interface{}.
	// The "type" discriminator is "hash" or "inline"; Maestro can include
	// resolved datum "bytes" even for type "hash" outputs, so the datum kind
	// must be decided by "type", not by the presence of "bytes".
	if datumMap, ok := raw.Datum.(map[string]any); ok {
		datumType, _ := datumMap["type"].(string)
		switch datumType {
		case "inline":
			datumCborHex, _ := datumMap["bytes"].(string)
			if datumCborHex == "" {
				return common.Utxo{}, errors.New("inline datum is missing its CBOR bytes")
			}
			// Inline datum: "bytes" field contains the CBOR hex of the datum.
			datumBytes, err := hex.DecodeString(datumCborHex)
			if err != nil {
				return common.Utxo{}, fmt.Errorf("invalid inline datum CBOR hex %q: %w", datumCborHex, err)
			}
			cborBytes, err := cbor.Encode([]any{1, cbor.Tag{Number: 24, Content: datumBytes}})
			if err != nil {
				return common.Utxo{}, fmt.Errorf("failed to encode inline datum option: %w", err)
			}
			var opt babbage.BabbageTransactionOutputDatumOption
			if err := opt.UnmarshalCBOR(cborBytes); err != nil {
				return common.Utxo{}, fmt.Errorf("failed to unmarshal inline datum option: %w", err)
			}
			output.DatumOption = &opt
		case "hash":
			hashHex, _ := datumMap["hash"].(string)
			if hashHex == "" {
				return common.Utxo{}, errors.New("hash datum is missing its hash")
			}
			// Datum hash reference only.
			hashBytes, err := hex.DecodeString(hashHex)
			if err != nil {
				return common.Utxo{}, fmt.Errorf("invalid datum hash hex %q: %w", hashHex, err)
			}
			if len(hashBytes) != common.Blake2b256Size {
				return common.Utxo{}, fmt.Errorf("invalid datum hash length: expected %d bytes, got %d", common.Blake2b256Size, len(hashBytes))
			}
			var hash common.Blake2b256
			copy(hash[:], hashBytes)
			cborBytes, err := cbor.Encode([]any{0, hash})
			if err != nil {
				return common.Utxo{}, fmt.Errorf("failed to encode datum option hash: %w", err)
			}
			var opt babbage.BabbageTransactionOutputDatumOption
			if err := opt.UnmarshalCBOR(cborBytes); err != nil {
				return common.Utxo{}, fmt.Errorf("failed to unmarshal datum option: %w", err)
			}
			output.DatumOption = &opt
		default:
			return common.Utxo{}, fmt.Errorf("unsupported maestro datum type %q", datumType)
		}
	}

	// Parse reference script if present, verifying the script bytes against
	// the script hash claimed by Maestro.
	if raw.ReferenceScript.Bytes != "" {
		scriptBytes, err := hex.DecodeString(raw.ReferenceScript.Bytes)
		if err != nil {
			return common.Utxo{}, fmt.Errorf("invalid reference script hex: %w", err)
		}
		ref, err := maestroScriptRef(raw.ReferenceScript.Type, scriptBytes, raw.ReferenceScript.Hash)
		if err != nil {
			return common.Utxo{}, fmt.Errorf("failed to parse reference script: %w", err)
		}
		output.TxOutScriptRef = ref
	}

	return common.Utxo{
		Id:     input,
		Output: &output,
	}, nil
}

// maestroScriptRef builds a ScriptRef from the Maestro script type and CBOR
// bytes. When Maestro supplies the script hash (expectedHashHex non-empty),
// the script bytes are verified against it rather than trusted as-is.
func maestroScriptRef(scriptType string, scriptCbor []byte, expectedHashHex string) (*common.ScriptRef, error) {
	var refType uint
	switch scriptType {
	case "native":
		refType = common.ScriptRefTypeNativeScript
	case "plutusv1":
		refType = common.ScriptRefTypePlutusV1
	case "plutusv2":
		refType = common.ScriptRefTypePlutusV2
	case "plutusv3":
		refType = common.ScriptRefTypePlutusV3
	default:
		return nil, fmt.Errorf("unknown script type %q", scriptType)
	}
	return backend.ScriptRefFromBytes(refType, scriptCbor, expectedHashHex)
}

// parseRedeemerPurpose maps a redeemer purpose string to a gouroboros
// RedeemerTag. backend.ParseRedeemerTag accepts spend/mint/cert/publish/reward/
// withdraw; Maestro emits the Conway short tags (e.g. "wdrl" for withdrawal)
// and some responses use the long spellings ("certificate"/"withdrawal"), so
// those are normalized to the accepted forms first (case-insensitively) before
// delegating.
func parseRedeemerPurpose(purpose string) (common.RedeemerTag, error) {
	switch strings.ToLower(strings.TrimSpace(purpose)) {
	case "certificate":
		return backend.ParseRedeemerTag("cert")
	case "withdrawal", "wdrl":
		return backend.ParseRedeemerTag("withdraw")
	default:
		return backend.ParseRedeemerTag(purpose)
	}
}

// evaluationsToExUnits converts a Maestro evaluate response into a redeemer
// ExUnits map. A response with zero evaluation results is an error: returning
// an empty map with a nil error would let callers silently keep zero
// execution budgets for their redeemers.
func evaluationsToExUnits(evals models.EvaluateTxResponse) (map[common.RedeemerKey]common.ExUnits, error) {
	if len(evals) == 0 {
		return nil, errors.New("script evaluation returned no results")
	}
	result := make(map[common.RedeemerKey]common.ExUnits, len(evals))
	for _, eval := range evals {
		if eval.RedeemerIndex < 0 {
			return nil, fmt.Errorf("negative redeemer index: %d", eval.RedeemerIndex)
		}
		if int64(eval.RedeemerIndex) > math.MaxUint32 {
			return nil, fmt.Errorf("redeemer index %d exceeds uint32 range", eval.RedeemerIndex)
		}
		tag, err := parseRedeemerPurpose(eval.RedeemerTag)
		if err != nil {
			return nil, fmt.Errorf("invalid redeemer tag %q: %w", eval.RedeemerTag, err)
		}
		key := common.RedeemerKey{Tag: tag, Index: uint32(eval.RedeemerIndex)}
		result[key] = common.ExUnits{Memory: eval.ExUnits.Mem, Steps: eval.ExUnits.Steps}
	}
	return result, nil
}

// adaptMaestroDelegation converts Maestro account info to connector delegation.
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
