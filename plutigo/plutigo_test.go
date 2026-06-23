package plutigo

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Salvionied/apollo/v2/backend"
	"github.com/Salvionied/apollo/v2/constants"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	connector "github.com/zenGate-Global/cardano-connector-go"
	"github.com/zenGate-Global/cardano-connector-go/blockfrost"
	fixture "github.com/zenGate-Global/cardano-connector-go/tests"
)

type stubProvider struct {
	network          int
	epoch            int
	epochErr         error
	tip              connector.Tip
	tipErr           error
	protocolParams   backend.ProtocolParameters
	protocolErr      error
	genesisParams    backend.GenesisParameters
	genesisErr       error
	utxosByAddress   []lcommon.Utxo
	utxosAddrErr     error
	utxosWithUnit    []lcommon.Utxo
	utxosWithUnitErr error
	utxoByUnit       *lcommon.Utxo
	utxoByUnitErr    error
	outRefsResult    []lcommon.Utxo
	outRefsErr       error
	outRefsCalls     int
	lastOutRefs      []connector.OutRef
	delegation       connector.Delegation
	delegationErr    error
	datum            lcommon.Datum
	datumErr         error
	awaitResult      bool
	awaitErr         error
	submitHash       string
	submitErr        error
	evalResult       map[lcommon.RedeemerKey]lcommon.ExUnits
	evalErr          error
	scriptCbor       string
	scriptErr        error
}

func (s *stubProvider) GetProtocolParameters(ctx context.Context) (backend.ProtocolParameters, error) {
	return s.protocolParams, s.protocolErr
}

func (s *stubProvider) GetGenesisParams(ctx context.Context) (backend.GenesisParameters, error) {
	return s.genesisParams, s.genesisErr
}

func (s *stubProvider) Network() int {
	return s.network
}

func (s *stubProvider) Epoch(ctx context.Context) (int, error) {
	return s.epoch, s.epochErr
}

func (s *stubProvider) GetTip(ctx context.Context) (connector.Tip, error) {
	return s.tip, s.tipErr
}

func (s *stubProvider) GetUtxosByAddress(ctx context.Context, addr string) ([]lcommon.Utxo, error) {
	return s.utxosByAddress, s.utxosAddrErr
}

func (s *stubProvider) GetUtxosWithUnit(ctx context.Context, addr string, unit string) ([]lcommon.Utxo, error) {
	return s.utxosWithUnit, s.utxosWithUnitErr
}

func (s *stubProvider) GetUtxoByUnit(ctx context.Context, unit string) (*lcommon.Utxo, error) {
	return s.utxoByUnit, s.utxoByUnitErr
}

func (s *stubProvider) GetUtxosByOutRef(ctx context.Context, outRefs []connector.OutRef) ([]lcommon.Utxo, error) {
	s.outRefsCalls++
	s.lastOutRefs = append([]connector.OutRef(nil), outRefs...)
	return s.outRefsResult, s.outRefsErr
}

func (s *stubProvider) GetDelegation(ctx context.Context, rewardAddress string) (connector.Delegation, error) {
	return s.delegation, s.delegationErr
}

func (s *stubProvider) GetDatum(ctx context.Context, datumHash string) (lcommon.Datum, error) {
	return s.datum, s.datumErr
}

func (s *stubProvider) AwaitTx(ctx context.Context, txHash string, checkInterval time.Duration) (bool, error) {
	return s.awaitResult, s.awaitErr
}

func (s *stubProvider) SubmitTx(ctx context.Context, tx []byte) (string, error) {
	return s.submitHash, s.submitErr
}

func (s *stubProvider) EvaluateTx(ctx context.Context, tx []byte, additionalUTxOs []lcommon.Utxo) (map[lcommon.RedeemerKey]lcommon.ExUnits, error) {
	return s.evalResult, s.evalErr
}

func (s *stubProvider) GetScriptCborByScriptHash(ctx context.Context, scriptHash string) (string, error) {
	return s.scriptCbor, s.scriptErr
}

type retryProvider struct {
	connector.Provider
}

func (r *retryProvider) GetProtocolParameters(ctx context.Context) (backend.ProtocolParameters, error) {
	return retryLookup(ctx, func(callCtx context.Context) (backend.ProtocolParameters, error) {
		return r.Provider.GetProtocolParameters(callCtx)
	})
}

func (r *retryProvider) GetGenesisParams(ctx context.Context) (backend.GenesisParameters, error) {
	return retryLookup(ctx, func(callCtx context.Context) (backend.GenesisParameters, error) {
		return r.Provider.GetGenesisParams(callCtx)
	})
}

func (r *retryProvider) GetTip(ctx context.Context) (connector.Tip, error) {
	return retryLookup(ctx, func(callCtx context.Context) (connector.Tip, error) {
		return r.Provider.GetTip(callCtx)
	})
}

func (r *retryProvider) GetDatum(ctx context.Context, datumHash string) (lcommon.Datum, error) {
	return retryLookup(ctx, func(callCtx context.Context) (lcommon.Datum, error) {
		return r.Provider.GetDatum(callCtx, datumHash)
	})
}

