package gamebank

import (
	"errors"
	"fmt"
)

// Error is any failure from the Game Banking Engine or from reaching it.
// Use errors.As to read it, and errors.Is with the Err* sentinels to test
// its category:
//
//	var e *gamebank.Error
//	if errors.Is(err, gamebank.ErrInsufficientFunds) && errors.As(err, &e) {
//		log.Printf("available %v", e.Details["available_balance"])
//	}
type Error struct {
	// Status is the HTTP status, or 0 when no response arrived.
	Status int
	// Code is the engine's stable error code, e.g. "insufficient_funds".
	Code    string
	Message string
	// Details carries extra context, e.g. available_balance.
	Details map[string]any
	// RequestID is the engine's correlation ID (X-Request-Id).
	RequestID string
	// RetryAfter is the number of seconds the engine asked the caller to wait.
	RetryAfter float64

	cause error
}

func (e *Error) Error() string {
	s := fmt.Sprintf("gamebank: %s: %s", e.Code, e.Message)
	if e.RequestID != "" {
		s += " (request " + e.RequestID + ")"
	}
	return s
}

func (e *Error) Unwrap() error { return e.cause }

type kind string

func (k kind) Error() string { return "gamebank: " + string(k) }

// Error categories, for errors.Is.
var (
	ErrAuthentication    error = kind("authentication failed (401)")
	ErrPermissionDenied  error = kind("permission denied (403)")
	ErrNotFound          error = kind("not found (404)")
	ErrConflict          error = kind("conflict (409)")
	ErrInvalidRequest    error = kind("invalid request (400, 413, 415)")
	ErrUnprocessable     error = kind("business rule failed (422)")
	ErrInsufficientFunds error = kind("insufficient funds")
	ErrLimitExceeded     error = kind("limit exceeded")
	ErrRateLimited       error = kind("rate limited (429)")
	ErrServer            error = kind("server error (5xx)")
	ErrConnection        error = kind("no response from the engine")
)

// Is reports whether the error belongs to a category; InsufficientFunds and
// LimitExceeded are also Unprocessable.
func (e *Error) Is(target error) bool {
	switch target {
	case ErrConnection:
		return e.Status == 0 && (e.Code == "connection_error" || e.Code == "timeout")
	case ErrAuthentication:
		return e.Status == 401
	case ErrPermissionDenied:
		return e.Status == 403
	case ErrNotFound:
		return e.Status == 404
	case ErrConflict:
		return e.Status == 409
	case ErrRateLimited:
		return e.Status == 429
	case ErrUnprocessable:
		return e.Status == 422
	case ErrInsufficientFunds:
		return e.Status == 422 && e.Code == "insufficient_funds"
	case ErrLimitExceeded:
		return e.Status == 422 && e.Code == "limit_exceeded"
	case ErrServer:
		return e.Status >= 500
	case ErrInvalidRequest:
		return (e.Status >= 400 && e.Status < 500 && !isOther4xx(e.Status)) || (e.Status == 0 && e.Code == "validation_error")
	}
	return false
}

func isOther4xx(s int) bool {
	switch s {
	case 401, 403, 404, 409, 422, 429:
		return true
	}
	return false
}

// AsError returns err as an *Error when it is one.
func AsError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
