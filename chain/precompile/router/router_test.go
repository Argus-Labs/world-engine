package router

import (
	"math/big"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cosmlib "pkg.berachain.dev/polaris/cosmos/lib"
	"pkg.berachain.dev/polaris/cosmos/precompile"
	testutil "pkg.berachain.dev/polaris/cosmos/testing/utils"
	"pkg.berachain.dev/polaris/lib/utils"
)

func TestRouterPrecompile(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cosmos/precompile/router")
}

var _ = Describe("Router precompile", func() {
	var (
		ctx    sdk.Context
		caller sdk.AccAddress
		//mockCtrl *gomock.Controller
		contract *Contract
	)

	BeforeEach(func() {
		ctx = testutil.NewContext()
		caller = sdk.AccAddress([]byte("bobert"))
		contract = utils.MustGetAs[*Contract](NewPrecompileContract(nil)) // TODO(technicallyty): put in router mock
	})

	When("Sending a message", func() {
		It("should fail if there are not enough arguments", func() {
			res, err := contract.Send(
				ctx,
				nil,
				cosmlib.AccAddressToEthAddress(caller),
				big.NewInt(0),
				false,
				"invalid",
			)
			Expect(err.Error()).To(Equal("expected 2 args, got 1"))
			Expect(res).To(BeNil())
		})
		It("should fail if the first arg is the wrong type", func() {
			res, err := contract.Send(
				ctx,
				nil,
				cosmlib.AccAddressToEthAddress(caller),
				big.NewInt(0),
				false,
				"foo", "bar",
			)
			Expect(err.Error()).To(Equal("expected bytes for arg[0]"))
			Expect(res).To(BeNil())
		})
		It("should fail if the second arg is the wrong type", func() {
			res, err := contract.Send(
				ctx,
				nil,
				cosmlib.AccAddressToEthAddress(caller),
				big.NewInt(0),
				false,
				[]byte("foo"), 15,
			)
			Expect(err).To(MatchError(precompile.ErrInvalidString))
			Expect(res).To(BeNil())
		})
	})
})
