package keepers

import (
	v1 "buf.build/gen/go/argus-labs/argus/protocolbuffers/go/v1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/argus-labs/argus/evmhooks"
	"github.com/argus-labs/argus/x/evm/types"

	g1 "buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"
)

var (
	// QuestContract is a simple contract that defines the Events (codenamed "Quests") to send to the game server.
	QuestContract *evmhooks.Quest
)

func init() {
	addr := common.HexToAddress("0x12345")
	var err error
	QuestContract, err = evmhooks.NewQuest(addr, nil)
	if err != nil {
		panic(err)
	}
}

var _ types.EvmHooks = &NakamaHook{}

// NakamaHook is an EVM hook that checks for evmhooks events at the end of EVM tx processing and sends the data to
// the game server (Nakama).
type NakamaHook struct {
	client       g1.NakamaClient
	nakamaTarget string
}

// NewNakamaHook returns an instance of Nakama Hooks.
// NOTE: the target is not dialed until the hook is called. If you need to test that the target is reachable, dial/ping
// it beforehand.
func NewNakamaHook(nakamaTarget string) types.EvmHooks {
	return &NakamaHook{nil, nakamaTarget}
}

// Connect connects to the nakama client specified by the target passed in the constructor.
func (n *NakamaHook) Connect() error {
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
func (n *NakamaHook) PostTxProcessing(ctx sdk.Context, msg core.Message, receipt *ethtypes.Receipt) error {
	if n.client == nil {
		if err := n.Connect(); err != nil {
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
