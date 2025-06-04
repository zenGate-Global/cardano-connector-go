package blockfrost

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/Salvionied/apollo/serialization"
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
	connector "github.com/zenGate-Global/cardano-connector-go"
)

// adaptBlockfrostAddressUTxOs converts Blockfrost address UTxOs to Apollo UTxOs
func adaptBlockfrostAddressUTxOs(
	bfUtxos []BlockfrostUTXO,
	originalAddress string,
	ctx context.Context,
	b *BlockfrostProvider,
) []UTxO.UTxO {
	utxos := make([]UTxO.UTxO, 0, len(bfUtxos))
	for _, bfUtxo := range bfUtxos {
		decodedTxID, err := hex.DecodeString(bfUtxo.TxHash)
		if err != nil {
			continue
		}
		txIn := TransactionInput.TransactionInput{
			TransactionId: decodedTxID,
			Index:         bfUtxo.OutputIndex,
		}

		var lovelaceAmount int64
		multiAssets := make(MultiAsset.MultiAsset[int64])

		for _, item := range bfUtxo.Amount {
			quantity, err := strconv.ParseInt(item.Quantity, 10, 64)
			if err != nil {
				continue
			}
			if item.Unit == "lovelace" {
				lovelaceAmount = quantity
			} else {
				policyIDHex := item.Unit[:56]
				assetNameHex := item.Unit[56:]
				policyID := Policy.PolicyId{Value: policyIDHex}
				assetName := AssetName.NewAssetNameFromHexString(assetNameHex)
				if _, ok := multiAssets[policyID]; !ok {
					multiAssets[policyID] = make(Asset.Asset[int64])
				}
				multiAssets[policyID][*assetName] = quantity
			}
		}

		var val Value.Value
		if len(multiAssets) > 0 {
			val = Value.Value{
				Am: Amount.Amount{
					Coin:  lovelaceAmount,
					Value: multiAssets,
				},
				HasAssets: true,
			}
		} else {
			val = Value.Value{Coin: lovelaceAmount, HasAssets: false}
		}

		isPostAlonzo := bfUtxo.DataHash != "" || bfUtxo.InlineDatum != "" ||
			bfUtxo.ReferenceScriptHash != ""

		addr, err := Address.DecodeAddress(originalAddress)
		if err != nil {
			continue
		}

		var txOut TransactionOutput.TransactionOutput

		if isPostAlonzo {
			txOut = TransactionOutput.TransactionOutput{
				IsPostAlonzo: true,
				PostAlonzo: TransactionOutput.TransactionOutputAlonzo{
					Address: addr,
					Amount:  val.ToAlonzoValue(),
					Datum:   nil,
				},
			}

			if bfUtxo.DataHash != "" {
				dhBytes, err := hex.DecodeString(bfUtxo.DataHash)
				if err == nil {
					datumOpt := PlutusData.DatumOptionHash(dhBytes)
					txOut.PostAlonzo.Datum = &datumOpt
				}
			}

			if bfUtxo.InlineDatum != "" {
				datumBytes, err := hex.DecodeString(bfUtxo.InlineDatum)
				if err == nil {
					var pd PlutusData.PlutusData
					if cbor.Unmarshal(datumBytes, &pd) == nil {
						datumOpt := PlutusData.DatumOptionInline(&pd)
						txOut.PostAlonzo.Datum = &datumOpt
					}
				}
			}

			if bfUtxo.ReferenceScriptHash != "" {
				scriptCbor, err := b.GetScriptCborByScriptHash(
					ctx,
					bfUtxo.ReferenceScriptHash,
				)
				if err != nil {
					continue
				}
				scriptCborBytes, err := hex.DecodeString(scriptCbor)
				if err != nil {
					continue
				}
				scriptRefContent := PlutusData.ScriptRef(scriptCborBytes)
				txOut.PostAlonzo.ScriptRef = &scriptRefContent
			}
		} else {
			txOut = TransactionOutput.TransactionOutput{
				IsPostAlonzo: false,
				PreAlonzo: TransactionOutput.TransactionOutputShelley{
					Address:   addr,
					Amount:    val,
					DatumHash: serialization.DatumHash{},
					HasDatum:  false,
				},
			}
		}

		utxos = append(utxos, UTxO.UTxO{Input: txIn, Output: txOut})
	}
	return utxos
}

