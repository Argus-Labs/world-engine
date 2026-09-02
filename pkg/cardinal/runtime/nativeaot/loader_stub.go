//go:build !cgo || (!linux && !darwin)

package nativeaot

import (
	"errors"

	cardinalruntime "github.com/argus-labs/world-engine/pkg/cardinal/runtime"
)

// ErrUnavailable reports that this build cannot load NativeAOT modules. NativeAOT loading requires
// cgo and a platform that has dlopen.
var ErrUnavailable = errors.New("nativeaot runtime loader unavailable")

// Runner provides the runtime API for a build that cannot load NativeAOT modules.
type Runner struct{}

var _ cardinalruntime.Runner = (*Runner)(nil)

// Open returns ErrUnavailable because this build cannot load NativeAOT modules.
func Open(
	string,
	[]byte,
	string,
	string,
) (*Runner, error) {
	return nil, ErrUnavailable
}

func (*Runner) Contract() cardinalruntime.Contract {
	return cardinalruntime.Contract{}
}

func (*Runner) Initialize(cardinalruntime.InitRequest) error {
	return ErrUnavailable
}

func (*Runner) Tick(cardinalruntime.TickRequest, []byte) (int, error) {
	return 0, ErrUnavailable
}

func (*Runner) Query(cardinalruntime.QueryRequest, []byte) (int, error) {
	return 0, ErrUnavailable
}

func (*Runner) Snapshot([]byte) (int, error) {
	return 0, ErrUnavailable
}

func (*Runner) Restore([]byte) error {
	return ErrUnavailable
}

func (*Runner) Close() error {
	return nil
}
