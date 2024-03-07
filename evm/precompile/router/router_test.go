package router

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
	"testing"

	"github.com/berachain/polaris/eth/accounts/abi"
	ethprecompile "github.com/berachain/polaris/eth/core/precompile"
	"github.com/berachain/polaris/lib/utils"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	generated "pkg.world.dev/world-engine/evm/precompile/contracts/bindings/cosmos/precompile/router"
)

type RouterTestSuite struct {
	suite.Suite
	sf       *ethprecompile.StatefulFactory
	contract *Contract
}

func TestRouter(t *testing.T) {
	suite.Run(t, &RouterTestSuite{})
}

func (r *RouterTestSuite) SetupTest() {
	r.contract = utils.MustGetAs[*Contract](
		NewPrecompileContract(
			nil,
		),
	)
	r.sf = ethprecompile.NewStatefulFactory()
}

func (r *RouterTestSuite) TestStaticRegistryKey() {
	r.Require().Equal(r.contract.RegistryKey(), common.BytesToAddress(authtypes.NewModuleAddress(name)))
}

func (r *RouterTestSuite) TestABIMethods() {
	var contractABI abi.ABI
	err := contractABI.UnmarshalJSON([]byte(generated.RouterMetaData.ABI))
	r.Require().NoError(err)
	r.Require().Equal(r.contract.ABIMethods(), contractABI.Methods)
}

func (r *RouterTestSuite) TestMatchPrecompileMethods() {
	_, err := r.sf.Build(r.contract, nil)
	r.Require().NoError(err)
}

func (r *RouterTestSuite) TestCustomValueDecoderIsNoop() {
	r.Require().Nil(r.contract.CustomValueDecoders())
}
