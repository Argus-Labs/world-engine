package sidecar

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v1 "github.com/argus-labs/argus/sidecar/v1"
)

func TestSidecar(t *testing.T) {
	c, err := grpc.Dial("localhost:5050", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	cc := v1.NewSidecarClient(c)
	res, err := cc.MintCoins(context.Background(), &v1.MsgMintCoins{Amount: 40, Denom: "uregen"})
	if err != nil {
		panic(err)
	}
	fmt.Println(res)
}
