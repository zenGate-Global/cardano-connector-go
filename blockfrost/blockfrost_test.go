package blockfrost

import (
	"context"
	"encoding/hex"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/Salvionied/cbor/v2"
	connector "github.com/mgpai22/cardano-connector-go"
	tests "github.com/mgpai22/cardano-connector-go/tests"
)

// compareRedeemers compares two slices of EvalRedeemer in an order-independent way
func compareRedeemers(a, b []connector.EvalRedeemer) bool {
	if len(a) != len(b) {
		return false
	}

	// Create copies to avoid modifying the original slices
	aCopy := make([]connector.EvalRedeemer, len(a))
	bCopy := make([]connector.EvalRedeemer, len(b))
	copy(aCopy, a)
	copy(bCopy, b)

	// Sort by Tag first, then by Index
	sortRedeemers := func(redeemers []connector.EvalRedeemer) {
		sort.Slice(redeemers, func(i, j int) bool {
			if redeemers[i].Tag != redeemers[j].Tag {
				return redeemers[i].Tag < redeemers[j].Tag
			}
			return redeemers[i].Index < redeemers[j].Index
		})
	}

	sortRedeemers(aCopy)
	sortRedeemers(bCopy)

	return reflect.DeepEqual(aCopy, bCopy)
}

// setupBlockfrost creates a Blockfrost provider for testing
func setupBlockfrost(t *testing.T) *BlockfrostProvider {
	t.Helper()

	projectID := os.Getenv("BLOCKFROST_KEY")
	if projectID == "" {
		t.Log("BLOCKFROST_KEY environment variable not set")
	}

	config := Config{
		ProjectID:   projectID,
		NetworkName: "preprod",
	}

	provider, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create Blockfrost provider: %v", err)
	}

	return provider
}