func (r *retryProvider) GetScriptCborByScriptHash(ctx context.Context, scriptHash string) (string, error) {
	return retryLookup(ctx, func(callCtx context.Context) (string, error) {
		return r.Provider.GetScriptCborByScriptHash(callCtx, scriptHash)
	})
}

func setupBlockfrostLocalEval(t *testing.T) *PlutigoProvider {
	t.Helper()

	projectID := os.Getenv("BLOCKFROST_KEY")
	if projectID == "" {
		t.Skip("BLOCKFROST_KEY environment variable not set")
	}

	resolver, err := blockfrost.New(blockfrost.Config{
		ProjectID:   projectID,
		NetworkName: "preprod",
		NetworkId:   int(constants.PREPROD),
	})
	if err != nil {
		t.Fatalf("Failed to create Blockfrost provider: %v", err)
	}

	localEval, err := Wrap(&retryProvider{Provider: resolver})
	if err != nil {
		t.Fatalf("Wrap failed: %v", err)
	}
	return localEval
}

func retryLookup[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		value, err := fn(ctx)
		if err == nil {
			return value, nil
		}
		if !isTimeoutLikeError(err) {
			return zero, err
		}
		lastErr = err
	}
	return zero, lastErr
}

func isTimeoutLikeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "Client.Timeout exceeded")
}

func TestWrap(t *testing.T) {
	provider := &stubProvider{network: 5}

	localEval, err := Wrap(provider)
	if err != nil {
		t.Fatalf("Wrap failed: %v", err)
	}

	if got := localEval.Network(); got != 5 {
		t.Fatalf("expected wrapped provider network 5, got %d", got)
	}
}

