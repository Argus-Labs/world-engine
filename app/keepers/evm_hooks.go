package argus

import (
	_ "embed"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v1 "github.com/argus-labs/argus/cosmos_receiver/v1"
	"github.com/argus-labs/argus/x/evm/types"
)

var _ types.EvmHooks = NakamaHook{}

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
	client v1.NakamaClient
}

func NewNakamaEVMHook(nakamaTarget string) (types.EvmHooks, error) {
	conn, err := grpc.Dial(nakamaTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	nkc := v1.NewNakamaClient(conn)
	return &NakamaHook{nkc}, nil
}

func (n NakamaHook) PostTxProcessing(ctx sdk.Context, msg core.Message, receipt *ethtypes.Receipt) error {

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
