//go:build cgo && (linux || darwin)

// Package nativeaot loads Cardinal NativeAOT modules through their stable C
// ABI. Loaded shared libraries remain resident for process lifetime.
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
	"fmt"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"unsafe"

	cardinalruntime "github.com/argus-labs/world-engine/pkg/cardinal/runtime"
)

const maxLastErrorSize = 64 * 1024

// Runner owns one NativeAOT module handle. Calls for a handle are serialized.
type Runner struct {
	mu       sync.Mutex
	library  *C.cardinal_nativeaot_library_v1
	handle   C.cardinal_runtime_handle_v1
	contract cardinalruntime.Contract
	closed   bool
}

var _ cardinalruntime.Runner = (*Runner)(nil)

// Open resolves path to a canonical absolute path and creates one module
// handle with borrowed config bytes. Callers with a known application contract
// should use OpenValidated so incompatible modules are rejected before create.
func Open(path string, config []byte) (*Runner, error) {
	return open(path, config, nil)
}

// OpenValidated loads a trusted module, validates its immutable contract, then
// creates one handle. No module config or create callback is invoked when
// validation fails.
func OpenValidated(
	path string,
	config []byte,
	requirement cardinalruntime.ContractRequirement,
) (*Runner, error) {
	return open(path, config, &requirement)
}

// Native libraries are executable code: callers must only pass trusted
// artifacts. The library is never passed to dlclose, including on errors.
func open(
	path string,
	config []byte,
	requirement *cardinalruntime.ContractRequirement,
) (*Runner, error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return nil, fmt.Errorf("open NativeAOT runtime: %w", cardinalruntime.ErrInvalidArgument)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve NativeAOT library %q: %w", path, err)
	}
	path, err = filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("resolve NativeAOT library %q: %w", absolutePath, err)
	}

	// Errors before a module handle exists are thread-local in the ABI.
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var loadError [1024]C.char
	library := C.cardinal_nativeaot_library_open(
		cPath,
		&loadError[0],
		C.size_t(len(loadError)),
	)
	if library == nil {
		message, _ := fixedCString(unsafe.Pointer(&loadError[0]), len(loadError))
		return nil, fmt.Errorf("load NativeAOT library %q: %s", path, message)
	}

	runner := &Runner{library: library}
	contract, err := runner.loadContract()
	if err != nil {
		C.cardinal_nativeaot_library_forget(library)
		return nil, err
	}
	if requirement != nil {
		if err = requirement.Validate(contract); err != nil {
			C.cardinal_nativeaot_library_forget(library)
			return nil, fmt.Errorf("validate NativeAOT contract: %w", err)
		}
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
		return nil, err
	}
	if createResult.handle == 0 {
		C.cardinal_nativeaot_library_forget(library)
		return nil, fmt.Errorf(
			"create: %w: module returned the reserved zero handle",
			cardinalruntime.ErrABIMismatch,
		)
	}

	runner.handle = createResult.handle
	return runner, nil
}

// Contract returns the immutable contract copied from the shared library.
func (r *Runner) Contract() cardinalruntime.Contract {
	return r.contract
}

// Initialize initializes this handle from an optional borrowed snapshot.
func (r *Runner) Initialize(request cardinalruntime.InitRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.checkCallLocked("initialize", cardinalruntime.CapabilityInitialize); err != nil {
		return err
	}
	status := C.cardinal_nativeaot_initialize(
		r.library,
		r.handle,
		bytePointer(request.Snapshot),
		C.size_t(len(request.Snapshot)),
	)
	goruntime.KeepAlive(request.Snapshot)
	return r.statusErrorLocked("initialize", status)
}

