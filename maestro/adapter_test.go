package maestro

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/Salvionied/apollo/serialization/TransactionInput"
	"github.com/Salvionied/apollo/serialization/TransactionOutput"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/txBuilding/Backend/Base"
	"github.com/Salvionied/cbor/v2"
)

// TestCBORRoundTripPreservation demonstrates that CBOR encoding is not
// guaranteed to be stable through an unmarshal/marshal cycle. While Apollo's
// Go-constructed outputs may round-trip cleanly, real on-chain CBOR from
// Maestro can use different encoding choices (integer widths, map key ordering,
// indefinite-length containers) that Apollo's Marshal does not reproduce.
func TestCBORRoundTripPreservation(t *testing.T) {
	utxo := ApolloEvalSample2UTxOs[0]

	reEncoded, err := cbor.Marshal(utxo.Output)
	if err != nil {
		t.Fatalf("cbor.Marshal failed: %v", err)
	}

	reEncodedFromPostAlonzo, err := cbor.Marshal(utxo.Output.PostAlonzo)
	if err != nil {
		t.Fatalf("cbor.Marshal PostAlonzo failed: %v", err)
	}

	t.Logf("TransactionOutput marshal: %s", hex.EncodeToString(reEncoded))
	t.Logf("PostAlonzo marshal:        %s", hex.EncodeToString(reEncodedFromPostAlonzo))

	var roundTripped TransactionOutput.TransactionOutput
	err = cbor.Unmarshal(reEncoded, &roundTripped)
	if err != nil {
		t.Fatalf("cbor.Unmarshal of re-encoded bytes failed: %v", err)
	}

	roundTrippedAgain, err := cbor.Marshal(roundTripped)
	if err != nil {
		t.Fatalf("second cbor.Marshal failed: %v", err)
	}

	t.Logf("Round-trip stable: %v", hex.EncodeToString(reEncoded) == hex.EncodeToString(roundTrippedAgain))
}

func TestNormalizeMaestroCostModels(t *testing.T) {
	raw := map[string]any{
		"plutus_v1": []any{int64(100), float64(200), int64(300)},
		"plutus_v2": []any{float64(1), float64(2), float64(3)},
		"plutus_v3": []any{int64(9), int64(8), int64(7)},
	}

	costModels, err := normalizeMaestroCostModels(raw)
	if err != nil {
		t.Fatalf("normalizeMaestroCostModels failed: %v", err)
	}

	expected := map[string][]int64{
		"PlutusV1": []int64{100, 200, 300},
		"PlutusV2": []int64{1, 2, 3},
		"PlutusV3": []int64{9, 8, 7},
	}

	if !reflect.DeepEqual(expected, costModels) {
		t.Fatalf("unexpected cost models: got %#v want %#v", costModels, expected)
	}
}

func TestNormalizeMaestroCostModelsRejectsMapEncodedVectors(t *testing.T) {
	raw := map[string]any{
		"plutus_v2": map[string]any{
			"0": 1,
			"1": 2,
		},
	}

	if _, err := normalizeMaestroCostModels(raw); err == nil {
		t.Fatal("expected normalizeMaestroCostModels to reject map-encoded vectors")
	}
}

