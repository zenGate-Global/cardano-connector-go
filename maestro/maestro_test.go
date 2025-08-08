package maestro

import (
	"context"
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"github.com/Salvionied/apollo/constants"
	"github.com/Salvionied/cbor/v2"
	"github.com/tj/assert"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

// setupMaestro creates a Maestro provider for testing
func setupMaestro(t *testing.T) *MaestroProvider {
	t.Helper()

	projectID := "NGWVQs06kfkHmioj49Qpv3DBw4uJWyX6"
	if projectID == "" {
		t.Log("MAESTRO_API_KEY environment variable not set")
	}

	config := Config{
		ProjectID:   projectID,
		NetworkName: "preprod",
		NetworkId:   int(constants.PREPROD),
	}

	provider, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create Maestro provider: %v", err)
	}

	return provider
}

func TestGetProtocolParameters(t *testing.T) {
	m := setupMaestro(t)
	ctx := context.Background()

	pp, err := m.GetProtocolParameters(ctx)
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
	m := setupMaestro(t)
	ctx := context.Background()

	_, err := m.GetGenesisParams(ctx)
	if err == nil {
		t.Fatal(
			"Expected GetGenesisParams to fail since Maestro doesn't support it",
		)
	}

	// Maestro does not provide a genesis parameters endpoint
	t.Logf("Expected error: %v", err)
}

func TestNetwork(t *testing.T) {
	m := setupMaestro(t)
	assert.Equal(
		t,
		int(constants.PREPROD),
		m.Network(),
		"Network should be preprod",
	)
}

func TestEpoch(t *testing.T) {
	m := setupMaestro(t)
	ctx := context.Background()
	epoch, err := m.Epoch(ctx)
	if err != nil {
		t.Fatalf("Epoch failed: %v", err)
	}

	assert.Equal(
		t,
		epoch >= 0,
		true,
		"Epoch should be greater than or equal to 0",
	)
}

func TestGetTip(t *testing.T) {
	m := setupMaestro(t)
	ctx := context.Background()

	tip, err := m.GetTip(ctx)
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
	m := setupMaestro(t)
	ctx := context.Background()

	// Test with an address that should have UTxOs
	utxos, err := m.GetUtxosByAddress(ctx, AddressToQuery)
	if err != nil {
		t.Fatalf("GetUtxosByAddress failed: %v", err)
	}

	// We expect this address to have UTxOs, but we won't assert a specific count
	// since it can change over time
	t.Logf("Found %d UTxOs", len(utxos))
}

