//go:build !cgo || (!linux && !darwin)

package nativeaot

import (
	cardinalruntime "github.com/argus-labs/world-engine/pkg/cardinal/runtime"
	"google.golang.org/protobuf/proto"
)

// ErrUnavailable reports that this build cannot load NativeAOT modules. NativeAOT loading requires
// cgo and a platform that has dlopen.
var ErrUnavailable = cardinalruntime.ErrUnsupported

// Runner provides the runtime API for a build that cannot load NativeAOT modules.
type Runner[In, Out, Snap proto.Message] struct{}

// Open returns ErrUnavailable because this build cannot load NativeAOT modules.
func Open[In, Out, Snap proto.Message](
	string,
	[]byte,
	string,
	string,
) (*Runner[In, Out, Snap], error) {
	return nil, ErrUnavailable
}

func (*Runner[In, Out, Snap]) Contract() cardinalruntime.Contract {
	return cardinalruntime.Contract{}
}

func (*Runner[In, Out, Snap]) Initialize(Snap) error {
	return ErrUnavailable
}

func (*Runner[In, Out, Snap]) Tick(uint64, uint64, In, Out) error {
	return ErrUnavailable
}

func (*Runner[In, Out, Snap]) Snapshot() (Snap, error) {
	var zero Snap
	return zero, ErrUnavailable
}

func (*Runner[In, Out, Snap]) Restore(Snap) error {
	return ErrUnavailable
}

func (*Runner[In, Out, Snap]) Close() error {
	return ErrUnavailable
}
