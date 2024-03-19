package message

import (
	"testing"

	"github.com/stretchr/testify/require"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
	"pkg.world.dev/world-engine/sign"
)

type ModifyScoreMsg struct {
	PlayerID types.EntityID
	Amount   int
}

type EmptyMsgResult struct{}

func TestIfNewMessageWillPanic(t *testing.T) {
	type NotStruct []int
	type AStruct struct{}
	assert.Panics(t, func() {
		NewMessageType[NotStruct, AStruct]("random")
	})
	assert.Panics(t, func() {
		NewMessageType[AStruct, NotStruct]("random")
	})
	assert.NotPanics(t, func() {
		NewMessageType[AStruct, AStruct]("random")
	})
}

func TestReadTypeNotStructs(t *testing.T) {
	defer func() {
		// test should trigger a panic. it is swallowed here.
		panicValue := recover()
		assert.Assert(t, panicValue != nil)

		defer func() {
			// deferred function should not fail
			panicValue = recover()
			assert.Assert(t, panicValue == nil)
		}()

		NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_score2")
	}()
	NewMessageType[string, string]("modify_score1")
}

func TestCanEncodeDecodeEVMTransactions(t *testing.T) {
	// the msg we are going to test against
	type FooMsg struct {
		X, Y uint64
		Name string
	}

	msg := FooMsg{1, 2, "foo"}
	// set up the Message.
	iMsg := NewMessageType[FooMsg, EmptyMsgResult]("FooMsg",
		WithMsgEVMSupport[FooMsg, EmptyMsgResult]())
	bz, err := iMsg.ABIEncode(msg)
	assert.NilError(t, err)

	// decode the evm bytes
	fooMsg, err := iMsg.DecodeEVMBytes(bz)
	assert.NilError(t, err)

	// we should be able to cast back to our concrete Go struct.
	f, ok := fooMsg.(FooMsg)
	assert.Equal(t, ok, true)
	assert.DeepEqual(t, f, msg)
}

func TestCannotDecodeEVMBeforeSetEVM(t *testing.T) {
	type foo struct{}
	msg := NewMessageType[foo, EmptyMsgResult]("foo")
	_, err := msg.DecodeEVMBytes([]byte{})
	assert.ErrorIs(t, err, ErrEVMTypeNotSet)
}

func TestCopyTransactions(t *testing.T) {
	type FooMsg struct {
		X int
	}
	txp := txpool.New()
	txp.AddTransaction(1, FooMsg{X: 3}, &sign.Transaction{PersonaTag: "foo"})
	txp.AddTransaction(2, FooMsg{X: 4}, &sign.Transaction{PersonaTag: "bar"})

	copyTxp := txp.CopyTransactions()
	assert.Equal(t, copyTxp.GetAmountOfTxs(), 2)
	assert.Equal(t, txp.GetAmountOfTxs(), 0)
}

func TestNewTransactionPanicsIfNoName(t *testing.T) {
	type Foo struct{}
	require.Panics(
		t, func() {
			NewMessageType[Foo, Foo]("")
		},
	)
}
