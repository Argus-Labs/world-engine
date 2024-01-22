package server

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

func (s *Server) registerTransactionHandler(path string) error {
	msgs, err := s.eng.ListMessages()
	if err != nil {
		return err
	}

	// some messages may have a custom path. we store them separately.
	msgNameToMsg := make(map[string]message.Message)
	customPathToMsg := make(map[string]message.Message)
	for _, msg := range msgs {
		if msg.Path() == "" {
			msgNameToMsg[msg.Name()] = msg
		} else {
			customPathToMsg[msg.Path()] = msg
		}
	}

	// for messages that do not have a custom handler path, we can handle all these under the wildcard path.
	s.app.Post(path, s.handleTransaction(msgNameToMsg, func(ctx *fiber.Ctx) string {
		return ctx.Params(s.txWildCard)
	}))

	// for messages with a custom handler path, we setup a separate handler for each.
	for _, msg := range customPathToMsg {
		m := msg
		s.app.Post(m.Path(), s.handleTransaction(customPathToMsg, func(ctx *fiber.Ctx) string {
			return ctx.Route().Path
		}))
	}

	return nil
}

func (s *Server) handleTransaction(
	msgTypes map[string]message.Message,
	getMsgTypeName func(*fiber.Ctx) string,
) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		msgTypeName := getMsgTypeName(ctx)
		msgType, exists := msgTypes[msgTypeName]
		if !exists {
			return fiber.NewError(fiber.StatusNotFound, "no handler registered for "+msgTypeName)
		}

		tx, msg, err := getMessageAndTx(ctx.Body(), msgType)
		if err != nil {
			return err
		}

		var signerAddress string
		if msgType.Name() == ecs.CreatePersonaMsg.Name() {
			// don't need to check the cast bc we already validated this above
			createPersonaMsg, _ := msg.(ecs.CreatePersona)
			signerAddress = createPersonaMsg.SignerAddress
		} else {
			signerAddress, err = s.eng.GetSignerForPersonaTag(tx.PersonaTag, 0)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "could not get signer for persona: "+err.Error())
			}
		}
		if !s.disableSignatureVerification {
			err = validateTransaction(tx, signerAddress, s.eng.Namespace().String(), tx.IsSystemTransaction())
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "failed to validate transaction: "+err.Error())
			}
			if err = s.eng.UseNonce(signerAddress, tx.Nonce); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to use nonce: "+err.Error())
			}
		}

		tick, hash := s.eng.AddTransaction(msgType.ID(), msg, tx)

		return ctx.JSON(&TransactionReply{
			TxHash: string(hash),
			Tick:   tick,
		})
	}
}

func getMessageAndTx(body []byte, mt message.Message) (*sign.Transaction, any, error) {
	if len(body) == 0 {
		return nil, nil, fiber.NewError(fiber.StatusBadRequest, "request body was empty")
	}
	tx, err := decodeTransaction(body)
	if err != nil {
		return nil, nil, fiber.NewError(fiber.StatusBadRequest, "transaction data malformed: "+err.Error())
	}
	msg, err := mt.Decode(tx.Body)
	if err != nil {
		return nil, nil, fiber.NewError(
			fiber.StatusBadRequest,
			"failed to decode message from transaction body: "+err.Error(),
		)
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
