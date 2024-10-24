package testutils

import (
	"crypto/ecdsa"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

var (
	privateKey *ecdsa.PrivateKey
)

func SetTestTimeout(t *testing.T, timeout time.Duration) {
	if _, ok := t.Deadline(); ok {
		// A deadline has already been set. Don't add an additional deadline.
		return
	}
	success := make(chan bool)
	t.Cleanup(func() {
		success <- true
	})
	go func() {
		select {
		case <-success:
			// test was successful. Do nothing
		case <-time.After(timeout):
			panic("test timed out")
		}
	}()
}

func UniqueSignatureWithName(name string) *sign.Transaction {
	if privateKey == nil {
		var err error
		privateKey, err = crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
	}
	// We only verify signatures when hitting the HTTP server, and in tests we're likely just adding transactions
	// directly to the World tx pool. It's OK if the signature does not match the payload.
	sig, err := sign.NewTransaction(privateKey, name, "namespace", `{"some":"data"}`)
	if err != nil {
		panic(err)
	}
	return sig
}

func UniqueSignature() *sign.Transaction {
	return UniqueSignatureWithName("some_persona_tag")
}

func GetMessage[In any, Out any](w *cardinal.World) (*cardinal.MessageType[In, Out], error) {
	var msg cardinal.MessageType[In, Out]
	msgType := reflect.TypeOf(msg)
	tempRes, ok := w.GetMessageByType(msgType)
	if !ok {
		return nil, eris.Errorf("Could not find %q, Message may not be registered.", msg.Name())
	}
	var _ types.Message = &msg
	res, ok := tempRes.(*cardinal.MessageType[In, Out])
	if !ok {
		return &msg, eris.New("wrong type")
	}
	return res, nil
}
