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
func IsSilent(err error) bool {
	if err == nil {
		return false
	}

	var silentErr *SilentError
	// stdlib errors.As crosses errors.Join's Unwrap() []error (kong wraps every command
	// error this way); eris.As only walks Unwrap() error and would miss a SilentError.
	return errors.As(err, &silentErr)
}

// ShouldPrint returns true if the error should be printed to the user.
// This is the inverse of IsSilent for more readable code.
func ShouldPrint(err error) bool {
	return !IsSilent(err)
}
