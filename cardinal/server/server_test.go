package server

import (
	"encoding/json"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
	"net/http"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
)

func TestTransactionHandler(t *testing.T) {
	type SendEnergyTx struct {
		From, To string
		Amount   uint64
	}
	count := 0
	w := inmem.NewECSWorldForTest(t)
	txId := 1
	sendTx := ecs.NewTransactionType[SendEnergyTx]()
	err := sendTx.SetID(transaction.TypeID(txId))
	assert.NilError(t, err)
	txh := NewTransactionHandler(w)
	endpoint := "move"
	err = txh.NewHandler(endpoint, func(w *ecs.World) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			tx := new(SendEnergyTx)
			if err := decode(request, tx); err != nil {
				panic(err)
			}
			sendTx.AddToQueue(w, *tx)
			count++
		}
	})
	assert.NilError(t, err)
	port := "4040"
	fullUrl := "http://localhost:" + port
	go txh.Serve("", "4040")

	tx := SendEnergyTx{
		From:   "me",
		To:     "you",
		Amount: 420,
	}
	bz, err := json.Marshal(&tx)
	assert.NilError(t, err)
	req, err := http.NewRequest("GET", fullUrl+"/"+endpoint, strings.NewReader(string(bz)))
	assert.NilError(t, err)
	_, err = http.DefaultClient.Do(req)
	assert.NilError(t, err)
	assert.Equal(t, 1, count)
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}
