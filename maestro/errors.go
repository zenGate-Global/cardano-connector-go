package maestro

import (
	"errors"
	"fmt"
	"net"
	"os"

	maestroClient "github.com/maestro-org/go-sdk/client"
	connector "github.com/zenGate-Global/cardano-connector-go"
)

// classifyMaestroErr maps SDK errors from the maestro fork into connector
// sentinels.  The original error is always preserved in the chain via %w so
// callers can still errors.As to *maestroClient.APIError if needed.
//
// Priority (first match wins):
//  1. Rate-limited (402 / 429)   → connector.ErrRateLimited
//  2. Not found (404)            → connector.ErrNotFound
//  3. Server error (5xx)         → connector.ErrProviderInternal
//  4. Context/network timeout    → connector.ErrTimeout
//  5. Everything else            → connector.ErrProviderInternal
func classifyMaestroErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, maestroClient.ErrRateLimited):
		return fmt.Errorf("%w: %w", connector.ErrRateLimited, err)
	case errors.Is(err, maestroClient.ErrNotFound):
		return fmt.Errorf("%w: %w", connector.ErrNotFound, err)
	case errors.Is(err, maestroClient.ErrServerError):
		return fmt.Errorf("%w: %w", connector.ErrProviderInternal, err)
	case isNetworkTimeout(err):
		return fmt.Errorf("%w: %w", connector.ErrTimeout, err)
	default:
		return fmt.Errorf("%w: %w", connector.ErrProviderInternal, err)
	}
}

// isNetworkTimeout reports whether err represents a deadline / network timeout.
func isNetworkTimeout(err error) bool {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
