package blockfrost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHydrateUtxoUnresolvableReferenceScriptIsBestEffort asserts that when a
// UTxO's reference script CBOR cannot be resolved (e.g. a native script whose
// /scripts/{hash}/cbor is empty / 404), chain-read hydration does NOT abort the
// whole GetUtxosByAddress: the UTxO is returned with a nil reference script and
// no error.
func TestHydrateUtxoUnresolvableReferenceScriptIsBestEffort(t *testing.T) {
	const (
		addr       = "addr_test1wpgexmeunzsykesf42d4eqet5yvzeap6trjnflxqtkcf66g0kpnxt"
		txHash     = "8ae470ef0000000000000000000000000000000000000000000000000000beef"
		scriptHash = "b7cafbba00000000000000000000000000000000000000000000beef"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/addresses/") && strings.HasSuffix(r.URL.Path, "/utxos"):
			page := r.URL.Query().Get("page")
			if page != "" && page != "1" {
				// Subsequent pages are empty to terminate pagination.
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{
				"address": "` + addr + `",
				"tx_hash": "` + txHash + `",
				"output_index": 0,
				"amount": [{"unit":"lovelace","quantity":"2000000"}],
				"data_hash": "",
				"inline_datum": null,
				"reference_script_hash": "` + scriptHash + `"
			}]`))
		case strings.Contains(r.URL.Path, "/scripts/") && strings.HasSuffix(r.URL.Path, "/cbor"):
			// Native scripts (and some others) have no /cbor; Blockfrost returns
			// 404. This must not abort hydration.
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status_code":404,"error":"Not Found","message":"The requested component has not been found."}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	provider, err := New(Config{BaseURL: srv.URL, ProjectID: "test", NetworkId: 0})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	utxos, err := provider.GetUtxosByAddress(context.Background(), addr)
	if err != nil {
		t.Fatalf("GetUtxosByAddress must not fail on an unresolvable reference script: %v", err)
	}
	if len(utxos) != 1 {
		t.Fatalf("expected 1 UTxO, got %d", len(utxos))
	}
	if got := utxos[0].Output.ScriptRef(); got != nil {
		t.Fatalf("expected nil reference script (unresolved), got %v", got)
	}
	if utxos[0].Output.Amount().Uint64() != 2000000 {
		t.Fatalf("expected lovelace 2000000, got %s", utxos[0].Output.Amount())
	}
}
