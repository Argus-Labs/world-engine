package cardinal_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fasthttp/websocket"
	"github.com/golang/mock/gomock"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/router/mocks"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
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

func TestForEachTransaction(t *testing.T) {
	testCases := []struct {
		name          string
		generateError bool
		numTx         int
		wantSuccess   bool
	}{
		{
			name:          "single successful transaction",
			generateError: false,
			numTx:         1,
			wantSuccess:   true,
		},
		{
			name:          "single failed transaction",
			generateError: true,
			numTx:         1,
			wantSuccess:   false,
		},
		{
			name:          "multiple successful transactions",
			generateError: false,
			numTx:         5,
			wantSuccess:   true,
		},
		{
			name:          "multiple mixed transactions",
			generateError: true,
			numTx:         3,
			wantSuccess:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := cardinal.NewTestFixture(t, nil)
			world := tf.World

			type SomeMsgRequest struct {
				GenerateError bool
			}
			type SomeMsgResponse struct {
				Successful bool
			}

			someMsgName := "some_msg"
			assert.NilError(t, cardinal.RegisterMessage[SomeMsgRequest, SomeMsgResponse](world, someMsgName))

			err := cardinal.RegisterSystems(world, func(wCtx cardinal.WorldContext) error {
				return cardinal.EachMessage[SomeMsgRequest, SomeMsgResponse](wCtx,
					func(t cardinal.TxData[SomeMsgRequest]) (result SomeMsgResponse, err error) {
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

			knownTxHashes := map[types.TxHash]SomeMsgRequest{}
			someMsg, ok := world.GetMessageByFullName("game." + someMsgName)
			assert.True(t, ok)
			for i := 0; i < tc.numTx; i++ {
				req := SomeMsgRequest{GenerateError: tc.generateError}
				txHash := tf.AddTransaction(someMsg.ID(), req, testutils.UniqueSignature())
				knownTxHashes[txHash] = req
			}

			tf.DoTick()

			receipts, err := world.GetTransactionReceiptsForTick(world.CurrentTick() - 1)
			assert.NilError(t, err)
			assert.Equal(t, len(knownTxHashes), len(receipts))
			for _, receipt := range receipts {
				request, ok := knownTxHashes[receipt.TxHash]
				assert.Check(t, ok)
				if tc.generateError {
					assert.Check(t, len(receipt.Errs) > 0)
				} else {
					assert.Equal(t, 0, len(receipt.Errs))
					assert.Equal(t, receipt.Result.(SomeMsgResponse), SomeMsgResponse{Successful: true})
				}
			}
		})
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

type EmptyMsgResult struct{}

func TestSystemExecution(t *testing.T) {
	testCases := []struct {
		name           string
		setupSystem    func(*cardinal.World) error
		setupEntities  func(cardinal.WorldContext) ([]types.EntityID, error)
		validateState  func(*testing.T, cardinal.WorldContext, []types.EntityID)
		numTicks       int
	}{
		{
			name: "counter increments each tick",
			setupSystem: func(world *cardinal.World) error {
				if err := cardinal.RegisterComponent[CounterComponent](world); err != nil {
					return err
				}
				return cardinal.RegisterSystems(
					world,
					func(wCtx cardinal.WorldContext) error {
						search := cardinal.NewSearch().Entity(filter.Exact(filter.Component[CounterComponent]()))
						id := search.MustFirst(wCtx)
						return cardinal.UpdateComponent[CounterComponent](
							wCtx, id, func(c *CounterComponent) *CounterComponent {
								c.Count++
								return c
							},
						)
					},
				)
			},
			setupEntities: func(wCtx cardinal.WorldContext) ([]types.EntityID, error) {
				id, err := cardinal.Create(wCtx, CounterComponent{})
				return []types.EntityID{id}, err
			},
			validateState: func(t *testing.T, wCtx cardinal.WorldContext, ids []types.EntityID) {
				c, err := cardinal.GetComponent[CounterComponent](wCtx, ids[0])
				assert.NilError(t, err)
				assert.Equal(t, 10, c.Count)
			},
			numTicks: 10,
		},
		{
			name: "score updates from transactions",
			setupSystem: func(world *cardinal.World) error {
				if err := cardinal.RegisterComponent[ScoreComponent](world); err != nil {
					return err
				}
				if err := cardinal.RegisterMessage[*ModifyScoreMsg, *EmptyMsgResult](world, "modify_score"); err != nil {
					return err
				}
				return cardinal.RegisterSystems(
					world,
					func(wCtx cardinal.WorldContext) error {
						return cardinal.EachMessage[*ModifyScoreMsg, *EmptyMsgResult](wCtx,
							func(msData cardinal.TxData[*ModifyScoreMsg]) (*EmptyMsgResult, error) {
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
			},
			setupEntities: func(wCtx cardinal.WorldContext) ([]types.EntityID, error) {
				return cardinal.CreateMany(wCtx, 100, ScoreComponent{})
			},
			validateState: func(t *testing.T, wCtx cardinal.WorldContext, ids []types.EntityID) {
				updates := map[int]int{5: 105, 10: 110, 50: 150}
				for i, id := range ids {
					wantScore := updates[i]
					s, err := cardinal.GetComponent[ScoreComponent](wCtx, id)
					assert.NilError(t, err)
					assert.Equal(t, wantScore, s.Score)
				}
			},
			numTicks: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := cardinal.NewTestFixture(t, nil)
			world := tf.World

			// Setup system
			assert.NilError(t, tc.setupSystem(world))
			tf.StartWorld()

			// Setup entities
			wCtx := cardinal.NewWorldContext(world)
			ids, err := tc.setupEntities(wCtx)
			assert.NilError(t, err)

			// Add transactions if needed
			if tc.name == "score updates from transactions" {
				modifyScoreMsg, err := testutils.GetMessage[*ModifyScoreMsg, *EmptyMsgResult](tf.World)
				assert.NilError(t, err)
				updates := map[int]int{5: 105, 10: 110, 50: 150}
				for idx, amount := range updates {
					tf.AddTransaction(
						modifyScoreMsg.ID(),
						&ModifyScoreMsg{
							PlayerID: ids[idx],
							Amount:   amount,
						},
					)
				}
			}

			// Execute ticks
			for i := 0; i < tc.numTicks; i++ {
				tf.DoTick()
			}

			// Validate final state
			tc.validateState(t, wCtx, ids)
		})
	}
}

// TestAddToPoolDuringTickDoesNotTimeout verifies that we can add a transaction to the transaction
// pool during a game tick, and the call does not block.
func TestAddToPoolDuringTickDoesNotTimeout(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World

	msgName := "modify_Score"
	assert.NilError(t, cardinal.RegisterMessage[*ModifyScoreMsg, *EmptyMsgResult](world, msgName))

	inSystemCh := make(chan struct{})
	defer func() { close(inSystemCh) }()
	// This system will block forever. This will give us a never-ending game tick that we can use
	// to verify that the addition of more transactions doesn't block.
	err := cardinal.RegisterSystems(
		world,
		func(cardinal.WorldContext) error {
			<-inSystemCh
			<-inSystemCh
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()
	modScore, ok := world.GetMessageByFullName("game." + msgName)
	assert.True(t, ok)
	tf.AddTransaction(modScore.ID(), &ModifyScoreMsg{})

	// Start a tick in the background.
	go func() {
		tf.DoTick()
	}()
	// Make sure we're actually in the system.
	inSystemCh <- struct{}{}

	// Make sure we can call addTransaction again in a reasonable amount of time
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

// TestTransactionsAreExecutedAtNextTick verifies that while a game tick is taking place, new transactions
// are added to some pool that is not processed until the NEXT tick.
func TestTransactionsAreExecutedAtNextTick(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)
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
		func(wCtx cardinal.WorldContext) error {
			modScoreMsg, err := testutils.GetMessage[*ModifyScoreMsg, *EmptyMsgResult](world)
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
		func(wCtx cardinal.WorldContext) error {
			modScoreMsg, err := testutils.GetMessage[*ModifyScoreMsg, *EmptyMsgResult](world)
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
	modScoreMsg, ok := world.GetMessageByFullName("game." + msgName)
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

func TestMessageRegistration(t *testing.T) {
	testCases := []struct {
		name        string
		register    func(*cardinal.World) error
		wantError   bool
		errorString string
	}{
		{
			name: "cannot register duplicate message type",
			register: func(w *cardinal.World) error {
				if err := cardinal.RegisterMessage[ModifyScoreMsg, EmptyMsgResult](w, "modify_score"); err != nil {
					return err
				}
				return cardinal.RegisterMessage[ModifyScoreMsg, EmptyMsgResult](w, "modify_score")
			},
			wantError:   true,
			errorString: "already registered",
		},
		{
			name: "cannot register different message with same name",
			register: func(w *cardinal.World) error {
				type SomeMsg struct{ X, Y, Z int }
				type OtherMsg struct{ Alpha, Beta string }
				if err := cardinal.RegisterMessage[SomeMsg, EmptyMsgResult](w, "name_match"); err != nil {
					return err
				}
				return cardinal.RegisterMessage[OtherMsg, EmptyMsgResult](w, "name_match")
			},
			wantError:   true,
			errorString: "already registered",
		},
		{
			name: "cannot register same message type multiple times",
			register: func(w *cardinal.World) error {
				if err := cardinal.RegisterMessage[ModifyScoreMsg, EmptyMsgResult](w, "first_registration"); err != nil {
					return err
				}
				return cardinal.RegisterMessage[ModifyScoreMsg, EmptyMsgResult](w, "second_registration")
			},
			wantError:   true,
			errorString: "already registered",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := cardinal.NewTestFixture(t, nil)
			err := tc.register(tf.World)
			if tc.wantError {
				assert.ErrorContains(t, err, tc.errorString)
			} else {
				assert.NilError(t, err)
			}
		})
	}
}

func TestTransactionProcessing(t *testing.T) {
	testCases := []struct {
		name            string
		setupSystem     func(*cardinal.World) error
		addTransaction  func(*cardinal.TestFixture, types.MessageID)
		validateReceipt func(*testing.T, []types.TransactionReceipt)
		validateState   func(*testing.T, int) // For validating system calls
	}{
		{
			name: "can get transaction errors and results",
			setupSystem: func(w *cardinal.World) error {
				type MoveMsg struct {
					DeltaX, DeltaY int
				}
				type MoveMsgResult struct {
					EndX, EndY int
				}
				msgName := "move"
				if err := cardinal.RegisterMessage[MoveMsg, MoveMsgResult](w, msgName); err != nil {
					return err
				}

				wantFirstError := errors.New("this is a transaction error")
				wantSecondError := errors.New("another transaction error")

				return cardinal.RegisterSystems(w, func(wCtx cardinal.WorldContext) error {
					moveMsg, err := testutils.GetMessage[MoveMsg, MoveMsgResult](w)
					if err != nil {
						return err
					}
					txData := moveMsg.In(wCtx)
					if len(txData) != 1 {
						return fmt.Errorf("expected 1 move transaction, got %d", len(txData))
					}
					tx := txData[0]
					moveMsg.AddError(wCtx, tx.Hash, wantFirstError)
					moveMsg.AddError(wCtx, tx.Hash, wantSecondError)
					moveMsg.SetResult(wCtx, tx.Hash, MoveMsgResult{42, 42})
					return nil
				})
			},
			addTransaction: func(tf *cardinal.TestFixture, msgID types.MessageID) {
				tf.AddTransaction(msgID, struct{ DeltaX, DeltaY int }{99, 100})
			},
			validateReceipt: func(t *testing.T, receipts []types.TransactionReceipt) {
				assert.Equal(t, 1, len(receipts))
				r := receipts[0]
				assert.Equal(t, 2, len(r.Errs))
				assert.ErrorContains(t, r.Errs[0], "this is a transaction error")
				assert.ErrorContains(t, r.Errs[1], "another transaction error")
				got, ok := r.Result.(struct{ EndX, EndY int })
				assert.Check(t, ok)
				assert.Equal(t, struct{ EndX, EndY int }{42, 42}, got)
			},
		},
		{
			name: "errors propagate between systems",
			setupSystem: func(w *cardinal.World) error {
				type MsgIn struct{ Number int }
				type MsgOut struct{ Number int }
				msgName := "number"
				if err := cardinal.RegisterMessage[MsgIn, MsgOut](w, msgName); err != nil {
					return err
				}

				wantErr := errors.New("some transaction error")
				systemCalls := 0

				// First system adds error
				if err := cardinal.RegisterSystems(w, func(wCtx cardinal.WorldContext) error {
					systemCalls++
					numTx, err := testutils.GetMessage[MsgIn, MsgOut](w)
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
				}); err != nil {
					return err
				}

				// Second system verifies error
				return cardinal.RegisterSystems(w, func(wCtx cardinal.WorldContext) error {
					systemCalls++
					numTx, err := testutils.GetMessage[MsgIn, MsgOut](w)
					if err != nil {
						return err
					}
					txs := numTx.In(wCtx)
					assert.Equal(t, 1, len(txs))
					hash := txs[0].Hash
					_, errs, ok := numTx.GetReceipt(wCtx, hash)
					assert.Check(t, ok)
					assert.Equal(t, 1, len(errs))
					assert.ErrorContains(t, errs[0], "some transaction error")
					return nil
				})
			},
			addTransaction: func(tf *cardinal.TestFixture, msgID types.MessageID) {
				tf.AddTransaction(msgID, struct{ Number int }{100})
			},
			validateReceipt: func(t *testing.T, receipts []types.TransactionReceipt) {
				assert.Equal(t, 1, len(receipts))
				r := receipts[0]
				assert.Equal(t, 1, len(r.Errs))
				assert.ErrorContains(t, r.Errs[0], "some transaction error")
			},
			validateState: func(t *testing.T, systemCalls int) {
				assert.Equal(t, 2, systemCalls)
			},
		},
		{
			name: "later systems can overwrite results",
			setupSystem: func(w *cardinal.World) error {
				type MsgIn struct{ Number int }
				type MsgOut struct{ Number int }
				msgName := "number"
				if err := cardinal.RegisterMessage[MsgIn, MsgOut](w, msgName); err != nil {
					return err
				}

				firstResult := MsgOut{1234}
				secondResult := MsgOut{5678}
				systemCalls := 0

				// First system sets initial result
				if err := cardinal.RegisterSystems(w, func(wCtx cardinal.WorldContext) error {
					systemCalls++
					numTx, err := testutils.GetMessage[MsgIn, MsgOut](w)
					if err != nil {
						return err
					}
					txs := numTx.In(wCtx)
					assert.Equal(t, 1, len(txs))
					hash := txs[0].Hash
					_, _, ok := numTx.GetReceipt(wCtx, hash)
					assert.Check(t, !ok)
					numTx.SetResult(wCtx, hash, firstResult)
					return nil
				}); err != nil {
					return err
				}

				// Second system overwrites result
				return cardinal.RegisterSystems(w, func(wCtx cardinal.WorldContext) error {
					systemCalls++
					numTx, err := testutils.GetMessage[MsgIn, MsgOut](w)
					if err != nil {
						return err
					}
					txs := numTx.In(wCtx)
					assert.Equal(t, 1, len(txs))
					hash := txs[0].Hash
					out, errs, ok := numTx.GetReceipt(wCtx, hash)
					assert.Check(t, ok)
					assert.Equal(t, 0, len(errs))
					assert.Equal(t, firstResult, out)
					numTx.SetResult(wCtx, hash, secondResult)
					return nil
				})
			},
			addTransaction: func(tf *cardinal.TestFixture, msgID types.MessageID) {
				tf.AddTransaction(msgID, struct{ Number int }{100})
			},
			validateReceipt: func(t *testing.T, receipts []types.TransactionReceipt) {
				assert.Equal(t, 1, len(receipts))
				r := receipts[0]
				assert.Equal(t, 0, len(r.Errs))
				got, ok := r.Result.(struct{ Number int })
				assert.Check(t, ok)
				assert.Equal(t, struct{ Number int }{5678}, got)
			},
			validateState: func(t *testing.T, systemCalls int) {
				assert.Equal(t, 2, systemCalls)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := cardinal.NewTestFixture(t, nil)
			world := tf.World

			assert.NilError(t, tc.setupSystem(world))
			tf.StartWorld()

			moveMsg, ok := world.GetMessageByFullName("game.move")
			assert.True(t, ok)
			tc.addTransaction(tf, moveMsg.ID())

			tf.DoTick()

			receipts, err := world.GetTransactionReceiptsForTick(world.CurrentTick() - 1)
			assert.NilError(t, err)
			tc.validateReceipt(t, receipts)
			if tc.validateState != nil {
				tc.validateState(t, 2) // Both test cases use 2 system calls
			}
		})
	}
}

func TestTransactionProcessing(t *testing.T) {
	testCases := []struct {
		name            string
		setupSystem     func(*cardinal.World) error
		addTransaction  func(*cardinal.TestFixture, types.MessageID)
		validateReceipt func(*testing.T, []types.TransactionReceipt)
		validateState   func(*testing.T, int) // For validating system calls
	}{
		{
			name: "can get transaction errors and results",
			setupSystem: func(w *cardinal.World) error {
				type MoveMsg struct {
					DeltaX, DeltaY int
				}
				type MoveMsgResult struct {
					EndX, EndY int
				}
				msgName := "move"
				if err := cardinal.RegisterMessage[MoveMsg, MoveMsgResult](w, msgName); err != nil {
					return err
				}

				wantFirstError := errors.New("this is a transaction error")
				wantSecondError := errors.New("another transaction error")

				return cardinal.RegisterSystems(w, func(wCtx cardinal.WorldContext) error {
					moveMsg, err := testutils.GetMessage[MoveMsg, MoveMsgResult](w)
					if err != nil {
						return err
					}
					txData := moveMsg.In(wCtx)
					if len(txData) != 1 {
						return fmt.Errorf("expected 1 move transaction, got %d", len(txData))
					}
					tx := txData[0]
					moveMsg.AddError(wCtx, tx.Hash, wantFirstError)
					moveMsg.AddError(wCtx, tx.Hash, wantSecondError)
					moveMsg.SetResult(wCtx, tx.Hash, MoveMsgResult{42, 42})
					return nil
				})
			},
			addTransaction: func(tf *cardinal.TestFixture, msgID types.MessageID) {
				tf.AddTransaction(msgID, struct{ DeltaX, DeltaY int }{99, 100})
			},
			validateReceipt: func(t *testing.T, receipts []types.TransactionReceipt) {
				assert.Equal(t, 1, len(receipts))
				r := receipts[0]
				assert.Equal(t, 2, len(r.Errs))
				assert.ErrorContains(t, r.Errs[0], "this is a transaction error")
				assert.ErrorContains(t, r.Errs[1], "another transaction error")
				got, ok := r.Result.(struct{ EndX, EndY int })
				assert.Check(t, ok)
				assert.Equal(t, struct{ EndX, EndY int }{42, 42}, got)
			},
		},
		{
			name: "errors propagate between systems",
			setupSystem: func(w *cardinal.World) error {
				type MsgIn struct{ Number int }
				type MsgOut struct{ Number int }
				msgName := "number"
				if err := cardinal.RegisterMessage[MsgIn, MsgOut](w, msgName); err != nil {
					return err
				}

				wantErr := errors.New("some transaction error")
				systemCalls := 0

				// First system adds error
				if err := cardinal.RegisterSystems(w, func(wCtx cardinal.WorldContext) error {
					systemCalls++
					numTx, err := testutils.GetMessage[MsgIn, MsgOut](w)
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
				}); err != nil {
					return err
				}

				// Second system verifies error
				return cardinal.RegisterSystems(w, func(wCtx cardinal.WorldContext) error {
					systemCalls++
					numTx, err := testutils.GetMessage[MsgIn, MsgOut](w)
					if err != nil {
						return err
					}
					txs := numTx.In(wCtx)
					assert.Equal(t, 1, len(txs))
					hash := txs[0].Hash
					_, errs, ok := numTx.GetReceipt(wCtx, hash)
					assert.Check(t, ok)
					assert.Equal(t, 1, len(errs))
					assert.ErrorContains(t, errs[0], "some transaction error")
					return nil
				})
			},
			addTransaction: func(tf *cardinal.TestFixture, msgID types.MessageID) {
				tf.AddTransaction(msgID, struct{ Number int }{100})
			},
			validateReceipt: func(t *testing.T, receipts []types.TransactionReceipt) {
				assert.Equal(t, 1, len(receipts))
				r := receipts[0]
				assert.Equal(t, 1, len(r.Errs))
				assert.ErrorContains(t, r.Errs[0], "some transaction error")
			},
			validateState: func(t *testing.T, systemCalls int) {
				assert.Equal(t, 2, systemCalls)
			},
		},
		{
			name: "later systems can overwrite results",
			setupSystem: func(w *cardinal.World) error {
				type MsgIn struct{ Number int }
				type MsgOut struct{ Number int }
				msgName := "number"
				if err := cardinal.RegisterMessage[MsgIn, MsgOut](w, msgName); err != nil {
					return err
				}

				firstResult := MsgOut{1234}
				secondResult := MsgOut{5678}
				systemCalls := 0

				// First system sets initial result
				if err := cardinal.RegisterSystems(w, func(wCtx cardinal.WorldContext) error {
					systemCalls++
					numTx, err := testutils.GetMessage[MsgIn, MsgOut](w)
					if err != nil {
						return err
					}
					txs := numTx.In(wCtx)
					assert.Equal(t, 1, len(txs))
					hash := txs[0].Hash
					_, _, ok := numTx.GetReceipt(wCtx, hash)
					assert.Check(t, !ok)
					numTx.SetResult(wCtx, hash, firstResult)
					return nil
				}); err != nil {
					return err
				}

				// Second system overwrites result
				return cardinal.RegisterSystems(w, func(wCtx cardinal.WorldContext) error {
					systemCalls++
					numTx, err := testutils.GetMessage[MsgIn, MsgOut](w)
					if err != nil {
						return err
					}
					txs := numTx.In(wCtx)
					assert.Equal(t, 1, len(txs))
					hash := txs[0].Hash
					out, errs, ok := numTx.GetReceipt(wCtx, hash)
					assert.Check(t, ok)
					assert.Equal(t, 0, len(errs))
					assert.Equal(t, firstResult, out)
					numTx.SetResult(wCtx, hash, secondResult)
					return nil
				})
			},
			addTransaction: func(tf *cardinal.TestFixture, msgID types.MessageID) {
				tf.AddTransaction(msgID, struct{ Number int }{100})
			},
			validateReceipt: func(t *testing.T, receipts []types.TransactionReceipt) {
				assert.Equal(t, 1, len(receipts))
				r := receipts[0]
				assert.Equal(t, 0, len(r.Errs))
				got, ok := r.Result.(struct{ Number int })
				assert.Check(t, ok)
				assert.Equal(t, struct{ Number int }{5678}, got)
			},
			validateState: func(t *testing.T, systemCalls int) {
				assert.Equal(t, 2, systemCalls)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tf := cardinal.NewTestFixture(t, nil)
			world := tf.World

			assert.NilError(t, tc.setupSystem(world))
			tf.StartWorld()

			moveMsg, ok := world.GetMessageByFullName("game.move")
			assert.True(t, ok)
			tc.addTransaction(tf, moveMsg.ID())

			tf.DoTick()

			receipts, err := world.GetTransactionReceiptsForTick(world.CurrentTick() - 1)
			assert.NilError(t, err)
			tc.validateReceipt(t, receipts)
			if tc.validateState != nil {
				tc.validateState(t, 2) // Both test cases use 2 system calls
			}
		})
	}
}

func TestTransactionExample(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)
	world, doTick := tf.World, tf.DoTick
	assert.NilError(t, cardinal.RegisterComponent[Health](world))
	msgName := "add_health"
	assert.NilError(t, cardinal.RegisterMessage[AddHealthToEntityTx, AddHealthToEntityResult](world, msgName))
	err := cardinal.RegisterSystems(world, func(wCtx cardinal.WorldContext) error {
		// test "In" method
		addHealthToEntity, err := testutils.GetMessage[AddHealthToEntityTx, AddHealthToEntityResult](world)
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
			func(tx cardinal.TxData[AddHealthToEntityTx]) (AddHealthToEntityResult, error) {
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
	addHealthToEntity, ok := world.GetMessageByFullName("game." + msgName)
	assert.True(t, ok)
	tf.AddTransaction(addHealthToEntity.ID(), AddHealthToEntityTx{idToModify, amountToModify}, payload)

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
	receipts, err := tf.World.GetTransactionReceiptsForTick(testWorldCtx.CurrentTick() - 1)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(receipts))
	assert.Equal(t, 1, len(receipts[0].Errs))
}

func TestCreatePersona(t *testing.T) {
	namespace := "custom-namespace"
	t.Setenv("CARDINAL_NAMESPACE", namespace)
	tf := cardinal.NewTestFixture(t, nil)
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
	sp, err := sign.NewSystemTransaction(goodKey, namespace, wantBody)
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

	// tick before shutdown
	tf.DoTick()
}

func TestNewWorld(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)
	assert.Equal(t, tf.World.Namespace(), cardinal.DefaultCardinalNamespace)
}

func TestNewWorldWithCustomNamespace(t *testing.T) {
	t.Setenv("CARDINAL_NAMESPACE", "custom-namespace")
	tf := cardinal.NewTestFixture(t, nil)
	assert.Equal(t, tf.World.Namespace(), "custom-namespace")
}

func TestCanQueryInsideSystem(t *testing.T) {
	testutils.SetTestTimeout(t, 10*time.Second)

	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	gotNumOfEntities := 0
	err := cardinal.RegisterSystems(world, func(wCtx cardinal.WorldContext) error {
		err := cardinal.NewSearch().Entity(filter.Exact(filter.Component[Foo]())).Each(wCtx, func(types.EntityID) bool {
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

func TestRandomNumberGenerator(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World
	testAmount := 50
	numbers1 := make([]int64, 0, testAmount)
	err := cardinal.RegisterSystems(world, func(context cardinal.WorldContext) error {
		time.Sleep(5 * time.Millisecond)
		numbers1 = append(numbers1, context.Rand().Int63())
		return nil
	})
	assert.NilError(t, err)
	tf.StartWorld()
	for i := 0; i < testAmount; i++ {
		tf.DoTick()
	}
	mapOfNums := make(map[int64]bool)
	for _, num := range numbers1 {
		_, ok := mapOfNums[num]
		assert.Assert(t, ok == false)
		mapOfNums[num] = true
	}
}

func TestCanGetTimestampFromWorldContext(t *testing.T) {
	var ts uint64
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World
	err := cardinal.RegisterSystems(world, func(context cardinal.WorldContext) error {
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
	tf := cardinal.NewTestFixture(t, nil)
	world, addr := tf.World, tf.BaseURL
	httpBaseURL := "http://" + addr
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))
	wantNumOfEntities := 10
	err := cardinal.RegisterInitSystems(world, func(wCtx cardinal.WorldContext) error {
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
	req, err := http.NewRequest(http.MethodGet, httpBaseURL+"/world", nil)
	assert.NilError(t, err)
	req.Header.Set("Origin", "http://www.bullshit.com") // test CORS
	resp, err := client.Do(req)
	assert.NilError(t, err)
	v := resp.Header.Get("Access-Control-Allow-Origin")
	assert.Equal(t, v, "*")
	assert.Equal(t, resp.StatusCode, 200)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(addr, "events"), nil)
	assert.NilError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, err := conn.ReadMessage()
		assert.Assert(t, websocket.IsCloseError(err, websocket.CloseNoStatusReceived))
	}()

	// Send a SIGINT signal.
	cmd := exec.Command("kill", "-SIGINT", strconv.Itoa(os.Getpid()))
	err = cmd.Run()
	assert.NilError(t, err)

	for world.IsGameRunning() {
		// wait until game loop is not running
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for goroutine to finish otherwise it will panic
	wg.Wait()
}

func TestCallsRegisterGameShardOnStartup(t *testing.T) {
	ctrl := gomock.NewController(t)
	rtr := mocks.NewMockRouter(ctrl)
	tf := cardinal.NewTestFixture(t, nil, cardinal.WithCustomRouter(rtr))

	rtr.EXPECT().Start().Times(1)
	rtr.EXPECT().RegisterGameShard(gomock.Any()).Times(1)
	rtr.EXPECT().SubmitTxBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	tf.DoTick()
}

func wsURL(addr, path string) string {
	return fmt.Sprintf("ws://%s/%s", addr, path)
}
