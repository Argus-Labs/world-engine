package router

import (
	"testing"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
	"pkg.berachain.dev/polaris/eth/accounts/abi"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/lib/utils"

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