func TestGetUtxosWithUnit(t *testing.T) {
	m := setupMaestro(t)
	ctx := context.Background()

	utxos, err := m.GetUtxosWithUnit(
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

	if !reflect.DeepEqual(utxos[0], ApolloDiscoveryUTxO) {
		t.Errorf(
			"Expected UTxO %+v, got %+v",
			ApolloDiscoveryUTxO,
			utxos[0],
		)
	}
}

func TestGetUtxoByUnit(t *testing.T) {
	m := setupMaestro(t)
	ctx := context.Background()

	utxo, err := m.GetUtxoByUnit(
		ctx,
		"4a83e031d4c37fc7ca6177a2f3581a8eec2ce155da91f59cfdb3bb28446973636f7665727956616c696461746f72",
	)
	if err != nil {
		t.Fatalf("GetUtxoByUnit failed: %v", err)
	}

	if utxo == nil {
		t.Fatal("Expected a UTxO but got nil")
	}

	if !reflect.DeepEqual(*utxo, ApolloDiscoveryUTxO) {
		t.Errorf("Expected UTxO %+v, got %+v", ApolloDiscoveryUTxO, *utxo)
	}
}

func TestGetUtxosByOutRef(t *testing.T) {
	m := setupMaestro(t)
	ctx := context.Background()

	outRefs := []connector.OutRef{
		{
			TxHash: "b50e73e74a3073bc44f555928702c0ae0f555a43f1afdce34b3294247dce022d",
			Index:  0,
		},
	}

	utxos, err := m.GetUtxosByOutRef(ctx, outRefs)
	if err != nil {
		t.Fatalf("GetUtxosByOutRef failed: %v", err)
	}

	if len(utxos) != 1 {
		t.Errorf("Expected 1 UTxO, got %d", len(utxos))
	}

	if !reflect.DeepEqual(utxos[0], ApolloDiscoveryUTxO) {
		t.Errorf(
			"Expected UTxO %+v, got %+v",
			ApolloDiscoveryUTxO,
			utxos[0],
		)
	}
}

func TestGetDelegation(t *testing.T) {
	m := setupMaestro(t)
	ctx := context.Background()

	delegation, err := m.GetDelegation(
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
	m := setupMaestro(t)
	ctx := context.Background()

	datum, err := m.GetDatum(
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

	if actualDatumHex != ExpectedDatum {
		t.Errorf(
			"Expected datum %s, got %s",
			ExpectedDatum,
			actualDatumHex,
		)
	}
}

func TestAwaitTx(t *testing.T) {
	m := setupMaestro(t)
	ctx := context.Background()

	// Test with a known confirmed transaction
	isConfirmed, err := m.AwaitTx(
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
	m := setupMaestro(t)
	ctx := context.Background()

	// Test with invalid transaction data
	invalidTxBytes := []byte{0x80} // Invalid CBOR

	_, err := m.SubmitTx(ctx, invalidTxBytes)
	if err == nil {
		t.Error("Expected SubmitTx to fail with invalid transaction data")
	}
}

func TestEvaluateTxSample1(t *testing.T) {
	m := setupMaestro(t)
	ctx := context.Background()

	tx1Bytes, _ := hex.DecodeString(ApolloEvalSample1Transaction)

	redeemers, err := m.EvaluateTx(ctx, tx1Bytes, ApolloEvalSample1UTxOs)
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if !reflect.DeepEqual(redeemers, ApolloEvalSample1RedeemersExUnits) {
		t.Errorf(
			"Expected redeemers %+v, got %+v",
			ApolloEvalSample1RedeemersExUnits,
			redeemers,
		)
	}
}

func TestEvaluateTxSample2(t *testing.T) {
	t.Skip("Skipping test: maestro does not allow malformed UTxOs")
	m := setupMaestro(t)
	ctx := context.Background()

	tx2Bytes, _ := hex.DecodeString(ApolloEvalSample2Transaction)

	redeemers, err := m.EvaluateTx(ctx, tx2Bytes, ApolloEvalSample2UTxOs)
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if !reflect.DeepEqual(redeemers, ApolloEvalSample2RedeemersExUnits) {
		t.Errorf(
			"Expected redeemers %+v, got %+v",
			ApolloEvalSample2RedeemersExUnits,
			redeemers,
		)
	}
}

func TestEvaluateTxSample3(t *testing.T) {
	t.Skip("Skipping test: maestro does not allow malformed UTxOs")
	m := setupMaestro(t)
	ctx := context.Background()

	tx3Bytes, _ := hex.DecodeString(ApolloEvalSample3Transaction)

	redeemers, err := m.EvaluateTx(ctx, tx3Bytes, ApolloEvalSample3UTxOs)
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if !reflect.DeepEqual(redeemers, ApolloEvalSample3RedeemersExUnits) {
		t.Errorf(
			"Expected redeemers %+v, got %+v",
			ApolloEvalSample3RedeemersExUnits,
			redeemers,
		)
	}
}

func TestGetScriptCborByScriptHash(t *testing.T) {
	m := setupMaestro(t)
	ctx := context.Background()

	scriptCbor, err := m.GetScriptCborByScriptHash(
		ctx,
		ScriptHashToQuery,
	)
	if err != nil {
		t.Fatalf("GetScriptCborByScriptHash failed: %v", err)
	}

	assert.Equal(t, scriptCbor, ExpectedScriptCbor)
}
