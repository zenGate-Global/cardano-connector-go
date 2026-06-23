package kupmios

import (
	"github.com/SundaeSwap-finance/kugo"
	"github.com/SundaeSwap-finance/ogmigo/v6"
)

type KupmiosProvider struct {
	ogmigoClient   *ogmigo.Client
	kugoClient     *kugo.Client
	ogmiosEndpoint string
	networkId      int
}

type Config struct {
	OgmigoEndpoint string
	KupoEndpoint   string
	NetworkId      int
}

// ogmiosProtocolParams mirrors the subset of the Ogmios
// queryLedgerState/protocolParameters response that we map onto
// backend.ProtocolParameters.
type ogmiosProtocolParams struct {
	MinFeeCoefficient  int64                  `json:"minFeeCoefficient"`
	MinFeeConstant     ogmiosLovelace         `json:"minFeeConstant"`
	MaxBlockBodySize   ogmiosBytes            `json:"maxBlockBodySize"`
	MaxBlockHeaderSize ogmiosBytes            `json:"maxBlockHeaderSize"`
	MaxTxSize          ogmiosBytes            `json:"maxTransactionSize"`
	StakeKeyDeposit    ogmiosLovelace         `json:"stakeCredentialDeposit"`
	PoolDeposit        ogmiosLovelace         `json:"stakePoolDeposit"`
	MinPoolCost        ogmiosLovelace         `json:"minStakePoolCost"`
	PoolInfluence      string                 `json:"stakePoolPledgeInfluence"`
	MonetaryExpansion  string                 `json:"monetaryExpansion"`
	TreasuryExpansion  string                 `json:"treasuryExpansion"`
	CollateralPercent  int                    `json:"collateralPercentage"`
	MaxCollateral      int                    `json:"maxCollateralInputs"`
	MaxValSize         ogmiosBytes            `json:"maxValueSize"`
	ScriptPrices       ogmiosPrices           `json:"scriptExecutionPrices"`
	MaxTxExUnits       ogmiosExUnits          `json:"maxExecutionUnitsPerTransaction"`
	MaxBlockExUnits    ogmiosExUnits          `json:"maxExecutionUnitsPerBlock"`
	MinUtxoDeposit     int64                  `json:"minUtxoDepositCoefficient"`
	MinUtxoConstant    ogmiosLovelace         `json:"minUtxoDepositConstant"`
	MaxRefScriptsSize  ogmiosBytes            `json:"maxReferenceScriptsSize"`
	MinFeeRefScripts   ogmiosMinFeeRefScripts `json:"minFeeReferenceScripts"`
	Version            ogmiosVersion          `json:"version"`
	CostModels         map[string][]int64     `json:"plutusCostModels"`
}

// ogmiosLovelace models an Ogmios v6 ada amount, which is nested as
// {"ada":{"lovelace":N}}. The Lovelace accessor returns the inner value.
type ogmiosLovelace struct {
	Ada struct {
		Lovelace int64 `json:"lovelace"`
	} `json:"ada"`
}

func (l ogmiosLovelace) lovelace() int64 {
	return l.Ada.Lovelace
}

type ogmiosBytes struct {
	Bytes int `json:"bytes"`
}

type ogmiosPrices struct {
	Memory string `json:"memory"`
	CPU    string `json:"cpu"`
}

type ogmiosExUnits struct {
	Memory int64 `json:"memory"`
	CPU    int64 `json:"cpu"`
}

type ogmiosMinFeeRefScripts struct {
	Range      int     `json:"range"`
	Base       float64 `json:"base"`
	Multiplier float64 `json:"multiplier"`
}

type ogmiosVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

// ogmiosGenesisConfig mirrors the subset of the Ogmios shelley genesis
// configuration that we map onto backend.GenesisParameters.
type ogmiosGenesisConfig struct {
	StartTime         string `json:"startTime"`
	NetworkMagic      int    `json:"networkMagic"`
	EpochLength       int    `json:"epochLength"`
	SlotsPerKesPeriod int    `json:"slotsPerKesPeriod"`
	MaxKesEvolutions  int    `json:"maxKesEvolutions"`
	SecurityParam     int    `json:"securityParameter"`
	UpdateQuorum      int    `json:"updateQuorum"`
	// ActiveSlotsCoefficient is a fraction like "1/20".
	ActiveSlots       string `json:"activeSlotsCoefficient"`
	MaxLovelaceSupply int64  `json:"maxLovelaceSupply"`
	SlotLength        struct {
		Milliseconds int `json:"milliseconds"`
	} `json:"slotLength"`
}
