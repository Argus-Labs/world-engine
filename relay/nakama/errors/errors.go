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
	// Private Key errros
	ErrNoStorageObjectFound       = errors.New("no storage object found")
	ErrTooManyStorageObjectsFound = errors.New("too many storage objects found")
)
