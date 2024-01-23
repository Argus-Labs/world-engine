package errors

import "errors"

// Error codes
const (
	OK = iota
	Cancelled
	Unknown
	InvalidArgument
	DeadlineExceeded
	NotFound
	AlreadyExists
	PermissionDenied
	ResourceExhausted
	FailedPrecondition
	Aborted
	OutOfRange
	Unimplemented
	Internal
	Unavailable
	DataLoss
	Unauthenticated
)

var (
	// Persona errors
	ErrPersonaTagStorageObjNotFound = errors.New("persona tag storage object not found")
	ErrNoPersonaTagForUser          = errors.New("user does not have a verified persona tag")
	ErrPersonaSignerAvailable       = errors.New("persona signer is available")
	ErrPersonaSignerUnknown         = errors.New("persona signer is unknown")

	// Private Key errros
	ErrNoStorageObjectFound       = errors.New("no storage object found")
	ErrTooManyStorageObjectsFound = errors.New("too many storage objects found")

	// Allowlist errors
	ErrNotAllowlisted     = errors.New("this user is not allowlisted")
	ErrInvalidBetaKey     = errors.New("invalid beta key")
	ErrBetaKeyAlreadyUsed = errors.New("beta key already used")
	ErrAlreadyVerified    = errors.New("this user is already verified by an existing beta key")
)
