package nakama

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/suite"
	"gotest.tools/assert"

	"github.com/argus-labs/argus/app/keepers"
	"github.com/argus-labs/argus/tests"
)

type Config struct {
	EnableDockerTests bool   `config:"ENABLE_DOCKER_TESTS"`
	NakamaURL         string `config:"NAKAMA_URL"`
}

type NakamaSuite struct {
	suite.Suite
	cfg Config
}

func (suite *NakamaSuite) SetupTest() {
	suite.cfg = tests.LoadConfig[Config]()
	if !suite.cfg.EnableDockerTests {
		suite.T().Skip("skipping test suite. these tests only runs in docker")
	}
}

func (suite *NakamaSuite) TestNakamaFromEVM() {
	qh := keepers.NewNakamaHook(suite.cfg.NakamaURL)
	ctx := sdk.Context{}.WithContext(context.Background())
	addr := common.HexToAddress("0x3A220f351252089D385b29beca14e27F204c296A")
	msg := types.NewMessage(addr, nil, 0, nil, 30, nil, nil, nil, nil, nil, false)
	logs := make([]*types.Log, 0)
	logs = append(logs, &types.Log{
		Address: addr,
		Topics:  []common.Hash{common.HexToHash("0xadf42909b380f9140633e3b84d758a4ffd81c45e18e5647f7636a8674012e9ed")},
		Data:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 113, 86, 43, 113, 153, 152, 115, 219, 91, 40, 109, 249, 87, 175, 25, 158, 201, 70, 23, 247},
	})
	err := qh.PostTxProcessing(ctx, msg, &types.Receipt{Logs: logs})
	assert.NilError(suite.T(), err)
}

func TestRunSuite(t *testing.T) {
	suite.Run(t, new(NakamaSuite))
}
