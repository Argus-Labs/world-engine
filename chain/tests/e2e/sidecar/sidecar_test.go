package sidecar

import (
	"context"
	"fmt"
	"testing"
	"time"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/sidecar/v1/sidecarv1grpc"
	sidecarv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/sidecar/v1"
	"github.com/avast/retry-go"
	"github.com/stretchr/testify/suite"
	"gotest.tools/assert"

	"github.com/argus-labs/world-engine/chain/runtime"
	argus "github.com/argus-labs/world-engine/chain/runtime/config"
	"github.com/argus-labs/world-engine/chain/utils"

	"cosmossdk.io/simapp/params"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

type SideCarSuite struct {
	suite.Suite
	cfg           TestingConfig
	sidecarClient sidecarv1grpc.SidecarClient
	addr          string // this addr is derived from the mnemonic in ../../scripts/single-node.sh
	encCfg        params.EncodingConfig
}

func (suite *SideCarSuite) SetupTest() {
	var err error
	suite.cfg, err = utils.LoadConfig[TestingConfig]()
	assert.NilError(suite.T(), err)
	if !suite.cfg.EnableDockerTests {
		suite.T().Skip("skipping test suite. these tests only runs in docker")
	}
	// this addr is derived from the mnemonic in scripts/single-node.sh
	suite.addr = "cosmos1tk7sluasye598msnjlujrp9hd67fl4gylx7z0z"
	suite.sidecarClient = GetSidecarClient(suite.T(), suite.cfg.SidecarURL)
	suite.encCfg = argus.MakeEncodingConfig(runtime.ModuleBasics)
}

// TestingConfig is a testing configuration. These values are typically set via docker.
// See the docker-compose file in the root directory. The "test" container's `environment` field
// sets the values that get loaded here.
type TestingConfig struct {
	EnableDockerTests bool   `config:"ENABLE_DOCKER_TESTS"`
	SidecarURL        string `config:"SIDECAR_URL"`
	ArgusNodeURL      string `config:"ARGUS_NODE_URL"`
	NakamaURL         string `config:"NAKAMA_URL"`
}

func (suite *SideCarSuite) TestSideCarE2E() {
	ctx := context.Background()

	denom := "TEST"
	amount := int64(10)
	_, err := suite.sidecarClient.MintCoins(ctx, &sidecarv1.MsgMintCoins{
		Amount: amount,
		Denom:  denom,
	})
	assert.NilError(suite.T(), err)

	cosmosQuerier := GetBankClient(suite.T(), suite.cfg.ArgusNodeURL)

	var qres *banktypes.QuerySupplyOfResponse
	err = retry.Do(func() error {
		var innerErr error
		qres, innerErr = cosmosQuerier.SupplyOf(ctx, &banktypes.QuerySupplyOfRequest{Denom: denom})
		if innerErr != nil {
			return innerErr
		}
		if amount != qres.Amount.Amount.Int64() {
			return fmt.Errorf("got amount %d, wanted %d", qres.Amount.Amount.Int64(), amount)
		}
		if denom != qres.Amount.Denom {
			return fmt.Errorf("got denom %s, wanted %s", qres.Amount.Denom, denom)
		}
		return nil
	}, retry.Delay(3*time.Second), retry.Attempts(20)) // at most will take 1 minute

	assert.NilError(suite.T(), err)
}

func TestRunSuite(t *testing.T) {
	suite.Run(t, new(SideCarSuite))
}