// Tick executes one simulation step into caller-owned output.
func (r *Runner) Tick(request cardinalruntime.TickRequest, output []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.checkCallLocked("tick", cardinalruntime.CapabilityTick); err != nil {
		return 0, err
	}
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

// Query executes an application-defined query into caller-owned output.
func (r *Runner) Query(request cardinalruntime.QueryRequest, output []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.checkCallLocked("query", cardinalruntime.CapabilityQuery); err != nil {
		return 0, err
	}
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

// Snapshot writes this handle's state into caller-owned output.
func (r *Runner) Snapshot(output []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.checkCallLocked("snapshot", cardinalruntime.CapabilitySnapshot); err != nil {
		return 0, err
	}
	result := C.cardinal_nativeaot_snapshot(
		r.library,
		r.handle,
		bytePointer(output),
		C.size_t(len(output)),
	)
	goruntime.KeepAlive(output)
	return r.outputResultLocked("snapshot", result, len(output))
}

// Restore replaces this handle's state from a borrowed snapshot.
func (r *Runner) Restore(snapshot []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.checkCallLocked("restore", cardinalruntime.CapabilityRestore); err != nil {
		return err
	}
	status := C.cardinal_nativeaot_restore(
		r.library,
		r.handle,
		bytePointer(snapshot),
		C.size_t(len(snapshot)),
	)
	goruntime.KeepAlive(snapshot)
	return r.statusErrorLocked("restore", status)
}

// Close destroys the module handle once. The shared library stays loaded.
func (r *Runner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	// A failing destroy no longer has a valid handle-bound error slot.
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

func (r *Runner) loadContract() (cardinalruntime.Contract, error) {
	var raw C.cardinal_runtime_contract_v1
	status := C.cardinal_nativeaot_get_contract(r.library, &raw)
	if status != C.CARDINAL_RUNTIME_STATUS_SUCCESS {
		return cardinalruntime.Contract{}, r.statusErrorLocked("get contract", status)
	}

	if uint32(raw.abi_version) != cardinalruntime.ABIVersion {
		return cardinalruntime.Contract{}, fmt.Errorf(
			"get contract: %w: module=%d host=%d",
			cardinalruntime.ErrABIMismatch,
			uint32(raw.abi_version),
			cardinalruntime.ABIVersion,
		)
	}
	expectedSize := uint32(C.sizeof_cardinal_runtime_contract_v1)
	if uint32(raw.struct_size) != expectedSize {
		return cardinalruntime.Contract{}, fmt.Errorf(
			"get contract: %w: contract size=%d expected=%d",
			cardinalruntime.ErrABIMismatch,
			uint32(raw.struct_size),
			expectedSize,
		)
	}
	for index, value := range raw.reserved {
		if value != 0 {
			return cardinalruntime.Contract{}, fmt.Errorf(
				"get contract: %w: reserved[%d]=%d expected=0",
				cardinalruntime.ErrABIMismatch,
				index,
				uint64(value),
			)
		}
	}

	name, terminated := fixedCString(unsafe.Pointer(&raw.name[0]), len(raw.name))
	if !terminated {
		return cardinalruntime.Contract{}, fmt.Errorf(
			"get contract: %w: name is not NUL-terminated",
			cardinalruntime.ErrABIMismatch,
		)
	}
	version, terminated := fixedCString(unsafe.Pointer(&raw.version[0]), len(raw.version))
	if !terminated {
		return cardinalruntime.Contract{}, fmt.Errorf(
			"get contract: %w: version is not NUL-terminated",
			cardinalruntime.ErrABIMismatch,
		)
	}

	contract := cardinalruntime.Contract{
		ABIVersion:   uint32(raw.abi_version),
		Capabilities: cardinalruntime.Capabilities(raw.capabilities),
		Name:         name,
		Version:      version,
	}
	copy(
		contract.SchemaHash[:],
		unsafe.Slice((*byte)(unsafe.Pointer(&raw.schema_hash[0])), len(contract.SchemaHash)),
	)
	return contract, nil
}

func (r *Runner) checkCallLocked(
	operation string,
	capability cardinalruntime.Capabilities,
) error {
	if r.closed {
		return fmt.Errorf("%s: %w", operation, cardinalruntime.ErrClosed)
	}
	if !r.contract.Supports(capability) {
		return fmt.Errorf("%s: %w", operation, cardinalruntime.ErrUnsupported)
	}
	return nil
}

func (r *Runner) outputResultLocked(
	operation string,
	result C.cardinal_nativeaot_call_result_v1,
	provided int,
) (int, error) {
	outputLen, valid := sizeToInt(result.output_len)
	if !valid {
		return 0, fmt.Errorf(
			"%s: %w: module reported an output length too large for this host",
			operation,
			cardinalruntime.ErrExecutionFailed,
		)
	}

	switch result.status {
	case C.CARDINAL_RUNTIME_STATUS_SUCCESS:
		if outputLen > provided {
			return 0, fmt.Errorf(
				"%s: %w: module reported %d bytes written into capacity %d",
				operation,
				cardinalruntime.ErrExecutionFailed,
				outputLen,
				provided,
			)
		}
		return outputLen, nil
	case C.CARDINAL_RUNTIME_STATUS_BUFFER_TOO_SMALL:
		if outputLen <= provided {
			return 0, fmt.Errorf(
				"%s: %w: module reported required capacity %d for capacity %d",
				operation,
				cardinalruntime.ErrExecutionFailed,
				outputLen,
				provided,
			)
		}
		return 0, &cardinalruntime.BufferSizeError{
			Operation: operation,
			Required:  outputLen,
			Provided:  provided,
		}
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

	message := r.lastErrorLocked()
	if message == "" {
		return fmt.Errorf("%s: %w (status %d)", operation, base, int32(status))
	}
	return fmt.Errorf("%s: %w: %s", operation, base, message)
}

func (r *Runner) lastErrorLocked() string {
	var local [512]byte
	result := C.cardinal_nativeaot_last_error(
		r.library,
		r.handle,
		bytePointer(local[:]),
		C.size_t(len(local)),
	)
	goruntime.KeepAlive(local)

	switch result.status {
	case C.CARDINAL_RUNTIME_STATUS_SUCCESS:
		length, valid := sizeToInt(result.output_len)
		if !valid || length > len(local) {
			return ""
		}
		return string(local[:length])
	case C.CARDINAL_RUNTIME_STATUS_BUFFER_TOO_SMALL:
		required, valid := sizeToInt(result.output_len)
		if !valid || required <= len(local) || required > maxLastErrorSize {
			return ""
		}
		buffer := make([]byte, required)
		result = C.cardinal_nativeaot_last_error(
			r.library,
			r.handle,
			bytePointer(buffer),
			C.size_t(len(buffer)),
		)
		goruntime.KeepAlive(buffer)
		length, valid := sizeToInt(result.output_len)
		if result.status != C.CARDINAL_RUNTIME_STATUS_SUCCESS ||
			!valid ||
			length > len(buffer) {
			return ""
		}
		return string(buffer[:length])
	default:
		return ""
	}
}

func bytePointer(data []byte) *C.uint8_t {
	if len(data) == 0 {
		return nil
	}
	return (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(data)))
}

func fixedCString(pointer unsafe.Pointer, capacity int) (string, bool) {
	data := unsafe.Slice((*byte)(pointer), capacity)
	terminator := bytes.IndexByte(data, 0)
	if terminator < 0 {
		return "", false
	}
	return string(data[:terminator]), true
}

func sizeToInt(value C.size_t) (int, bool) {
	const maxInt = int(^uint(0) >> 1)
	if uint64(value) > uint64(maxInt) {
		return 0, false
	}
	return int(value), true
}
