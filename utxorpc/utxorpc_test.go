package utxorpc

import (
	"context"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Salvionied/apollo/v2/constants"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	connector "github.com/zenGate-Global/cardano-connector-go"
	"github.com/zenGate-Global/cardano-connector-go/tests"
)

func setupUtxorpc(t *testing.T) *UtxorpcProvider {
	t.Helper()

	utxorpcURL := os.Getenv("UTXORPC_URL")
	if utxorpcURL == "" {
		utxorpcURL = "https://preprod.utxorpc-v0.demeter.run"
	}

	dmtrAPIKey := os.Getenv("DMTR_API_KEY")
	if dmtrAPIKey == "" {
		t.Log("DMTR_API_KEY environment variable not set")
	}

	config := Config{
		BaseUrl:   utxorpcURL,
		ApiKey:    dmtrAPIKey,
		NetworkId: int(constants.PREPROD),
	}

	provider, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create UTXORPC provider: %v", err)
	}

	return provider
}

func TestGetProtocolParameters(t *testing.T) {
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	pp, err := utxorpc.GetProtocolParameters(ctx)
	if err != nil {
		t.Fatalf("GetProtocolParameters failed: %v", err)
	}

	// Basic validation that we got protocol parameters
	if pp.MinFeeConstant == 0 {
		t.Error("Expected non-zero MinFeeConstant")
	}
	if pp.MinFeeCoefficient == 0 {
		t.Error("Expected non-zero MinFeeCoefficient")
	}
	if pp.MaxTxSize == 0 {
		t.Error("Expected non-zero MaxTxSize")
	}
}

func TestGetGenesisParams(t *testing.T) {
	t.Skip("Skipping genesis params test - Utxorpc does not support it")
}

func TestNetwork(t *testing.T) {
	utxorpc := setupUtxorpc(t)
	assert.Equal(
		t,
		int(constants.PREPROD),
		utxorpc.Network(),
		"Network should be preprod",
	)
}

func TestEpoch(t *testing.T) {
	t.Skip("Skipping epoch test - Utxorpc does not support it")
}

func TestGetTip(t *testing.T) {
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	tip, err := utxorpc.GetTip(ctx)
	if err != nil {
		t.Fatalf("GetTip failed: %v", err)
	}

	t.Logf("Tip: %+v", tip)

	assert.True(t, tip.Slot > 93412488, "Slot should be greater than 93412488")
	assert.True(
		t,
		tip.Height > 3548804,
		"Height should be greater than 3548804",
	)
	assert.True(t, len(tip.Hash) == 64, "Hash should be 64 characters long")
}

func TestGetUtxos(t *testing.T) {
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	// Test with an address that should have UTxOs
	utxos, err := utxorpc.GetUtxosByAddress(ctx, tests.AddressToQuery)
	if err != nil {
		t.Fatalf("GetUtxosByAddress failed: %v", err)
	}

	// We expect this address to have UTxOs, but we won't assert a specific count
	// since it can change over time
	t.Logf("Found %d UTxOs", len(utxos))
}

func TestGetUtxosWithUnit(t *testing.T) {
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	utxos, err := utxorpc.GetUtxosWithUnit(
		ctx,
		"addr_test1wpgexmeunzsykesf42d4eqet5yvzeap6trjnflxqtkcf66g0kpnxt",
		"4a83e031d4c37fc7ca6177a2f3581a8eec2ce155da91f59cfdb3bb28446973636f7665727956616c696461746f72",
	)
	if err != nil {
		t.Fatalf("GetUtxosWithUnitByAddress failed: %v", err)
	}

	if len(utxos) == 0 {
		t.Error("Expected at least one UTxO with the specified unit")
	}

	t.Logf("Found %d UTxOs with the specified unit", len(utxos))

	if !tests.UtxosEqual(utxos[0], tests.ApolloDiscoveryUTxO) {
		t.Errorf("UTxO mismatch: %s", tests.UtxoDiff(utxos[0], tests.ApolloDiscoveryUTxO))
	}
}

func TestGetUtxoByUnit(t *testing.T) {
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	utxo, err := utxorpc.GetUtxoByUnit(
		ctx,
		"4a83e031d4c37fc7ca6177a2f3581a8eec2ce155da91f59cfdb3bb28446973636f7665727956616c696461746f72",
	)
	if err != nil {
		t.Fatalf("GetUtxoByUnit failed: %v", err)
	}

	if utxo == nil {
		t.Fatal("Expected a UTxO but got nil")
	}

	if !tests.UtxosEqual(*utxo, tests.ApolloDiscoveryUTxO) {
		t.Errorf("UTxO mismatch: %s", tests.UtxoDiff(*utxo, tests.ApolloDiscoveryUTxO))
	}
}

func TestGetUtxosByOutRef(t *testing.T) {
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	outRefs := []connector.OutRef{
		{
			TxHash: "b50e73e74a3073bc44f555928702c0ae0f555a43f1afdce34b3294247dce022d",
			Index:  0,
		},
	}

	utxos, err := utxorpc.GetUtxosByOutRef(ctx, outRefs)
	if err != nil {
		t.Fatalf("GetUtxosByOutRef failed: %v", err)
	}

	if len(utxos) != 1 {
		t.Errorf("Expected 1 UTxO, got %d", len(utxos))
	}

	if !tests.UtxosEqual(utxos[0], tests.ApolloDiscoveryUTxO) {
		t.Errorf("UTxO mismatch: %s", tests.UtxoDiff(utxos[0], tests.ApolloDiscoveryUTxO))
	}
}

