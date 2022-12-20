package keepers

import (
	_ "embed"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/argus-labs/argus/x/evm/types"

	g1 "buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"

	v1 "buf.build/gen/go/argus-labs/argus/protocolbuffers/go/v1"
)

var _ types.EvmHooks = &NakamaHook{}

var (
	GameContract *Quest
)

func init() {
	addr := common.HexToAddress("0x12345")
	var err error
	GameContract, err = NewQuest(addr, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("init ran")
}

type NakamaHook struct {
	client       g1.NakamaClient
	nakamaTarget string
}

func NewNakamaEVMHook(nakamaTarget string) (types.EvmHooks, error) {

	return &NakamaHook{nil, nakamaTarget}, nil
}

func (n *NakamaHook) Connect() error {
	conn, err := grpc.Dial(n.nakamaTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	nkc := g1.NewNakamaClient(conn)
	n.client = nkc
	return nil
}

func (n *NakamaHook) PostTxProcessing(ctx sdk.Context, msg core.Message, receipt *ethtypes.Receipt) error {
	if n.client == nil {
		if err := n.Connect(); err != nil {
			return err
		}
	}

	for _, ethLog := range receipt.Logs {

		_, err := GameContract.ParseQuestComplete(*ethLog)
		if err != nil {
			continue
		}
		_, err = n.client.CompleteQuest(ctx.Context(), &v1.MsgCompleteQuest{Addr: msg.From().String(), QuestId: "foobar"})
		if err != nil {
			return err
		}

	}
	return nil
}
