package cardinal_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/world"
)

type AddHealthToEntityTx struct {
	TargetID types.EntityID
	Amount   int
}

type AddHealthToEntityResult struct{}

type Rawbodytx struct {
	PersonaTag    string `json:"personaTag"`
	SignerAddress string `json:"signerAddress"`
}

type Foo struct{}

func (Foo) Name() string { return "foo" }

type Bar struct{}

func (Bar) Name() string { return "bar" }

type Health struct {
	Value int
}

func (Health) Name() string { return "health" }

type SomeMsg struct {
	GenerateError bool
}

func (SomeMsg) Name() string { return "some-msg" }

type SomeMsgResponse struct {
	Successful bool
}

func TestForEachTransaction(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)

	assert.NilError(t, world.RegisterMessage[SomeMsg](tf.World()))
	assert.NilError(t, world.RegisterSystems(tf.World(),
		func(wCtx world.WorldContext) error {
			return world.EachMessage[SomeMsg](wCtx,
				func(t world.Tx[SomeMsg]) (any, error) {
					if t.Msg.GenerateError {
						return nil, errors.New("some error")
					}
					return SomeMsgResponse{
						Successful: true,
					}, nil
				},
			)
		},
	))

	tf.StartWorld()

	// Add 10 transactions to the tx pool and keep track of the hashes that we just world.Created
	knownTxHashes := make(map[common.Hash]SomeMsg)
	for i := 0; i < 10; i++ {
		msg := SomeMsg{GenerateError: i%2 == 0}
		txHash := tf.AddTransaction(SomeMsg{}.Name(), &msg)
		knownTxHashes[txHash] = msg
	}

	// Perform a engine tick
	tf.DoTick()

	// Verify the receipts for the previous tick are what we expect
	for txHash, msg := range knownTxHashes {
		receipt, err := tf.World().GetReceipt(txHash)
		assert.NilError(t, err)

		if msg.GenerateError {
			assert.NotEmpty(t, receipt.Error)
		} else {
			assert.Empty(t, receipt.Error)

			var result SomeMsgResponse
			err = json.Unmarshal(receipt.Result, &result)
			assert.NilError(t, err)

			assert.Equal(t, result, SomeMsgResponse{Successful: true})
		}
	}
}

type CounterComponent struct {
	Count int
}

func (CounterComponent) Name() string {
	return "count"
}

type ScoreComponent struct {
	Score int
}

func (ScoreComponent) Name() string {
	return "score"
}

type ModifyScoreMsg struct {
	PlayerID types.EntityID
	Amount   int
}

func (ModifyScoreMsg) Name() string {
	return "modify-score"
}

type EmptyMsgResult struct{}

func TestSystemsAreExecutedDuringGameTick(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)

	assert.NilError(t, world.RegisterComponent[CounterComponent](tf.World()))

	err := world.RegisterSystems(tf.World(), func(wCtx world.WorldContext) error {
		id := wCtx.Search(filter.Exact(CounterComponent{})).MustFirst()
		return world.UpdateComponent[CounterComponent](
			wCtx, id, func(c *CounterComponent) *CounterComponent {
				c.Count++
				return c
			},
		)
	})
	assert.NilError(t, err)

	err = world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		var err error
		_, err = world.Create(wCtx, CounterComponent{})
		assert.NilError(t, err)
		return nil
	})
	assert.NilError(t, err)

	tf.StartWorld()

	for i := 0; i < 10; i++ {
		tf.DoTick()
	}
}

