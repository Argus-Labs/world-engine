package sidecar

import (
	"context"
	"testing"
	"time"

	jlconfig "github.com/JeremyLoy/config"
	"gotest.tools/assert"

	sidecar "github.com/argus-labs/argus/sidecar/v1"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// TestingConfig is a testing configuration. These values are typically set via docker.
// See the docker-compose file in the root directory. The "test" container's `environment` field
// sets the values that get loaded here.
type TestingConfig struct {
	SidecarURL   string `config:"SIDECAR_URL"`
	ArgusNodeURL string `config:"ARGUS_NODE_URL"`
}

// LoadConfig loads the config from env variables.
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

	client := GetSidecarClient(t, cfg.SidecarURL)

	denom := "TESTCOIN"
	amount := int64(10)
	_, err := client.MintCoins(ctx, &sidecar.MsgMintCoins{
		Amount: amount,
		Denom:  denom,
	})
	assert.NilError(t, err)

	cosmosQuerier := GetBankClient(t, cfg.ArgusNodeURL)

	time.Sleep(5 * time.Second) // wait for block inclusion

	qres, err := cosmosQuerier.SupplyOf(ctx, &banktypes.QuerySupplyOfRequest{Denom: denom})
	assert.NilError(t, err)

	assert.Equal(t, amount, qres.Amount.Amount.Int64())
	assert.Equal(t, denom, qres.Amount.Denom)
}
