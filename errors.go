package connector

import (
	"errors"
	"fmt"
)

// Common error values returned by Provider implementations.
var (
	// ErrNotFound indicates that the requested resource could not be found.
	// For example, a UTxO, transaction, or datum hash not present on the chain/service.
	ErrNotFound = errors.New("connector: resource not found")

	// ErrRateLimited indicates that the request was rate-limited by the underlying provider.
	ErrRateLimited = errors.New("connector: request rate limited by provider")

	// ErrTxSubmissionFailed indicates a general failure during transaction submission.
	// This could be due to network issues, node rejection for reasons other than script failure, etc.
	// For specific script validation failures during submission, ErrEvaluationFailed might be more appropriate
	// if the provider distinguishes this.
	ErrTxSubmissionFailed = errors.New(
		"connector: transaction submission failed",
	)

	// ErrEvaluationFailed indicates that transaction script evaluation failed.
	// This is typically returned by the EvaluateTx method or if a SubmitTx attempt
	// results in a clear script validation error from the node/provider.
	ErrEvaluationFailed = errors.New(
		"connector: transaction script evaluation failed",
	)

	// ErrInvalidAddress indicates that a provided address or credential string was malformed
	// or could not be parsed by the provider.
	ErrInvalidAddress = errors.New(
		"connector: invalid address or credential format",
	)

	// ErrInvalidUnit indicates that a provided unit was malformed
	ErrInvalidUnit = errors.New("connector: invalid unit format")

	// ErrNotImplemented indicates that a specific method in the Provider interface
	// is not implemented by the current concrete provider.
	ErrNotImplemented = errors.New(
		"connector: method not implemented by this provider",
	)

	// ErrInvalidInput indicates that one or more inputs to a provider method were invalid
	// (e.g., malformed transaction hash, incorrect parameter type).
	ErrInvalidInput = errors.New("connector: invalid input provided to method")

	// ErrProviderInternal indicates an unexpected error occurred within the provider itself
	// or the underlying service it communicates with, not directly attributable to user input
	// or standard blockchain conditions like "not found".
	ErrProviderInternal = errors.New("connector: internal provider error")

	// ErrTimeout indicates that an operation timed out, often related to context cancellation
	// or a provider-specific timeout for an API call.
	ErrTimeout = errors.New("connector: operation timed out")

	// ErrTxTooLarge indicates that the transaction is too large to be submitted.
	ErrTxTooLarge = errors.New("connector: transaction too large")

	// ErrValueNotConserved indicates an error related to transaction value balancing.
	ErrValueNotConserved = errors.New(
		"connector: transaction value not conserved",
	)

	// ErrBadInputs indicates that some of the transaction inputs are bad (e.g. already spent).
	ErrBadInputs = errors.New("connector: bad transaction inputs")

	// ErrMultipleUTXOs indicates that multiple UTXOs were found for a given unit.
	ErrMultipleUTXOs = errors.New(
		"connector: multiple UTXOs found for a given unit",
	)
)

// APIError represents a more detailed error from a provider's API.
// Providers can wrap more specific API errors in this struct or return it directly
// if the error doesn't map cleanly to one of the standard connector.ErrXxx values.
type APIError struct {
	// StatusCode is the HTTP status code from the API provider, if applicable.
	StatusCode int
	// ProviderCode is a provider-specific error code or string, if available.
	ProviderCode string
	// Message is a human-readable error message.
	Message string
	// Details can hold additional structured error information from the provider.
	Details interface{}
	// UnderlyingErr is the original error returned by the HTTP client or other internal operations.
	UnderlyingErr error
}

// Error implements the error interface for APIError.
func (e *APIError) Error() string {
	if e.UnderlyingErr != nil {
		return fmt.Sprintf(
			"API error: status %d, provider_code '%s', msg '%s' (underlying: %v)",
			e.StatusCode,
			e.ProviderCode,
			e.Message,
			e.UnderlyingErr,
		)
	}
	return fmt.Sprintf(
		"API error: status %d, provider_code '%s', msg '%s'",
		e.StatusCode,
		e.ProviderCode,
		e.Message,
	)
}

// Unwrap provides compatibility for errors.Is and errors.As.
func (e *APIError) Unwrap() error {
	return e.UnderlyingErr
}

// --- Helper functions for error checking ---

// IsNotFound checks if an error is, or wraps, ErrNotFound.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsRateLimited checks if an error is, or wraps, ErrRateLimited.
func IsRateLimited(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

// IsEvaluationFailed checks if an error is, or wraps, ErrEvaluationFailed.
func IsEvaluationFailed(err error) bool {
	return errors.Is(err, ErrEvaluationFailed)
}

// (Add similar IsXxx helpers for other common errors as needed)
