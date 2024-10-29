package world

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrWrongNamespace             = eris.New("incorrect namespace")
	ErrSystemTransactionRequired  = eris.New("system transaction required")
	ErrSystemTransactionForbidden = eris.New("system transaction forbidden")
	ErrNoPersonaTag               = eris.New("persona tag is required")
)

// RegisterMessage registers a message
func (w *World) RegisterMessage(msgType message.MessageType, msgReflectType reflect.Type) error {
	name := msgType.Name()

	// Checks if the message is already previously registered.
	if err := errors.Join(w.isMessageNameUnique(name), w.isMessageTypeUnique(msgReflectType)); err != nil {
		return err
	}

	w.registeredMessages[name] = msgType
	w.registeredMessagesByType[msgReflectType] = msgType

	return nil
}

// RegisteredMessages returns the list of all registered messages
func (w *World) RegisteredMessages() []types.EndpointInfo {
	messageInfo := make([]types.EndpointInfo, 0, len(w.registeredMessages))
	for _, msg := range w.registeredMessages {
		messageInfo = append(messageInfo, types.EndpointInfo{
			Name:   msg.Name(),
			Fields: msg.GetInFieldInformation(),
			URL:    utils.GetTxURL(msg.Name()),
		})
	}
	return messageInfo
}

// GetEVMTxs gets all the txs in the queue that originated from the EVM.
// NOTE: this is called ONLY in the copied tx queue in world.doTick, so we do not need to use the mutex here.
func (w *World) GetEVMTxs() []types.TxData {
	transactions := make([]types.TxData, 0)

	for _, txs := range w.txMap {
		// skip if theres nothing
		if len(txs) == 0 {
			continue
		}
		for _, tx := range txs {
			if tx.EVMSourceTxHash != nil {
				transactions = append(transactions, tx)
			}
		}
	}
	return transactions
}

func (w *World) AddTransaction(msgName string, tx *sign.Transaction) (*common.Hash, error) {
	msgType, ok := w.GetMessage(msgName)
	if !ok {
		return nil, eris.Errorf("message %q not registered", msgName)
	}

	msg, err := msgType.Decode(tx.Body)
	if err != nil {
		return nil, eris.New("invalid message body for given message type")
	}

	if w.config.CardinalVerifySignature {
		if err := w.checkTx(msgName, tx, "", false); err != nil {
			return nil, eris.Wrap(err, "failed to validate transaction")
		}
	}

	return w.addTransaction(msgName, tx, msg, nil)
}

func (w *World) AddEVMTransaction(
	msgName string, tx *sign.Transaction, evmSender string, evmTxHash common.Hash,
) (*common.Hash, error) {
	msgType, ok := w.GetMessage(msgName)
	if !ok {
		return nil, eris.Errorf("message %q not registered", msgName)
	}

	if !msgType.IsEVMCompatible() {
		return nil, eris.Errorf("message %q is not EVM compatible", msgName)
	}

	msg, err := msgType.DecodeEVMBytes(tx.Body)
	if err != nil {
		return nil, eris.New("invalid message body for given message type")
	}

	if !w.config.CardinalVerifySignature {
		if err := w.checkTx(msgName, tx, evmSender, false); err != nil {
			return nil, eris.Wrap(err, "failed to validate transaction")
		}
	}

	return w.addTransaction(msgName, tx, msg, &evmTxHash)
}

func (w *World) addTransaction(msgName string, tx *sign.Transaction, msg message.Message, evmTxHash *common.Hash) (
	*common.Hash, error,
) {
	w.mux.Lock()
	defer w.mux.Unlock()

	w.txMap[msgName] = append(w.txMap[msgName], types.TxData{
		Tx:              tx,
		Msg:             msg,
		EVMSourceTxHash: evmTxHash,
	})
	w.txsInPool++

	if err := w.rs.UseNonce(tx.PersonaTag, tx.Nonce); err != nil {
		return nil, eris.Wrap(err, "failed to use nonce")
	}

	return tx.HashHex(), nil
}

func (w *World) CopyTransactions(ctx context.Context) types.TxMap {
	_, span := w.tracer.Start(ddotel.ContextWithStartOptions(ctx, ddtracer.Measured()), "world.copy-transactions")
	defer span.End()

	w.mux.Lock()
	defer w.mux.Unlock()

	// Save a copy of the txMap object
	txMapCopy := w.txMap

	// Zero out the txMap object
	w.txMap = types.TxMap{}
	w.txsInPool = 0

	// Return a pointer to the copied txMap object
	return txMapCopy
}

func (w *World) checkTx(msgName string, tx *sign.Transaction, evmSender string, isSystemTx bool) error {
	return w.View(func(wCtx WorldContextReadOnly) error {
		if tx.Namespace != w.Namespace() {
			return eris.Wrap(ErrWrongNamespace, fmt.Sprintf("expected %q got %q", w.Namespace(), tx.Namespace))
		}
		if isSystemTx && !tx.IsSystemTransaction() {
			return ErrSystemTransactionRequired
		}
		if !isSystemTx && tx.IsSystemTransaction() {
			return ErrSystemTransactionForbidden
		}

		// If the evm sender is provided, use that. Otherwise, use the signer of the transaction.
		// It is important to note that an EVM transaction does not have a signer, so we must use the evm sender.
		var txSignerHex string
		if evmSender != "" {
			txSignerHex = evmSender
		} else {
			txSigner, err := tx.Signer()
			if err != nil {
				return err
			}
			txSignerHex = txSigner.Hex()
		}

		// Start persona validation. Only check persona tag if the message is not a CreatePersona message.
		// TODO: Consider making persona creation automatic.
		var cpMsg CreatePersona
		if msgName != cpMsg.Name() {
			if tx.PersonaTag == "" {
				return ErrNoPersonaTag
			}

			personaComp, _, err := w.pm.Get(wCtx, tx.PersonaTag)
			if err != nil {
				return eris.Wrap(err, "failed to get persona component")
			}

			switch {
			case txSignerHex == personaComp.SignerAddress:
				return nil
			case slices.Contains(personaComp.AuthorizedAddresses, txSignerHex):
				return nil
			default:
				return eris.Errorf(
					"%q is not authorized to sign transactions on behalf of persona %q",
					txSignerHex,
					personaComp.PersonaTag,
				)
			}
		}

		return nil
	})
}

// GetMessage returns the message with the given full name, if it exists.
func (w *World) GetMessage(msgName string) (message.MessageType, bool) {
	msg, ok := w.registeredMessages[msgName]
	return msg, ok
}

// isMessageNameUnique checks if the message name already exist in messages map.
func (w *World) isMessageNameUnique(msgName string) error {
	_, ok := w.registeredMessages[msgName]
	if ok {
		return eris.Errorf("message %q is already registered", msgName)
	}
	return nil
}

// isMessageTypeUnique checks if the message type name already exist in messages map.
func (w *World) isMessageTypeUnique(msgReflectType reflect.Type) error {
	_, ok := w.registeredMessagesByType[msgReflectType]
	if ok {
		return eris.Errorf("message type %q is already registered", msgReflectType)
	}
	return nil
}
