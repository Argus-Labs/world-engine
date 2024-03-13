package cardinal_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"pkg.world.dev/world-engine/cardinal/router/mocks"

	"github.com/fasthttp/websocket"

	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"

	"github.com/ethereum/go-ethereum/crypto"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/sign"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

type Foo struct{}

func (Foo) Name() string { return "foo" }

type Bar struct{}

func (Bar) Name() string { return "bar" }

type Qux struct{}

func (Qux) Name() string { return "qux" }

type Rawbodytx struct {
	PersonaTag    string `json:"personaTag"`
	SignerAddress string `json:"signerAddress"`
}

type Health struct {
	Value int
}

func (Health) Name() string { return "health" }

type AddHealthToEntityTx struct {
	TargetID types.EntityID
	Amount   int
}

type AddHealthToEntityResult struct{}

func TestForEachTransaction(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	type SomeMsgRequest struct {
		GenerateError bool
	}
	type SomeMsgResponse struct {
		Successful bool
	}

	someMsgName := "some_msg"
	assert.NilError(t, cardinal.RegisterMessage[SomeMsgRequest, SomeMsgResponse](world, someMsgName))

	err := cardinal.RegisterSystems(world, func(wCtx engine.Context) error {
		return cardinal.EachMessage[SomeMsgRequest, SomeMsgResponse](wCtx,
			func(t message.TxData[SomeMsgRequest]) (result SomeMsgResponse, err error) {
				if t.Msg.GenerateError {
					return result, errors.New("some error")
				}
				return SomeMsgResponse{
					Successful: true,
				}, nil
			})
	})
	assert.NilError(t, err)
	tf.StartWorld()

	// Add 10 transactions to the tx pool and keep track of the hashes that we just cardinal.Created
	knownTxHashes := map[types.TxHash]SomeMsgRequest{}
	for i := 0; i < 10; i++ {
		someMsg, ok := world.GetMessageByName(someMsgName)
		assert.True(t, ok)
		req := SomeMsgRequest{GenerateError: i%2 == 0}
		txHash := tf.AddTransaction(someMsg.ID(), req, testutils.UniqueSignature())
		knownTxHashes[txHash] = req
	}

	// Perform a engine tick
	tf.DoTick()

	// Verify the receipts for the previous tick are what we expect
	receipts, err := world.GetTransactionReceiptsForTick(world.CurrentTick() - 1)
	assert.NilError(t, err)
	assert.Equal(t, len(knownTxHashes), len(receipts))
	for _, receipt := range receipts {
		request, ok := knownTxHashes[receipt.TxHash]
		assert.Check(t, ok)
		if request.GenerateError {
			assert.Check(t, len(receipt.Errs) > 0)
		} else {
			assert.Equal(t, 0, len(receipt.Errs))
			assert.Equal(t, receipt.Result.(SomeMsgResponse), SomeMsgResponse{Successful: true})
		}
	}
}

type CounterComponent struct {
	Count int
}

func (CounterComponent) Name() string {
	return "count"
}