// adaptBlockfrostTxOutputToApolloUTxO converts a Blockfrost transaction output to Apollo UTxO
func adaptBlockfrostTxOutputToApolloUTxO(
	txHashStr string,
	bfOut Base.Output,
	ctx context.Context,
	b *BlockfrostProvider,
) (UTxO.UTxO, error) {
	txHashBytes, err := hex.DecodeString(txHashStr)
	if err != nil {
		return UTxO.UTxO{}, fmt.Errorf("invalid tx_hash hex: %w", err)
	}
	input := TransactionInput.TransactionInput{
		TransactionId: txHashBytes,
		Index:         bfOut.OutputIndex,
	}

	addr, err := Address.DecodeAddress(bfOut.Address)
	if err != nil {
		return UTxO.UTxO{}, fmt.Errorf(
			"failed to decode address %s: %w",
			bfOut.Address,
			err,
		)
	}

	var lovelaceAmount int64
	multiAssets := make(MultiAsset.MultiAsset[int64])
	for _, item := range bfOut.Amount {
		quantity, errConv := strconv.ParseInt(item.Quantity, 10, 64)
		if errConv != nil {
			return UTxO.UTxO{}, fmt.Errorf(
				"invalid quantity %s for unit %s: %w",
				item.Quantity,
				item.Unit,
				errConv,
			)
		}
		if item.Unit == "lovelace" {
			lovelaceAmount = quantity
		} else {
			policyIDHex := item.Unit[:56]
			assetNameHex := item.Unit[56:]
			policyID := Policy.PolicyId{Value: policyIDHex}
			assetName := AssetName.NewAssetNameFromHexString(assetNameHex)
			if _, ok := multiAssets[policyID]; !ok {
				multiAssets[policyID] = make(Asset.Asset[int64])
			}
			multiAssets[policyID][*assetName] = quantity
		}
	}
	var val Value.Value
	if len(multiAssets) > 0 {
		val = Value.Value{
			Am:        Amount.Amount{Coin: lovelaceAmount, Value: multiAssets},
			HasAssets: true,
		}
	} else {
		val = Value.Value{Coin: lovelaceAmount, HasAssets: false}
	}

	var output TransactionOutput.TransactionOutput

	// Determine era based on presence of datum-related fields and script references:
	// Pre-Alonzo: No datum-related field at all
	// Alonzo-era: Has DataHash but no InlineDatum or ReferenceScriptHash
	// Babbage-era: Has InlineDatum or ReferenceScriptHash
	isPostAlonzo := bfOut.DataHash != "" || bfOut.InlineDatum != "" ||
		bfOut.ReferenceScriptHash != ""

	if isPostAlonzo {
		// Post-Alonzo era (Alonzo or Babbage)
		output = TransactionOutput.TransactionOutput{
			IsPostAlonzo: true,
			PostAlonzo: TransactionOutput.TransactionOutputAlonzo{
				Address: addr,
				Amount:  val.ToAlonzoValue(),
				Datum:   nil,
			},
		}

		// Handle datum hash (Alonzo-era feature)
		if bfOut.DataHash != "" {
			dhBytes, err := hex.DecodeString(bfOut.DataHash)
			if err == nil {
				datumOpt := PlutusData.DatumOptionHash(dhBytes)
				output.PostAlonzo.Datum = &datumOpt
			}
		}

		// Handle inline datum (Babbage-era feature) - takes precedence over datum hash
		if bfOut.InlineDatum != "" {
			decoded, err := hex.DecodeString(bfOut.InlineDatum)
			if err != nil {
				return UTxO.UTxO{}, fmt.Errorf(
					"failed to decode inline datum: %w",
					err,
				)
			}
			var plutusData PlutusData.PlutusData
			err = cbor.Unmarshal(decoded, &plutusData)
			if err != nil {
				return UTxO.UTxO{}, fmt.Errorf(
					"failed to unmarshal inline datum: %w",
					err,
				)
			}

			datumOpt := PlutusData.DatumOptionInline(&plutusData)
			output.PostAlonzo.Datum = &datumOpt
		}

		// Handle reference script (Babbage-era feature)
		if bfOut.ReferenceScriptHash != "" {
			scriptCbor, err := b.GetScriptCborByScriptHash(
				ctx,
				bfOut.ReferenceScriptHash,
			)
			if err != nil {
				return UTxO.UTxO{}, fmt.Errorf(
					"failed to get script cbor: %w",
					err,
				)
			}
			scriptCborBytes, err := hex.DecodeString(scriptCbor)
			if err != nil {
				return UTxO.UTxO{}, fmt.Errorf(
					"failed to decode script cbor: %w",
					err,
				)
			}
			scriptRefContent := PlutusData.ScriptRef(scriptCborBytes)
			output.PostAlonzo.ScriptRef = &scriptRefContent
		}
	} else {
		output = TransactionOutput.TransactionOutput{
			IsPostAlonzo: false,
			PreAlonzo: TransactionOutput.TransactionOutputShelley{
				Address:   addr,
				Amount:    val,
				DatumHash: serialization.DatumHash{},
				HasDatum:  false,
			},
		}
	}

	return UTxO.UTxO{Input: input, Output: output}, nil
}

