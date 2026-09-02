//go:build cgo && (linux || darwin)

// Package nativeaot loads Cardinal NativeAOT modules through a stable C ABI. The process keeps
// each shared library loaded until the process exits.
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
	goruntime "runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalruntime "github.com/argus-labs/world-engine/pkg/cardinal/runtime"
	"github.com/rotisserie/eris"
)

// Runner owns one NativeAOT module handle. Runner serializes all calls that use this handle.
type Runner struct {
	mu       sync.Mutex
	library  *C.cardinal_nativeaot_library_v1
	handle   C.cardinal_runtime_handle_v1
	contract cardinalruntime.Contract
	closed   bool
}

var _ cardinalruntime.Runner = (*Runner)(nil)

// Open loads a trusted module. It checks the module ABI, name, and version. Open then calls create
// to make one handle. If a contract check fails, Open does not call create or pass the configuration.
func Open(
	path string,
	config []byte,
	expectedName string,
	expectedVersion string,
) (*Runner, error) {
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
		C.size_t(len(loadError)),
	)
	if library == nil {
		message, _, terminated := bytes.Cut(loadError[:], []byte{0})
		assert.That(terminated, "dynamic loader error is not NUL-terminated")

		return nil, eris.Errorf("load NativeAOT library %q: %s", path, string(message))
	}

	runner := &Runner{library: library}
	contract, err := runner.loadContract()
	if err != nil {
		C.cardinal_nativeaot_library_forget(library)
		return nil, eris.Wrap(err, "load NativeAOT contract")
	}
	if contract.Name != expectedName {
		C.cardinal_nativeaot_library_forget(library)
		return nil, eris.Wrap(
			&cardinalruntime.ContractMismatchError{
				Field:    "name",
				Expected: expectedName,
				Actual:   contract.Name,
			},
			"validate NativeAOT contract",
		)
	}
	if contract.Version != expectedVersion {
		C.cardinal_nativeaot_library_forget(library)
		return nil, eris.Wrap(
			&cardinalruntime.ContractMismatchError{
				Field:    "version",
				Expected: expectedVersion,
				Actual:   contract.Version,
			},
			"validate NativeAOT contract",
		)
	}
	runner.contract = contract

	createResult := C.cardinal_nativeaot_create(
		library,
		bytePointer(config),
		C.size_t(len(config)),
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

func (r *Runner) loadContract() (cardinalruntime.Contract, error) {
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

	name, _, terminated := bytes.Cut(
		unsafe.Slice((*byte)(unsafe.Pointer(&raw.name[0])), len(raw.name)),
		[]byte{0},
	)
	if !terminated {
		return cardinalruntime.Contract{}, eris.Wrap(
			cardinalruntime.ErrABIMismatch,
			"get contract: name is not NUL-terminated",
		)
	}
	version, _, terminated := bytes.Cut(
		unsafe.Slice((*byte)(unsafe.Pointer(&raw.version[0])), len(raw.version)),
		[]byte{0},
	)
	if !terminated {
		return cardinalruntime.Contract{}, eris.Wrap(
			cardinalruntime.ErrABIMismatch,
			"get contract: version is not NUL-terminated",
		)
	}

	contract := cardinalruntime.Contract{
		ABIVersion: uint32(raw.abi_version),
		Name:       string(name),
		Version:    string(version),
	}
	return contract, nil
}

// Contract returns the module contract that Open copied from the shared library.
func (r *Runner) Contract() cardinalruntime.Contract {
	return r.contract
}

// Initialize initializes the handle with an optional snapshot.
func (r *Runner) Initialize(request cardinalruntime.InitRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	assert.That(!r.closed, "runtime runner is closed")

	status := C.cardinal_nativeaot_initialize(
		r.library,
		r.handle,
		bytePointer(request.Snapshot),
		C.size_t(len(request.Snapshot)),
	)
	goruntime.KeepAlive(request.Snapshot)

	return r.statusErrorLocked("initialize", status)
}

// Tick runs one simulation step. It writes the result to output. The caller owns output.
func (r *Runner) Tick(request cardinalruntime.TickRequest, output []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	assert.That(!r.closed, "runtime runner is closed")

	result := C.cardinal_nativeaot_tick(
		r.library,
		r.handle,
		C.uint64_t(request.Tick),
		C.uint64_t(request.FixedDeltaNS),
		bytePointer(request.Input),
		C.size_t(len(request.Input)),
		bytePointer(output),
		C.size_t(len(output)),
	)
	goruntime.KeepAlive(request.Input)
	goruntime.KeepAlive(output)

	return r.outputResultLocked("tick", result, len(output))
}

// Query runs an application-defined query. It writes the result to output. The caller owns output.
func (r *Runner) Query(request cardinalruntime.QueryRequest, output []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	assert.That(!r.closed, "runtime runner is closed")

	result := C.cardinal_nativeaot_query(
		r.library,
		r.handle,
		C.uint32_t(request.Kind),
		bytePointer(request.Input),
		C.size_t(len(request.Input)),
		bytePointer(output),
		C.size_t(len(output)),
	)
	goruntime.KeepAlive(request.Input)
	goruntime.KeepAlive(output)

	return r.outputResultLocked("query", result, len(output))
}

// Snapshot writes the handle state to output. The caller owns output.
func (r *Runner) Snapshot(output []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	assert.That(!r.closed, "runtime runner is closed")

	result := C.cardinal_nativeaot_snapshot(
		r.library,
		r.handle,
		bytePointer(output),
		C.size_t(len(output)),
	)
	goruntime.KeepAlive(output)

	return r.outputResultLocked("snapshot", result, len(output))
}

// Restore replaces the handle state with snapshot. Restore borrows snapshot for this call.
func (r *Runner) Restore(snapshot []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	assert.That(!r.closed, "runtime runner is closed")

	status := C.cardinal_nativeaot_restore(
		r.library,
		r.handle,
		bytePointer(snapshot),
		C.size_t(len(snapshot)),
	)
	goruntime.KeepAlive(snapshot)

	return r.statusErrorLocked("restore", status)
}

// Close destroys the module handle. Call Close only one time. The process keeps the shared library
// loaded.
func (r *Runner) Close() error {
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

func (r *Runner) outputResultLocked(
	operation string,
	result C.cardinal_nativeaot_call_result_v1,
	provided int,
) (int, error) {
	outputLen, valid := sizeToInt(result.output_len)
	if !valid {
		return 0, eris.Wrapf(
			cardinalruntime.ErrExecutionFailed,
			"%s: module reported an output length too large for this host",
			operation,
		)
	}

	switch result.status {
	case C.CARDINAL_RUNTIME_STATUS_SUCCESS:
		if outputLen > provided {
			return 0, eris.Wrapf(
				cardinalruntime.ErrExecutionFailed,
				"%s: module reported %d bytes written into capacity %d",
				operation,
				outputLen,
				provided,
			)
		}
		return outputLen, nil
	case C.CARDINAL_RUNTIME_STATUS_BUFFER_TOO_SMALL:
		if outputLen <= provided {
			return 0, eris.Wrapf(
				cardinalruntime.ErrExecutionFailed,
				"%s: module reported required capacity %d for capacity %d",
				operation,
				outputLen,
				provided,
			)
		}
		return 0, eris.Wrap(
			&cardinalruntime.BufferSizeError{
				Operation: operation,
				Required:  outputLen,
				Provided:  provided,
			},
			"execute NativeAOT runtime operation",
		)
	default:
		return 0, r.statusErrorLocked(operation, result.status)
	}
}

func (r *Runner) statusErrorLocked(operation string, status C.int32_t) error {
	if status == C.CARDINAL_RUNTIME_STATUS_SUCCESS {
		return nil
	}

	var base error
	switch status {
	case C.CARDINAL_RUNTIME_STATUS_BUFFER_TOO_SMALL:
		base = cardinalruntime.ErrBufferTooSmall
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
		length, valid := sizeToInt(result.output_len)
		if valid && length <= len(local) {
			message = string(local[:length])
		}
	}

	if message == "" {
		return eris.Wrapf(base, "%s: status %d", operation, int32(status))
	}
	return eris.Wrapf(base, "%s: %s", operation, message)
}

func bytePointer(data []byte) *C.uint8_t {
	if len(data) == 0 {
		return nil
	}
	return (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(data)))
}

func sizeToInt(value C.size_t) (int, bool) {
	const maxInt = int(^uint(0) >> 1)
	if uint64(value) > uint64(maxInt) {
		return 0, false
	}
	return int(value), true
}