func TestTransactionAreAppliedToSomeEntities(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)

	assert.NilError(t, world.RegisterComponent[ScoreComponent](tf.World()))
	assert.NilError(t, world.RegisterMessage[ModifyScoreMsg](tf.World()))

	var ids []types.EntityID
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		var err error
		ids, err = world.CreateMany(wCtx, 100, ScoreComponent{})
		assert.NilError(t, err)
		return nil
	}))

	assert.NilError(t, world.RegisterSystems(tf.World(),
		func(wCtx world.WorldContext) error {
			return world.EachMessage[ModifyScoreMsg](wCtx,
				func(msData world.Tx[ModifyScoreMsg]) (any, error) {
					ms := msData.Msg
					err := world.UpdateComponent[ScoreComponent](
						wCtx, ms.PlayerID, func(s *ScoreComponent) *ScoreComponent {
							s.Score += ms.Amount
							return s
						},
					)
					assert.NilError(t, err)
					return &EmptyMsgResult{}, nil
				},
			)
		},
	))

	tf.StartWorld()
	tf.DoTick()

	// Entities at index 5, 10 and 50 will be updated with some values
	tf.AddTransaction(
		ModifyScoreMsg{}.Name(),
		&ModifyScoreMsg{
			PlayerID: ids[5],
			Amount:   105,
		},
	)
	tf.AddTransaction(
		ModifyScoreMsg{}.Name(),
		&ModifyScoreMsg{
			PlayerID: ids[10],
			Amount:   110,
		},
	)
	tf.AddTransaction(
		ModifyScoreMsg{}.Name(),
		&ModifyScoreMsg{
			PlayerID: ids[50],
			Amount:   150,
		},
	)

	tf.DoTick()

	for i, id := range ids {
		wantScore := 0
		switch i {
		case 5:
			wantScore = 105
		case 10:
			wantScore = 110
		case 50:
			wantScore = 150
		}
		tf.World().View(func(wCtx world.WorldContextReadOnly) error {
			s, err := world.GetComponent[ScoreComponent](wCtx, id)
			assert.NilError(t, err)
			assert.Equal(t, wantScore, s.Score)
			return nil
		})
	}
}

// TestAddToPoolDuringTickDoesNotTimeout verifies that we can add a transaction to the transaction
// pool during a game tick, and the call does not block.
func TestAddToPoolDuringTickDoesNotTimeout(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterMessage[ModifyScoreMsg](tf.World()))

	shouldBlock := false
	inSystemCh := make(chan struct{})
	defer func() { close(inSystemCh) }()

	// This system will block forever. This will give us a never-ending game tick that we can use
	// to verify that the addition of more transactions doesn't block.
	err := world.RegisterSystems(tf.World(), func(world.WorldContext) error {
		if shouldBlock {
			<-inSystemCh
			<-inSystemCh
		}
		return nil
	})
	assert.NilError(t, err)

	tf.StartWorld()

	tf.AddTransaction(ModifyScoreMsg{}.Name(), &ModifyScoreMsg{})

	// Start a tick in the background.
	go func() {
		shouldBlock = true
		tf.DoTick()
	}()

	// Make sure we're actually in the system.
	inSystemCh <- struct{}{}

	// Make sure we can call addTransaction again in a reasonable amount of time
	timeout := time.After(500 * time.Millisecond)
	doneWithAddTx := make(chan struct{})

	go func() {
		tf.AddTransaction(ModifyScoreMsg{}.Name(), &ModifyScoreMsg{})
		doneWithAddTx <- struct{}{}
	}()

	select {
	case <-doneWithAddTx:
	// happy path
	case <-timeout:
		t.Fatal("timeout while trying to addTransaction")
	}
	// release the system
	inSystemCh <- struct{}{}

	// Second tick to make sure all transaction processed before shutdown
	tickDone := make(chan struct{})
	go func() {
		tf.DoTick()
		tickDone <- struct{}{}
	}()
	inSystemCh <- struct{}{}
	inSystemCh <- struct{}{}

	// wait for tick done to prevent panic on shutdown
	<-tickDone
}

func TestCannotRegisterDuplicateTransaction(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterMessage[ModifyScoreMsg](tf.World()))
	assert.IsError(t, world.RegisterMessage[ModifyScoreMsg](tf.World()))
}

type MoveMsg struct {
	DeltaX, DeltaY int
}

func (MoveMsg) Name() string {
	return "move"
}

type MoveMsgResult struct {
	EndX, EndY int
}

