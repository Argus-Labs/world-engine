package tick

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/goccy/go-json"
)

// Receipt contains a transaction hash, an arbitrary result, and a list of errors.
type Receipt struct {
	TxHash    common.Hash     `json:"txHash"`
	EVMTxHash string          `json:"-"`
	Result    json.RawMessage `json:"result"`
	Error     string          `json:"error"`
}
