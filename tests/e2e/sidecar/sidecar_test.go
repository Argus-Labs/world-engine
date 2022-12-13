package sidecar

import (
	"context"
	"testing"
	"time"

	"buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"
	jlconfig "github.com/JeremyLoy/config"
	"github.com/stretchr/testify/suite"
	"gotest.tools/assert"

	sidecar "buf.build/gen/go/argus-labs/argus/protocolbuffers/go/v1"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

type TestSuite struct {
	suite.Suite
	cfg           TestingConfig
	sidecarClient sidecarv1grpc.SidecarClient
	addr          string
}

func (suite *TestSuite) SetupTest() {
	suite.cfg = LoadConfig()
	if !suite.cfg.EnableDockerTests {
		suite.T().Skip("skipping test suite. these tests only runs in docker")
	}
	suite.addr = "cosmos1tk7sluasye598msnjlujrp9hd67fl4gylx7z0z" // this addr is derived from the mnemonic in contrib/single-node.sh
	suite.sidecarClient = GetSidecarClient(suite.T(), suite.cfg.SidecarURL)
}

// TestingConfig is a testing configuration. These values are typically set via docker.
// See the docker-compose file in the root directory. The "test" container's `environment` field
// sets the values that get loaded here.
type TestingConfig struct {
	EnableDockerTests bool   `config:"ENABLE_DOCKER_TESTS"`
	SidecarURL        string `config:"SIDECAR_URL"`
	ArgusNodeURL      string `config:"ARGUS_NODE_URL"`
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

func (suite *TestSuite) TestSideCarE2E() {
	ctx := context.Background()

	denom := "TESTCOIN"
	amount := int64(10)
	_, err := suite.sidecarClient.MintCoins(ctx, &sidecar.MsgMintCoins{
		Amount: amount,
		Denom:  denom,
	})
	assert.NilError(suite.T(), err)

	cosmosQuerier := GetBankClient(suite.T(), suite.cfg.ArgusNodeURL)

	time.Sleep(5 * time.Second) // wait for block inclusion

	qres, err := cosmosQuerier.SupplyOf(ctx, &banktypes.QuerySupplyOfRequest{Denom: denom})
	assert.NilError(suite.T(), err)

	assert.Equal(suite.T(), amount, qres.Amount.Amount.Int64())
	assert.Equal(suite.T(), denom, qres.Amount.Denom)
}

func (suite *TestSuite) TestMessagePool() {
	ctx := context.Background()
	recip := "cosmos15m3xll76c40cavsf4qvdx237f02qpyjp3yyv3s"
	denom := "stake"
	amount := int64(5)
	_, err := suite.sidecarClient.SendCoins(ctx, &sidecar.MsgSendCoins{
		Sender:    suite.addr,
		Recipient: recip,
		Denom:     denom,
		Amount:    uint64(amount),
	})
	assert.NilError(suite.T(), err)

	time.Sleep(6 * time.Second)
	cosmosQuerier := GetBankClient(suite.T(), suite.cfg.ArgusNodeURL)
	res, err := cosmosQuerier.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: recip,
		Denom:   denom,
	})
	assert.NilError(suite.T(), err)
	assert.Equal(suite.T(), res.Balance.Amount.Int64(), amount)
}

func TestKeeperTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
