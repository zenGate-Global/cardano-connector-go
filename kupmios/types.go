package kupmios

import (
	"github.com/SundaeSwap-finance/kugo"
	"github.com/SundaeSwap-finance/ogmigo/v6"
)

type KupmiosProvider struct {
	ogmigoClient *ogmigo.Client
	kugoClient   *kugo.Client
	networkId    int
}

type Config struct {
	OgmigoEndpoint string
	KugoEndpoint   string
	NetworkId      int
}

type OgmiosScript struct {
	Language string `json:"language"`
	CBOR     string `json:"cbor"`
}

type Lovelace struct {
	Lovelace uint64 `json:"lovelace"`
}

type AdaValue struct {
	Ada Lovelace `json:"ada"`
}

type Prices struct {
	Memory string `json:"memory"`
	Cpu    string `json:"cpu"`
}

type Bytes struct {
	Bytes uint64 `json:"bytes"`
}

type Version struct {
	Major uint64 `json:"major"`
	Minor uint64 `json:"minor"`
	Patch uint64 `json:"patch,omitempty"`
}

type MinFeeReferenceScripts struct {
	Range      uint64  `json:"range"`
	Base       float64 `json:"base"`
	Multiplier float64 `json:"multiplier"`
}

type CommitteeThresholds struct {
	Default             string `json:"default"`
	StateOfNoConfidence string `json:"stateOfNoConfidence"`
}

type DRepProtocolParametersUpdateThresholds struct {
	Economic   string `json:"economic"`
	Governance string `json:"governance"`
	Network    string `json:"network"`
	Technical  string `json:"technical"`
}

type DelegateRepresentativeVotingThresholds struct {
	Constitution             string                                 `json:"constitution"`
	ConstitutionalCommittee  CommitteeThresholds                    `json:"constitutionalCommittee"`
	HardForkInitiation       string                                 `json:"hardForkInitiation"`
	NoConfidence             string                                 `json:"noConfidence"`
	ProtocolParametersUpdate DRepProtocolParametersUpdateThresholds `json:"protocolParametersUpdate"`
	TreasuryWithdrawals      string                                 `json:"treasuryWithdrawals"`
}

type SPProtocolParametersUpdateThresholds struct {
	Security string `json:"security"`
}

type StakePoolVotingThresholds struct {
	ConstitutionalCommittee  CommitteeThresholds                  `json:"constitutionalCommittee"`
	HardForkInitiation       string                               `json:"hardForkInitiation"`
	NoConfidence             string                               `json:"noConfidence"`
	ProtocolParametersUpdate SPProtocolParametersUpdateThresholds `json:"protocolParametersUpdate"`
}

type OgmiosProtocolParameters struct {
	MinFeeConstant                  AdaValue               `json:"minFeeConstant"`
	MinFeeCoefficient               uint64                 `json:"minFeeCoefficient"`
	MaxBlockBodySize                Bytes                  `json:"maxBlockBodySize"`
	MaxTransactionSize              Bytes                  `json:"maxTransactionSize"`
	MaxBlockHeaderSize              Bytes                  `json:"maxBlockHeaderSize"`
	StakeCredentialDeposit          AdaValue               `json:"stakeCredentialDeposit"`
	StakePoolDeposit                AdaValue               `json:"stakePoolDeposit"`
	StakePoolPledgeInfluence        string                 `json:"stakePoolPledgeInfluence"`
	MonetaryExpansion               string                 `json:"monetaryExpansion"`
	TreasuryExpansion               string                 `json:"treasuryExpansion"`
	ExtraEntropy                    *string                `json:"extraEntropy,omitempty"`
	MaxValueSize                    Bytes                  `json:"maxValueSize"`
	ScriptExecutionPrices           Prices                 `json:"scriptExecutionPrices"`
	MinUtxoDepositCoefficient       uint64                 `json:"minUtxoDepositCoefficient"`
	MinUtxoDepositConstant          AdaValue               `json:"minUtxoDepositConstant"`
	MinStakePoolCost                AdaValue               `json:"minStakePoolCost"`
	MaxExecutionUnitsPerTransaction ogmigo.ExUnitsBudget   `json:"maxExecutionUnitsPerTransaction"`
	MaxExecutionUnitsPerBlock       ogmigo.ExUnitsBudget   `json:"maxExecutionUnitsPerBlock"`
	CollateralPercentage            uint64                 `json:"collateralPercentage"`
	MaxCollateralInputs             uint64                 `json:"maxCollateralInputs"`
	MaxReferenceScriptsSize         Bytes                  `json:"maxReferenceScriptsSize"`
	MinFeeReferenceScripts          MinFeeReferenceScripts `json:"minFeeReferenceScripts"`
	Version                         Version                `json:"version"`
	PlutusCostModels                map[string][]int64     `json:"plutusCostModels"`

	ConstitutionalCommitteeMaxTermLength   uint64                                 `json:"constitutionalCommitteeMaxTermLength"`
	ConstitutionalCommitteeMinSize         uint64                                 `json:"constitutionalCommitteeMinSize"`
	DelegateRepresentativeDeposit          AdaValue                               `json:"delegateRepresentativeDeposit"`
	DelegateRepresentativeMaxIdleTime      uint64                                 `json:"delegateRepresentativeMaxIdleTime"`
	DelegateRepresentativeVotingThresholds DelegateRepresentativeVotingThresholds `json:"delegateRepresentativeVotingThresholds"`
	DesiredNumberOfStakePools              uint64                                 `json:"desiredNumberOfStakePools"`
	GovernanceActionDeposit                AdaValue                               `json:"governanceActionDeposit"`
	GovernanceActionLifetime               uint64                                 `json:"governanceActionLifetime"`
	StakePoolRetirementEpochBound          uint64                                 `json:"stakePoolRetirementEpochBound"`
	StakePoolVotingThresholds              StakePoolVotingThresholds              `json:"stakePoolVotingThresholds"`
}

type ShelleyGenesisParams struct {
	StartTime              string `json:"startTime"`
	NetworkMagic           int    `json:"networkMagic"`
	ActiveSlotsCoefficient string `json:"activeSlotsCoefficient"` // fraction like "1/20"
	SecurityParameter      int    `json:"securityParameter"`
	EpochLength            int    `json:"epochLength"`
	SlotsPerKesPeriod      int    `json:"slotsPerKesPeriod"`
	MaxKesEvolutions       int    `json:"maxKesEvolutions"`
	SlotLength             struct {
		Milliseconds int `json:"milliseconds"`
	} `json:"slotLength"`
	UpdateQuorum      int   `json:"updateQuorum"`
	MaxLovelaceSupply int64 `json:"maxLovelaceSupply"`
}