func TestMergeMaestroProtocolParamsUsesPresetForMissingFields(t *testing.T) {
	current := Base.ProtocolParameters{
		MinFeeConstant:   1,
		CoinsPerUtxoByte: "4310",
		CoinsPerUtxoWord: "0",
	}
	preset := Base.ProtocolParameters{
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

// TestAdaptApolloUtxosToMaestro_NoCache verifies the adapter produces valid
// output structs when no CBOR cache is available (falls back to re-marshaling).
func TestAdaptApolloUtxosToMaestro_NoCache(t *testing.T) {
	result, err := adaptApolloUtxosToMaestro(ApolloEvalSample1UTxOs, nil)
	if err != nil {
		t.Fatalf("adaptApolloUtxosToMaestro failed: %v", err)
	}

	if len(result) != len(ApolloEvalSample1UTxOs) {
		t.Fatalf("expected %d additional utxos, got %d", len(ApolloEvalSample1UTxOs), len(result))
	}

	for i, au := range result {
		expectedHash := hex.EncodeToString(ApolloEvalSample1UTxOs[i].Input.TransactionId)
		if au.TxHash != expectedHash {
			t.Errorf("[%d] expected tx hash %s, got %s", i, expectedHash, au.TxHash)
		}
		if au.Index != ApolloEvalSample1UTxOs[i].Input.Index {
			t.Errorf("[%d] expected index %d, got %d", i, ApolloEvalSample1UTxOs[i].Input.Index, au.Index)
		}
		if au.TxoutCbor == "" {
			t.Errorf("[%d] txout_cbor is empty", i)
		}
	}
}

// TestAdaptApolloUtxosToMaestro_WithCache verifies the adapter uses cached
// raw CBOR when available instead of re-marshaling through Apollo.
func TestAdaptApolloUtxosToMaestro_WithCache(t *testing.T) {
	// Simulate a known "original" CBOR string from Maestro
	const fakeMaestroCbor = "deadbeef01020304"
	txHash := hex.EncodeToString(ApolloEvalSample2UTxOs[0].Input.TransactionId)
	idx := ApolloEvalSample2UTxOs[0].Input.Index
	cacheKey := utxoCacheKey(txHash, idx)

	cache := map[string]string{
		cacheKey: fakeMaestroCbor,
	}
	lookup := func(key string) (string, bool) {
		v, ok := cache[key]
		return v, ok
	}

	result, err := adaptApolloUtxosToMaestro(ApolloEvalSample2UTxOs, lookup)
	if err != nil {
		t.Fatalf("adaptApolloUtxosToMaestro failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].TxoutCbor != fakeMaestroCbor {
		t.Errorf("expected cached CBOR %q, got %q", fakeMaestroCbor, result[0].TxoutCbor)
	}
}

// TestAdaptApolloUtxosToMaestro_CacheMiss verifies the adapter falls back
// to re-marshaling when the cache doesn't have the UTxO.
func TestAdaptApolloUtxosToMaestro_CacheMiss(t *testing.T) {
	cache := map[string]string{} // empty cache
	lookup := func(key string) (string, bool) {
		v, ok := cache[key]
		return v, ok
	}

	result, err := adaptApolloUtxosToMaestro(ApolloEvalSample1UTxOs[:1], lookup)
	if err != nil {
		t.Fatalf("adaptApolloUtxosToMaestro failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	// Should have re-marshaled (not empty)
	if result[0].TxoutCbor == "" {
		t.Error("txout_cbor should not be empty on cache miss")
	}

	// Verify the re-marshaled output is valid CBOR
	decoded, err := hex.DecodeString(result[0].TxoutCbor)
	if err != nil {
		t.Fatalf("invalid hex in txout_cbor: %v", err)
	}
	var output TransactionOutput.TransactionOutput
	if err := cbor.Unmarshal(decoded, &output); err != nil {
		t.Fatalf("re-marshaled CBOR is not valid TransactionOutput: %v", err)
	}
}

// TestMaestroProvider_CacheRawCbor verifies the provider's caching methods.
func TestMaestroProvider_CacheRawCbor(t *testing.T) {
	p := &MaestroProvider{}

	p.cacheRawCbor("abc123", 0, "deadbeef")
	p.cacheRawCbor("abc123", 1, "cafebabe")

	got, ok := p.lookupRawCbor("abc123#0")
	if !ok || got != "deadbeef" {
		t.Errorf("expected deadbeef, got %q (ok=%v)", got, ok)
	}

	got, ok = p.lookupRawCbor("abc123#1")
	if !ok || got != "cafebabe" {
		t.Errorf("expected cafebabe, got %q (ok=%v)", got, ok)
	}

	_, ok = p.lookupRawCbor("nonexistent#0")
	if ok {
		t.Error("expected cache miss for nonexistent key")
	}
}

// TestMaestroProvider_EvaluateUsesCache is an integration-style test verifying
// that EvaluateTx would use cached CBOR. We test this by exercising the
// adapter with the provider's lookup method.
func TestMaestroProvider_EvaluateUsesCache(t *testing.T) {
	p := &MaestroProvider{}

	// Simulate caching UTxOs from a prior fetch
	for _, u := range ApolloEvalSample1UTxOs {
		txHash := hex.EncodeToString(u.Input.TransactionId)
		// Use a known "original" CBOR to distinguish from re-marshaled
		fakeCbor := "original_" + txHash[:8]
		p.cacheRawCbor(txHash, u.Input.Index, fakeCbor)
	}

	// The adapter should use cached values
	result, err := adaptApolloUtxosToMaestro(ApolloEvalSample1UTxOs, p.lookupRawCbor)
	if err != nil {
		t.Fatalf("adaptApolloUtxosToMaestro failed: %v", err)
	}

	for i, au := range result {
		txHash := hex.EncodeToString(ApolloEvalSample1UTxOs[i].Input.TransactionId)
		expected := "original_" + txHash[:8]
		if au.TxoutCbor != expected {
			t.Errorf("[%d] expected cached CBOR %q, got %q", i, expected, au.TxoutCbor)
		}
	}
}

// TestUtxoCacheKey verifies the cache key format.
func TestUtxoCacheKey(t *testing.T) {
	tests := []struct {
		txHash   string
		index    int
		expected string
	}{
		{"abc123", 0, "abc123#0"},
		{"def456", 42, "def456#42"},
		{"", 0, "#0"},
	}
	for _, tt := range tests {
		got := utxoCacheKey(tt.txHash, tt.index)
		if got != tt.expected {
			t.Errorf("utxoCacheKey(%q, %d) = %q, want %q", tt.txHash, tt.index, got, tt.expected)
		}
	}
}

// TestMaestroRoundTrip_SimulateFullFlow simulates the full flow:
// 1. Maestro returns a UTxO with txout_cbor
// 2. We decode it and cache the raw CBOR
// 3. We need to send it back for evaluation
// 4. The adapter uses the cached original bytes
func TestMaestroRoundTrip_SimulateFullFlow(t *testing.T) {
	p := &MaestroProvider{}

	// Simulate what adaptMaestroUtxoToApolloUtxo returns
	simOutput := TransactionOutput.TransactionOutput{
		IsPostAlonzo: true,
		PostAlonzo: TransactionOutput.TransactionOutputAlonzo{
			Address:   evalSample2Addr,
			Amount:    assetsToApolloValue(14000000, nil).ToAlonzoValue(),
			Datum:     datumHexToApolloDatumOption("d87980"),
			ScriptRef: evalSample2ScriptRef,
		},
	}

	// Get "original" CBOR as Maestro would serve it
	originalCbor, err := cbor.Marshal(simOutput)
	if err != nil {
		t.Fatalf("Failed to create original CBOR: %v", err)
	}
	originalHex := hex.EncodeToString(originalCbor)

	// Cache the raw CBOR (simulating what GetUtxosByAddress does)
	txHash := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	p.cacheRawCbor(txHash, 0, originalHex)

	// Decode into Apollo (simulating adaptMaestroUtxoToApolloUtxo)
	var decoded TransactionOutput.TransactionOutput
	err = cbor.Unmarshal(originalCbor, &decoded)
	if err != nil {
		t.Fatalf("Failed to decode CBOR: %v", err)
	}

	// Build the UTxO that would be passed to EvaluateTx
	utxo := UTxO.UTxO{
		Input: TransactionInput.TransactionInput{
			TransactionId: mustDecodeHex("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			Index:         0,
		},
		Output: decoded,
	}

	// Run the adapter with the provider's cache
	result, err := adaptApolloUtxosToMaestro([]UTxO.UTxO{utxo}, p.lookupRawCbor)
	if err != nil {
		t.Fatalf("adaptApolloUtxosToMaestro failed: %v", err)
	}

	// The CBOR should be the original bytes, not re-marshaled
	if result[0].TxoutCbor != originalHex {
		t.Errorf("adapter did not preserve original CBOR")
		t.Logf("  Expected: %s", originalHex)
		t.Logf("  Got:      %s", result[0].TxoutCbor)
	}
}