// adaptBlockfrostAccountToDelegation converts Blockfrost account details to connector delegation
func adaptBlockfrostAccountToDelegation(
	bfAcc BlockfrostAccountDetails,
) connector.Delegation {
	rewards := uint64(0)
	if bfAcc.WithdrawableAmount != "" {
		parsedRewards, err := strconv.ParseUint(
			bfAcc.WithdrawableAmount,
			10,
			64,
		)
		if err == nil {
			rewards = parsedRewards
		}
	}

	poolID := ""
	if bfAcc.PoolId != nil { // PoolId is a *string, so check for nil
		poolID = *bfAcc.PoolId
	}

	// Blockfrost's "active" field for an account means it's on-chain.
	// Actual delegation "activeness" to a specific pool is better determined by pool_id being non-null.
	// The `active_epoch` field can also be used to determine if the delegation is current.
	// For simplicity here, if poolId is present, we consider it an active delegation.
	// The definition of "Active" in connector.Delegation might need refinement
	// based on whether it means "account is active" or "delegation to a pool is active".
	// Assuming "delegation to a pool is active":
	delegationActive := poolID != "" && bfAcc.Active

	return connector.Delegation{
		PoolId:  poolID,
		Rewards: rewards,
		Active:  delegationActive, // Or bfAcc.Active if that's the intended meaning
		// Epoch: // bfAcc.ActiveEpoch could be used here if not null
	}
}

// adaptBlockfrostEvalResult converts Blockfrost evaluation result to connector eval redeemers
func adaptBlockfrostEvalResult(
	bfEvalResp bfEvalResult,
) []connector.EvalRedeemer {
	results := make([]connector.EvalRedeemer, 0, len(bfEvalResp.Result))
	for key, units := range bfEvalResp.Result {
		parts := strings.Split(key, ":")
		if len(parts) != 2 {
			continue
		}
		tagStr, indexStr := parts[0], parts[1]
		index, err := strconv.ParseUint(indexStr, 10, 32)
		if err != nil {
			continue
		}
		var tag Redeemer.RedeemerTag
		switch strings.ToLower(tagStr) {
		case "spend":
			tag = Redeemer.SPEND
		case "mint":
			tag = Redeemer.MINT
		case "cert":
			tag = Redeemer.CERT
		case "reward":
			tag = Redeemer.REWARD
		case "withdraw":
			tag = Redeemer.REWARD
		default:
			continue
		}
		mem := units.Memory
		steps := units.Steps
		results = append(results, connector.EvalRedeemer{
			Tag:   tag,
			Index: uint32(index),
			ExUnits: Redeemer.ExecutionUnits{
				Mem:   int64(mem),
				Steps: int64(steps),
			},
		})
	}
	return results
}

// ToBaseParams converts BlockfrostProtocolParameters to Base.ProtocolParameters
func (p BlockfrostProtocolParameters) ToBaseParams() Base.ProtocolParameters {
	costModels := make(map[string][]int64)
	for key, nestedMap := range p.CostModels {
		var values []int64
		for _, value := range nestedMap {
			values = append(values, value)
		}
		costModels[key] = values
	}

	return Base.ProtocolParameters{
		MinFeeConstant:                   p.MinFeeConstant,
		MinFeeCoefficient:                p.MinFeeCoefficient,
		MaxBlockSize:                     p.MaxBlockSize,
		MaxTxSize:                        p.MaxTxSize,
		MaxBlockHeaderSize:               p.MaxBlockHeaderSize,
		KeyDeposits:                      p.KeyDeposits,
		PoolDeposits:                     p.PoolDeposits,
		PooolInfluence:                   p.PooolInfluence,
		MonetaryExpansion:                p.MonetaryExpansion,
		TreasuryExpansion:                p.TreasuryExpansion,
		DecentralizationParam:            p.DecentralizationParam,
		ExtraEntropy:                     p.ExtraEntropy,
		ProtocolMajorVersion:             p.ProtocolMajorVersion,
		ProtocolMinorVersion:             p.ProtocolMinorVersion,
		MinUtxo:                          p.MinUtxo,
		MinPoolCost:                      p.MinPoolCost,
		PriceMem:                         p.PriceMem,
		PriceStep:                        p.PriceStep,
		MaxTxExMem:                       p.MaxTxExMem,
		MaxTxExSteps:                     p.MaxTxExSteps,
		MaxBlockExMem:                    p.MaxBlockExMem,
		MaxBlockExSteps:                  p.MaxBlockExSteps,
		MaxValSize:                       p.MaxValSize,
		CollateralPercent:                p.CollateralPercent,
		MaxCollateralInuts:               p.MaxCollateralInuts,
		CoinsPerUtxoWord:                 p.CoinsPerUtxoWord,
		CoinsPerUtxoByte:                 p.CoinsPerUtxoByte,
		CostModels:                       costModels,
		MaximumReferenceScriptsSize:      p.MaximumReferenceScriptsSize,
		MinFeeReferenceScriptsRange:      p.MinFeeReferenceScriptsRange,
		MinFeeReferenceScriptsBase:       p.MinFeeReferenceScriptsBase,
		MinFeeReferenceScriptsMultiplier: p.MinFeeReferenceScriptsMultiplier,
	}
}