func TestGetProtocolParameters(t *testing.T) {
	bf := setupBlockfrost(t)
	ctx := context.Background()

	pp, err := bf.GetProtocolParameters(ctx)
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

func TestGetUtxos(t *testing.T) {
	bf := setupBlockfrost(t)
	ctx := context.Background()

	// Test with an address that should have UTxOs
	utxos, err := bf.GetUtxosByAddress(ctx, tests.AddressToQuery)
	if err != nil {
		t.Fatalf("GetUtxosByAddress failed: %v", err)
	}

	// We expect this address to have UTxOs, but we won't assert a specific count
	// since it can change over time
	t.Logf("Found %d UTxOs", len(utxos))
}

func TestGetUtxosWithUnit(t *testing.T) {
	bf := setupBlockfrost(t)
	ctx := context.Background()

	utxos, err := bf.GetUtxosWithUnit(
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

	if !reflect.DeepEqual(utxos[0], tests.ApolloDiscoveryUTxO) {
		t.Errorf(
			"Expected UTxO %+v, got %+v",
			tests.ApolloDiscoveryUTxO,
			utxos[0],
		)
	}
}

func TestGetUtxoByUnit(t *testing.T) {
	bf := setupBlockfrost(t)
	ctx := context.Background()

	utxo, err := bf.GetUtxoByUnit(
		ctx,
		"4a83e031d4c37fc7ca6177a2f3581a8eec2ce155da91f59cfdb3bb28446973636f7665727956616c696461746f72",
	)
	if err != nil {
		t.Fatalf("GetUtxoByUnit failed: %v", err)
	}

	if utxo == nil {
		t.Fatal("Expected a UTxO but got nil")
	}

	if !reflect.DeepEqual(*utxo, tests.ApolloDiscoveryUTxO) {
		t.Errorf("Expected UTxO %+v, got %+v", tests.ApolloDiscoveryUTxO, *utxo)
	}
}

func TestGetUtxosByOutRef(t *testing.T) {
	bf := setupBlockfrost(t)
	ctx := context.Background()

	outRefs := []connector.OutRef{
		{
			TxHash: "b50e73e74a3073bc44f555928702c0ae0f555a43f1afdce34b3294247dce022d",
			Index:  0,
		},
	}

	utxos, err := bf.GetUtxosByOutRef(ctx, outRefs)
	if err != nil {
		t.Fatalf("GetUtxosByOutRef failed: %v", err)
	}

	if len(utxos) != 1 {
		t.Errorf("Expected 1 UTxO, got %d", len(utxos))
	}

	if !reflect.DeepEqual(utxos[0], tests.ApolloDiscoveryUTxO) {
		t.Errorf(
			"Expected UTxO %+v, got %+v",
			tests.ApolloDiscoveryUTxO,
			utxos[0],
		)
	}
}

func TestGetDelegation(t *testing.T) {
	bf := setupBlockfrost(t)
	ctx := context.Background()

	delegation, err := bf.GetDelegation(
		ctx,
		"stake_test17zt3vxfjx9pjnpnapa65lx375p2utwxmpc8afj053h0l3vgc8a3g3",
	)
	if err != nil {
		t.Fatalf("GetDelegation failed: %v", err)
	}

	// Basic validation - delegation info should be returned
	// We don't assert specific values since delegation status can change
	t.Logf("Delegation - Active: %v, Rewards: %d, PoolId: %s",
		delegation.Active, delegation.Rewards, delegation.PoolId)
}

func TestGetDatum(t *testing.T) {
	bf := setupBlockfrost(t)
	ctx := context.Background()

	datum, err := bf.GetDatum(
		ctx,
		"9781f0bc32835479f5051e367556df615a9040714fe7df167782df8e3e5b76df",
	)
	if err != nil {
		t.Fatalf("GetDatum failed: %v", err)
	}

	datumBytes, err := cbor.Marshal(datum)
	if err != nil {
		t.Fatalf("Failed to marshal datum: %v", err)
	}

	actualDatumHex := hex.EncodeToString(datumBytes)

	if actualDatumHex != tests.ExpectedDatum {
		t.Errorf(
			"Expected datum %s, got %s",
			tests.ExpectedDatum,
			actualDatumHex,
		)
	}
}

func TestAwaitTx(t *testing.T) {
	bf := setupBlockfrost(t)
	ctx := context.Background()

	// Test with a known confirmed transaction
	isConfirmed, err := bf.AwaitTx(
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
	bf := setupBlockfrost(t)
	ctx := context.Background()

	// Test with invalid transaction data
	invalidTxBytes := []byte{0x80} // Invalid CBOR

	_, err := bf.SubmitTx(ctx, invalidTxBytes)
	if err == nil {
		t.Error("Expected SubmitTx to fail with invalid transaction data")
	}
}

func TestEvaluateTxSample1(t *testing.T) {
	bf := setupBlockfrost(t)
	ctx := context.Background()

	tx1Bytes, _ := hex.DecodeString(tests.ApolloEvalSample1Transaction)

	redeemers, err := bf.EvaluateTx(ctx, tx1Bytes, tests.ApolloEvalSample1UTxOs)
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if !compareRedeemers(redeemers, tests.ApolloEvalSample1RedeemersExUnits) {
		t.Errorf(
			"Expected redeemers %+v, got %+v",
			tests.ApolloEvalSample1RedeemersExUnits,
			redeemers,
		)
	}
}

// Invalid request: failed to decode payload from base64 or base16.
func TestEvaluateTxSample2(t *testing.T) {
	t.Skip("Skipping sample 2 - Blockfrost TX evaluation issue")
	bf := setupBlockfrost(t)
	ctx := context.Background()

	tx2Bytes, _ := hex.DecodeString(tests.ApolloEvalSample2Transaction)

	redeemers, err := bf.EvaluateTx(ctx, tx2Bytes, tests.ApolloEvalSample2UTxOs)
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if !compareRedeemers(redeemers, tests.ApolloEvalSample2RedeemersExUnits) {
		t.Errorf(
			"Expected redeemers %+v, got %+v",
			tests.ApolloEvalSample2RedeemersExUnits,
			redeemers,
		)
	}
}

// NOTE: The following transaction doesn't work with Blockfrost's TX evaluation.
// This is likely because they have not upgraded from Ogmios 5.6 to Ogmios 6.0 or to the latest version.
// Error: Could not evaluate the transaction: {"type":"jsonwsp/fault","version":"1.0","servicename":"ogmios","fault":{"code":"client","string":"Invalid request: failed to decode payload from base64 or base16."},"reflection":{"id":"17f6c075-6d70-444e-a0e5-7cbbd064508c"}}.
func TestEvaluateTxSample3(t *testing.T) {
	t.Skip(
		"Skipping sample 3 - Blockfrost TX evaluation issue with outdated Ogmios version",
	)
	bf := setupBlockfrost(t)
	ctx := context.Background()

	tx3Bytes, _ := hex.DecodeString(tests.ApolloEvalSample3Transaction)

	redeemers, err := bf.EvaluateTx(ctx, tx3Bytes, tests.ApolloEvalSample3UTxOs)
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if !compareRedeemers(redeemers, tests.ApolloEvalSample3RedeemersExUnits) {
		t.Errorf(
			"Expected redeemers %+v, got %+v",
			tests.ApolloEvalSample3RedeemersExUnits,
			redeemers,
		)
	}
}
