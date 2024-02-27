package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang/mock/gomock"
	"google.golang.org/grpc"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/router/mocks"
	"pkg.world.dev/world-engine/cardinal/types"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	shard "pkg.world.dev/world-engine/rift/shard/v2"
	"pkg.world.dev/world-engine/sign"
	"testing"
)

var _ types.Message = &mockMsg{}

type mockMsg struct {
	evmCompat      bool
	name           string
	id             types.MessageID
	msgValue       any
	decodeEVMBytes func() ([]byte, error)
}

func (f *mockMsg) SetID(id types.MessageID) error {
	f.id = id
	return nil
}

func (f *mockMsg) Name() string {
	return f.name
}

func (f *mockMsg) Group() string {
	return ""
}

func (f *mockMsg) ID() types.MessageID {
	return f.id
}

func (f *mockMsg) Encode(a any) ([]byte, error) {
	return json.Marshal(a)
}

func (f *mockMsg) Decode(bytes []byte) (any, error) {
	err := json.Unmarshal(bytes, f.msgValue)
	return f.msgValue, err
}

func (f *mockMsg) DecodeEVMBytes(_ []byte) (any, error) {
	return f.decodeEVMBytes()
}

func (f *mockMsg) ABIEncode(_ any) ([]byte, error) {
	return nil, nil
}

func (f *mockMsg) IsEVMCompatible() bool {
	return f.evmCompat
}

func TestRouter_SendMessage_NonCompatibleEVMMessage(t *testing.T) {
	rtr, provider := getTestRouterAndProvider(t)
	msg := &mockMsg{evmCompat: false}
	name := "foobar"
	provider.EXPECT().GetMessageByName(name).Return(msg, true).Times(1)

	res, err := rtr.server.SendMessage(context.Background(), &routerv1.SendMessageRequest{MessageId: name})
	assert.NilError(t, err)
	assert.Equal(t, res.Code, CodeUnsupportedMessage)
}

func TestRouter_SendMessage_FailedDecode(t *testing.T) {
	rtr, provider := getTestRouterAndProvider(t)
	msg := &mockMsg{
		evmCompat: true, decodeEVMBytes: func() ([]byte, error) {
			return nil, fmt.Errorf("some error")
		},
	}
	name := "foo"

	provider.EXPECT().GetMessageByName(name).Return(msg, true).Times(1)

	res, err := rtr.server.SendMessage(context.Background(), &routerv1.SendMessageRequest{MessageId: name})
	assert.NilError(t, err)
	assert.Equal(t, res.Code, CodeInvalidFormat)
}

func TestRouter_SendMessage_PersonaNotFound(t *testing.T) {
	router, provider := getTestRouterAndProvider(t)
	msg := &mockMsg{
		evmCompat: true, decodeEVMBytes: func() ([]byte, error) {
			return []byte("hello"), nil
		},
	}
	name := "foo"
	sender := "0xtyler"

	provider.EXPECT().GetMessageByName(name).Return(msg, true).Times(1)
	provider.EXPECT().GetPersonaForEVMAddress(sender).Return("", fmt.Errorf("not found")).Times(1)

	res, err := router.server.SendMessage(
		context.Background(),
		&routerv1.SendMessageRequest{MessageId: name, Sender: sender},
	)
	assert.NilError(t, err)
	assert.Equal(t, res.Code, CodeUnauthorized)
}

func TestRouter_SendMessage_ResultDoesNotExist(t *testing.T) {
	router, provider := getTestRouterAndProvider(t)
	msgValue := []byte("hello")
	msg := &mockMsg{
		id: 5, evmCompat: true, decodeEVMBytes: func() ([]byte, error) {
			return msgValue, nil
		},
	}
	msgName := "foo"
	sender := "0xtyler"
	persona := "tyler"
	evmTxHash := "0xFooBarBaz"

	req := &routerv1.SendMessageRequest{
		Sender:    sender,
		MessageId: msgName,
		EvmTxHash: evmTxHash,
	}

	provider.EXPECT().GetMessageByName(msgName).Return(msg, true).Times(1)
	provider.EXPECT().GetPersonaForEVMAddress(sender).Return(persona, nil).Times(1)
	provider.EXPECT().AddEVMTransaction(msg.id, msgValue, &sign.Transaction{PersonaTag: persona}, evmTxHash).Times(1)
	provider.EXPECT().WaitForNextTick().Return(true).Times(1)
	provider.EXPECT().ConsumeEVMMsgResult(evmTxHash).Return(nil, nil, "", false).Times(1)

	res, err := router.server.SendMessage(context.Background(), req)
	assert.NilError(t, err)
	assert.Equal(t, res.Code, CodeNoResult)
}

