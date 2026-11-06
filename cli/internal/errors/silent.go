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

// ShouldPrint returns true if the error should be printed to the user as normal
// error output (and captured by Sentry). A cancel (Ctrl+C) is an expected user
// action, not a failure — JoinFailures treats it the same way — so it is never
// printed or captured by Sentry, regardless of where it originated. This is the
// safety net for call sites that return a cancel without first wrapping it in
// NewSilent. --verbose still logs the full chain via logger.Errors before this
// gate is reached, so a silenced cancel is only hidden from the normal output.
//
// The cancel check uses [errors.Is], which walks every branch of an [errors.Join],
// so a join that carries a cancel anywhere is suppressed wholesale — unlike
// JoinFailures, which strips cancels and preserves any real failure. No
// production route produces such a raw mixed join at this gate (kong pairs
// runErr with a nil hookErr; phasebox wraps canceled joins in NewSilent).
func ShouldPrint(err error) bool {
	return !IsSilent(err) && !errors.Is(err, context.Canceled)
}

// JoinFailures joins errs' real failures. A cancel (Ctrl+C) or silent error
// isn't one, so it can't hide one; it's returned only if nothing failed.
func JoinFailures(errs ...error) error {
	var failures []error
	var canceled error
	for _, err := range errs {
		switch {
		case err == nil:
		case IsSilent(err) || errors.Is(err, context.Canceled):
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
