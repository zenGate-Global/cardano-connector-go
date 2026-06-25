package maestro

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/Salvionied/apollo/v2/backend"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
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
		ScriptExecutionPrices:    models.StringExUnits{Memory: "577/10000", Cpu: "721/10000000"},
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

// TestAdaptMaestroProtocolParamsMapsRefScriptFields asserts that the Conway
// reference-script fee parameters Maestro supplies live (base/range and the
// maximum reference-script size) are mapped into the apollo backend params, and
// that the multiplier is intentionally left zero so apollo applies its 1.2
// default (apollo's field is an int and cannot hold Maestro's fractional 1.2).
func TestAdaptMaestroProtocolParamsMapsRefScriptFields(t *testing.T) {
	data := baseMaestroParams()
	data.PlutusCostModels = map[string]any{}
	data.MinFeeReferenceScripts = models.MinFeeReferenceScripts{
		Base:       15,
		Range:      25600,
		Multiplier: 1.2,
	}
	data.MaxReferenceScriptsSize = models.BytesSize{Bytes: 204800}

	pp, err := adaptMaestroProtocolParams(data)
	if err != nil {
		t.Fatalf("adaptMaestroProtocolParams failed: %v", err)
	}

	if pp.MinFeeReferenceScriptsBase != 15 {
		t.Errorf("MinFeeReferenceScriptsBase = %d, want 15", pp.MinFeeReferenceScriptsBase)
	}
	if pp.MinFeeReferenceScriptsRange != 25600 {
		t.Errorf("MinFeeReferenceScriptsRange = %d, want 25600", pp.MinFeeReferenceScriptsRange)
	}
	if pp.MaximumReferenceScriptsSize != 204800 {
		t.Errorf("MaximumReferenceScriptsSize = %d, want 204800", pp.MaximumReferenceScriptsSize)
	}
	// Multiplier must stay zero: Maestro's 1.2 does not fit apollo's int field,
	// so the adapter leaves it unset and apollo falls back to its 1.2 default.
	if pp.MinFeeReferenceScriptsMultiplier != 0 {
		t.Errorf("MinFeeReferenceScriptsMultiplier = %d, want 0", pp.MinFeeReferenceScriptsMultiplier)
	}

	// Merging against the package preset must preserve the live values: the
	// preset's ref-script fields are all zero (the multiplier preset bug that
	// forced 15 is fixed), so live data always wins.
	preset, err := resolveProtocolParamsPreset("mainnet")
	if err != nil {
		t.Fatalf("resolveProtocolParamsPreset failed: %v", err)
	}
	merged := mergeMaestroProtocolParams(pp, preset)
	if merged.MinFeeReferenceScriptsBase != 15 {
		t.Errorf("merged MinFeeReferenceScriptsBase = %d, want 15", merged.MinFeeReferenceScriptsBase)
	}
	if merged.MinFeeReferenceScriptsRange != 25600 {
		t.Errorf("merged MinFeeReferenceScriptsRange = %d, want 25600", merged.MinFeeReferenceScriptsRange)
	}
	if merged.MaximumReferenceScriptsSize != 204800 {
		t.Errorf("merged MaximumReferenceScriptsSize = %d, want 204800", merged.MaximumReferenceScriptsSize)
	}
	if merged.MinFeeReferenceScriptsMultiplier != 0 {
		t.Errorf("merged MinFeeReferenceScriptsMultiplier = %d, want 0 (apollo default 1.2)", merged.MinFeeReferenceScriptsMultiplier)
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

// TestUnwrapMaestroScriptCborDoubleWrapped asserts that a double-CBOR-wrapped
// script (Maestro's form) has exactly one byte-string layer stripped, yielding
// the canonical single-wrapped form.
func TestUnwrapMaestroScriptCborDoubleWrapped(t *testing.T) {
	// Canonical single-wrapped Plutus script: a CBOR byte string wrapping raw
	// (flat) UPLC bytes.
	uplc := []byte{0x01, 0x00, 0x00, 0x22, 0x00, 0x11}
	single, err := cbor.Encode(uplc)
	if err != nil {
		t.Fatalf("encode single: %v", err)
	}
	// Maestro's double wrap: an outer byte string around the canonical form.
	double, err := cbor.Encode(single)
	if err != nil {
		t.Fatalf("encode double: %v", err)
	}

	got, err := unwrapMaestroScriptCbor(hex.EncodeToString(double))
	if err != nil {
		t.Fatalf("unwrapMaestroScriptCbor failed: %v", err)
	}
	if got != hex.EncodeToString(single) {
		t.Fatalf("double-wrapped: got %s, want canonical single form %s", got, hex.EncodeToString(single))
	}
}

// TestUnwrapMaestroScriptCborSingleWrappedUnchanged asserts that a canonical
// single-wrapped script (the form Blockfrost/Kupo/the fixture use) is returned
// unchanged, NOT over-stripped to raw UPLC.
func TestUnwrapMaestroScriptCborSingleWrappedUnchanged(t *testing.T) {
	// Raw UPLC must not begin with a CBOR byte-string major type (0x40-0x5f);
	// 0x01 is a small unsigned int header, which is what real flat UPLC starts
	// with, so it is correctly detected as "not a byte string".
	uplc := []byte{0x01, 0x00, 0x00, 0x22, 0x00, 0x11}
	single, err := cbor.Encode(uplc)
	if err != nil {
		t.Fatalf("encode single: %v", err)
	}
	singleHex := hex.EncodeToString(single)

	got, err := unwrapMaestroScriptCbor(singleHex)
	if err != nil {
		t.Fatalf("unwrapMaestroScriptCbor failed: %v", err)
	}
	if got != singleHex {
		t.Fatalf("single-wrapped form must be unchanged: got %s, want %s", got, singleHex)
	}
}

// TestMaestroAdditionalUtxos asserts that resolved UTxOs are converted into the
// Maestro additional_utxos wire form: tx_hash + index from the input, and
// txout_cbor = the canonical CBOR of the resolved output (round-trips back to an
// equivalent output).
func TestMaestroAdditionalUtxos(t *testing.T) {
	const txHashHex = "b50e73e74a3073bc44f555928702c0ae0f555a43f1afdce34b3294247dce022d"
	const lovelace uint64 = 11977490
	address, err := common.NewAddress("addr_test1wpgexmeunzsykesf42d4eqet5yvzeap6trjnflxqtkcf66g0kpnxt")
	if err != nil {
		t.Fatalf("NewAddress failed: %v", err)
	}
	txHashBytes, err := hex.DecodeString(txHashHex)
	if err != nil {
		t.Fatalf("decode tx hash: %v", err)
	}
	var txId common.Blake2b256
	copy(txId[:], txHashBytes)
	utxo := common.Utxo{
		Id: shelley.ShelleyTransactionInput{TxId: txId, OutputIndex: 0},
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: address,
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: lovelace},
		},
	}

	addl, err := maestroAdditionalUtxos([]common.Utxo{utxo})
	if err != nil {
		t.Fatalf("maestroAdditionalUtxos failed: %v", err)
	}
	if len(addl) != 1 {
		t.Fatalf("expected 1 additional utxo, got %d", len(addl))
	}
	got := addl[0]

	if got.TxHash != txHashHex {
		t.Errorf("tx_hash: got %s, want %s", got.TxHash, txHashHex)
	}
	if got.Index != 0 {
		t.Errorf("index: got %d, want 0", got.Index)
	}

	// txout_cbor must be the canonical output CBOR: decode it back (era-generic)
	// and confirm it matches the original output's address and lovelace.
	outBytes, err := hex.DecodeString(got.TxoutCbor)
	if err != nil {
		t.Fatalf("invalid txout_cbor hex: %v", err)
	}
	decoded, err := ledger.NewTransactionOutputFromCbor(outBytes)
	if err != nil {
		t.Fatalf("txout_cbor did not decode as an output: %v", err)
	}
	if decoded.Address().String() != address.String() {
		t.Errorf("decoded address mismatch: %s != %s", decoded.Address().String(), address.String())
	}
	if decoded.Amount().Uint64() != lovelace {
		t.Errorf("decoded lovelace mismatch: %d != %d", decoded.Amount().Uint64(), lovelace)
	}
}

