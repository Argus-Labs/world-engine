//go:build cgo && (linux || darwin)

// Package nativeaot loads Cardinal NativeAOT modules through C ABI version 1.
// Modules exchange typed protobuf messages. Open checks the module contract before it creates a handle.
// Runner serializes calls on each handle. It decodes module-owned bytes into caller-owned messages.
// Close destroys the handle. The shared library stays loaded until the process exits.
package nativeaot

/*
#cgo CFLAGS: -std=c11
#cgo linux LDFLAGS: -ldl
#include <stdlib.h>
#include "loader_unix.h"
*/
import "C"

import (
	"bytes"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalruntime "github.com/argus-labs/world-engine/pkg/cardinal/runtime"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Runner owns one NativeAOT module handle. Runner serializes all calls that use this handle.
type Runner[In, Out, Snap proto.Message] struct {
	mu       sync.Mutex
	library  *C.cardinal_nativeaot_library_v1
	handle   C.cardinal_runtime_handle_v1
	contract cardinalruntime.Contract
	closed   bool
	inBuf    []byte
	snapType protoreflect.MessageType
}

// Open loads a trusted module. Type arguments must be generated protobuf message pointer types.
// It checks the module ABI, name, version, and protobuf type names. Open then calls create
// to make one handle. If a contract check fails, Open does not call create or pass the configuration.
func Open[In, Out, Snap proto.Message](
	path string,
	config []byte,
	expectedName string,
	expectedVersion string,
) (*Runner[In, Out, Snap], error) {
	inType, err := messageType[In]()
	if err != nil {
		return nil, eris.Wrap(err, "input type")
	}
	outType, err := messageType[Out]()
	if err != nil {
		return nil, eris.Wrap(err, "output type")
	}
	snapType, err := messageType[Snap]()
	if err != nil {
		return nil, eris.Wrap(err, "snapshot type")
	}

	// C.CString stops at the first NUL byte. Reject a path that contains a NUL byte.
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return nil, eris.Wrap(cardinalruntime.ErrInvalidArgument, "open NativeAOT runtime")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, eris.Wrapf(err, "resolve NativeAOT library %q", path)
	}
	path, err = filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return nil, eris.Wrapf(err, "resolve NativeAOT library %q", absolutePath)
	}

	// Keep this goroutine on one native thread while it reads a pre-handle error.
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var loadError [1024]byte
	library := C.cardinal_nativeaot_library_open(
		cPath,
		(*C.char)(unsafe.Pointer(&loadError[0])),
		C.uint64_t(len(loadError)),
	)
	if library == nil {
		message, _, terminated := bytes.Cut(loadError[:], []byte{0})
		assert.That(terminated, "dynamic loader error is not NUL-terminated")

		return nil, eris.Errorf("load NativeAOT library %q: %s", path, string(message))
	}

	runner := &Runner[In, Out, Snap]{library: library, snapType: snapType}
	contract, err := runner.loadContract(cardinalruntime.Contract{
		Name:         expectedName,
		Version:      expectedVersion,
		InputType:    string(inType.Descriptor().FullName()),
		OutputType:   string(outType.Descriptor().FullName()),
		SnapshotType: string(snapType.Descriptor().FullName()),
	})
	if err != nil {
		C.cardinal_nativeaot_library_forget(library)
		return nil, eris.Wrap(err, "load NativeAOT contract")
	}
	runner.contract = contract

	createResult := C.cardinal_nativeaot_create(
		library,
		bytePointer(config),
		C.uint64_t(len(config)),
	)
	goruntime.KeepAlive(config)
	if createResult.status != C.CARDINAL_RUNTIME_STATUS_SUCCESS {
		err = runner.statusErrorLocked("create", createResult.status)
		C.cardinal_nativeaot_library_forget(library)
		return nil, eris.Wrap(err, "create NativeAOT runtime")
	}
	assert.That(createResult.handle != 0, "created runtime handle must not be zero")

	runner.handle = createResult.handle
	return runner, nil
}

func messageType[T proto.Message]() (protoreflect.MessageType, error) {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil || t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct {
		return nil, eris.Wrap(cardinalruntime.ErrInvalidArgument, "expected a generated protobuf message pointer type")
	}

	prototype, ok := reflect.New(t.Elem()).Interface().(T)
	assert.That(ok, "new protobuf prototype must have the requested Go type")

	messageType := prototype.ProtoReflect().Type()
	if _, ok = messageType.New().Interface().(T); !ok {
		return nil, eris.Wrapf(cardinalruntime.ErrInvalidArgument, "protobuf reflection cannot construct %v", t)
	}

	return messageType, nil
}

