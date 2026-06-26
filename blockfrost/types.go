package blockfrost

import (
	"encoding/json"
	"net/http"
)

type BlockfrostProvider struct {
	httpClient                *http.Client
	baseURL                   string
	projectID                 string
	networkName               string // e.g., "mainnet", "preprod" (used for default URL)
	networkId                 int
	customSubmissionEndpoints []string
}

// --- BlockFrost evaluate-with-utxos request types ---
//
// /utils/txs/evaluate/utxos proxies Ogmios. The additionalUtxoSet uses the
// [txIn, txOut] pair schema, but the txOut value must be in the Ogmios-v6
// shape: {"ada": {"lovelace": N}, "<policyHex>": {"<assetNameHex>": N}}. (The
// older {coins, assets:{"policyHex.assetNameHex": N}} shape is rejected by the
// proxy with "failed to decode payload from base64 or base16" because the
// "policyHex.assetNameHex" key is not valid base16.) A bare datum hash is
// "datumHash"; reference scripts are
// {"plutus:v1"|"plutus:v2"|"plutus:v3"|"plutus:v4": "<base16 script>"}.

type bfTxIn struct {
	TxId  string `json:"txId"`
	Index int    `json:"index"`
}

// bfValue is the Ogmios-v6 value object. The "ada" entry carries lovelace under
// "lovelace"; every other entry is keyed by policy id hex and maps asset name
// hex (empty string for the empty asset name) to quantity.
type bfValue map[string]map[string]int64

type bfScriptRef struct {
	PlutusV1 *string `json:"plutus:v1,omitempty"`
	PlutusV2 *string `json:"plutus:v2,omitempty"`
	PlutusV3 *string `json:"plutus:v3,omitempty"`
	PlutusV4 *string `json:"plutus:v4,omitempty"`
}

type bfTxOut struct {
	Address string  `json:"address"`
	Value   bfValue `json:"value"`
	// DatumHash uses the Ogmios-v5 camelCase key "datumHash" (a bare datum hash
	// digest); inline datums go under "datum".
	DatumHash *string      `json:"datumHash,omitempty"`
	Datum     *string      `json:"datum,omitempty"`
	ScriptRef *bfScriptRef `json:"script,omitempty"`
}

// bfAdditionalUtxoItem is a [txIn, txOut] pair.
type bfAdditionalUtxoItem [2]interface{}

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

// bfProtocolParams is the BlockFrost /epochs/latest/parameters response.
type bfProtocolParams struct {
	MinFeeA            int64   `json:"min_fee_a"`
	MinFeeB            int64   `json:"min_fee_b"`
	MaxBlockSize       int64   `json:"max_block_size"`
	MaxTxSize          int64   `json:"max_tx_size"`
	MaxBlockHeaderSize int64   `json:"max_block_header_size"`
	KeyDeposit         string  `json:"key_deposit"`
	PoolDeposit        string  `json:"pool_deposit"`
	A0                 float64 `json:"a0"`
	Rho                float64 `json:"rho"`
	Tau                float64 `json:"tau"`
	Decentralisation   float64 `json:"decentralisation_param"`
	ExtraEntropy       string  `json:"extra_entropy"`
	ProtocolMajorVer   int     `json:"protocol_major_ver"`
	ProtocolMinorVer   int     `json:"protocol_minor_ver"`
	MinUtxo            string  `json:"min_utxo"`
	MinPoolCost        string  `json:"min_pool_cost"`
	PriceMem           float64 `json:"price_mem"`
	PriceStep          float64 `json:"price_step"`
	// The execution-unit fields use BlockFrost's actual
	// /epochs/latest/parameters JSON field names (short forms:
	// max_tx_ex_mem, max_tx_ex_steps, max_block_ex_mem, max_block_ex_steps).
	// The live BlockFrost API returns exactly these keys; do not switch to the
	// long forms (e.g. max_tx_execution_units_memory), which unmarshal empty.
	MaxTxExMem        string `json:"max_tx_ex_mem"`
	MaxTxExSteps      string `json:"max_tx_ex_steps"`
	MaxBlockExMem     string `json:"max_block_ex_mem"`
	MaxBlockExSteps   string `json:"max_block_ex_steps"`
	MaxValSize        string `json:"max_val_size"`
	CollateralPercent int64  `json:"collateral_percent"`
	MaxCollateralIn   int64  `json:"max_collateral_inputs"`
	CoinsPerUtxoWord  string `json:"coins_per_utxo_word"`
	CoinsPerUtxoSize  string `json:"coins_per_utxo_size"`
	// CostModels is the named/keyed form ({"PlutusV1": {"addInteger-...": n}}).
	// Its parameter ORDER is NOT the ledger's canonical positional order, so it
	// must not be flattened by sorting parameter names. CostModelsRaw is the
	// array form ({"PlutusV1": [n, ...]}) already in canonical ledger order and
	// is preferred when present (required for correct local script evaluation).
	CostModels    json.RawMessage `json:"cost_models"`
	CostModelsRaw json.RawMessage `json:"cost_models_raw"`

	MaximumReferenceScriptsSize      int `json:"maximum_reference_scripts_size"`
	MinFeeReferenceScriptsRange      int `json:"min_fee_reference_scripts_range"`
	MinFeeReferenceScriptsBase       int `json:"min_fee_reference_scripts_base"`
	MinFeeReferenceScriptsMultiplier int `json:"min_fee_reference_scripts_multiplier"`
	// MinFeeRefScriptCostPerByte is the flat reference-script base price
	// (lovelace per byte) that BlockFrost actually returns in
	// /epochs/{n}/parameters. BlockFrost does NOT send the structured
	// min_fee_reference_scripts_{base,range,multiplier} triple, so those fields
	// above unmarshal to 0; this flat field carries the real value. apollo's
	// RefScriptFeePerByte() falls back to it when the structured base is 0.
	MinFeeRefScriptCostPerByte float64 `json:"min_fee_ref_script_cost_per_byte"`
}

type bfGenesisParams struct {
	ActiveSlotsCoefficient float64 `json:"active_slots_coefficient"`
	UpdateQuorum           int     `json:"update_quorum"`
	NetworkMagic           int     `json:"network_magic"`
	EpochLength            int     `json:"epoch_length"`
	MaxLovelaceSupply      int64   `json:"max_lovelace_supply,string"`
	SystemStart            int64   `json:"system_start"`
	SlotLength             int     `json:"slot_length"`
	SlotsPerKesPeriod      int     `json:"slots_per_kes_period"`
	MaxKesEvolutions       int     `json:"max_kes_evolutions"`
	SecurityParam          int     `json:"security_param"`
}

// bfAddressUTxO is a UTxO as returned by /addresses/{addr}/utxos and
// /txs/{hash}/utxos. InlineDatum is kept as raw JSON so the original CBOR bytes
// are preserved exactly (no JSON decode/re-encode round-trip).
type bfAddressUTxO struct {
	Address             string            `json:"address"`
	TxHash              string            `json:"tx_hash"`
	OutputIndex         int               `json:"output_index"`
	Amount              []bfAddressAmount `json:"amount"`
	Block               string            `json:"block"`
	DataHash            string            `json:"data_hash"`
	InlineDatum         json.RawMessage   `json:"inline_datum"`
	ReferenceScriptHash string            `json:"reference_script_hash"`
}

type bfAddressAmount struct {
	Unit     string `json:"unit"`
	Quantity string `json:"quantity"`
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
