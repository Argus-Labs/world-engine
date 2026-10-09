// Package runtime defines transport-independent contracts and errors for Cardinal simulation modules.
package runtime

import (
	"errors"
	"fmt"
	"strconv"
)

const ABIVersion uint32 = 1

// Contract identifies a module and the protobuf full names of its input, output, and snapshot types.
type Contract struct {
	ABIVersion   uint32
	Name         string
	Version      string
	InputType    string
	OutputType   string
	SnapshotType string
}

var (
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
	case "input_type":
		return fmt.Sprintf("%s: module input type %s, want %s", ErrContractMismatch, actual, expected)
	case "output_type":
		return fmt.Sprintf("%s: module output type %s, want %s", ErrContractMismatch, actual, expected)
	case "snapshot_type":
		return fmt.Sprintf("%s: module snapshot type %s, want %s", ErrContractMismatch, actual, expected)
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