func (r *Runner[In, Out, Snap]) loadContract(expected cardinalruntime.Contract) (cardinalruntime.Contract, error) {
	assert.That(r.library != nil, "runtime library must not be nil")

	var raw C.cardinal_runtime_contract_v1
	status := C.cardinal_nativeaot_get_contract(r.library, &raw)
	if status != C.CARDINAL_RUNTIME_STATUS_SUCCESS {
		return cardinalruntime.Contract{}, r.statusErrorLocked("get contract", status)
	}

	if uint32(raw.abi_version) != cardinalruntime.ABIVersion {
		return cardinalruntime.Contract{}, eris.Wrapf(
			cardinalruntime.ErrABIMismatch,
			"get contract: module=%d host=%d",
			uint32(raw.abi_version),
			cardinalruntime.ABIVersion,
		)
	}

	contract := cardinalruntime.Contract{ABIVersion: uint32(raw.abi_version)}
	for _, field := range []struct {
		name     string
		data     []C.char
		dest     *string
		expected string
	}{
		{"name", raw.name[:], &contract.Name, expected.Name},
		{"version", raw.version[:], &contract.Version, expected.Version},
		{"input_type", raw.input_type[:], &contract.InputType, expected.InputType},
		{"output_type", raw.output_type[:], &contract.OutputType, expected.OutputType},
		{"snapshot_type", raw.snapshot_type[:], &contract.SnapshotType, expected.SnapshotType},
	} {
		value, _, terminated := bytes.Cut(
			unsafe.Slice((*byte)(unsafe.Pointer(&field.data[0])), len(field.data)), []byte{0},
		)
		if !terminated {
			return cardinalruntime.Contract{}, eris.Wrapf(
				cardinalruntime.ErrABIMismatch, "get contract: %s is not NUL-terminated", field.name,
			)
		}
		*field.dest = string(value)
		if *field.dest != field.expected {
			return cardinalruntime.Contract{}, eris.Wrap(
				&cardinalruntime.ContractMismatchError{
					Field: field.name, Expected: field.expected, Actual: *field.dest,
				},
				"validate NativeAOT contract",
			)
		}
	}
	return contract, nil
}

// Contract returns the module contract that Open copied from the shared library.
func (r *Runner[In, Out, Snap]) Contract() cardinalruntime.Contract {
	return r.contract
}

// Initialize initializes the handle. A nil snapshot starts a new world.
func (r *Runner[In, Out, Snap]) Initialize(snapshot Snap) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	assert.That(!r.closed, "runtime runner is closed")

	r.inBuf = r.inBuf[:0]
	if !nilMessage(snapshot) {
		var err error
		r.inBuf, err = proto.MarshalOptions{}.MarshalAppend(r.inBuf, snapshot)
		if err != nil {
			return eris.Wrapf(cardinalruntime.ErrInvalidArgument, "initialize: marshal snapshot: %v", err)
		}
	}
	status := C.cardinal_nativeaot_initialize(
		r.library,
		r.handle,
		bytePointer(r.inBuf),
		C.uint64_t(len(r.inBuf)),
	)
	goruntime.KeepAlive(r.inBuf)

	return r.statusErrorLocked("initialize", status)
}

// Tick runs one simulation step and replaces the contents of the caller-owned output message.
// Input and output must be non-nil. The caller must not access output concurrently with this call.
// Decoding completes under the handle lock; output does not borrow the module's native buffer.
// On error, output must not be consumed: decoding may have partially modified it.
func (r *Runner[In, Out, Snap]) Tick(tick uint64, fixedDeltaNS uint64, input In, output Out) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	assert.That(!r.closed, "runtime runner is closed")

	if nilMessage(input) {
		return eris.Wrap(cardinalruntime.ErrInvalidArgument, "tick: input is nil")
	}
	if nilMessage(output) {
		return eris.Wrap(cardinalruntime.ErrInvalidArgument, "tick: output is nil")
	}
	var err error
	r.inBuf, err = proto.MarshalOptions{}.MarshalAppend(r.inBuf[:0], input)
	if err != nil {
		return eris.Wrapf(cardinalruntime.ErrInvalidArgument, "tick: marshal input: %v", err)
	}
	result := C.cardinal_nativeaot_tick(
		r.library,
		r.handle,
		C.uint64_t(tick),
		C.uint64_t(fixedDeltaNS),
		bytePointer(r.inBuf),
		C.uint64_t(len(r.inBuf)),
	)
	goruntime.KeepAlive(r.inBuf)

	return r.decodeOutputLocked("tick", result, output)
}

// Snapshot returns a new protobuf message containing the handle state.
func (r *Runner[In, Out, Snap]) Snapshot() (Snap, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	assert.That(!r.closed, "runtime runner is closed")

	result := C.cardinal_nativeaot_snapshot(r.library, r.handle)
	output, ok := r.snapType.New().Interface().(Snap)
	assert.That(ok, "protobuf snapshot type must match the type validated by Open")
	if err := r.decodeOutputLocked("snapshot", result, output); err != nil {
		var zero Snap
		return zero, err
	}
	return output, nil
}

