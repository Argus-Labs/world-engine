package namespace

import (
	ethprecompile "github.com/berachain/polaris/eth/core/precompile"
	"github.com/berachain/polaris/lib/utils"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
	generated "pkg.world.dev/world-engine/evm/precompile/contracts/bindings/cosmos/precompile/namespace"
	"testing"
)

type NamespaceTestSuite struct {
	suite.Suite
	sf       *ethprecompile.StatefulFactory
	contract *Contract
}

func TestNamespace(t *testing.T) {
	suite.Run(t, &NamespaceTestSuite{})
}

func (r *NamespaceTestSuite) SetupTest() {
	r.contract = utils.MustGetAs[*Contract](
		NewPrecompileContract(
			nil, nil,
		),
	)
	r.sf = ethprecompile.NewStatefulFactory()
}

func (r *NamespaceTestSuite) TestStaticRegistryKey() {
	r.Require().Equal(r.contract.RegistryKey(), common.BytesToAddress(authtypes.NewModuleAddress(name)))
}

func (r *NamespaceTestSuite) TestABIMethods() {
	var contractABI abi.ABI
	err := contractABI.UnmarshalJSON([]byte(generated.NamespaceMetaData.ABI))
	r.Require().NoError(err)
	r.Require().Equal(r.contract.ABIMethods(), contractABI.Methods)
}

func (r *NamespaceTestSuite) TestMatchPrecompileMethods() {
	_, err := r.sf.Build(r.contract, nil)
	r.Require().NoError(err)
}

func (r *NamespaceTestSuite) TestCustomValueDecoderIsNoop() {
	r.Require().Nil(r.contract.CustomValueDecoders())
}
