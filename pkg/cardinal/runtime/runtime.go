// Package runtime defines a transport-independent interface for a Cardinal simulation module.
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

// InitRequest contains an optional snapshot. Initialize borrows Snapshot only for the call.
type InitRequest struct {
	Snapshot []byte
}

// TickRequest contains the data for one deterministic simulation step. Tick borrows Input only for
// the call.
type TickRequest struct {
	Tick         uint64
	FixedDeltaNS uint64
	Input        []byte
}

// QueryRequest contains an application-defined query. Query borrows Input only for the call.
type QueryRequest struct {
	Kind  uint32
	Input []byte
}

// Runner owns one isolated module instance.
//
// Tick, Query, and Snapshot write to output buffers that the caller owns. Each method returns the
// number of bytes that it writes. These methods return BufferSizeError if an output buffer is too
// small. A call that returns BufferSizeError does not change the module state or the output buffer.
//
// An implementation borrows all request and output buffers. It must not keep a buffer after the
// method returns. The caller can retry a call after BufferSizeError. The caller must not use the
// same Runner between the first call and the retry.
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

// BufferSizeError reports the output capacity that an operation requires.
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
