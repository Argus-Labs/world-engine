package message

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
	"pkg.world.dev/world-engine/sign"
)

type EmptyMsgResult struct{}

func TestIfNewMessageWillPanic(t *testing.T) {
	type NotStruct []int
	type AStruct struct{}
	assert.Panics(t, func() {
		NewMessageType[NotStruct, AStruct]("random")
	})
	assert.Panics(t, func() {
		NewMessageType[NotStruct, NotStruct]("random")
	})
	assert.Panics(t, func() {
		NewMessageType[AStruct, NotStruct]("random")
	})
	assert.NotPanics(t, func() {
		NewMessageType[AStruct, AStruct]("random")
	})
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

func TestMessageTypePanicsIfNoName(t *testing.T) {
	type Foo struct{}
	assert.Panics(
		t, func() {
			NewMessageType[Foo, Foo]("")
		},
	)
}

func TestMessageFullName(t *testing.T) {
	defaultMsg := NewMessageType[struct{}, struct{}]("foo")
	assert.Equal(t, defaultMsg.FullName(), defaultGroup+".foo")

	withGroup := NewMessageType[struct{}, struct{}](
		"foo",
		WithCustomMessageGroup[struct{}, struct{}]("bar"),
	)
	assert.Equal(t, withGroup.FullName(), "bar.foo")
}

func TestIsValidMessageText(t *testing.T) {
	testCases := []struct {
		testName       string
		value          string
		expectedResult bool
	}{
		{
			testName:       "all alphabetical",
			value:          "foo",
			expectedResult: true,
		},
		{
			testName:       "alphanumeric",
			value:          "foo123",
			expectedResult: true,
		},
		{
			testName:       "alphanumeric with dash",
			value:          "foo-123",
			expectedResult: true,
		},
		{
			testName:       "alphanumeric with underscore",
			value:          "foo_123",
			expectedResult: true,
		},
		{
			testName:       "alphanumeric with dash and underscore",
			value:          "foo-bar_123",
			expectedResult: true,
		},
		{
			testName:       "single alphabetical character",
			value:          "a",
			expectedResult: false,
		},
		{
			testName:       "single digit",
			value:          "7",
			expectedResult: false,
		},
		{
			testName:       "empty",
			value:          "",
			expectedResult: false,
		},
		{
			testName:       "dash only",
			value:          "-",
			expectedResult: false,
		},
		{
			testName:       "underscore only",
			value:          "_",
			expectedResult: false,
		},
		{
			testName:       "starts with dash",
			value:          "-foo",
			expectedResult: false,
		},
		{
			testName:       "starts with underscore",
			value:          "_foo",
			expectedResult: false,
		},
		{
			testName:       "ends with dash",
			value:          "foo-",
			expectedResult: false,
		},
		{
			testName:       "ends with underscore",
			value:          "foo_",
			expectedResult: false,
		},
		{
			testName:       "contains space",
			value:          "foo bar",
			expectedResult: false,
		},
		{
			testName:       "contains special character",
			value:          "foo$bar",
			expectedResult: false,
		},
		{
			testName:       "contains non-ASCII character",
			value:          "foóbar",
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			assert.Equal(t, isValidMessageText(tc.value), tc.expectedResult,
				"expected %s to be %t", tc.value, tc.expectedResult)
		})
	}
}
