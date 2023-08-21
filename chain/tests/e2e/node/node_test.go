package node

/*
import (
	"context"
	"testing"
	"time"

	"github.com/avast/retry-go"
	"github.com/stretchr/testify/suite"
	"gotest.tools/assert"

	"pkg.world.dev/world-engine/chain/runtime"
	argus "pkg.world.dev/world-engine/chain/runtime/config"
	"pkg.world.dev/world-engine/chain/utils"

	"cosmossdk.io/simapp/params"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

type TestSuite struct {
	suite.Suite
	cfg    TestingConfig
	addr   string // this addr is derived from the mnemonic in ../../scripts/single-node.sh
	encCfg params.EncodingConfig
}

func (suite *TestSuite) SetupTest() {
	var err error
	suite.cfg, err = utils.LoadConfig[TestingConfig]()
	assert.NilError(suite.T(), err)
	if !suite.cfg.EnableDockerTests {
		suite.T().Skip("skipping test suite. these tests only runs in docker")
	}
	// this addr is derived from the mnemonic in scripts/single-node.sh
	suite.addr = "cosmos1tk7sluasye598msnjlujrp9hd67fl4gylx7z0z"
	suite.encCfg = argus.MakeEncodingConfig(runtime.ModuleBasics)
}

// TestingConfig is a testing configuration. These values are typically set via docker.
// See the docker-compose file in the root directory. The "test" container's `environment` field
// sets the values that get loaded here.
type TestingConfig struct {
	EnableDockerTests bool   `config:"ENABLE_DOCKER_TESTS"`
	ArgusNodeURL      string `config:"ARGUS_NODE_URL"`
}

func (suite *TestSuite) TestNode() {
	ctx := context.Background()

	cosmosQuerier := GetBankClient(suite.T(), suite.cfg.ArgusNodeURL)

	var qres *banktypes.QueryTotalSupplyResponse
	err := retry.Do(func() error {
		var innerErr error
		qres, innerErr = cosmosQuerier.TotalSupply(ctx, &banktypes.QueryTotalSupplyRequest{})
		if innerErr != nil {
			return innerErr
		}
		return nil
	}, retry.Delay(3*time.Second), retry.Attempts(20)) // at most will take 1 minute

	assert.NilError(suite.T(), err)
	assert.Check(suite.T(), qres != nil)
}

func TestRunSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}


*/
