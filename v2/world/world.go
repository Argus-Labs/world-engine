package world

import (
	"encoding/json"
	"sync"

	"github.com/coocood/freecache"
	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"pkg.world.dev/world-engine/cardinal/v2/config"
	"pkg.world.dev/world-engine/cardinal/v2/gamestate"
	"pkg.world.dev/world-engine/cardinal/v2/gamestate/search"
	"pkg.world.dev/world-engine/cardinal/v2/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/v2/storage/redis"
	"pkg.world.dev/world-engine/cardinal/v2/tick"
	"pkg.world.dev/world-engine/cardinal/v2/types"
	"pkg.world.dev/world-engine/cardinal/v2/types/message"
)

const (
	ReceiptCacheSize = 10000
)

type World struct {
	state *gamestate.State
	pm    *PersonaManager

	// System
	// Registered systems in the order that they were registered.
	// This is represented as a list as maps in Go are unordered.
	registeredSystems     []systemType
	registeredInitSystems []systemType

	// Tx
	txMap              message.TxMap
	txsInPool          int
	mux                *sync.Mutex
	registeredMessages map[string]message.MessageInternal

	// Query
	registeredQueriesByGroup map[string]map[string]Query // group:name:query

	// Storage
	rs *redis.Storage

	// Config
	config *config.Config

	// Telemetry
	tracer trace.Tracer // Tracer for World

	lastFinalizedTickID int64
	namespace           types.Namespace
	receipts            *freecache.Cache
}

// New creates a new World object using Redis as the storage layer
func New(rs *redis.Storage, opts ...Option) (*World, error) {
	if rs == nil {
		return nil, eris.New("redis storage is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, eris.Wrap(err, "Failed to load config to start world")
	}

	if cfg.CardinalRollupEnabled {
		log.Info().Msgf("Creating a new Cardinal world in rollup mode")
	} else {
		log.Warn().Msg("Cardinal is running in development mode without rollup sequencing. " +
			"If you intended to run this for production use, set CARDINAL_ROLLUP=true")
	}

	s, err := gamestate.New(rs)
	if err != nil {
		return nil, err
	}

	w := &World{
		state: s,
		pm:    nil,

		// System
		registeredSystems:     make([]systemType, 0),
		registeredInitSystems: make([]systemType, 0),

		// Tx
		txMap:              make(message.TxMap),
		txsInPool:          0,
		mux:                new(sync.Mutex),
		registeredMessages: make(map[string]message.MessageInternal),

		// Query
		registeredQueriesByGroup: make(map[string]map[string]Query),

		// Storage
		rs: rs,

		// Config
		config: cfg,

		// Telemetry
		tracer: otel.Tracer("cardinal"),

		lastFinalizedTickID: -1,
		namespace:           types.Namespace(cfg.CardinalNamespace),
		receipts:            freecache.NewCache(ReceiptCacheSize),
	}
	for _, opt := range opts {
		opt(w)
	}

	w.pm, err = newPersonaManager(w)
	if err != nil {
		return nil, err
	}

	return w, nil
}

// Init marks the world as ready for use.
func (w *World) Init() error {
	var err error

	if err = w.state.Init(); err != nil {
		return err
	}

	w.lastFinalizedTickID, err = w.state.FinalizedState().GetLastFinalizedTick()
	if err != nil {
		return err
	}

	if err = w.pm.Init(NewWorldContextReadOnly(w.state, w.pm)); err != nil {
		return err
	}

	return nil
}

func (w *World) Persona() *PersonaManager {
	return w.pm
}

func (w *World) State() *gamestate.State {
	return w.state
}

func (w *World) Namespace() string {
	return string(w.namespace)
}

func (w *World) Search(compFilter filter.ComponentFilter) *search.Search {
	return search.New(w.state.FinalizedState(), compFilter)
}

func (w *World) View(viewFn func(wCtx WorldContextReadOnly) error) error {
	return viewFn(NewWorldContextReadOnly(w.State(), w.pm))
}

func (w *World) GetReceiptBytes(hash common.Hash) (json.RawMessage, error) {
	receiptBz, err := w.receipts.Get(hash.Bytes())
	if err != nil {
		return nil, ErrInvalidReceiptTxHash
	}
	return receiptBz, nil
}

func (w *World) GetReceiptsBytes(txHashes []common.Hash) (map[common.Hash]json.RawMessage, error) {
	receipts := make(map[common.Hash]json.RawMessage)
	for _, txHash := range txHashes {
		receipt, err := w.GetReceiptBytes(txHash)
		if err != nil {
			if eris.Is(err, ErrInvalidReceiptTxHash) {
				receipts[txHash] = nil
				continue
			}
			return nil, eris.Wrap(err, "failed to get receipts")
		}
		receipts[txHash] = receipt
	}
	return receipts, nil
}

func (w *World) GetReceipt(hash common.Hash) (*tick.Receipt, error) {
	receiptBz, err := w.GetReceiptBytes(hash)
	if err != nil {
		return nil, eris.Wrap(err, "failed to get receipt")
	}

	var receipt tick.Receipt
	err = json.Unmarshal(receiptBz, &receipt)
	if err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal receipt")
	}

	return &receipt, nil
}

func (w *World) GetReceipts(txHashes []common.Hash) (map[common.Hash]*tick.Receipt, error) {
	receipts := make(map[common.Hash]*tick.Receipt)
	for _, txHash := range txHashes {
		receipt, err := w.GetReceipt(txHash)
		if err != nil {
			if eris.Is(err, ErrInvalidReceiptTxHash) {
				receipts[txHash] = nil
				continue
			}
			return nil, eris.Wrap(err, "failed to get receipt")
		}
		receipts[txHash] = receipt
	}
	return receipts, nil
}
