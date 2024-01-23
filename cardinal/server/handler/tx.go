package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrNoPersonaTag               = errors.New("persona tag is required")
	ErrWrongNamespace             = errors.New("incorrect namespace")
	ErrSystemTransactionRequired  = errors.New("system transaction required")
	ErrSystemTransactionForbidden = errors.New("system transaction forbidden")
)

// TransactionReply is the type that is sent back to clients after a transaction is added to the queue.
type TransactionReply struct {
	TxHash string
	Tick   uint64
}

func PostTransaction(msgTypes map[string]message.Message, eng *ecs.Engine, disableSigVerification bool, wildcard string) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		msgTypeName := ctx.Params(wildcard)
		msgType, exists := msgTypes[msgTypeName]
		if !exists {
			return fiber.NewError(fiber.StatusNotFound, "message type not found")
		}
		return handleTx(ctx, eng, msgType, disableSigVerification)
	}
}

func PostCustomPathTransaction(msg message.Message, eng *ecs.Engine, disableSigVerification bool) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		return handleTx(ctx, eng, msg, disableSigVerification)
	}
}

func handleTx(ctx *fiber.Ctx, eng *ecs.Engine, msgType message.Message, disableSigVerification bool) error {
	tx, msg, err := getMessageAndTx(ctx.Body(), msgType)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	var signerAddress string
	if msgType.Name() == ecs.CreatePersonaMsg.Name() {
		// don't need to check the cast bc we already validated this above
		createPersonaMsg, _ := msg.(ecs.CreatePersona)
		signerAddress = createPersonaMsg.SignerAddress
	} else {
		signerAddress, err = eng.GetSignerForPersonaTag(tx.PersonaTag, 0)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "could not get signer for persona: "+err.Error())
		}
	}
	if !disableSigVerification {
		err = validateTransaction(tx, signerAddress, eng.Namespace().String(), tx.IsSystemTransaction())
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "failed to validate transaction: "+err.Error())
		}
		if err = eng.UseNonce(signerAddress, tx.Nonce); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to use nonce: "+err.Error())
		}
	}

	tick, hash := eng.AddTransaction(msgType.ID(), msg, tx)

	return ctx.JSON(&TransactionReply{
		TxHash: string(hash),
		Tick:   tick,
	})
}

func getMessageAndTx(body []byte, mt message.Message) (*sign.Transaction, any, error) {
	if len(body) == 0 {
		return nil, nil, errors.New("request body was empty")
	}
	tx, err := decodeTransaction(body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode transaction: %w", err)
	}
	msg, err := mt.Decode(tx.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode message from transaction body: %w", err)
	}

	return tx, msg, nil
}

func validateTransaction(tx *sign.Transaction, signerAddr, namespace string, systemTx bool) error {
	if tx.PersonaTag == "" {
		return ErrNoPersonaTag
	}
	if tx.Namespace != namespace {
		return eris.Wrap(ErrWrongNamespace, fmt.Sprintf("expected %q got %q", namespace, tx.Namespace))
	}
	if systemTx && !tx.IsSystemTransaction() {
		return eris.Wrap(ErrSystemTransactionRequired, "")
	}
	if !systemTx && tx.IsSystemTransaction() {
		return eris.Wrap(ErrSystemTransactionForbidden, "")
	}
	return eris.Wrap(tx.Verify(signerAddr), "")
}

func decodeTransaction(bz []byte) (*sign.Transaction, error) {
	tx := new(sign.Transaction)
	err := json.Unmarshal(bz, tx)
	return tx, err
}
