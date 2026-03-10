package utxorpc

import "github.com/Salvionied/apollo/txBuilding/Backend/Base"

// These scalar defaults were fetched from Blockfrost /epochs/latest/parameters
// for mainnet, preprod, and preview on 2026-03-10. uTxoRPC still supplies live
// protocol parameters and cost models; this preset only fills fields that may
// be missing or zeroed in the current gateway response.
var protocolParamsPreset = Base.ProtocolParameters{
	MinFeeConstant:                   155381,
	MinFeeCoefficient:                44,
	MaxBlockSize:                     90112,
	MaxTxSize:                        16384,
	MaxBlockHeaderSize:               1100,
	KeyDeposits:                      "2000000",
	PoolDeposits:                     "500000000",
	PooolInfluence:                   0.3,
	MonetaryExpansion:                0.003,
	TreasuryExpansion:                0.2,
	DecentralizationParam:            0,
	ExtraEntropy:                     "",
	ProtocolMajorVersion:             10,
	ProtocolMinorVersion:             0,
	MinUtxo:                          "4310",
	MinPoolCost:                      "170000000",
	PriceMem:                         0.0577,
	PriceStep:                        0.0000721,
	MaxTxExMem:                       "16500000",
	MaxTxExSteps:                     "10000000000",
	MaxBlockExMem:                    "72000000",
	MaxBlockExSteps:                  "20000000000",
	MaxValSize:                       "5000",
	CollateralPercent:                150,
	MaxCollateralInuts:               3,
	CoinsPerUtxoWord:                 "4310",
	CoinsPerUtxoByte:                 "4310",
	MaximumReferenceScriptsSize:      0,
	MinFeeReferenceScriptsRange:      0,
	MinFeeReferenceScriptsBase:       0,
	MinFeeReferenceScriptsMultiplier: 15,
}

func mergeProtocolParamsWithPreset(
	current Base.ProtocolParameters,
	preset Base.ProtocolParameters,
) Base.ProtocolParameters {
	if current.MinFeeConstant == 0 {
		current.MinFeeConstant = preset.MinFeeConstant
	}
	if current.MinFeeCoefficient == 0 {
		current.MinFeeCoefficient = preset.MinFeeCoefficient
	}
	if current.MaxBlockSize == 0 {
		current.MaxBlockSize = preset.MaxBlockSize
	}
	if current.MaxTxSize == 0 {
		current.MaxTxSize = preset.MaxTxSize
	}
	if current.MaxBlockHeaderSize == 0 {
		current.MaxBlockHeaderSize = preset.MaxBlockHeaderSize
	}
	if current.KeyDeposits == "" {
		current.KeyDeposits = preset.KeyDeposits
	}
	if current.PoolDeposits == "" {
		current.PoolDeposits = preset.PoolDeposits
	}
	if current.PooolInfluence == 0 {
		current.PooolInfluence = preset.PooolInfluence
	}
	if current.MonetaryExpansion == 0 {
		current.MonetaryExpansion = preset.MonetaryExpansion
	}
	if current.TreasuryExpansion == 0 {
		current.TreasuryExpansion = preset.TreasuryExpansion
	}
	current.DecentralizationParam = preset.DecentralizationParam
	current.ExtraEntropy = preset.ExtraEntropy
	if current.ProtocolMajorVersion == 0 {
		current.ProtocolMajorVersion = preset.ProtocolMajorVersion
	}
	if current.ProtocolMinorVersion == 0 {
		current.ProtocolMinorVersion = preset.ProtocolMinorVersion
	}
	if current.MinUtxo == "" {
		current.MinUtxo = preset.MinUtxo
	}
	if current.MinPoolCost == "" {
		current.MinPoolCost = preset.MinPoolCost
	}
	if current.PriceMem == 0 {
		current.PriceMem = preset.PriceMem
	}
	if current.PriceStep == 0 {
		current.PriceStep = preset.PriceStep
	}
	if current.MaxTxExMem == "" {
		current.MaxTxExMem = preset.MaxTxExMem
	}
	if current.MaxTxExSteps == "" {
		current.MaxTxExSteps = preset.MaxTxExSteps
	}
	if current.MaxBlockExMem == "" {
		current.MaxBlockExMem = preset.MaxBlockExMem
	}
	if current.MaxBlockExSteps == "" {
		current.MaxBlockExSteps = preset.MaxBlockExSteps
	}
	if current.MaxValSize == "" {
		current.MaxValSize = preset.MaxValSize
	}
	if current.CollateralPercent == 0 {
		current.CollateralPercent = preset.CollateralPercent
	}
	if current.MaxCollateralInuts == 0 {
		current.MaxCollateralInuts = preset.MaxCollateralInuts
	}
	if current.CoinsPerUtxoByte == "" || current.CoinsPerUtxoByte == "0" {
		current.CoinsPerUtxoByte = preset.CoinsPerUtxoByte
	}
	if current.CoinsPerUtxoWord == "" || current.CoinsPerUtxoWord == "0" {
		current.CoinsPerUtxoWord = preset.CoinsPerUtxoWord
	}
	if current.MaximumReferenceScriptsSize == 0 {
		current.MaximumReferenceScriptsSize = preset.MaximumReferenceScriptsSize
	}
	if current.MinFeeReferenceScriptsRange == 0 {
		current.MinFeeReferenceScriptsRange = preset.MinFeeReferenceScriptsRange
	}
	if current.MinFeeReferenceScriptsBase == 0 {
		current.MinFeeReferenceScriptsBase = preset.MinFeeReferenceScriptsBase
	}
	if current.MinFeeReferenceScriptsMultiplier == 0 {
		current.MinFeeReferenceScriptsMultiplier = preset.MinFeeReferenceScriptsMultiplier
	}
	return current
}
