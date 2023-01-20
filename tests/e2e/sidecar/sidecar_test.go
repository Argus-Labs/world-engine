package sidecar

import (
	"context"
	"testing"
	"time"

	"buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"
	sidecarv1 "buf.build/gen/go/argus-labs/argus/protocolbuffers/go/v1"
	"github.com/stretchr/testify/suite"
	"gotest.tools/assert"

	argus "github.com/argus-labs/argus/app"
	"github.com/argus-labs/argus/app/simparams"
	"github.com/argus-labs/argus/tests"
	v1 "github.com/argus-labs/argus/x/adapter/types/v1"
)

type SideCarSuite struct {
	suite.Suite
	cfg           TestingConfig
	sidecarClient sidecarv1grpc.SidecarClient
	addr          string // this addr is derived from the mnemonic in ../../contrib/single-node.sh
	encCfg        simparams.EncodingConfig
}

func (suite *SideCarSuite) SetupTest() {
	suite.cfg = tests.LoadConfig[TestingConfig]()
	if !suite.cfg.EnableDockerTests {
		suite.T().Skip("skipping test suite. these tests only runs in docker")
	}
	suite.addr = "cosmos1tk7sluasye598msnjlujrp9hd67fl4gylx7z0z" // this addr is derived from the mnemonic in contrib/single-node.sh
	suite.sidecarClient = GetSidecarClient(suite.T(), suite.cfg.SidecarURL)
	suite.encCfg = argus.MakeTestEncodingConfig()
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
	//ctx := context.Background()
	//
	//denom := "TESTCOIN"
	//amount := int64(10)
	//_, err := suite.sidecarClient.MintCoins(ctx, &sidecarv1.MsgMintCoins{
	//	Amount: amount,
	//	Denom:  denom,
	//})
	//assert.NilError(suite.T(), err)
	//
	//cosmosQuerier := GetBankClient(suite.T(), suite.cfg.ArgusNodeURL)
	//
	//time.Sleep(5 * time.Second) // wait for block inclusion
	//
	//qres, err := cosmosQuerier.SupplyOf(ctx, &banktypes.QuerySupplyOfRequest{Denom: denom})
	//assert.NilError(suite.T(), err)
	//
	//assert.Equal(suite.T(), amount, qres.Amount.Amount.Int64())
	//assert.Equal(suite.T(), denom, qres.Amount.Denom)
}

func (suite *SideCarSuite) TestMessagePool() {
	//ctx := context.Background()
	//recip := "cosmos15m3xll76c40cavsf4qvdx237f02qpyjp3yyv3s" // random cosmos address for testing purposes
	//denom := "stake"
	//amount := int64(5)
	//_, err := suite.sidecarClient.SendCoins(ctx, &sidecarv1.MsgSendCoins{
	//	Sender:    suite.addr,
	//	Recipient: recip,
	//	Denom:     denom,
	//	Amount:    uint64(amount),
	//})
	//assert.NilError(suite.T(), err)
	//
	//time.Sleep(6 * time.Second)
	//cosmosQuerier := GetBankClient(suite.T(), suite.cfg.ArgusNodeURL)
	//res, err := cosmosQuerier.Balance(ctx, &banktypes.QueryBalanceRequest{
	//	Address: recip,
	//	Denom:   denom,
	//})
	//assert.NilError(suite.T(), err)
	//assert.Equal(suite.T(), res.Balance.Amount.Int64(), amount)
}

func (suite *SideCarSuite) TestGameStateStream() {
	t := suite.T()
	ctx := context.Background()
	send := &sidecarv1.MsgUpdateGameState{NumPlanets: 15}
	_, err := suite.sidecarClient.UpdateGameState(ctx, send)
	assert.NilError(t, err)
	time.Sleep(6 * time.Second)
	querier := GetAdapterClient(t, suite.cfg.ArgusNodeURL)
	res, err := querier.GameState(ctx, &v1.QueryGameStateRequest{})
	assert.NilError(t, err)
	assert.Equal(t, res.NumPlanets, send.NumPlanets)
}

func TestRunSuite(t *testing.T) {
	suite.Run(t, new(SideCarSuite))
}
