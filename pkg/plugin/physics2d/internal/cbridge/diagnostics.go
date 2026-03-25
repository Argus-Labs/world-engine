// Diagnostic wiring for the Box2D cgo bridge.
//
// This file owns two pieces of infrastructure that turn opaque Box2D
// assertion crashes into readable structured log lines:
//
//  1. A package init() that installs the C-side assert handler exactly once,
//     before any other cbridge call can run. Because Go guarantees package
//     init ordering, any caller importing cbridge gets the handler for free.
//
//  2. bridgeOnBox2DAssert — a Go function exported to C via //export and
//     invoked from bridge_assert_handler (in bridge.c) whenever Box2D trips
//     a B2_ASSERT. It logs the failure through the structured logger at
//     fatal level; zerolog's .Fatal() helper terminates the process with
//     exit code 1 after the log line is written. We deliberately do *not*
//     let Box2D reach __builtin_trap(): a C trap produces a SIGILL that
//     Go's runtime catches and annotates with a full multi-page goroutine
//     dump plus register state, which visually buries the one line that
//     actually explains the crash. Exiting from the Go callback skips the
//     goroutine dump entirely and makes our structured log line the final
//     entry in the shard's output — which is what someone triaging the
//     crash actually wants to see.
//
// Shards are still crash-fast on failed preconditions — we are only
// changing *how* the process dies, not whether it dies.
//
// The C side (bridge.c) owns the BRIDGE_OP context-tracking macro and the
// bridge_assert_handler callback that forwards to bridgeOnBox2DAssert below.
//
// This file is deliberately kept free of C function definitions in its cgo
// preamble (only #include directives), because cgo forbids mixing //export
// declarations with preamble function bodies in the same file.

package cbridge

/*
#include "bridge.h"
*/
import "C"

import (
	"github.com/argus-labs/world-engine/pkg/telemetry"
)

// init installs the Box2D assert handler so assertions are routed through
// the structured logger instead of producing opaque SIGILL crashes. Runs
// once per process when the cbridge package is first imported, which
// happens before any physics2d plugin code can execute.
//
//nolint:gochecknoinits // Must run on package load before any cgo into Box2D
func init() {
	C.bridge_init_diagnostics()
}

// bridgeOnBox2DAssert is invoked from C (bridge_assert_handler) when Box2D
// trips a B2_ASSERT. It logs the assertion with full context — Box2D's own
// condition/file/line plus the bridge operation, entity id, and shape
// index recorded by BRIDGE_OP — and then terminates the process.
//
// This function does not return. Control enters from inside a cgo C→Go
// transition and leaves via zerolog's .Fatal() helper (which calls
// os.Exit(1) after writing the log line), so the C caller in
// bridge_assert_handler is never resumed and Box2D never gets a chance
// to __builtin_trap().
//
// Invariants worth keeping in mind when editing this function:
//
//   - We are on the same goroutine that made the offending cgo call.
//     The Go scheduler and runtime are in a normal state; it is safe
//     to allocate and call into the logger.
//   - DO NOT panic: panicking from inside a cgo callback aborts the
//     process with a less informative error than the structured log
//     line we are trying to emit.
//   - Keep the happy path tight. This is a crash path, but we want the
//     log line to reach stderr before the process tears down, so avoid
//     introducing work that could block (network logging sinks, mutexes
//     held elsewhere, etc).
//
//export bridgeOnBox2DAssert
func bridgeOnBox2DAssert(
	condition *C.char, fileName *C.char, lineNumber C.int,
	op *C.char, entityID C.uint32_t, shapeIndex C.int32_t,
) {
	logger := telemetry.GetGlobalLogger("physics2d.cbridge")
	logger.Fatal().
		Str("condition", C.GoString(condition)).
		Str("box2d_file", C.GoString(fileName)).
		Int("box2d_line", int(lineNumber)).
		Str("bridge_op", C.GoString(op)).
		Uint32("entity_id", uint32(entityID)).
		Int32("shape_index", int32(shapeIndex)).
		Msg("FATAL box2d assertion — physics2d shard terminating")
}
