// Package runtime defines the transport-neutral boundary for running a Cardinal simulation module.
package runtime

import (
	"errors"
	"fmt"
	"strconv"
)

const ABIVersion uint32 = 1

// Contract identifies a module.
type Contract struct {
	ABIVersion uint32
	Name       string
	Version    string
}

// InitRequest supplies an optional snapshot to initialize a newly created runner. The snapshot is
// borrowed for the duration of Initialize.
type InitRequest struct {
	Snapshot []byte
}

// TickRequest supplies one deterministic simulation step. Input is borrowed for the duration of
// Tick.
type TickRequest struct {
	Tick         uint64
	FixedDeltaNS uint64
	Input        []byte
}

// QueryRequest supplies an application-defined query. Input is borrowed for the duration of Query.
type QueryRequest struct {
	Kind  uint32
	Input []byte
}

// Runner owns one isolated module instance.
//
// Tick, Query, and Snapshot write into caller-owned output buffers and return the number of bytes
// written. Implementations return BufferSizeError when the buffer is too small. Implementations
// must not retain request or output buffers after a call returns. Callers that probe and retry
// after BufferSizeError must prevent another operation from using the same Runner between those
// calls.
type Runner interface {
	Contract() Contract
	Initialize(InitRequest) error
	Tick(TickRequest, []byte) (int, error)
	Query(QueryRequest, []byte) (int, error)
	Snapshot([]byte) (int, error)
	Restore([]byte) error
	Close() error
}

var (
	ErrBufferTooSmall   = errors.New("runtime output buffer too small")
	ErrInvalidArgument  = errors.New("runtime invalid argument")
	ErrInvalidHandle    = errors.New("runtime invalid handle")
	ErrInvalidState     = errors.New("runtime invalid state")
	ErrUnsupported      = errors.New("runtime operation unsupported")
	ErrExecutionFailed  = errors.New("runtime execution failed")
	ErrABIMismatch      = errors.New("runtime ABI mismatch")
	ErrContractMismatch = errors.New("runtime contract mismatch")
)

// ContractMismatchError identifies one incompatible module contract field.
type ContractMismatchError struct {
	Field    string
	Expected any
	Actual   any
}

func (e *ContractMismatchError) Error() string {
	expected := formatContractValue(e.Expected)
	actual := formatContractValue(e.Actual)

	switch e.Field {
	case "name":
		return fmt.Sprintf("%s: module name %s, want %s", ErrContractMismatch, actual, expected)
	case "version":
		return fmt.Sprintf("%s: module version %s, want %s", ErrContractMismatch, actual, expected)
	default:
		return fmt.Sprintf("%s: %s: got %s, want %s", ErrContractMismatch, e.Field, actual, expected)
	}
}

// Unwrap makes ContractMismatchError match ErrContractMismatch with errors.Is.
func (e *ContractMismatchError) Unwrap() error {
	return ErrContractMismatch
}

func formatContractValue(value any) string {
	switch value := value.(type) {
	case string:
		return strconv.Quote(value)
	default:
		return fmt.Sprint(value)
	}
}

// BufferSizeError reports the caller-owned output capacity required by a call.
type BufferSizeError struct {
	Operation string
	Required  int
	Provided  int
}

func (e *BufferSizeError) Error() string {
	return e.Operation + ": output buffer too small: required " +
		strconv.Itoa(e.Required) + " bytes, provided " + strconv.Itoa(e.Provided)
}

// Unwrap makes BufferSizeError match ErrBufferTooSmall with errors.Is.
func (e *BufferSizeError) Unwrap() error {
	return ErrBufferTooSmall
}
