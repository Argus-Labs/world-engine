package errors

import "errors"

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
//
// Uses the standard errors.As (not eris.As) so it traverses errors.Join wrappers
// (Unwrap() []error), which kong applies to command return values. eris.As v0.5.4
// only supports Unwrap() error and cannot see through a join, causing a SilentError
// returned from a command (e.g. spinner.Run on Ctrl+C) to be invisibly printed and
// sent to Sentry instead of being suppressed.
func IsSilent(err error) bool {
	if err == nil {
		return false
	}

	var silentErr *SilentError
	return errors.As(err, &silentErr)
}

// ShouldPrint returns true if the error should be printed to the user.
// This is the inverse of IsSilent for more readable code.
func ShouldPrint(err error) bool {
	return !IsSilent(err)
}
