package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/runtime/middleware/untyped"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"

	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"
)

func (handler *Handler) processTransaction(tx transaction.ITransaction, payload []byte, sp *sign.SignedPayload) (*TransactionReply, error) {
	txVal, err := tx.Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to decode transaction: %w", err)
	}
	return handler.submitTransaction(txVal, tx, sp)
}

func getTxFromParams(pathParam string, params interface{}, txNameToTx map[string]transaction.ITransaction) (transaction.ITransaction, error) {
	mappedParams, ok := params.(map[string]interface{})
	if !ok {
		return nil, errors.New("params not readable")
	}
	txType, ok := mappedParams[pathParam]
	if !ok {
		return nil, errors.New("params do not contain txType from the path /tx/game/{txType}")
	}
	txTypeString, ok := txType.(string)
	if !ok {
		return nil, errors.New("txType needs to be a string from path")
	}
	tx, ok := txNameToTx[txTypeString]
	if !ok {
		return nil, errors.New(fmt.Sprintf("could not locate transaction type: %s", txTypeString))
	}
	return tx, nil
}

func (handler *Handler) getBodyAndSigFromParams(
	params interface{},
	isSystemTransaction bool) ([]byte, *sign.SignedPayload, error) {
	mappedParams, ok := params.(map[string]interface{})
	if !ok {
		return nil, nil, errors.New("params not readable")
	}
	txBody, ok := mappedParams["txBody"]
	if !ok {
		return nil, nil, errors.New("params do not contain txBody from the body of the http request")
	}
	txBodyMap, ok := txBody.(map[string]interface{})
	if !ok {
		return nil, nil, errors.New("txBody needs to be a json object in the body")

	}
	payload, sp, err := handler.verifySignatureOfMapRequest(txBodyMap, isSystemTransaction)
	if err != nil {
		return nil, nil, err
	}
	return payload, sp, nil
}

// register transaction handlers on swagger server
func (handler *Handler) registerTxHandlerSwagger(api *untyped.API) error {
	world := handler.w
	txs, err := world.ListTransactions()
	if err != nil {
		return err
	}

	txNameToTx := make(map[string]transaction.ITransaction)
	for _, tx := range txs {
		txNameToTx[tx.Name()] = tx
	}

	gameHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		payload, sp, err := handler.getBodyAndSigFromParams(params, false)
		if err != nil {
			return nil, err
		}
		tx, err := getTxFromParams("txType", params, txNameToTx)
		if err != nil {
			return middleware.Error(404, err), nil
		}
		return handler.processTransaction(tx, payload, sp)
	})

	createPersonaHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		payload, sp, err := handler.getBodyAndSigFromParams(params, true)
		if err != nil {
			if errors.Is(err, ErrorInvalidSignature) || errors.Is(err, ErrorSystemTransactionRequired) {
				return middleware.Error(401, err), nil
			}
			return nil, err
		}

		txReply, err := handler.generateCreatePersonaResponseFromPayload(payload, sp, ecs.CreatePersonaTx)
		if err != nil {
			return nil, err
		}
		return &txReply, nil
	})

	authorizePersonaAddressHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		rawPayload, signedPayload, err := handler.getBodyAndSigFromParams(params, false)
		if err != nil {
			return nil, err
		}
		return handler.processTransaction(ecs.AuthorizePersonaAddressTx, rawPayload, signedPayload)
	})
	api.RegisterOperation("POST", "/tx/game/{txType}", gameHandler)
	api.RegisterOperation("POST", "/tx/persona/create-persona", createPersonaHandler)
	api.RegisterOperation("POST", "/tx/persona/authorize-persona-address", authorizePersonaAddressHandler)

	return nil
}

// submitTransaction submits a transaction to the game world, as well as the blockchain.
func (handler *Handler) submitTransaction(txVal any, tx transaction.ITransaction, sp *sign.SignedPayload) (*TransactionReply, error) {
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
			return nil, errors.New("unable to submit transactions: game world is recovering state")
		}
		log.Debug().Msgf("TX %d: tick %d: hash %s: submitted to base shard", tx.ID(), txReply.Tick, txReply.TxHash)
		err := handler.adapter.Submit(context.Background(), sp, uint64(tx.ID()), txReply.Tick)
		if err != nil {
			return nil, fmt.Errorf("error submitting transaction to base shard: %w", err)
		}
	} else {
		log.Debug().Msg("not submitting transaction to base shard")
	}
	return txReply, nil
}
