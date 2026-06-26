package blockfrost

import (
	"encoding/json"
	"testing"
)

// TestToProtocolParamsBlockfrostFlatRefScriptCost asserts the required fix:
// BlockFrost's /epochs/{n}/parameters returns ONLY the flat
// "min_fee_ref_script_cost_per_byte" field and NONE of the structured
// "min_fee_reference_scripts_{base,range,multiplier}" keys. The adapter must
// surface that flat value into apollo's backend.ProtocolParameters so apollo can
// price Conway reference-script fees. Before the fix the flat field was dropped
// (no struct field), the structured base stayed 0, and apollo's
// RefScriptFeePerByte() returned 0 -> FeeTooSmallUTxO on any tx using ref scripts.
func TestToProtocolParamsBlockfrostFlatRefScriptCost(t *testing.T) {
	// Representative BlockFrost epoch-parameters fragment. It carries the flat
	// field BlockFrost actually returns and deliberately omits every structured
	// min_fee_reference_scripts_* key, mirroring the live API response.
	raw := `{
		"min_fee_a": 44,
		"min_fee_b": 155381,
		"max_block_size": 90112,
		"max_tx_size": 16384,
		"max_block_header_size": 1100,
		"key_deposit": "2000000",
		"pool_deposit": "500000000",
		"coins_per_utxo_size": "4310",
		"collateral_percent": 150,
		"max_collateral_inputs": 3,
		"maximum_reference_scripts_size": 200000,
		"min_fee_ref_script_cost_per_byte": 15
	}`

	var p bfProtocolParams
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("failed to unmarshal BlockFrost params: %v", err)
	}

	// The flat field must be parsed off the wire...
	if p.MinFeeRefScriptCostPerByte != 15 {
		t.Fatalf(
			"expected bfProtocolParams.MinFeeRefScriptCostPerByte == 15, got %v",
			p.MinFeeRefScriptCostPerByte,
		)
	}
	// ...and the absent structured field must stay zero.
	if p.MinFeeReferenceScriptsBase != 0 {
		t.Fatalf(
			"expected absent MinFeeReferenceScriptsBase == 0, got %d",
			p.MinFeeReferenceScriptsBase,
		)
	}

	pp, err := p.toProtocolParams()
	if err != nil {
		t.Fatalf("toProtocolParams failed: %v", err)
	}

	// The conversion must carry the flat value into apollo's params...
	if pp.MinFeeRefScriptCostPerByte != 15 {
		t.Fatalf(
			"expected backend MinFeeRefScriptCostPerByte == 15, got %v",
			pp.MinFeeRefScriptCostPerByte,
		)
	}
	// ...without inventing a structured base.
	if pp.MinFeeReferenceScriptsBase != 0 {
		t.Fatalf(
			"expected backend MinFeeReferenceScriptsBase == 0, got %d",
			pp.MinFeeReferenceScriptsBase,
		)
	}

	// End-to-end: apollo's reference-script pricing now resolves to the flat
	// value (structured base is 0, so RefScriptFeePerByte() falls back to it).
	if got := pp.RefScriptFeePerByte(); got != 15 {
		t.Fatalf("expected RefScriptFeePerByte() == 15 (flat fallback), got %v", got)
	}
}

// TestToProtocolParamsStructuredRefScriptBaseWins documents that the existing
// structured-field mapping is kept intact: when a provider supplies a non-zero
// min_fee_reference_scripts_base, apollo prefers it over the flat field. This
// guards against the fix accidentally regressing the structured path.
func TestToProtocolParamsStructuredRefScriptBaseWins(t *testing.T) {
	raw := `{
		"min_fee_reference_scripts_base": 44,
		"min_fee_reference_scripts_range": 25600,
		"min_fee_reference_scripts_multiplier": 1,
		"min_fee_ref_script_cost_per_byte": 15
	}`

	var p bfProtocolParams
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}

	pp, err := p.toProtocolParams()
	if err != nil {
		t.Fatalf("toProtocolParams failed: %v", err)
	}

	if pp.MinFeeReferenceScriptsBase != 44 {
		t.Fatalf(
			"expected structured base 44 to be mapped, got %d",
			pp.MinFeeReferenceScriptsBase,
		)
	}
	if pp.MinFeeRefScriptCostPerByte != 15 {
		t.Fatalf(
			"expected flat field 15 to still be mapped, got %v",
			pp.MinFeeRefScriptCostPerByte,
		)
	}
	// Structured base wins over the flat fallback.
	if got := pp.RefScriptFeePerByte(); got != 44 {
		t.Fatalf(
			"expected RefScriptFeePerByte() == 44 (structured preferred), got %v",
			got,
		)
	}
}
