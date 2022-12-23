package keepers

import (
	_ "embed"
	"fmt"

	v1 "buf.build/gen/go/argus-labs/argus/protocolbuffers/go/v1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	quest "github.com/argus-labs/argus/quests"
	"github.com/argus-labs/argus/x/evm/types"

	g1 "buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"
)

var (
	// QuestContract is a simple contract that defines the Events (codenamed "Quests") to send to the game server.
	QuestContract *quest.Quest
)

func init() {
	addr := common.HexToAddress("0x12345")
	var err error
	QuestContract, err = quest.NewQuest(addr, nil)
	if err != nil {
		panic(err)
	}
}

var _ types.EvmHooks = &QuestHook{}

// QuestHook is an EVM hook that checks for quest events at the end of EVM tx processing and sends the data to
// the game server (Nakama).
type QuestHook struct {
	client       g1.NakamaClient
	nakamaTarget string
}

// NewQuestHook returns an instance of QuestHook.
// NOTE: the target is not dialed until the hook is called. If you need to test that the target is reachable, dial/ping
// it beforehand.
func NewQuestHook(nakamaTarget string) types.EvmHooks {
	return &QuestHook{nil, nakamaTarget}
}

// Connect connects to the nakama client specified by the target passed in the constructor.
func (n *QuestHook) Connect() error {
	conn, err := grpc.Dial(n.nakamaTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	nkc := g1.NewNakamaClient(conn)
	n.client = nkc
	return nil
}

// PostTxProcessing implements EVMHooks.PostTxProcessing. It is called at the end of EVM tx processing.
// Errors returned here will cause the tx to NOT be committed to state.
func (n *QuestHook) PostTxProcessing(ctx sdk.Context, msg core.Message, receipt *ethtypes.Receipt) error {
	if n.client == nil {
		if err := n.Connect(); err != nil {
			fmt.Println("error occurred here...")
			return err
		}
	}

	for _, ethLog := range receipt.Logs {

		_, err := QuestContract.ParseQuestComplete(*ethLog)
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