func TestTransaction_Result(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)

	// Each transaction now needs an input and an output
	assert.NilError(t, world.RegisterMessage[MoveMsg](tf.World()))

	wantDeltaX, wantDeltaY := 99, 100

	isReady := false
	err := world.RegisterSystems(tf.World(), func(wCtx world.WorldContext) error {
		if isReady {
			// This new In function returns a triplet of information:
			// 1) The transaction input
			// 2) An EntityID that uniquely identifies this specific transaction
			// 3) The signature
			// This function would replace both "In" and "TxsAndSigsIn"
			txData := make([]world.Tx[MoveMsg], 0)
			err := world.EachMessage[MoveMsg](wCtx, func(tx world.Tx[MoveMsg]) (any, error) {
				// The input for the transaction is found at tx.Val
				txData = append(txData, tx)
				return MoveMsgResult{EndX: 42, EndY: 42}, nil
			})
			assert.NilError(t, err)
			fmt.Println(txData)
			assert.Equal(t, 1, len(txData), "expected 1 move transaction")
			tx := txData[0]

			// The input for the transaction is found at tx.Val
			assert.Equal(t, wantDeltaX, tx.Msg.DeltaX)
			assert.Equal(t, wantDeltaY, tx.Msg.DeltaY)
		}
		return nil
	})
	assert.NilError(t, err)
	tf.StartWorld()

	txHash := tf.AddTransaction(MoveMsg{}.Name(), MoveMsg{99, 100})

	// Tick the game so the transaction is processed
	isReady = true
	tf.DoTick()

	receipt, err := tf.World().GetReceipt(txHash)
	assert.NilError(t, err)

	var got MoveMsgResult
	err = json.Unmarshal(receipt.Result, &got)
	assert.NilError(t, err)

	assert.Equal(t, MoveMsgResult{42, 42}, got)
}

func TestTransaction_Error(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)

	// Each transaction now needs an input and an output
	assert.NilError(t, world.RegisterMessage[MoveMsg](tf.World()))

	wantFirstError := errors.New("this is a transaction error")
	wantDeltaX, wantDeltaY := 99, 100

	isReady := false
	err := world.RegisterSystems(tf.World(), func(wCtx world.WorldContext) error {
		if isReady {
			// This new In function returns a triplet of information:
			// 1) The transaction input
			// 2) An EntityID that uniquely identifies this specific transaction
			// 3) The signature
			// This function would replace both "In" and "TxsAndSigsIn"
			txData := make([]world.Tx[MoveMsg], 0)
			err := world.EachMessage[MoveMsg](wCtx,
				func(tx world.Tx[MoveMsg]) (any, error) {
					// The input for the transaction is found at tx.Val
					txData = append(txData, tx)
					return nil, wantFirstError
				},
			)
			assert.NilError(t, err)

			assert.Equal(t, 1, len(txData), "expected 1 move transaction")
			tx := txData[0]

			// The input for the transaction is found at tx.Val
			assert.Equal(t, wantDeltaX, tx.Msg.DeltaX)
			assert.Equal(t, wantDeltaY, tx.Msg.DeltaY)
		}
		return nil
	})
	assert.NilError(t, err)
	tf.StartWorld()

	txHash := tf.AddTransaction(MoveMsg{}.Name(), MoveMsg{99, 100})

	// Tick the game so the transaction is processed
	isReady = true
	tf.DoTick()

	receipt, err := tf.World().GetReceipt(txHash)
	assert.NilError(t, err)
	assert.Equal(t, wantFirstError.Error(), receipt.Error)
}

func TestCanGetTimestampFromWorldContext(t *testing.T) {
	var ts int64
	tf := cardinal.NewTestCardinal(t, nil)
	err := world.RegisterSystems(tf.World(), func(context world.WorldContext) error {
		ts = context.Timestamp()
		return nil
	})
	assert.NilError(t, err)
	tf.StartWorld()
	tf.DoTick()
	lastTS := ts
	time.Sleep(time.Second)
	tf.DoTick()
	assert.Check(t, ts > lastTS)
}

func wsURL(addr, path string) string {
	return fmt.Sprintf("ws://%s/%s", addr, path)
}
