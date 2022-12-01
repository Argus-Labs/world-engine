package sidecar

import (
	"context"
	"fmt"
	"testing"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/tendermint/tendermint/libs/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v1 "github.com/argus-labs/argus/sidecar/v1"
)

func TestSidecar(t *testing.T) {
	err := StartSidecar(&baseapp.MsgServiceRouter{}, &baseapp.GRPCQueryRouter{}, log.TestingLogger())
	if err != nil {
		panic(err)
	}
	c, err := grpc.Dial("localhost:5050", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	cc := v1.NewSidecarClient(c)
	res, err := cc.Ping(context.Background(), &v1.MsgPing{Id: "foobar"})
	if err != nil {
		panic(err)
	}
	fmt.Println(res)
}