// Restore replaces the handle state with a non-nil snapshot.
func (r *Runner[In, Out, Snap]) Restore(snapshot Snap) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	assert.That(!r.closed, "runtime runner is closed")

	if nilMessage(snapshot) {
		return eris.Wrap(cardinalruntime.ErrInvalidArgument, "restore: snapshot is nil")
	}
	var err error
	r.inBuf, err = proto.MarshalOptions{}.MarshalAppend(r.inBuf[:0], snapshot)
	if err != nil {
		return eris.Wrapf(cardinalruntime.ErrInvalidArgument, "restore: marshal snapshot: %v", err)
	}
	status := C.cardinal_nativeaot_restore(
		r.library,
		r.handle,
		bytePointer(r.inBuf),
		C.uint64_t(len(r.inBuf)),
	)
	goruntime.KeepAlive(r.inBuf)

	return r.statusErrorLocked("restore", status)
}

// Close destroys the module handle. Call Close only one time. The process keeps the shared library
// loaded.
func (r *Runner[In, Out, Snap]) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	assert.That(!r.closed, "runtime runner is closed")

	// Destroy consumes the handle, even when it fails. Stay on one native thread while reading the
	// destroy error.
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()

	status := C.cardinal_nativeaot_destroy(r.library, r.handle)
	var err error
	if status != C.CARDINAL_RUNTIME_STATUS_SUCCESS {
		err = r.statusErrorLocked("destroy", status)
	}

	library := r.library
	r.closed = true
	r.handle = 0
	r.library = nil
	C.cardinal_nativeaot_library_forget(library)

	return err
}

func (r *Runner[In, Out, Snap]) decodeOutputLocked(
	operation string,
	result C.cardinal_nativeaot_call_result_v1,
	output proto.Message,
) error {
	if err := r.statusErrorLocked(operation, result.status); err != nil {
		return err
	}
	outputLen, valid := uint64ToInt(result.output_len)
	if !valid {
		return eris.Wrapf(
			cardinalruntime.ErrExecutionFailed,
			"%s: module reported an output length too large for this host",
			operation,
		)
	}

	if result.output == nil && outputLen != 0 {
		return eris.Wrapf(cardinalruntime.ErrExecutionFailed, "%s: module returned nil output with nonzero length", operation)
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(result.output)), outputLen)
	if err := proto.Unmarshal(data, output); err != nil {
		return eris.Wrapf(cardinalruntime.ErrExecutionFailed, "%s: unmarshal output: %v", operation, err)
	}
	return nil
}

func (r *Runner[In, Out, Snap]) statusErrorLocked(operation string, status C.int32_t) error {
	if status == C.CARDINAL_RUNTIME_STATUS_SUCCESS {
		return nil
	}

	var base error
	switch status {
	case C.CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT:
		base = cardinalruntime.ErrInvalidArgument
	case C.CARDINAL_RUNTIME_STATUS_INVALID_HANDLE:
		base = cardinalruntime.ErrInvalidHandle
	case C.CARDINAL_RUNTIME_STATUS_INVALID_STATE:
		base = cardinalruntime.ErrInvalidState
	case C.CARDINAL_RUNTIME_STATUS_UNSUPPORTED:
		base = cardinalruntime.ErrUnsupported
	case C.CARDINAL_RUNTIME_STATUS_EXECUTION_FAILED:
		base = cardinalruntime.ErrExecutionFailed
	case C.CARDINAL_RUNTIME_STATUS_ABI_MISMATCH:
		base = cardinalruntime.ErrABIMismatch
	default:
		base = cardinalruntime.ErrExecutionFailed
	}

	var local [C.CARDINAL_RUNTIME_V1_LAST_ERROR_CAPACITY]byte
	result := C.cardinal_nativeaot_last_error(
		r.library,
		r.handle,
		bytePointer(local[:]),
	)
	goruntime.KeepAlive(local)

	var message string
	if result.status == C.CARDINAL_RUNTIME_STATUS_SUCCESS {
		length, valid := uint64ToInt(result.output_len)
		if valid && length <= len(local) {
			message = string(local[:length])
		}
	}

	if message == "" {
		return eris.Wrapf(base, "%s: status %d", operation, int32(status))
	}
	return eris.Wrapf(base, "%s: %s", operation, message)
}

// nilMessage checks for a nil pointer without reflection. The proto.Message constraint does not
// require a pointer or a comparable type. Thus, Go rejects both message == nil and message == zero.
// An interface that contains a typed nil pointer is not a nil interface. Compare two interfaces with
// the same concrete type instead. Open checks that T is a pointer type, so this comparison is safe and
// needs no allocation.
func nilMessage[T proto.Message](message T) bool {
	var zero T
	return any(message) == any(zero)
}

func bytePointer(data []byte) *C.uint8_t {
	if len(data) == 0 {
		return nil
	}
	return (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(data)))
}

func uint64ToInt(value C.uint64_t) (int, bool) {
	const maxInt = int(^uint(0) >> 1)
	if uint64(value) > uint64(maxInt) {
		return 0, false
	}
	return int(value), true
}
