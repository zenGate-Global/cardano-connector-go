package maestro

import (
	"encoding/hex"
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
) Base.ProtocolParameters {
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
	// CHECK HERE
	// protocolParams.MinUtxo = ppFromApi.Data.
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
	// protocolParams.CostModels = ppFromApi.Data.CostModels
	protocolParams.MaximumReferenceScriptsSize = 0
	protocolParams.MinFeeReferenceScriptsRange = 0
	protocolParams.MinFeeReferenceScriptsBase = 0
	protocolParams.MinFeeReferenceScriptsMultiplier = 0

	return protocolParams
}

// adaptMaestroUtxoToApolloUtxo converts a Maestro UTxO to Apollo UTxO
func adaptMaestroUtxoToApolloUtxo(mUtxo models.Utxo) (UTxO.UTxO, error) {
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
		return UTxO.UTxO{}, err
	}
	utxo.Output = output

	return utxo, nil
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

// adaptApolloUtxosToMaestro converts Apollo UTxOs to Maestro format for evaluation
func adaptApolloUtxosToMaestro(
	apollo []UTxO.UTxO,
) ([]models.AdditionalUtxo, error) {
	out := make([]models.AdditionalUtxo, 0, len(apollo))
	for _, u := range apollo {
		txHash := hex.EncodeToString(u.Input.TransactionId)

		idx := int64(u.Input.Index)

		raw, err := cbor.Marshal(u.Output)
		if err != nil {
			return nil, fmt.Errorf(
				"cbor-marshal output for %s#%d: %w",
				txHash,
				idx,
				err,
			)
		}
		txoutCbor := hex.EncodeToString(raw)

		out = append(out, models.AdditionalUtxo{
			TxHash:    txHash,
			Index:     int(idx),
			TxoutCbor: txoutCbor,
		})
	}
	return out, nil
}