func TestOverridesBeatWrappedProvider(t *testing.T) {
	provider := &stubProvider{
		protocolParams: backend.ProtocolParameters{ProtocolMajorVersion: 10},
		genesisParams:  backend.GenesisParameters{SystemStart: 1},
	}
	protocolOverride := &backend.ProtocolParameters{ProtocolMajorVersion: 99}
	genesisOverride := &backend.GenesisParameters{SystemStart: 42}

	localEval, err := New(Config{
		Provider:               provider,
		ProtocolParamsOverride: protocolOverride,
		GenesisParamsOverride:  genesisOverride,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	protocolParams, err := localEval.GetProtocolParameters(ctx)
	if err != nil {
		t.Fatalf("GetProtocolParameters failed: %v", err)
	}
	if protocolParams.ProtocolMajorVersion != 99 {
		t.Fatalf("expected protocol override 99, got %d", protocolParams.ProtocolMajorVersion)
	}

	genesisParams, err := localEval.GetGenesisParams(ctx)
	if err != nil {
		t.Fatalf("GetGenesisParams failed: %v", err)
	}
	if genesisParams.SystemStart != 42 {
		t.Fatalf("expected genesis override 42, got %d", genesisParams.SystemStart)
	}
}

func TestDelegatesNonEvaluateMethods(t *testing.T) {
	provider := &stubProvider{
		network: 11,
		epoch:   12,
		tip: connector.Tip{
			Hash:   "tip-hash",
			Slot:   13,
			Height: 14,
		},
		awaitResult: true,
		submitHash:  "submitted-hash",
	}

	localEval, err := New(Config{Provider: provider})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	if got := localEval.Network(); got != 11 {
		t.Fatalf("expected network 11, got %d", got)
	}
	if epoch, err := localEval.Epoch(ctx); err != nil || epoch != 12 {
		t.Fatalf("expected epoch 12 with nil error, got %d and %v", epoch, err)
	}
	if tip, err := localEval.GetTip(ctx); err != nil || tip.Hash != "tip-hash" || tip.Slot != 13 || tip.Height != 14 {
		t.Fatalf("unexpected tip result: %#v err=%v", tip, err)
	}
	if ok, err := localEval.AwaitTx(ctx, "abc", time.Second); err != nil || !ok {
		t.Fatalf("expected AwaitTx delegation success, got %v and %v", ok, err)
	}
	if txHash, err := localEval.SubmitTx(ctx, []byte{1, 2, 3}); err != nil || txHash != "submitted-hash" {
		t.Fatalf("expected SubmitTx delegation success, got %q and %v", txHash, err)
	}
}

func TestEvaluateTxUsesWrappedProviderForInputResolution(t *testing.T) {
	txBytes, err := hex.DecodeString(fixture.ApolloEvalSample1Transaction)
	if err != nil {
		t.Fatalf("decode fixture tx: %v", err)
	}

	protocolErr := errors.New("protocol lookup hit wrapped provider")
	provider := &stubProvider{
		outRefsResult: fixture.ApolloEvalSample1UTxOs,
		protocolErr:   protocolErr,
	}

	localEval, err := New(Config{Provider: provider})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	_, err = localEval.EvaluateTx(context.Background(), txBytes, nil)
	if err == nil {
		t.Fatal("expected EvaluateTx to fail once wrapped provider protocol lookup is reached")
	}
	if !errors.Is(err, protocolErr) {
		t.Fatalf("expected wrapped protocol error, got %v", err)
	}
	if provider.outRefsCalls == 0 {
		t.Fatal("expected wrapped provider to be used for missing input resolution")
	}
	if len(provider.lastOutRefs) == 0 {
		t.Fatal("expected at least one requested out-ref")
	}
}

// TestReferenceScriptSurvivesResolverPath asserts that a reference script
// carried on a resolved input's output (gouroboros common.Utxo) is found by the
// evaluator's script-resolution path. This replaces the old apollo v1
// apolloUtxoToLedger bridge test: in apollo v2 the resolver returns
// common.Utxo values directly, so the assertion is that the script ref survives
// intact and is matched by its hash.
func TestReferenceScriptSurvivesResolverPath(t *testing.T) {
	utxo := fixture.ApolloDiscoveryUTxO

	script := utxo.Output.ScriptRef()
	if script == nil {
		t.Fatal("fixture discovery UTxO must carry a reference script")
	}
	scriptHash := script.Hash().String()

	resolved, ok := findScriptInResolvedInputs([]lcommon.Utxo{utxo}, scriptHash)
	if !ok {
		t.Fatalf("reference script %s was not found in resolved inputs", scriptHash)
	}
	if resolved.version != scriptVersionV2 {
		t.Fatalf("expected PlutusV2 reference script, got version %v", resolved.version)
	}

	// A resolved input without a script ref must be skipped, not matched.
	noScript := utxo
	outNoScript := *utxo.Output.(*babbage.BabbageTransactionOutput)
	outNoScript.TxOutScriptRef = nil
	noScript.Output = &outNoScript
	if _, ok := findScriptInResolvedInputs([]lcommon.Utxo{noScript}, scriptHash); ok {
		t.Fatal("expected no match when the resolved input carries no script ref")
	}
}

func TestEvaluateTxSample1WithBlockfrostResolver(t *testing.T) {
	localEval := setupBlockfrostLocalEval(t)
	ctx := context.Background()

	txBytes, err := hex.DecodeString(fixture.ApolloEvalSample1Transaction)
	if err != nil {
		t.Fatalf("decode fixture tx: %v", err)
	}

	redeemers, err := localEval.EvaluateTx(ctx, txBytes, fixture.ApolloEvalSample1UTxOs)
	if err != nil {
		t.Fatalf("EvaluateTx failed: %v", err)
	}

	if ok, diff := fixture.RedeemersApproxEqual(redeemers, fixture.ApolloEvalSample1RedeemersExUnits, 0.02); !ok {
		t.Fatalf("redeemers mismatch (>2%% drift): %s", diff)
	}
}

func TestEvaluateTxSample2WithBlockfrostResolver(t *testing.T) {
	localEval := setupBlockfrostLocalEval(t)
	ctx := context.Background()

	txBytes, err := hex.DecodeString(fixture.ApolloEvalSample2Transaction)
	if err != nil {
		t.Fatalf("decode fixture tx: %v", err)
	}

	redeemers := evaluateTxOrSkipIfResolverMissing(
		t,
		ctx,
		localEval,
		txBytes,
		fixture.ApolloEvalSample2UTxOs,
	)

	if ok, diff := fixture.RedeemersApproxEqual(redeemers, fixture.ApolloEvalSample2RedeemersExUnits, 0.02); !ok {
		t.Fatalf("redeemers mismatch (>2%% drift): %s", diff)
	}
}

func TestEvaluateTxSample3WithBlockfrostResolver(t *testing.T) {
	localEval := setupBlockfrostLocalEval(t)
	ctx := context.Background()

	txBytes, err := hex.DecodeString(fixture.ApolloEvalSample3Transaction)
	if err != nil {
		t.Fatalf("decode fixture tx: %v", err)
	}

	redeemers := evaluateTxOrSkipIfResolverMissing(
		t,
		ctx,
		localEval,
		txBytes,
		fixture.ApolloEvalSample3UTxOs,
	)

	if ok, diff := fixture.RedeemersApproxEqual(redeemers, fixture.ApolloEvalSample3RedeemersExUnits, 0.02); !ok {
		t.Fatalf("redeemers mismatch (>2%% drift): %s", diff)
	}
}

func evaluateTxOrSkipIfResolverMissing(
	t *testing.T,
	ctx context.Context,
	localEval *PlutigoProvider,
	txBytes []byte,
	additionalUTxOs []lcommon.Utxo,
) map[lcommon.RedeemerKey]lcommon.ExUnits {
	t.Helper()

	redeemers, err := localEval.EvaluateTx(ctx, txBytes, additionalUTxOs)
	if err != nil {
		if errors.Is(err, connector.ErrNotFound) {
			t.Skipf("Skipping local eval fixture because the wrapped resolver cannot supply required chain data: %v", err)
		}
		t.Fatalf("EvaluateTx failed: %v", err)
	}
	return redeemers
}
