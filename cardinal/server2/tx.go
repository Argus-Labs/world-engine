package server2

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

func (handler *Handler) processTransaction(tx message.Message, payload []byte, sp *sign.Transaction,
) (*TransactionReply, error) {
	txVal, err := tx.Decode(payload)
	if err != nil {
		return nil, eris.Wrap(err, "unable to decode transaction")
	}
	return handler.submitTransaction(txVal, tx, sp)
}

func getTxFromParams(c *fiber.Ctx, txNameToTx map[string]message.Message,
) (message.Message, error) {
	txType := c.Params("txType")
	if txType == "" {
		return nil, eris.New("params do not contain txType from the path /tx/game/{txType}")
	}
	tx, ok := txNameToTx[txType]
	if !ok {
		return nil, eris.Errorf("could not locate transaction type: %s", txType)
	}
	return tx, nil
}

// TODO: Refactor this to unmarshall body to SignedPayload
func (handler *Handler) getBodyAndSigFromParams(
	params interface{},
	isSystemTransaction bool) ([]byte, *sign.Transaction, error) {
	mappedParams, ok := params.(map[string]interface{})
	if !ok {
		return nil, nil, eris.New("params not readable")
	}
	txBody, ok := mappedParams["txBody"]
	if !ok {
		return nil, nil, eris.New("params do not contain txBody from the body of the http request")
	}
	txBodyMap, ok := txBody.(map[string]interface{})
	if !ok {
		return nil, nil, eris.New("txBody needs to be a json object in the body")
	}
	payload, sp, err := handler.verifySignatureOfMapRequest(txBodyMap, isSystemTransaction)
	if err != nil {
		return nil, nil, eris.Wrap(err, "error verifying signature of map request")
	}
	return payload, sp, nil
}

// submitTransaction submits a transaction to the game world, as well as the blockchain.
func (handler *Handler) submitTransaction(txVal any, tx message.Message, sp *sign.Transaction,
) (*TransactionReply, error) {
	log.Debug().Msgf("submitting transaction %d: %v", tx.ID(), txVal)
	tick, txHash := handler.w.AddTransaction(tx.ID(), txVal, sp)
	txReply := &TransactionReply{
		TxHash: string(txHash),
		Tick:   tick,
	}
	// check if we have an adapter
	if handler.adapter != nil {
		// if the world is recovering via adapter, we shouldn't accept transactions.
		if handler.w.IsRecovering() {
			return nil, eris.New("unable to submit transactions: game world is recovering state")
		}
		log.Debug().Msgf("TX %d: tick %d: hash %s: submitted to base shard", tx.ID(), txReply.Tick, txReply.TxHash)
		err := handler.adapter.Submit(context.Background(), sp, uint64(tx.ID()), txReply.Tick)
		if err != nil {
			return nil, eris.Wrap(err, "error submitting transaction to base shard")
		}
	} else {
		log.Debug().Msg("not submitting transaction to base shard")
	}
	return txReply, nil
}

func (handler *Handler) registerTxHandler() error {
	world := handler.w
	txs, err := world.ListMessages()
	if err != nil {
		return err
	}

	txNameToTx := make(map[string]message.Message)
	for _, tx := range txs {
		txNameToTx[tx.Name()] = tx
	}

	gameHandler := func(c *fiber.Ctx) error {
		body := c.Body()
		payload, sp, err := handler.getBodyAndSigFromParams(body, false)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		tx, err := getTxFromParams(c, txNameToTx)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		// TODO: We want to return (TxReply, err), not sure what the best way to do that is here:
		return c.JSON(interface{}(handler.processTransaction(tx, payload, sp)))
	}

	createPersonaHandler := func(c *fiber.Ctx) error {
		body := c.Body()
		payload, sp, err := handler.getBodyAndSigFromParams(body, true)
		if err != nil {
			if eris.Is(err, eris.Cause(ErrInvalidSignature)) || eris.Is(err, eris.Cause(ErrSystemTransactionRequired)) {
				return fiber.NewError(fiber.StatusUnauthorized, eris.ToString(err, true))
			}
			return fiber.NewError(fiber.StatusInternalServerError, eris.ToString(err, true))
		}

		txReply, err := handler.generateCreatePersonaResponseFromPayload(payload, sp, ecs.CreatePersonaMsg)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(&txReply)
	}

	handler.server.Post("/tx/game/:{txType}", gameHandler)
	handler.server.Post("tx/persona/create-persona", createPersonaHandler)
	return nil
}
