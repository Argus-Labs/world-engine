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

func (s *Server) registerTransactionHandler() error {
	msgs, err := s.eng.ListMessages()
	if err != nil {
		return err
	}
	msgNameToMsg := make(map[string]message.Message)
	for _, msg := range msgs {
		msgNameToMsg[msg.Name()] = msg
	}

	s.app.Post("/tx/game/:{tx_type}", func(ctx *fiber.Ctx) error {
		txType := ctx.Route().Name
		msgType, exists := msgNameToMsg[txType]
		if !exists {
			return fiber.NewError(fiber.StatusNotFound, "no handler registered for "+txType)
		}
		body := ctx.Body()
		if len(body) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "request body was empty")
		}
		tx, err := getTransactionFromBody(body)
		if err != nil {
			return err
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
				return err
			}
		}
		if !s.disableSignatureVerification {
			err = validateTransaction(tx, signerAddress, s.eng.Namespace().String(), true) // TODO: need to deal with this somehow
			if err != nil {
				return err
			}
			if err = s.eng.UseNonce(signerAddress, tx.Nonce); err != nil {
				return err
			}
		}

		tick, hash := s.eng.AddTransaction(msgType.ID(), msg, tx)

		return ctx.JSON(&TransactionReply{
			TxHash: string(hash),
			Tick:   tick,
		})
	})
	return nil
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

func getTransactionFromBody(bz []byte) (*sign.Transaction, error) {
	tx := new(sign.Transaction)
	err := json.Unmarshal(bz, tx)
	return tx, err
}
