package maestro

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/Salvionied/apollo/v2/backend"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/maestro-org/go-sdk/models"
)

const maestroTestAddr = "addr_test1wpgexmeunzsykesf42d4eqet5yvzeap6trjnflxqtkcf66g0kpnxt"

// TestMaestroUtxoToCommonPrefersTxOutCbor asserts that when Maestro supplies the
// resolved output CBOR (txout_cbor, requested via params.WithCbor()), the output
// is decoded era-generically from those bytes rather than reconstructed from the
// JSON fields.
func TestMaestroUtxoToCommonPrefersTxOutCbor(t *testing.T) {
	address, err := common.NewAddress(maestroTestAddr)
	if err != nil {
		t.Fatalf("NewAddress failed: %v", err)
	}
	addrBytes, err := address.Bytes()
	if err != nil {
		t.Fatalf("address.Bytes failed: %v", err)
	}

	// Minimal Babbage (map-encoded) transaction output: {0: address, 1: coin}.
	const lovelace uint64 = 2_000_000
	outBytes, err := cbor.Encode(map[int]any{0: addrBytes, 1: lovelace})
	if err != nil {
		t.Fatalf("encode output: %v", err)
	}

	raw := models.Utxo{
		TxHash:    "b50e73e74a3073bc44f555928702c0ae0f555a43f1afdce34b3294247dce022d",
		Index:     0,
		Address:   maestroTestAddr,
		TxOutCbor: hex.EncodeToString(outBytes),
		// Intentionally leave Assets/Datum empty: the CBOR path must be used and
		// must not depend on the JSON-field reconstruction.
	}

	utxo, err := maestroUtxoToCommon(raw, address)
	if err != nil {
		t.Fatalf("maestroUtxoToCommon failed: %v", err)
	}
	if got := utxo.Output.Amount(); got == nil || got.Uint64() != lovelace {
		t.Fatalf("decoded lovelace = %v, want %d", got, lovelace)
	}
	if utxo.Id.Index() != 0 {
		t.Fatalf("decoded index = %d, want 0", utxo.Id.Index())
	}
}

// TestMaestroUtxoToCommonFallsBackToJSONFields asserts that when txout_cbor is
// absent, the output is reconstructed from the JSON asset/datum fields.
func TestMaestroUtxoToCommonFallsBackToJSONFields(t *testing.T) {
	address, err := common.NewAddress(maestroTestAddr)
	if err != nil {
		t.Fatalf("NewAddress failed: %v", err)
	}

	raw := models.Utxo{
		TxHash:  "b50e73e74a3073bc44f555928702c0ae0f555a43f1afdce34b3294247dce022d",
		Index:   0,
		Address: maestroTestAddr,
		Assets:  []models.Asset{{Unit: "lovelace", Amount: 3_000_000}},
	}

	utxo, err := maestroUtxoToCommon(raw, address)
	if err != nil {
		t.Fatalf("maestroUtxoToCommon failed: %v", err)
	}
	if got := utxo.Output.Amount(); got == nil || got.Uint64() != 3_000_000 {
		t.Fatalf("decoded lovelace = %v, want 3000000", got)
	}
}

// baseMaestroParams returns a models.ProtocolParams populated with the minimal
// scalar/fraction fields that adaptMaestroProtocolParams requires (so the
// fraction and version parsing does not error), leaving cost models to the
// individual test to set.
func baseMaestroParams() models.ProtocolParams {
	return models.ProtocolParams{
		ScriptExecutionPrices:    models.StringExUnits{Memory: "577/10000", Steps: "721/10000000"},
		StakePoolPledgeInfluence: "3/10",
		MonetaryExpansion:        "3/1000",
		TreasuryExpansion:        "1/5",
		ProtocolVersion:          models.ProtocolVersion{Major: 10, Minor: 0},
	}
}

// TestAdaptMaestroProtocolParamsCostModelsArrayShape asserts that array-encoded
// cost-model vectors are parsed into the canonical PlutusVN keys with their
// values preserved in order.
func TestAdaptMaestroProtocolParamsCostModelsArrayShape(t *testing.T) {
	data := baseMaestroParams()
	data.PlutusCostModels = map[string]any{
		"plutus:v1": []any{float64(100), float64(200), float64(300)},
		"plutus:v2": []any{float64(1), float64(2)},
	}

	pp, err := adaptMaestroProtocolParams(data)
	if err != nil {
		t.Fatalf("adaptMaestroProtocolParams failed: %v", err)
	}

	want := map[string][]int64{
		"PlutusV1": {100, 200, 300},
		"PlutusV2": {1, 2},
	}
	if !reflect.DeepEqual(pp.CostModels, want) {
		t.Fatalf("cost models = %+v, want %+v", pp.CostModels, want)
	}
}

