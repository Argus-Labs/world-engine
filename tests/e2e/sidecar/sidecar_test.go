package sidecar

import (
	"context"
	"testing"
	"time"

	jlconfig "github.com/JeremyLoy/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/assert"

	sidecar "github.com/argus-labs/argus/sidecar/v1"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

type TestingConfig struct {
	SidecarURL   string `config:"SIDECAR_URL"`
	ArgusNodeURL string `config:"ARGUS_NODE_URL"`
}

func LoadConfig() TestingConfig {
	var cfg TestingConfig
	err := jlconfig.FromEnv().To(&cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}

func TestSideCarE2E(t *testing.T) {
	cfg := LoadConfig()
	ctx := context.Background()

	t.Logf("got config: %+v", cfg)
	conn, err := grpc.Dial(cfg.SidecarURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)

	denom := "TESTCOIN"
	amount := int64(10)
	client := sidecar.NewSidecarClient(conn)
	_, err = client.MintCoins(ctx, &sidecar.MsgMintCoins{
		Amount: amount,
		Denom:  denom,
	})
	assert.NilError(t, err)

	cosmosConn, err := grpc.Dial(cfg.ArgusNodeURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	cosmosQuerier := banktypes.NewQueryClient(cosmosConn)

	time.Sleep(5 * time.Second) // wait for block inclusion
	qres, err := cosmosQuerier.SupplyOf(ctx, &banktypes.QuerySupplyOfRequest{Denom: denom})
	assert.NilError(t, err)

	assert.Equal(t, amount, qres.Amount.Amount.Int64())
	assert.Equal(t, denom, qres.Amount.Denom)
}
