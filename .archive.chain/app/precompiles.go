package argus

import (
	"github.com/ethereum/go-ethereum/core/vm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var _ vm.PrecompiledContract = sidecarPrecompile{}

type sidecarPrecompile struct {
	// nakamaTarget is the gRPC endpoint of nakama for receiving data from the argus app.
	nakamaTarget string
}

func (s sidecarPrecompile) RequiredGas(input []byte) uint64 {
	//TODO(Tyler): decide values? should be a param?
	return 3000
}

func (s sidecarPrecompile) Run(input []byte) ([]byte, error) {
	conn, err := grpc.Dial(s.nakamaTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	_ = conn
	// should we be making external calls to
	return nil, nil
}