// TestAdaptMaestroProtocolParamsRejectsMapEncodedCostModels asserts that a
// map-encoded cost-model vector (keyed parameter names instead of an ordered
// array) is rejected rather than silently dropped or mis-ordered.
func TestAdaptMaestroProtocolParamsRejectsMapEncodedCostModels(t *testing.T) {
	data := baseMaestroParams()
	data.PlutusCostModels = map[string]any{
		"plutus:v1": map[string]any{"addInteger-cpu-arguments-intercept": float64(100)},
	}

	if _, err := adaptMaestroProtocolParams(data); err == nil {
		t.Fatal("expected error for map-encoded cost model vector, got nil")
	}
}

// TestAdaptMaestroProtocolParamsRejectsNonIntegralCost asserts that a
// non-integral cost value is rejected rather than truncated.
func TestAdaptMaestroProtocolParamsRejectsNonIntegralCost(t *testing.T) {
	data := baseMaestroParams()
	data.PlutusCostModels = map[string]any{
		"plutus:v1": []any{float64(1.5)},
	}

	if _, err := adaptMaestroProtocolParams(data); err == nil {
		t.Fatal("expected error for non-integral cost value, got nil")
	}
}

// TestAdaptMaestroProtocolParamsMapsLiveRatioAndVersionFields asserts that the
// ratio/version fields Maestro supplies live (pool influence, monetary/treasury
// expansion, protocol version) are mapped, not left zero for the preset.
func TestAdaptMaestroProtocolParamsMapsLiveRatioAndVersionFields(t *testing.T) {
	data := baseMaestroParams()
	data.PlutusCostModels = map[string]any{}

	pp, err := adaptMaestroProtocolParams(data)
	if err != nil {
		t.Fatalf("adaptMaestroProtocolParams failed: %v", err)
	}

	if pp.PoolInfluence != 0.3 {
		t.Errorf("PoolInfluence = %v, want 0.3", pp.PoolInfluence)
	}
	if pp.MonetaryExpansion != 0.003 {
		t.Errorf("MonetaryExpansion = %v, want 0.003", pp.MonetaryExpansion)
	}
	if pp.TreasuryExpansion != 0.2 {
		t.Errorf("TreasuryExpansion = %v, want 0.2", pp.TreasuryExpansion)
	}
	if pp.ProtocolMajorVersion != 10 {
		t.Errorf("ProtocolMajorVersion = %d, want 10", pp.ProtocolMajorVersion)
	}
}

// Cost-model shape coverage that previously lived in TestNormalizeMaestroCostModels
// (apollo v1) is reimplemented against the v2 types in
// TestAdaptMaestroProtocolParamsCostModels* below, which exercise
// adaptMaestroProtocolParams' inline parsing + maestroCostModelKey directly.
//
// TODO(apollo-v2): The following tests were removed during the apollo v2 /
// gouroboros migration because the code they exercised no longer exists:
//   - TestCBORRoundTripPreservation: relied on apollo v1 TransactionOutput CBOR
//     round-tripping.
//   - TestAdaptApolloUtxosToMaestro_* and the rawCbor cache tests
//     (TestMaestroProvider_CacheRawCbor, TestMaestroProvider_EvaluateUsesCache,
//     TestUtxoCacheKey, TestMaestroRoundTrip_SimulateFullFlow): the additional-
//     UTxO adapter and the rawCbor cache were dropped because the maestro v2
//     EvaluateTx IGNORES additionalUTxOs (SDK type cannot represent the
//     documented wire format). Re-add coverage if/when the SDK gains object-
//     shaped additional_utxos support.

func TestMaestroCostModelKey(t *testing.T) {
	cases := map[string]string{
		"plutus:v1": "PlutusV1",
		"plutus:v2": "PlutusV2",
		"plutus:v3": "PlutusV3",
		"plutus_v1": "PlutusV1",
		"plutus_v2": "PlutusV2",
		"plutus_v3": "PlutusV3",
		"unknown":   "unknown",
	}
	for in, want := range cases {
		if got := maestroCostModelKey(in); got != want {
			t.Errorf("maestroCostModelKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMergeMaestroProtocolParamsUsesPresetForMissingFields(t *testing.T) {
	current := backend.ProtocolParameters{
		MinFeeConstant:   1,
		CoinsPerUtxoByte: "4310",
		CoinsPerUtxoWord: "0",
	}
	preset := backend.ProtocolParameters{
		MinUtxo:                          "4310",
		CoinsPerUtxoWord:                 "4310",
		MinFeeReferenceScriptsMultiplier: 15,
	}

	merged := mergeMaestroProtocolParams(current, preset)

	if merged.MinUtxo != "4310" {
		t.Fatalf("expected MinUtxo to be filled from preset, got %q", merged.MinUtxo)
	}
	if merged.CoinsPerUtxoWord != "4310" {
		t.Fatalf("expected CoinsPerUtxoWord to be filled from preset, got %q", merged.CoinsPerUtxoWord)
	}
	if merged.MinFeeReferenceScriptsMultiplier != 15 {
		t.Fatalf(
			"expected MinFeeReferenceScriptsMultiplier to be filled from preset, got %d",
			merged.MinFeeReferenceScriptsMultiplier,
		)
	}
	if merged.MinFeeConstant != 1 {
		t.Fatalf("expected existing fields to be preserved, got %d", merged.MinFeeConstant)
	}
}
