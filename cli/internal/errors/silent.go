package errors

import (
	"context"
	"errors"
)

// SilentError wraps an error to indicate it should not be printed to the user.
// This is useful for expected user cancellations, validation failures that
// have already been handled, etc.
type SilentError struct {
	err error
}

// Error implements the error interface.
func (s *SilentError) Error() string {
	return s.err.Error()
}

// Unwrap implements the unwrapper interface for error chains.
func (s *SilentError) Unwrap() error {
	return s.err
}

// NewSilent wraps an error as a silent error that shouldn't be printed.
func NewSilent(err error) error {
	if err == nil {
		return nil
	}
	return &SilentError{err: err}
}

// IsSilent checks if an error is marked as silent and shouldn't be printed.
// errors.As, not eris.As: kong wraps every command error in errors.Join,
// which eris can't see into.
func IsSilent(err error) bool {
	if err == nil {
		return false
	}

	var silentErr *SilentError
	return errors.As(err, &silentErr)
}

// ShouldPrint reports whether main should print and capture err. Silent errors
// and errors matching [context.Canceled] suppress the whole error, including
// mixed joins. Verbose logging happens separately.
func ShouldPrint(err error) bool {
	return !isSilentOrCancel(err)
}

func isSilentOrCancel(err error) bool {
	return IsSilent(err) || errors.Is(err, context.Canceled)
}

// JoinFailures joins separately supplied real failures. A cancel (Ctrl+C) or
// silent error supplied as its own argument isn't a real failure, so it can't
// hide one; it's returned only if nothing failed. A single [errors.Join] of a
// real error and a cancel is classified wholesale by isSilentOrCancel, so the
// real error is discarded in that form — pass failures as separate arguments
// to preserve them.
func JoinFailures(errs ...error) error {
	var failures []error
	var canceled error
	for _, err := range errs {
		switch {
		case err == nil:
		case isSilentOrCancel(err):
			if canceled == nil {
				canceled = err
			}
		default:
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	return canceled
}
