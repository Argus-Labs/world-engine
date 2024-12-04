package world

import (
	"context"
	"fmt"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/v2/server/utils"
	"pkg.world.dev/world-engine/cardinal/v2/types"
	"pkg.world.dev/world-engine/cardinal/v2/types/message"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrWrongNamespace             = eris.New("incorrect namespace")
	ErrSystemTransactionRequired  = eris.New("system transaction required")
	ErrSystemTransactionForbidden = eris.New("system transaction forbidden")
	ErrNoPersonaTag               = eris.New("persona tag is required")
)

// RegisteredMessages returns the list of all registered messages
func (w *World) RegisteredMessages() []types.EndpointInfo {
	messageInfo := make([]types.EndpointInfo, 0, len(w.registeredMessages))
	for _, msg := range w.registeredMessages {
		messageInfo = append(messageInfo, types.EndpointInfo{
			Name:   msg.Name(),
			Fields: msg.GetSchema(),
			URL:    utils.GetTxURL(msg.Name()),
		})
	}
	return messageInfo
}

func (w *World) AddTransaction(msgName string, rawTx *sign.Transaction) (common.Hash, error) {
	msgType, ok := w.registeredMessages[msgName]
	if !ok {
		return common.Hash{}, eris.Errorf("message %q not registered", msgName)
	}

	tx, err := msgType.Decode(rawTx)
	if err != nil {
		return common.Hash{}, eris.Wrap(err, "failed to decode transaction's message")
	}

	if w.config.CardinalVerifySignature {
		if err := w.checkTx(msgName, tx); err != nil {
			return common.Hash{}, eris.Wrap(err, "failed to verify transaction's signature")
		}
	}

	w.mux.Lock()
	defer w.mux.Unlock()

	w.txMap[msgName] = append(w.txMap[msgName], tx)
	w.txsInPool++

	// TODO: Migrate Ed's TTL-based signature verification here
	// ....

	return tx.Hash(), nil
}

func (w *World) CopyTransactions(ctx context.Context) message.TxMap {
	_, span := w.tracer.Start(ddotel.ContextWithStartOptions(ctx, ddtracer.Measured()), "world.copy-transactions")
	defer span.End()

	w.mux.Lock()
	defer w.mux.Unlock()

	// Save a copy of the txMap object
	txMapCopy := w.txMap

	// Zero out the txMap object
	w.txMap = message.TxMap{}
	w.txsInPool = 0

	// Return a pointer to the copied txMap object
	return txMapCopy
}

func (w *World) checkTx(msgName string, tx message.Tx) error {
	return w.View(func(wCtx WorldContextReadOnly) error {
		if tx.Namespace() != w.Namespace() {
			return eris.Wrap(ErrWrongNamespace, fmt.Sprintf("expected %q got %q", w.Namespace(), tx.Namespace()))
		}

		signer, err := tx.Signer()
		if err != nil {
			return err
		}

		if err := tx.Verify(signer); err != nil {
			return err
		}

		// TODO: Consider making persona creation automatic.
		var cpMsg CreatePersona
		if msgName != cpMsg.Name() {
			// Start persona validation. Only check persona tag if the message is not a CreatePersona message.
			if tx.PersonaTag() == "" {
				return ErrNoPersonaTag
			}

			personaComp, _, err := w.pm.Get(wCtx, tx.PersonaTag())
			if err != nil {
				return eris.Wrap(err, "failed to get persona component")
			}

			switch {
			// The signer is the persona's owner.
			case signer.Hex() == personaComp.SignerAddress:
				return nil

			// The signer is in the authorized address list.
			case slices.Contains(personaComp.AuthorizedAddresses, signer.Hex()):
				return nil

			// The signer is not authorized to sign on behalf of the persona.
			default:
				return eris.Errorf(
					"%q is not authorized to sign transactions on behalf of persona %q",
					signer.Hex(),
					personaComp.PersonaTag,
				)
			}
		}

		return nil
	})
}
