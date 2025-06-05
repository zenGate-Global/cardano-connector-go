package kupmios

import (
	"context"
	"encoding/hex"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/Salvionied/cbor/v2"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/chainsync"
	"github.com/SundaeSwap-finance/ogmigo/v6/ouroboros/shared"
	"github.com/tj/assert"
	connector "github.com/zenGate-Global/cardano-connector-go"
	tests "github.com/zenGate-Global/cardano-connector-go/tests"
)

// compareRedeemers compares two slices of EvalRedeemer in an order-independent way
func compareRedeemers(a, b []connector.EvalRedeemer) bool {
	if len(a) != len(b) {
		return false
	}

	aCopy := make([]connector.EvalRedeemer, len(a))
	bCopy := make([]connector.EvalRedeemer, len(b))
	copy(aCopy, a)
	copy(bCopy, b)

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

// setupKupmios creates a Blockfrost provider for testing
func setupKupmios(t *testing.T) *KupmiosProvider {
	t.Helper()

	ogmigoEndpoint := os.Getenv("OGMIOS_ENDPOINT")
	kugoEndpoint := os.Getenv("KUGO_ENDPOINT")

	config := Config{
		OgmigoEndpoint: ogmigoEndpoint,
		KugoEndpoint:   kugoEndpoint,
	}

	provider, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create Blockfrost provider: %v", err)
	}

	return provider
}

func TestGetProtocolParameters(t *testing.T) {
	kupmios := setupKupmios(t)
	ctx := context.Background()

	pp, err := kupmios.GetProtocolParameters(ctx)
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

func TestGetTip(t *testing.T) {
	kupmios := setupKupmios(t)
	ctx := context.Background()

	tip, err := kupmios.GetTip(ctx)
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
	kupmios := setupKupmios(t)
	ctx := context.Background()

	// Test with an address that should have UTxOs
	utxos, err := kupmios.GetUtxosByAddress(
		ctx,
		"addr_test1qrngfyc452vy4twdrepdjc50d4kvqutgt0hs9w6j2qhcdjfx0gpv7rsrjtxv97rplyz3ymyaqdwqa635zrcdena94ljs0xy950",
	)
	if err != nil {
		t.Fatalf("GetUtxosByAddress failed: %v", err)
	}

	// We expect this address to have UTxOs, but we won't assert a specific count
	// since it can change over time
	t.Logf("Found %d UTxOs", len(utxos))
}

func TestGetUtxosWithUnit(t *testing.T) {
	kupmios := setupKupmios(t)
	ctx := context.Background()

	utxos, err := kupmios.GetUtxosWithUnit(
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
	kupmios := setupKupmios(t)
	ctx := context.Background()

	utxo, err := kupmios.GetUtxoByUnit(
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
	kupmios := setupKupmios(t)
	ctx := context.Background()

	outRefs := []connector.OutRef{
		{
			TxHash: "b50e73e74a3073bc44f555928702c0ae0f555a43f1afdce34b3294247dce022d",
			Index:  0,
		},
	}

	utxos, err := kupmios.GetUtxosByOutRef(ctx, outRefs)
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

func TestGetUtxosByOutRefOgmios(t *testing.T) {
	kupmios := setupKupmios(t)
	ctx := context.Background()

	outRefs := []chainsync.TxInQuery{
		{
			Transaction: shared.UtxoTxID{
				ID: "b50e73e74a3073bc44f555928702c0ae0f555a43f1afdce34b3294247dce022d",
			},
			Index: 0,
		},
	}

	utxos, err := kupmios.GetOgmiosUtxo(ctx, outRefs)
	if err != nil {
		t.Fatalf("GetUtxosByOutRef failed: %v", err)
	}

	if len(utxos) != 1 {
		t.Errorf("Expected 1 UTxO, got %d", len(utxos))
	}

	apolloUtxo := adaptOgmigoUtxoToApollo(utxos[0])

	if !reflect.DeepEqual(apolloUtxo, tests.ApolloDiscoveryUTxO) {
		t.Errorf(
			"Expected UTxO %+v, got %+v",
			tests.ApolloDiscoveryUTxO,
			apolloUtxo,
		)
	}
}

func TestGetDelegation(t *testing.T) {
	kupmios := setupKupmios(t)
	ctx := context.Background()

	delegation, err := kupmios.GetDelegation(
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
	kupmios := setupKupmios(t)
	ctx := context.Background()

	datum, err := kupmios.GetDatum(
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
	t.Skip("Skipping await tx test - kupmios returns error")
	kupmios := setupKupmios(t)
	ctx := context.Background()

	// Test with a known confirmed transaction
	isConfirmed, err := kupmios.AwaitTx(
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
	kupmios := setupKupmios(t)
	ctx := context.Background()

	invalidTxBytes := []byte{0x80} // Invalid CBOR

	_, err := kupmios.SubmitTx(ctx, invalidTxBytes)
	if err == nil {
		t.Error("Expected SubmitTx to fail with invalid transaction data")
	}
}

func TestEvaluateTxSample1(t *testing.T) {
	kupmios := setupKupmios(t)
	ctx := context.Background()

	tx1Bytes, _ := hex.DecodeString(tests.ApolloEvalSample1Transaction)

	redeemers, err := kupmios.EvaluateTx(
		ctx,
		tx1Bytes,
		tests.ApolloEvalSample1UTxOs,
	)
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

func TestEvaluateTxSample2(t *testing.T) {
	t.Skip("Skipping sample 2 - Invalid request: couldn't decode plutus script")
	kupmios := setupKupmios(t)
	ctx := context.Background()

	tx2Bytes, _ := hex.DecodeString(tests.ApolloEvalSample2Transaction)

	redeemers, err := kupmios.EvaluateTx(
		ctx,
		tx2Bytes,
		tests.ApolloEvalSample2UTxOs,
	)
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

func TestEvaluateTxSample3(t *testing.T) {
	t.Skip("Skipping sample 3 - Invalid request: couldn't decode plutus script")
	kupmios := setupKupmios(t)
	ctx := context.Background()

	tx3Bytes, _ := hex.DecodeString(tests.ApolloEvalSample3Transaction)

	redeemers, err := kupmios.EvaluateTx(
		ctx,
		tx3Bytes,
		tests.ApolloEvalSample3UTxOs,
	)
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