func TestGetDelegation(t *testing.T) {
	t.Skip("Skipping delegation test - Utxorpc does not support it")
}

func TestGetDatum(t *testing.T) {
	t.Skip("Skipping datum test - Utxorpc does not support it")
}

func TestAwaitTx(t *testing.T) {
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	// Test with a known confirmed transaction
	isConfirmed, err := utxorpc.AwaitTx(
		ctx,
		"2a1f95a9d85bf556a3dc889831593ee963ba491ca7164d930b3af0802a9796d0",
		1*time.Second,
	)
	if err != nil {
		t.Fatalf("AwaitTx failed: %v", err)
	}

	if !isConfirmed {
		t.Error("Expected transaction to be confirmed")
	}
}

func TestSubmitTxBadRequest(t *testing.T) {
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	// Test with invalid transaction data
	invalidTxBytes := []byte{0x80} // Invalid CBOR

	_, err := utxorpc.SubmitTx(ctx, invalidTxBytes)
	if err == nil {
		t.Error("Expected SubmitTx to fail with invalid transaction data")
	}
}

func TestEvaluateTxSample1(t *testing.T) {
	t.Skip("Skipping sample 1 - Utxorpc does not support it")
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	tx1Bytes, _ := hex.DecodeString(tests.ApolloEvalSample1Transaction)

	redeemers, err := utxorpc.EvaluateTx(ctx, tx1Bytes, tests.ApolloEvalSample1UTxOs)
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if ok, diff := tests.RedeemersApproxEqual(redeemers, tests.ApolloEvalSample1RedeemersExUnits, 0.02); !ok {
		t.Errorf("redeemers mismatch (>2%% drift): %s", diff)
	}
}

// Invalid request: failed to decode payload from base64 or base16.
func TestEvaluateTxSample2(t *testing.T) {
	t.Skip("Skipping sample 2 - Utxorpc does not support it")
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	tx2Bytes, _ := hex.DecodeString(tests.ApolloEvalSample2Transaction)

	redeemers, err := utxorpc.EvaluateTx(ctx, tx2Bytes, tests.ApolloEvalSample2UTxOs)
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if ok, diff := tests.RedeemersApproxEqual(redeemers, tests.ApolloEvalSample2RedeemersExUnits, 0.02); !ok {
		t.Errorf("redeemers mismatch (>2%% drift): %s", diff)
	}
}

// NOTE: The following transaction doesn't work with Blockfrost's TX evaluation.
func TestEvaluateTxSample3(t *testing.T) {
	t.Skip("Skipping sample 3 - Utxorpc does not support it")
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	tx3Bytes, _ := hex.DecodeString(tests.ApolloEvalSample3Transaction)

	redeemers, err := utxorpc.EvaluateTx(ctx, tx3Bytes, tests.ApolloEvalSample3UTxOs)
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if ok, diff := tests.RedeemersApproxEqual(redeemers, tests.ApolloEvalSample3RedeemersExUnits, 0.02); !ok {
		t.Errorf("redeemers mismatch (>2%% drift): %s", diff)
	}
}

func TestEvaluateTxSample4(t *testing.T) {
	t.Skip(
		"Skipping sample 4 - unimplemented - want to test eval with no additional utxos",
	)
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	tx3Bytes, _ := hex.DecodeString("")

	redeemers, err := utxorpc.EvaluateTx(ctx, tx3Bytes, []common.Utxo{})
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if ok, diff := tests.RedeemersApproxEqual(redeemers, tests.ApolloEvalSample3RedeemersExUnits, 0.02); !ok {
		t.Errorf("redeemers mismatch (>2%% drift): %s", diff)
	}
}

// TestEvaluateTxIgnoresAdditionalUTxOs asserts the Provider contract: passing a
// non-empty additionalUTxOs set must NOT be rejected up front. The utxorpc
// backend cannot forward those UTxOs, but it must ignore them rather than
// erroring. We point at an unroutable endpoint with an already-cancelled
// context so the call fails fast; the assertion is that whatever error comes
// back is a transport/RPC failure, not an "additional UTxOs not supported"
// rejection.
func TestEvaluateTxIgnoresAdditionalUTxOs(t *testing.T) {
	provider, err := New(Config{
		BaseUrl:   "http://127.0.0.1:1",
		NetworkId: int(constants.PREPROD),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // fail fast without reaching a live server

	_, err = provider.EvaluateTx(ctx, []byte{0x80}, tests.ApolloEvalSample1UTxOs)
	if err == nil {
		// A nil error here would be surprising (no server), but it still
		// satisfies the contract: additional UTxOs were not rejected.
		return
	}
	if strings.Contains(err.Error(), "does not support additional UTxOs") {
		t.Fatalf("EvaluateTx must ignore additional UTxOs, not reject them: %v", err)
	}
}

func TestGetScriptCborByScriptHash(t *testing.T) {
	t.Skip(
		"Skipping script cbor by script hash test - Utxorpc does not support it",
	)
	utxorpc := setupUtxorpc(t)
	ctx := context.Background()

	scriptCbor, err := utxorpc.GetScriptCborByScriptHash(
		ctx,
		tests.ScriptHashToQuery,
	)
	if err != nil {
		t.Fatalf("GetScriptCborByScriptHash failed: %v", err)
	}

	assert.Equal(t, scriptCbor, tests.ExpectedScriptCbor)
}
