package server

import (
	"net/http"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types/message"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/runtime/middleware/untyped"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"

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

func getTxFromParams(pathParam string, params interface{}, txNameToTx map[string]message.Message,
) (message.Message, error) {
	mappedParams, ok := params.(map[string]interface{})
	if !ok {
		return nil, eris.New("params not readable")
	}
	txType, ok := mappedParams[pathParam]
	if !ok {
		return nil, eris.New("params do not contain txType from the path /tx/game/{txType}")
	}
	txTypeString, ok := txType.(string)
	if !ok {
		return nil, eris.New("txType needs to be a string from path")
	}
	tx, ok := txNameToTx[txTypeString]
	if !ok {
		return nil, eris.Errorf("could not locate transaction type: %s", txTypeString)
	}
	return tx, nil
}

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

// register transaction handlers on swagger server.
func (handler *Handler) registerTxHandlerSwagger(api *untyped.API) error {
	world := handler.w
	txs, err := world.ListMessages()
	if err != nil {
		return err
	}

	txNameToTx := make(map[string]message.Message)
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
			return middleware.Error(http.StatusNotFound, err), nil
		}
		return handler.processTransaction(tx, payload, sp)
	})

	createPersonaHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		payload, sp, err := handler.getBodyAndSigFromParams(params, true)
		if err != nil {
			if eris.Is(err, eris.Cause(ErrInvalidSignature)) || eris.Is(err, eris.Cause(ErrSystemTransactionRequired)) {
				return middleware.Error(http.StatusUnauthorized, eris.ToString(err, true)), nil
			}
			return middleware.Error(http.StatusInternalServerError, eris.ToJSON(err, true)), nil
		}

		txReply, err := handler.generateCreatePersonaResponseFromPayload(payload, sp, ecs.CreatePersonaMsg)
		if err != nil {
			return nil, err
		}
		return &txReply, nil
	})

	api.RegisterOperation("POST", "/tx/game/{txType}", gameHandler)
	api.RegisterOperation("POST", "/tx/persona/create-persona", createPersonaHandler)

	return nil
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
	return txReply, nil
}