func TestSystemsAreExecutedDuringGameTick(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	assert.NilError(t, cardinal.RegisterComponent[CounterComponent](world))

	wCtx := cardinal.NewWorldContext(world)

	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			search := cardinal.NewSearch(wCtx, filter.Exact(CounterComponent{}))
			id := search.MustFirst()
			return cardinal.UpdateComponent[CounterComponent](
				wCtx, id, func(c *CounterComponent) *CounterComponent {
					c.Count++
					return c
				},
			)
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()
	id, err := cardinal.Create(wCtx, CounterComponent{})
	assert.NilError(t, err)

	for i := 0; i < 10; i++ {
		tf.DoTick()
	}

	c, err := cardinal.GetComponent[CounterComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 10, c.Count)
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

type EmptyMsgResult struct{}

func TestTransactionAreAppliedToSomeEntities(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[ScoreComponent](world))

	assert.NilError(t, cardinal.RegisterMessage[*ModifyScoreMsg, *EmptyMsgResult](world, "modify_score"))

	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			return cardinal.EachMessage[*ModifyScoreMsg, *EmptyMsgResult](wCtx,
				func(msData message.TxData[*ModifyScoreMsg]) (*EmptyMsgResult, error) {
					ms := msData.Msg
					err := cardinal.UpdateComponent[ScoreComponent](
						wCtx, ms.PlayerID, func(s *ScoreComponent) *ScoreComponent {
							s.Score += ms.Amount
							return s
						},
					)
					assert.Check(t, err == nil)
					return &EmptyMsgResult{}, nil
				})
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	ids, err := cardinal.CreateMany(wCtx, 100, ScoreComponent{})
	assert.NilError(t, err)
	// Entities at index 5, 10 and 50 will be updated with some values
	modifyScoreMsg, err := testutils.GetMessage[*ModifyScoreMsg, *EmptyMsgResult](wCtx)
	assert.NilError(t, err)
	tf.AddTransaction(
		modifyScoreMsg.ID(), &ModifyScoreMsg{
			PlayerID: ids[5],
			Amount:   105,
		},
	)
	tf.AddTransaction(
		modifyScoreMsg.ID(), &ModifyScoreMsg{
			PlayerID: ids[10],
			Amount:   110,
		},
	)
	tf.AddTransaction(
		modifyScoreMsg.ID(), &ModifyScoreMsg{
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
		s, err := cardinal.GetComponent[ScoreComponent](wCtx, id)
		assert.NilError(t, err)
		assert.Equal(t, wantScore, s.Score)
	}
}

// TestAddToPoolDuringTickDoesNotTimeout verifies that we can add a transaction to the transaction
// pool during a game tick, and the call does not block.
func TestAddToPoolDuringTickDoesNotTimeout(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	msgName := "modify_Score"
	assert.NilError(t, cardinal.RegisterMessage[*ModifyScoreMsg, *EmptyMsgResult](world, msgName))

	inSystemCh := make(chan struct{})
	defer func() { close(inSystemCh) }()
	// This system will block forever. This will give us a never-ending game tick that we can use
	// to verify that the addition of more transactions doesn't block.
	err := cardinal.RegisterSystems(
		world,
		func(engine.Context) error {
			<-inSystemCh
			<-inSystemCh
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()
	modScore, ok := world.GetMessageByName(msgName)
	assert.True(t, ok)
	tf.AddTransaction(modScore.ID(), &ModifyScoreMsg{})

	// Start a tick in the background.
	go func() {
		tf.DoTick()
	}()
	// Make sure we're actually in the System.
	inSystemCh <- struct{}{}

	// Make sure we can call AddTransaction again in a reasonable amount of time
	timeout := time.After(500 * time.Millisecond)
	doneWithAddTx := make(chan struct{})

	go func() {
		tf.AddTransaction(modScore.ID(), &ModifyScoreMsg{})
		doneWithAddTx <- struct{}{}
	}()

	select {
	case <-doneWithAddTx:
	// happy path
	case <-timeout:
		t.Fatal("timeout while trying to AddTransaction")
	}
}

// TestTransactionsAreExecutedAtNextTick verifies that while a game tick is taking place, new transactions
// are added to some pool that is not processed until the NEXT tick.
func TestTransactionsAreExecutedAtNextTick(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	msgName := "modify_score"
	assert.NilError(t, cardinal.RegisterMessage[*ModifyScoreMsg, *EmptyMsgResult](world, msgName))
	tickStart := tf.StartTickCh
	tickDone := tf.DoneTickCh

	modScoreCountCh := make(chan int)

	// Create two system that report how many instances of the ModifyScoreMsg exist in the
	// transaction pool. These counts should be the same for each tick. modScoreCountCh is an unbuffered channel
	// so these systems will block while writing to modScoreCountCh. This allows the test to ensure that we can run
	// commands mid-tick.
	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			modScoreMsg, err := testutils.GetMessage[*ModifyScoreMsg, *EmptyMsgResult](wCtx)
			if err != nil {
				return err
			}
			modScores := modScoreMsg.In(wCtx)
			modScoreCountCh <- len(modScores)
			return nil
		},
	)
	assert.NilError(t, err)

	err = cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			modScoreMsg, err := testutils.GetMessage[*ModifyScoreMsg, *EmptyMsgResult](wCtx)
			if err != nil {
				return err
			}
			modScores := modScoreMsg.In(wCtx)
			modScoreCountCh <- len(modScores)
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()
	modScoreMsg, ok := world.GetMessageByName(msgName)
	assert.True(t, ok)
	tf.AddTransaction(modScoreMsg.ID(), &ModifyScoreMsg{})

	// Start the game tick. The tick will block while waiting to write to modScoreCountCh
	tickStart <- time.Now()

	// In the first system, we should see 1 modify score transaction
	count := <-modScoreCountCh
	assert.Equal(t, 1, count)

	// Add two transactions mid-tick.
	tf.AddTransaction(modScoreMsg.ID(), &ModifyScoreMsg{})
	tf.AddTransaction(modScoreMsg.ID(), &ModifyScoreMsg{})

	// The tick is still not over, so we should still only see 1 modify score transaction
	count = <-modScoreCountCh
	assert.Equal(t, 1, count)

	// Block until the tick has completed.
	<-tickDone

	// Start the next tick.
	tickStart <- time.Now()

	// This second tick should find 2 ModifyScore transactions. They were added in the middle of the previous tick.
	count = <-modScoreCountCh
	assert.Equal(t, 2, count)
	count = <-modScoreCountCh
	assert.Equal(t, 2, count)

	// Block until the tick has completed.
	<-tickDone

	// In this final tick, we should see no modify score transactions
	tickStart <- time.Now()
	count = <-modScoreCountCh
	assert.Equal(t, 0, count)
	count = <-modScoreCountCh
	assert.Equal(t, 0, count)
	<-tickDone
}

func TestCannotRegisterDuplicateTransaction(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterMessage[ModifyScoreMsg, EmptyMsgResult](world, "modify_score"))
	assert.IsError(t, cardinal.RegisterMessage[ModifyScoreMsg, EmptyMsgResult](world, "modify_score"))
}