func TestRouter_SendMessage_TxSuccess(t *testing.T) {
	router, provider := getTestRouterAndProvider(t)
	msgValue := []byte("hello")
	msg := &mockMsg{
		id: 5, evmCompat: true, decodeEVMBytes: func() ([]byte, error) {
			return msgValue, nil
		},
	}
	msgName := "foo"
	sender := "0xtyler"
	persona := "tyler"
	evmTxHash := "0xFooBarBaz"

	req := &routerv1.SendMessageRequest{
		Sender:    sender,
		MessageId: msgName,
		EvmTxHash: evmTxHash,
	}

	provider.EXPECT().GetMessageByName(msgName).Return(msg, true).Times(1)
	provider.EXPECT().GetPersonaForEVMAddress(sender).Return(persona, nil).Times(1)
	provider.EXPECT().AddEVMTransaction(msg.id, msgValue, &sign.Transaction{PersonaTag: persona}, evmTxHash).Times(1)
	provider.EXPECT().WaitForNextTick().Return(true).Times(1)
	provider.EXPECT().ConsumeEVMMsgResult(evmTxHash).Return([]byte("response"), nil, evmTxHash, true).Times(1)

	res, err := router.server.SendMessage(context.Background(), req)
	assert.NilError(t, err)
	assert.Equal(t, res.Code, CodeSuccess)
}

func TestRouter_SendMessage_TxFailed(t *testing.T) {
	router, provider := getTestRouterAndProvider(t)
	msgValue := []byte("hello")
	msg := &mockMsg{
		id: 5, evmCompat: true, decodeEVMBytes: func() ([]byte, error) {
			return msgValue, nil
		},
	}
	msgName := "foo"
	sender := "0xtyler"
	persona := "tyler"
	evmTxHash := "0xFooBarBaz"

	req := &routerv1.SendMessageRequest{
		Sender:    sender,
		MessageId: msgName,
		EvmTxHash: evmTxHash,
	}

	provider.EXPECT().GetMessageByName(msgName).Return(msg, true).Times(1)
	provider.EXPECT().GetPersonaForEVMAddress(sender).Return(persona, nil).Times(1)
	provider.EXPECT().AddEVMTransaction(msg.id, msgValue, &sign.Transaction{PersonaTag: persona}, evmTxHash).Times(1)
	provider.EXPECT().WaitForNextTick().Return(true).Times(1)
	provider.EXPECT().
		ConsumeEVMMsgResult(evmTxHash).
		Return([]byte("response"), []error{errors.New("oh no"), errors.New("oh no1")}, evmTxHash, true).
		Times(1)

	res, err := router.server.SendMessage(context.Background(), req)
	assert.NilError(t, err)
	assert.Equal(t, res.Code, CodeTxFailed)
}

var _ shard.TransactionHandlerClient = &fakeTxHandler{}

type fakeTxHandler struct {
	req *shard.RegisterGameShardRequest
}

func (f *fakeTxHandler) RegisterGameShard(ctx context.Context, in *shard.RegisterGameShardRequest, opts ...grpc.CallOption) (*shard.RegisterGameShardResponse, error) {
	f.req = in
	return nil, nil
}

func (f *fakeTxHandler) Submit(ctx context.Context, in *shard.SubmitTransactionsRequest, opts ...grpc.CallOption) (*shard.SubmitTransactionsResponse, error) {
	panic("implement me")
}

func TestRegisterCalledWithCorrectParams(t *testing.T) {
	rtr, _ := getTestRouterAndProvider(t)
	rtr.namespace = "foobar"
	rtr.serverAddr = "meow:9000"
	txHandler := &fakeTxHandler{}
	rtr.ShardSequencer = txHandler
	err := rtr.RegisterGameShard(context.Background())
	assert.NilError(t, err)

	assert.Equal(t, txHandler.req.Namespace, rtr.namespace)
	assert.Equal(t, txHandler.req.RouterAddress, rtr.serverAddr)
}

func getTestRouterAndProvider(t *testing.T) (*router, *mocks.MockProvider) {
	ctrl := gomock.NewController(t)
	provider := mocks.NewMockProvider(ctrl)

	return &router{provider: provider, server: newEvmServer(provider)}, provider
}
