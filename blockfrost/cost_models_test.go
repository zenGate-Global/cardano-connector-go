package blockfrost

import (
	"encoding/json"
	"testing"
)

func TestParseCostModelsValidRaw(t *testing.T) {
	raw := json.RawMessage(`{"PlutusV1":[1,2,3],"PlutusV2":[4,5]}`)
	models, err := parseCostModels(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models["PlutusV1"]) != 3 || models["PlutusV2"][1] != 5 {
		t.Fatalf("unexpected cost models: %+v", models)
	}
}

// TestParseCostModelsMalformedRawErrors asserts the required fix: a present but
// malformed cost_models_raw (and no valid array cost_models fallback) must fail
// loud, not silently return nil cost models.
func TestParseCostModelsMalformedRawErrors(t *testing.T) {
	// Present but not a map of int64 arrays (named/keyed form mistakenly placed
	// in cost_models_raw).
	raw := json.RawMessage(`{"PlutusV1":{"addInteger-cpu-arguments-intercept":205665}}`)
	if _, err := parseCostModels(raw, nil); err == nil {
		t.Fatal("expected error for malformed cost_models_raw, got nil")
	}

	// Present but a JSON scalar.
	raw2 := json.RawMessage(`"oops"`)
	if _, err := parseCostModels(raw2, nil); err == nil {
		t.Fatal("expected error for non-object cost_models_raw, got nil")
	}
}

// TestParseCostModelsMalformedRawFallsBackToArrayNamed asserts that a malformed
// cost_models_raw is rescued by a valid array-encoded cost_models.
func TestParseCostModelsMalformedRawFallsBackToArrayNamed(t *testing.T) {
	raw := json.RawMessage(`"oops"`)
	named := json.RawMessage(`{"PlutusV1":[1,2,3]}`)
	models, err := parseCostModels(raw, named)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models["PlutusV1"]) != 3 {
		t.Fatalf("expected fallback to array-form cost_models, got %+v", models)
	}
}

// TestParseCostModelsNamedKeyedErrors asserts that a named/keyed-only cost model
// (non-canonical order, no cost_models_raw) is rejected rather than flattened.
func TestParseCostModelsNamedKeyedErrors(t *testing.T) {
	named := json.RawMessage(`{"PlutusV1":{"addInteger-cpu-arguments-intercept":205665}}`)
	if _, err := parseCostModels(nil, named); err == nil {
		t.Fatal("expected error for named/keyed-only cost models, got nil")
	}
}

// TestParseCostModelsAbsentReturnsNil asserts that when neither field is present
// the result is nil with no error (some endpoints omit cost models entirely).
func TestParseCostModelsAbsentReturnsNil(t *testing.T) {
	models, err := parseCostModels(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models != nil {
		t.Fatalf("expected nil cost models, got %+v", models)
	}

	// Explicit JSON null in both fields is also "absent".
	models2, err := parseCostModels(json.RawMessage(`null`), json.RawMessage(`null`))
	if err != nil {
		t.Fatalf("unexpected error for null fields: %v", err)
	}
	if models2 != nil {
		t.Fatalf("expected nil cost models for null fields, got %+v", models2)
	}
}