func TestCannotCallRegisterTransactionsMultipleTimes(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterMessage[ModifyScoreMsg, EmptyMsgResult](world, "modify_score"))
	assert.Check(t, nil != cardinal.RegisterMessage[ModifyScoreMsg, EmptyMsgResult](world, "modify_score"))
}

func TestCannotHaveDuplicateTransactionNames(t *testing.T) {
	type SomeMsg struct {
		X, Y, Z int
	}
	type OtherMsg struct {
		Alpha, Beta string
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	err := cardinal.RegisterMessage[SomeMsg, EmptyMsgResult](world, "name_match")
	assert.NilError(t, err)
	err = cardinal.RegisterMessage[OtherMsg, EmptyMsgResult](world, "name_match")
	assert.IsError(t, err)
}

func TestCanGetTransactionErrorsAndResults(t *testing.T) {
	type MoveMsg struct {
		DeltaX, DeltaY int
	}
	type MoveMsgResult struct {
		EndX, EndY int
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	// Each transaction now needs an input and an output
	msgName := "move"
	assert.NilError(t, cardinal.RegisterMessage[MoveMsg, MoveMsgResult](world, msgName))

	wantFirstError := errors.New("this is a transaction error")
	wantSecondError := errors.New("another transaction error")
	wantDeltaX, wantDeltaY := 99, 100

	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			// This new In function returns a triplet of information:
			// 1) The transaction input
			// 2) An EntityID that uniquely identifies this specific transaction
			// 3) The signature
			// This function would replace both "In" and "TxsAndSigsIn"
			moveMsg, err := testutils.GetMessage[MoveMsg, MoveMsgResult](wCtx)
			assert.NilError(t, err)
			txData := moveMsg.In(wCtx)
			assert.Equal(t, 1, len(txData), "expected 1 move transaction")
			tx := txData[0]
			// The input for the transaction is found at tx.Val
			assert.Equal(t, wantDeltaX, tx.Msg.DeltaX)
			assert.Equal(t, wantDeltaY, tx.Msg.DeltaY)

			// AddError will associate an error with the tx.TxHash. Multiple errors can be
			// associated with a transaction.
			moveMsg.AddError(wCtx, tx.Hash, wantFirstError)
			moveMsg.AddError(wCtx, tx.Hash, wantSecondError)

			// SetResult sets the output for the transaction. Only one output can be set
			// for a tx.TxHash (the last assigned result will clobber other results)
			moveMsg.SetResult(wCtx, tx.Hash, MoveMsgResult{42, 42})
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()
	moveMsg, ok := world.GetMessageByName(msgName)
	assert.True(t, ok)
	_ = tf.AddTransaction(moveMsg.ID(), MoveMsg{99, 100})

	// Tick the game so the transaction is processed
	tf.DoTick()

	tick := world.CurrentTick() - 1
	receipts, err := world.GetTransactionReceiptsForTick(tick)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(receipts))
	r := receipts[0]
	assert.Equal(t, 2, len(r.Errs))
	assert.ErrorIs(t, wantFirstError, r.Errs[0])
	assert.ErrorIs(t, wantSecondError, r.Errs[1])
	got, ok := r.Result.(MoveMsgResult)
	assert.Check(t, ok)
	assert.Equal(t, MoveMsgResult{42, 42}, got)
}

func TestSystemCanFindErrorsFromEarlierSystem(t *testing.T) {
	type MsgIn struct {
		Number int
	}
	type MsgOut struct {
		Number int
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	msgName := "number"
	assert.NilError(t, cardinal.RegisterMessage[MsgIn, MsgOut](world, msgName))
	wantErr := errors.New("some transaction error")
	systemCalls := 0
	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			systemCalls++
			numTx, err := testutils.GetMessage[MsgIn, MsgOut](wCtx)
			if err != nil {
				return err
			}
			txs := numTx.In(wCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			_, _, ok := numTx.GetReceipt(wCtx, hash)
			assert.Check(t, !ok)
			numTx.AddError(wCtx, hash, wantErr)
			return nil
		},
	)
	assert.NilError(t, err)

	err = cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			systemCalls++
			numTx, err := testutils.GetMessage[MsgIn, MsgOut](wCtx)
			if err != nil {
				return err
			}
			txs := numTx.In(wCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			_, errs, ok := numTx.GetReceipt(wCtx, hash)
			assert.Check(t, ok)
			assert.Equal(t, 1, len(errs))
			assert.ErrorIs(t, wantErr, errs[0])
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()
	numTx, ok := world.GetMessageByName(msgName)
	assert.True(t, ok)
	_ = tf.AddTransaction(numTx.ID(), MsgIn{100})

	tf.DoTick()
	assert.Equal(t, 2, systemCalls)
}

func TestSystemCanClobberTransactionResult(t *testing.T) {
	type MsgIn struct {
		Number int
	}
	type MsgOut struct {
		Number int
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	msgName := "number"
	assert.NilError(t, cardinal.RegisterMessage[MsgIn, MsgOut](world, msgName))
	systemCalls := 0

	firstResult := MsgOut{1234}
	secondResult := MsgOut{5678}
	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			systemCalls++
			numTx, err := testutils.GetMessage[MsgIn, MsgOut](wCtx)
			assert.NilError(t, err)
			txs := numTx.In(wCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			_, _, ok := numTx.GetReceipt(wCtx, hash)
			assert.Check(t, !ok)
			numTx.SetResult(wCtx, hash, firstResult)
			return nil
		},
	)
	assert.NilError(t, err)

	err = cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			systemCalls++
			numTx, err := testutils.GetMessage[MsgIn, MsgOut](wCtx)
			if err != nil {
				return err
			}
			txs := numTx.In(wCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			out, errs, ok := numTx.GetReceipt(wCtx, hash)
			assert.Check(t, ok)
			assert.Equal(t, 0, len(errs))
			assert.Equal(t, MsgOut{1234}, out)
			numTx.SetResult(wCtx, hash, secondResult)
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()

	numTx, ok := world.GetMessageByName(msgName)
	assert.True(t, ok)
	_ = tf.AddTransaction(numTx.ID(), MsgIn{100})

	tf.DoTick()

	prevTick := world.CurrentTick() - 1
	receipts, err := world.GetTransactionReceiptsForTick(prevTick)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(receipts))
	r := receipts[0]
	assert.Equal(t, 0, len(r.Errs))
	gotResult, ok := r.Result.(MsgOut)
	assert.Check(t, ok)
	assert.Equal(t, secondResult, gotResult)
}

func TestTransactionExample(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world, doTick := tf.World, tf.DoTick
	assert.NilError(t, cardinal.RegisterComponent[Health](world))
	msgName := "add_health"
	assert.NilError(t, cardinal.RegisterMessage[AddHealthToEntityTx, AddHealthToEntityResult](world, msgName))
	err := cardinal.RegisterSystems(world, func(wCtx engine.Context) error {
		// test "In" method
		addHealthToEntity, err := testutils.GetMessage[AddHealthToEntityTx, AddHealthToEntityResult](wCtx)
		if err != nil {
			return err
		}
		for _, tx := range addHealthToEntity.In(wCtx) {
			targetID := tx.Msg.TargetID
			err := cardinal.UpdateComponent[Health](wCtx, targetID, func(h *Health) *Health {
				h.Value = tx.Msg.Amount
				return h
			})
			assert.Check(t, err == nil)
		}
		// test same as above but with .Each
		addHealthToEntity.Each(wCtx,
			func(tx message.TxData[AddHealthToEntityTx]) (AddHealthToEntityResult, error) {
				targetID := tx.Msg.TargetID
				err := cardinal.UpdateComponent[Health](wCtx, targetID,
					func(h *Health) *Health {
						h.Value = tx.Msg.Amount
						return h
					})
				assert.Check(t, err == nil)
				return AddHealthToEntityResult{}, errors.New("fake tx error")
			})

		return nil
	})
	assert.NilError(t, err)

	testWorldCtx := cardinal.NewWorldContext(world)
	doTick()
	ids, err := cardinal.CreateMany(testWorldCtx, 10, Health{})
	assert.NilError(t, err)

	// Queue up the transaction.
	idToModify := ids[3]
	amountToModify := 20
	payload := testutils.UniqueSignature()
	addHealthToEntity, ok := world.GetMessageByName(msgName)
	assert.True(t, ok)
	testutils.AddTransactionToWorldByAnyTransaction(
		world, addHealthToEntity,
		AddHealthToEntityTx{idToModify, amountToModify}, payload,
	)

	// The health change should be applied during this tick
	doTick()

	// Make sure the target entity had its health updated.
	for _, id := range ids {
		var health *Health
		health, err = cardinal.GetComponent[Health](testWorldCtx, id)
		assert.NilError(t, err)
		if id == idToModify {
			assert.Equal(t, amountToModify, health.Value)
		} else {
			assert.Equal(t, 0, health.Value)
		}
	}
	// Make sure transaction errors are recorded in the receipt
	receipts, err := testWorldCtx.GetTransactionReceiptsForTick(testWorldCtx.CurrentTick() - 1)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(receipts))
	assert.Equal(t, 1, len(receipts[0].Errs))
}

func TestCreatePersona(t *testing.T) {
	namespace := "custom-namespace"
	t.Setenv("CARDINAL_NAMESPACE", namespace)
	tf := testutils.NewTestFixture(t, nil)
	addr := tf.BaseURL
	tf.DoTick()

	goodKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	body := Rawbodytx{
		PersonaTag:    "a",
		SignerAddress: crypto.PubkeyToAddress(goodKey.PublicKey).Hex(),
	}
	wantBody, err := json.Marshal(body)
	assert.NilError(t, err)
	wantNonce := uint64(100)
	sp, err := sign.NewSystemTransaction(goodKey, namespace, wantNonce, wantBody)
	assert.NilError(t, err)
	bodyBytes, err := json.Marshal(sp)
	assert.NilError(t, err)
	client := &http.Client{}
	req, err := http.NewRequest(
		http.MethodPost, "http://"+addr+"/tx/persona/create-persona", bytes.NewBuffer(bodyBytes))
	assert.NilError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
}

func TestNewWorld(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	assert.Equal(t, tf.World.Namespace(), cardinal.DefaultNamespace)
}

func TestNewWorldWithCustomNamespace(t *testing.T) {
	t.Setenv("CARDINAL_NAMESPACE", "custom-namespace")
	tf := testutils.NewTestFixture(t, nil)
	assert.Equal(t, tf.World.Namespace(), "custom-namespace")
}

func TestCanQueryInsideSystem(t *testing.T) {
	testutils.SetTestTimeout(t, 10*time.Second)

	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	gotNumOfEntities := 0
	err := cardinal.RegisterSystems(world, func(wCtx engine.Context) error {
		err := cardinal.NewSearch(wCtx, filter.Exact(Foo{})).Each(func(types.EntityID) bool {
			gotNumOfEntities++
			return true
		})
		assert.NilError(t, err)
		return nil
	})
	assert.NilError(t, err)

	tf.DoTick()
	wantNumOfEntities := 10
	wCtx := cardinal.NewWorldContext(world)
	_, err = cardinal.CreateMany(wCtx, wantNumOfEntities, Foo{})
	assert.NilError(t, err)
	tf.DoTick()
	assert.Equal(t, world.CurrentTick(), uint64(2))
	assert.Equal(t, gotNumOfEntities, wantNumOfEntities)
}

func TestCanGetTimestampFromWorldContext(t *testing.T) {
	var ts uint64
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	err := cardinal.RegisterSystems(world, func(context engine.Context) error {
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

func TestShutdownViaSignal(t *testing.T) {
	// If this test is frozen then it failed to shut down, create a failure with panic.
	testutils.SetTestTimeout(t, 10*time.Second)
	tf := testutils.NewTestFixture(t, nil)
	world, addr := tf.World, tf.BaseURL
	httpBaseURL := "http://" + addr
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))
	wantNumOfEntities := 10
	err := cardinal.RegisterInitSystems(world, func(wCtx engine.Context) error {
		_, err := cardinal.CreateMany(wCtx, wantNumOfEntities/2, Foo{})
		if err != nil {
			return err
		}
		return nil
	})
	assert.NilError(t, err)
	tf.StartWorld()
	wCtx := cardinal.NewWorldContext(world)
	_, err = cardinal.CreateMany(wCtx, wantNumOfEntities/2, Foo{})
	assert.NilError(t, err)
	// test CORS with cardinal
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, httpBaseURL+"/query/http/endpoints", nil)
	assert.NilError(t, err)
	req.Header.Set("Origin", "http://www.bullshit.com") // test CORS
	resp, err := client.Do(req)
	assert.NilError(t, err)
	v := resp.Header.Get("Access-Control-Allow-Origin")
	assert.Equal(t, v, "*")
	assert.Equal(t, resp.StatusCode, 200)

	wsBaseURL := "ws://" + addr
	conn, _, err := websocket.DefaultDialer.Dial(wsBaseURL+"/events", nil)
	assert.NilError(t, err)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		_, _, err := conn.ReadMessage()
		assert.Assert(t, websocket.IsCloseError(err, websocket.CloseAbnormalClosure))
		wg.Done()
	}()

	// Send a SIGINT signal.
	cmd := exec.Command("kill", "-SIGINT", strconv.Itoa(os.Getpid()))
	err = cmd.Run()
	assert.NilError(t, err)

	for world.IsGameRunning() {
		// wait until game loop is not running
		time.Sleep(50 * time.Millisecond)
	}
}

func TestWithPrettyLog_LogIsNotJSONFormatted(t *testing.T) {
	world := testutils.NewTestFixture(t, nil, cardinal.WithPrettyLog()).World
	assert.NotNil(t, world.Logger)

	r, w, _ := os.Pipe()
	os.Stderr = w

	world.Logger.Info().Msg("test")
	err := w.Close()
	assert.NilError(t, err)

	output, err := io.ReadAll(r)
	assert.NilError(t, err)
	assert.Assert(t, !isValidJSON(output))
}

func TestCallsRegisterGameShardOnStartup(t *testing.T) {
	ctrl := gomock.NewController(t)
	rtr := mocks.NewMockRouter(ctrl)
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	world.SetRouter(rtr)

	rtr.EXPECT().Start().Times(1)
	rtr.EXPECT().RegisterGameShard(gomock.Any()).Times(1)
	rtr.EXPECT().SubmitTxBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	tf.DoTick()
}

// isValidJSON tests if a string is valid JSON.
func isValidJSON(bz []byte) bool {
	var js map[string]interface{}
	return json.Unmarshal(bz, &js) == nil
}
