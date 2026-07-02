package maestro

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Salvionied/apollo/v2/constants"
	maestroClient "github.com/maestro-org/go-sdk/client"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

// --- classifyMaestroErr unit tests ---

func TestClassifyMaestroErr_Nil(t *testing.T) {
	if got := classifyMaestroErr(nil); got != nil {
		t.Errorf("classifyMaestroErr(nil) = %v, want nil", got)
	}
}

func TestClassifyMaestroErr_Table(t *testing.T) {
	// fakeNet is a minimal net.Error for timeout simulation.
	type fakeNetErr struct{ timeout bool }
	_ = (*fakeNetErr)(nil)

	cases := []struct {
		name     string
		input    error
		wantIs   error
		dontWant error
	}{
		{
			name:   "402 → ErrRateLimited",
			input:  &maestroClient.APIError{StatusCode: 402, Message: "quota exceeded"},
			wantIs: connector.ErrRateLimited,
		},
		{
			name:   "429 → ErrRateLimited",
			input:  &maestroClient.APIError{StatusCode: 429, Message: "too many requests"},
			wantIs: connector.ErrRateLimited,
		},
		{
			name:     "404 → ErrNotFound",
			input:    &maestroClient.APIError{StatusCode: 404, Message: "not found"},
			wantIs:   connector.ErrNotFound,
			dontWant: connector.ErrRateLimited,
		},
		{
			name:   "500 → ErrProviderInternal",
			input:  &maestroClient.APIError{StatusCode: 500, Body: "internal server error"},
			wantIs: connector.ErrProviderInternal,
		},
		{
			name:   "502 HTML body → ErrProviderInternal",
			input:  &maestroClient.APIError{StatusCode: 502, Body: "<html>Bad Gateway</html>"},
			wantIs: connector.ErrProviderInternal,
		},
		{
			name:   "503 → ErrProviderInternal",
			input:  &maestroClient.APIError{StatusCode: 503, Message: "service unavailable"},
			wantIs: connector.ErrProviderInternal,
		},
		{
			name:   "generic error → ErrProviderInternal",
			input:  fmt.Errorf("some unknown network error"),
			wantIs: connector.ErrProviderInternal,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyMaestroErr(tc.input)
			if got == nil {
				t.Fatal("classifyMaestroErr returned nil for non-nil input")
			}
			if !errors.Is(got, tc.wantIs) {
				t.Errorf("errors.Is(got, %v) = false, got error: %v", tc.wantIs, got)
			}
			if tc.dontWant != nil && errors.Is(got, tc.dontWant) {
				t.Errorf("errors.Is(got, %v) = true but should NOT match", tc.dontWant)
			}
			// Original error must be preserved in chain.
			if !errors.Is(got, tc.input) {
				t.Errorf("original error not in chain; got: %v", got)
			}
		})
	}
}

// TestClassifyMaestroErr_Timeout verifies that a net.Error with Timeout()=true
// maps to connector.ErrTimeout.
func TestClassifyMaestroErr_Timeout(t *testing.T) {
	netErr := &testNetError{timeout: true}
	got := classifyMaestroErr(netErr)
	if !errors.Is(got, connector.ErrTimeout) {
		t.Errorf("expected ErrTimeout for net.Error timeout, got: %v", got)
	}
}

// testNetError is a minimal net.Error implementation.
type testNetError struct{ timeout bool }

func (e *testNetError) Error() string   { return "test net timeout" }
func (e *testNetError) Timeout() bool   { return e.timeout }
func (e *testNetError) Temporary() bool { return false }

var _ net.Error = (*testNetError)(nil)

// --- Integration-style tests against an httptest server ---

// newTestMaestroProvider builds a MaestroProvider pointed at a test server URL.
func newTestMaestroProvider(t *testing.T, serverURL string) *MaestroProvider {
	t.Helper()
	config := Config{
		ProjectID:   "test-key",
		NetworkName: "preprod",
		NetworkId:   int(constants.PREPROD),
	}
	provider, err := New(config)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	// Point the underlying SDK client at the test server.
	provider.client.BaseUrl = serverURL
	provider.client.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	return provider
}

// TestGetProtocolParameters_402_ClassifiesRateLimited wires a 402 response
// and confirms the returned error carries connector.ErrRateLimited.
func TestGetProtocolParameters_402_ClassifiesRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired) // 402
		w.Write([]byte(`{"code":402,"message":"quota exceeded"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	provider := newTestMaestroProvider(t, srv.URL)
	_, err := provider.GetProtocolParameters(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, connector.ErrRateLimited) {
		t.Errorf("expected connector.ErrRateLimited, got: %v", err)
	}
}

// TestGetUtxosByOutRef_502_ClassifiesProviderInternal wires a 502 HTML error
// and confirms the returned error carries connector.ErrProviderInternal.
func TestGetUtxosByOutRef_502_ClassifiesProviderInternal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(502)
		w.Write([]byte(`<html><body>502 Bad Gateway</body></html>`)) //nolint:errcheck
	}))
	defer srv.Close()

	provider := newTestMaestroProvider(t, srv.URL)
	_, err := provider.GetUtxosByOutRef(context.Background(), []connector.OutRef{
		{TxHash: "deadbeef00000000000000000000000000000000000000000000000000000000", Index: 0},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, connector.ErrProviderInternal) {
		t.Errorf("expected connector.ErrProviderInternal, got: %v", err)
	}
}

// TestGetUtxosByOutRef_404_ClassifiesNotFound confirms a 404 maps to ErrNotFound.
func TestGetUtxosByOutRef_404_ClassifiesNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":404,"message":"utxo not found"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	provider := newTestMaestroProvider(t, srv.URL)
	_, err := provider.GetUtxosByOutRef(context.Background(), []connector.OutRef{
		{TxHash: "deadbeef00000000000000000000000000000000000000000000000000000000", Index: 0},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, connector.ErrNotFound) {
		t.Errorf("expected connector.ErrNotFound, got: %v", err)
	}
}

// TestEvaluateTx_429_ClassifiesRateLimited confirms EvaluateTx 429 → ErrRateLimited.
func TestEvaluateTx_429_ClassifiesRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"code":429,"message":"too many requests"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	provider := newTestMaestroProvider(t, srv.URL)
	_, err := provider.EvaluateTx(context.Background(), []byte("deadbeef"), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, connector.ErrRateLimited) {
		t.Errorf("expected connector.ErrRateLimited, got: %v", err)
	}
}