// TestMaestroAdditionalUtxosEmpty asserts an empty input yields no additional
// UTxOs and no error.
func TestMaestroAdditionalUtxosEmpty(t *testing.T) {
	addl, err := maestroAdditionalUtxos(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addl != nil {
		t.Fatalf("expected nil, got %+v", addl)
	}
}

// TestParseRedeemerPurposeWdrl asserts the Conway short tag "wdrl" (which Maestro
// emits for withdrawal redeemers) maps to the reward/withdraw tag.
func TestParseRedeemerPurposeWdrl(t *testing.T) {
	for _, s := range []string{"wdrl", "WDRL", "withdrawal", "withdraw", "reward"} {
		tag, err := parseRedeemerPurpose(s)
		if err != nil {
			t.Fatalf("parseRedeemerPurpose(%q) failed: %v", s, err)
		}
		if tag != common.RedeemerTagReward {
			t.Errorf("parseRedeemerPurpose(%q) = %v, want RedeemerTagReward", s, tag)
		}
	}
	for _, s := range []string{"certificate", "cert", "publish"} {
		tag, err := parseRedeemerPurpose(s)
		if err != nil {
			t.Fatalf("parseRedeemerPurpose(%q) failed: %v", s, err)
		}
		if tag != common.RedeemerTagCert {
			t.Errorf("parseRedeemerPurpose(%q) = %v, want RedeemerTagCert", s, tag)
		}
	}
}
