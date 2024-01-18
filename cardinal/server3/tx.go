package server3

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
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

type TransactionReply struct {
	TxHash string
	Tick   uint64
}

func (s *Server) registerTransactionHandler(path string) error {
	msgs, err := s.eng.ListMessages()
	if err != nil {
		return err
	}
	msgNameToMsg := make(map[string]message.Message)
	customPathToMsg := make(map[string]message.Message)
	for _, msg := range msgs {
		if msg.Path() == "" {
			msgNameToMsg[msg.Name()] = msg
		} else {
			customPathToMsg[msg.Path()] = msg
		}
	}

	s.app.Post(path, s.handleTransaction(msgNameToMsg, func(ctx *fiber.Ctx) string {
		return ctx.Params(s.txWildCard)
	}))

	for _, msg := range customPathToMsg {
		m := msg
		s.app.Post(m.Path(), s.handleTransaction(customPathToMsg, func(ctx *fiber.Ctx) string {
			return ctx.Route().Path
		}))
	}

	return nil
}

func (s *Server) handleTransaction(msgTypes map[string]message.Message, getMsgTypeName func(*fiber.Ctx) string) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		msgTypeName := getMsgTypeName(ctx)
		msgType, exists := msgTypes[msgTypeName]
		if !exists {
			return fiber.NewError(fiber.StatusNotFound, "no handler registered for "+msgTypeName)
		}
		body := ctx.Body()
		if len(body) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "request body was empty")
		}
		tx, err := decodeTransaction(body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "transaction data malformed: "+err.Error())
		}
		msg, err := msgType.Decode(tx.Body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "failed to decode message from transaction body: "+err.Error())
		}
		var signerAddress string
		if msgType.Name() == ecs.CreatePersonaMsg.Name() {
			// don't need to check the cast bc we already validated this above
			createPersonaMsg := msg.(ecs.CreatePersona)
			signerAddress = createPersonaMsg.SignerAddress
		} else {
			signerAddress, err = s.eng.GetSignerForPersonaTag(tx.PersonaTag, 0)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "could not get signer for persona: "+err.Error())
			}
		}
		if !s.disableSignatureVerification {
			err = validateTransaction(tx, signerAddress, s.eng.Namespace().String(), true) // TODO: need to deal with this somehow
			if err != nil {
				fmt.Println("The error: ", err.Error())
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

func validateTransaction(tx *sign.Transaction, signerAddr, namespace string, systemTx bool) error {
	if tx.PersonaTag == "" {
		return ErrNoPersonaTag
	}
	if tx.Namespace != namespace {
		return fmt.Errorf("expected %q got %q: %w", namespace, tx.Namespace, ErrWrongNamespace)
	}
	if systemTx && !tx.IsSystemTransaction() {
		return ErrSystemTransactionRequired
	}
	if !systemTx && tx.IsSystemTransaction() {
		return ErrSystemTransactionForbidden
	}
	if err := tx.Verify(signerAddr); err != nil {
		return err
	}
	return nil
}

func decodeTransaction(bz []byte) (*sign.Transaction, error) {
	tx := new(sign.Transaction)
	err := json.Unmarshal(bz, tx)
	return tx, err
}
