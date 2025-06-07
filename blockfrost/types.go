package blockfrost

import (
	"net/http"

	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
)

type BlockfrostProvider struct {
	httpClient                *http.Client
	baseURL                   string
	projectID                 string
	networkName               string // e.g., "mainnet", "preprod" (used for default URL)
	networkId                 int
	customSubmissionEndpoints []string
}

type bfEvalResult struct {
	Result map[string]bfExecutionUnits `json:"EvaluationResult"`
}

// Updated structures to match the correct Blockfrost API format
type bfTxIn struct {
	TxId  string `json:"txId"`  // Changed from txHash to txId
	Index int    `json:"index"` // Changed from outputIndex to index
}

type bfValue struct {
	Coins  int64            `json:"coins"`
	Assets map[string]int64 `json:"assets,omitempty"`
}

type bfScriptRef struct {
	PlutusV1 *string `json:"plutus:v1,omitempty"`
	PlutusV2 *string `json:"plutus:v2,omitempty"`
	PlutusV3 *string `json:"plutus:v3,omitempty"`
}

type bfTxOut struct {
	Address   string       `json:"address"`
	Value     bfValue      `json:"value"` // Changed from Amount to Value
	DatumHash *string      `json:"datum_hash,omitempty"`
	Datum     *string      `json:"datum,omitempty"`
	ScriptRef *bfScriptRef `json:"script,omitempty"`
}

type bfAdditionalUtxoItem [2]interface{}

type bfExecutionUnits struct {
	Memory int `json:"memory"`
	Steps  int `json:"steps"`
}

type bfEvalRequest struct {
	Cbor              string                 `json:"cbor"`
	AdditionalUtxoSet []bfAdditionalUtxoItem `json:"additionalUtxoSet"`
}

type bfScriptCbor struct {
	ScriptCbor string `json:"cbor"`
}

type Config struct {
	ProjectID                 string
	NetworkName               string // e.g., "mainnet", "preprod", "preview"
	NetworkId                 int
	BaseURL                   string // Optional: if you need to override default Blockfrost URL
	HTTPClient                *http.Client
	CustomSubmissionEndpoints []string // For custom tx submission
}

type BlockfrostAccountDetails struct {
	StakeAddress       string  `json:"stake_address"`
	Active             bool    `json:"active"`
	ActiveEpoch        *int    `json:"active_epoch"` // Nullable
	ControlledAmount   string  `json:"controlled_amount"`
	RewardsSum         string  `json:"rewards_sum"`
	WithdrawalsSum     string  `json:"withdrawals_sum"`
	ReservesSum        string  `json:"reserves_sum"`
	TreasurySum        string  `json:"treasury_sum"`
	WithdrawableAmount string  `json:"withdrawable_amount"` // This is key for rewards
	PoolId             *string `json:"pool_id"`             // Nullable; this is the delegation target
	DeRepId            *string `json:"derep_id"`            // Nullable; this is the delegation target
}

// BlockfrostProtocolParameters represents the protocol parameters response from Blockfrost API
type BlockfrostProtocolParameters struct {
	MinFeeConstant                   int                         `json:"min_fee_b"`
	MinFeeCoefficient                int                         `json:"min_fee_a"`
	MaxBlockSize                     int                         `json:"max_block_size"`
	MaxTxSize                        int                         `json:"max_tx_size"`
	MaxBlockHeaderSize               int                         `json:"max_block_header_size"`
	KeyDeposits                      string                      `json:"key_deposit"`
	PoolDeposits                     string                      `json:"pool_deposit"`
	PooolInfluence                   float32                     `json:"a0"`
	MonetaryExpansion                float32                     `json:"rho"`
	TreasuryExpansion                float32                     `json:"tau"`
	DecentralizationParam            float32                     `json:"decentralisation_param"`
	ExtraEntropy                     string                      `json:"extra_entropy"`
	ProtocolMajorVersion             int                         `json:"protocol_major_ver"`
	ProtocolMinorVersion             int                         `json:"protocol_minor_ver"`
	MinUtxo                          string                      `json:"min_utxo"`
	MinPoolCost                      string                      `json:"min_pool_cost"`
	PriceMem                         float32                     `json:"price_mem"`
	PriceStep                        float32                     `json:"price_step"`
	MaxTxExMem                       string                      `json:"max_tx_ex_mem"`
	MaxTxExSteps                     string                      `json:"max_tx_ex_steps"`
	MaxBlockExMem                    string                      `json:"max_block_ex_mem"`
	MaxBlockExSteps                  string                      `json:"max_block_ex_steps"`
	MaxValSize                       string                      `json:"max_val_size"`
	CollateralPercent                int                         `json:"collateral_percent"`
	MaxCollateralInuts               int                         `json:"max_collateral_inputs"`
	CoinsPerUtxoWord                 string                      `json:"coins_per_utxo_word"`
	CoinsPerUtxoByte                 string                      `json:"coins_per_utxo_byte"`
	CostModels                       map[string]map[string]int64 `json:"cost_models"`
	MaximumReferenceScriptsSize      int                         `json:"maximum_reference_scripts_size"`
	MinFeeReferenceScriptsRange      int                         `json:"min_fee_reference_scripts_range"`
	MinFeeReferenceScriptsBase       int                         `json:"min_fee_reference_scripts_base"`
	MinFeeReferenceScriptsMultiplier int                         `json:"min_fee_reference_scripts_multiplier"`
}

type BlockfrostGenesisParameters struct {
	ActiveSlotsCoefficient float32 `json:"active_slots_coefficient"`
	UpdateQuorum           int     `json:"update_quorum"`
	MaxLovelaceSupply      string  `json:"max_lovelace_supply"`
	NetworkMagic           int     `json:"network_magic"`
	EpochLength            int     `json:"epoch_length"`
	SystemStart            int     `json:"system_start"`
	SlotsPerKesPeriod      int     `json:"slots_per_kes_period"`
	SlotLength             int     `json:"slot_length"`
	MaxKesEvolutions       int     `json:"max_kes_evolutions"`
	SecurityParam          int     `json:"security_param"`
}

type BlockfrostUTXO struct {
	// Transaction hash of the UTXO
	Address string `json:"address"`
	TxHash  string `json:"tx_hash"`

	// UTXO index in the transaction
	OutputIndex int                  `json:"output_index"`
	Amount      []Base.AddressAmount `json:"amount"`

	// Block hash of the UTXO
	Block string `json:"block"`

	// The hash of the transaction output datum
	DataHash            string `json:"data_hash"`
	InlineDatum         string `json:"inline_datum"`
	ReferenceScriptHash string `json:"reference_script_hash"`
}

type BlockfrostEpoch struct {
	Epoch          int    `json:"epoch"`
	StartTime      int64  `json:"start_time"`
	EndTime        int64  `json:"end_time"`
	FirstBlockTime int64  `json:"first_block_time"`
	LastBlockTime  int64  `json:"last_block_time"`
	BlockCount     int    `json:"block_count"`
	TxCount        int    `json:"tx_count"`
	Output         string `json:"output"`
	Fees           string `json:"fees"`
	ActiveStake    string `json:"active_stake"`
}
