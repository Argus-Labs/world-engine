package cardinal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/worldstage"
	"pkg.world.dev/world-engine/sign"
)

func TestIfPanicMessageLogged(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	t.Setenv("REDIS_ADDRESS", miniRedis.Addr())

	// replaces internal Logger with one that logs to the buf variable above.
	neverTick := make(chan time.Time)

	// Create a logger that writes to a buffer so we can check the output
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)

	world, err := NewWorld(
		WithTickChannel(neverTick),
		WithPort(getOpenPort(t)),
		WithCustomLogger(bufLogger),
	)

	assert.NilError(t, err)

	// In this test, our "buggy" system fails once Power reaches 3
	errorTxt := "BIG ERROR OH NO"
	err = RegisterSystems(
		world,
		func(engine.Context) error {
			panic(errorTxt)
		},
	)
	assert.NilError(t, err)
	go func() {
		err = world.StartGame()
		assert.NilError(t, err)
	}()
	<-world.worldStage.NotifyOnStage(worldstage.Running)
	defer func() {
		assert.NilError(t, world.Shutdown())
	}()

	ctx := context.Background()

	defer func() {
		if panicValue := recover(); panicValue != nil {
			// This test should swallow a panic
			lastjson, err := findLastJSON(buf.Bytes())
			assert.NilError(t, err)
			values := map[string]string{}
			err = json.Unmarshal(lastjson, &values)
			assert.NilError(t, err)
			msg, ok := values["message"]
			assert.Assert(t, ok)
			assert.Contains(t, msg, "Tick: 0, Current running system:")
			panicString, ok := panicValue.(string)
			assert.Assert(t, ok)
			assert.Contains(t, panicString, errorTxt)
		} else {
			assert.Assert(t, false) // This test should create a panic.
		}
	}()

	world.tickTheEngine(ctx, nil)
}

type ScalarComponentStatic struct {
	Val int
}

type ScalarComponentToggle struct {
	Val int
}

func (ScalarComponentStatic) Name() string {
	return "static"
}

func (ScalarComponentToggle) Name() string {
	return "toggle"
}

func TestCanRecoverStateAfterFailedArchetypeChange(t *testing.T) {
	miniRedis := miniredis.RunT(t)
	t.Setenv("REDIS_ADDRESS", miniRedis.Addr())

	ctx := context.Background()

	for _, firstEngineIteration := range []bool{true, false} {
		world, err := NewWorld(WithPort(getOpenPort(t)))
		assert.NilError(t, err)

		assert.NilError(t, RegisterComponent[ScalarComponentStatic](world))
		assert.NilError(t, RegisterComponent[ScalarComponentToggle](world))

		wCtx := NewWorldContext(world)

		errorToggleComponent := errors.New("problem with toggle component")
		err = RegisterSystems(
			world,
			func(wCtx engine.Context) error {
				// Get the one and only entity ID
				q := NewSearch(wCtx, filter.Contains(ScalarComponentStatic{}))
				id, err := q.First()
				assert.NilError(t, err)

				s, err := GetComponent[ScalarComponentStatic](wCtx, id)
				assert.NilError(t, err)
				s.Val++
				assert.NilError(t, SetComponent[ScalarComponentStatic](wCtx, id, s))
				if s.Val%2 == 1 {
					assert.NilError(t, AddComponentTo[ScalarComponentToggle](wCtx, id))
				} else {
					assert.NilError(t, RemoveComponentFrom[ScalarComponentToggle](wCtx, id))
				}

				if firstEngineIteration && s.Val == 5 {
					return errorToggleComponent
				}

				return nil
			},
		)
		assert.NilError(t, err)

		go func() {
			err = world.StartGame()
			assert.NilError(t, err)
		}()
		<-world.worldStage.NotifyOnStage(worldstage.Running)

		if firstEngineIteration {
			_, err := Create(wCtx, ScalarComponentStatic{})
			assert.NilError(t, err)
		}
		q := NewSearch(wCtx, filter.Contains(ScalarComponentStatic{}))
		id, err := q.First()
		assert.NilError(t, err)

		if firstEngineIteration {
			for i := 0; i < 4; i++ {
				world.tickTheEngine(ctx, nil)
			}
			// After 4 ticks, static.Val should be 4 and toggle should have just been removed from the entity.
			_, err = GetComponent[ScalarComponentToggle](wCtx, id)
			assert.ErrorIs(t, iterators.ErrComponentNotOnEntity, eris.Cause(err))

			// Ticking again should result in an error
			err = doTickCapturePanic(ctx, world)
			assert.ErrorContains(t, err, errorToggleComponent.Error())
		} else {
			// At this second iteration, the errorToggleComponent bug has been fixed. static.Val should be 5
			// and toggle should have just been added to the entity.
			_, err = GetComponent[ScalarComponentToggle](wCtx, id)
			assert.NilError(t, err)

			s, err := GetComponent[ScalarComponentStatic](wCtx, id)

			assert.NilError(t, err)
			assert.Equal(t, 5, s.Val)
		}

		assert.NilError(t, world.Shutdown())
	}

	miniRedis.Close()
}

type PowerComp struct {
	Val float64
}

func (PowerComp) Name() string {
	return "powerComp"
}

func TestCanRecoverTransactionsFromFailedSystemRun(t *testing.T) {
	rs := miniredis.RunT(t)
	t.Setenv("REDIS_ADDRESS", rs.Addr())

	ctx := context.Background()

	errorBadPowerChange := errors.New("bad power change message")
	for _, isBuggyIteration := range []bool{true, false} {
		world, err := NewWorld(WithPort(getOpenPort(t)))
		assert.NilError(t, err)

		assert.NilError(t, RegisterComponent[PowerComp](world))
		msgName := "change_power"
		assert.NilError(t, RegisterMessage[PowerComp, PowerComp](world, msgName))

		err = RegisterSystems(
			world,
			func(wCtx engine.Context) error {
				q := NewSearch(wCtx, filter.Contains(PowerComp{}))
				id := q.MustFirst()
				entityPower, err := GetComponent[PowerComp](wCtx, id)
				assert.NilError(t, err)
				powerTx, err := getMessage[PowerComp, PowerComp](wCtx)
				assert.NilError(t, err)
				changes := powerTx.In(wCtx)
				assert.Equal(t, 1, len(changes))
				entityPower.Val += changes[0].Msg.Val
				assert.NilError(t, SetComponent[PowerComp](wCtx, id, entityPower))

				if isBuggyIteration && changes[0].Msg.Val == 666 {
					return errorBadPowerChange
				}
				return nil
			},
		)
		assert.NilError(t, err)
		go func() {
			err = world.StartGame()
			assert.NilError(t, err)
		}()
		<-world.worldStage.NotifyOnStage(worldstage.Running)

		wCtx := NewWorldContext(world)
		// Only Create the entity for the first iteration
		if isBuggyIteration {
			_, err := Create(wCtx, PowerComp{})
			assert.NilError(t, err)
		}

		// fetchPower is a small helper to get the power of the only entity in the engine
		fetchPower := func() float64 {
			q := NewSearch(wCtx, filter.Contains(PowerComp{}))
			id, err := q.First()
			assert.NilError(t, err)
			power, err := GetComponent[PowerComp](wCtx, id)
			assert.NilError(t, err)
			return power.Val
		}
		powerTx, ok := world.GetMessageByFullName("game." + msgName)
		if isBuggyIteration {
			// perform a few ticks that will not result in an error
			assert.True(t, ok)
			world.AddTransaction(powerTx.ID(), PowerComp{1000}, &sign.Transaction{Nonce: 1, Signature: fakeSignature(t, 1)})
			world.tickTheEngine(ctx, nil)
			world.AddTransaction(powerTx.ID(), PowerComp{1000}, &sign.Transaction{Nonce: 2, Signature: fakeSignature(t, 2)})
			world.tickTheEngine(ctx, nil)
			world.AddTransaction(powerTx.ID(), PowerComp{1000}, &sign.Transaction{Nonce: 3, Signature: fakeSignature(t, 3)})
			world.tickTheEngine(ctx, nil)
			assert.Equal(t, float64(3000), fetchPower())

			// In this "buggy" iteration, the above system cannot handle a power of 666.
			world.AddTransaction(powerTx.ID(), PowerComp{666}, &sign.Transaction{Nonce: 4, Signature: fakeSignature(t, 4)})
			err = doTickCapturePanic(ctx, world)
			assert.ErrorContains(t, err, errorBadPowerChange.Error())
		} else {
			// Loading the game state above should successfully re-process that final 666 messages.
			assert.Equal(t, float64(3666), fetchPower())

			// One more tick for good measure
			world.AddTransaction(powerTx.ID(), PowerComp{1000}, &sign.Transaction{Nonce: 5, Signature: fakeSignature(t, 5)})
			world.tickTheEngine(ctx, nil)

			assert.Equal(t, float64(4666), fetchPower())
		}

		assert.NilError(t, world.Shutdown())
	}
	rs.Close()
}

func fakeSignature(t *testing.T, nonce uint64) string {
	goodKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	sp, err := sign.NewTransaction(goodKey, "foo", "bar", nonce, `{"msg": "this is a request body"}`)
	assert.NilError(t, err)
	return sp.Signature
}

type onePowerComponent struct {
	Power int
}

func (onePowerComponent) Name() string {
	return "onePower"
}

type twoPowerComponent struct {
	Power int
}

func (twoPowerComponent) Name() string {
	return "twoPower"
}

func TestCanIdentifyAndFixSystemError(t *testing.T) {
	rs := miniredis.RunT(t)
	t.Setenv("REDIS_ADDRESS", rs.Addr())

	ctx := context.Background()

	world, err := NewWorld(WithPort(getOpenPort(t)))
	assert.NilError(t, err)

	assert.NilError(t, RegisterComponent[onePowerComponent](world))

	errorSystem := errors.New("3 power? That's too much, man")

	// In this test, our "buggy" system fails once Power reaches 3
	err = RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			search := NewSearch(wCtx, filter.Exact(onePowerComponent{}))
			id := search.MustFirst()
			p, err := GetComponent[onePowerComponent](wCtx, id)
			if err != nil {
				return err
			}
			p.Power++
			if p.Power >= 3 {
				return errorSystem
			}
			return SetComponent[onePowerComponent](wCtx, id, p)
		},
	)
	assert.NilError(t, err)

	go func() {
		err = world.StartGame()
		assert.NilError(t, err)
	}()
	<-world.worldStage.NotifyOnStage(worldstage.Running)

	id, err := Create(NewWorldContext(world), onePowerComponent{})
	assert.NilError(t, err)

	// Power is set to 1
	world.tickTheEngine(ctx, nil)
	// Power is set to 2
	world.tickTheEngine(ctx, nil)
	// Power is set to 3, then the System fails
	err = doTickCapturePanic(ctx, world)
	assert.ErrorContains(t, err, errorSystem.Error())

	assert.NilError(t, world.Shutdown())

	// Set up a new engine using the same storage layer
	world2, err := NewWorld(WithPort(getOpenPort(t)))
	assert.NilError(t, err)
	assert.NilError(t, RegisterComponent[onePowerComponent](world2))
	assert.NilError(t, RegisterComponent[twoPowerComponent](world2))

	// this is our fixed system that can handle Power levels of 3 and higher
	err = RegisterSystems(
		world2,
		func(wCtx engine.Context) error {
			p, err := GetComponent[onePowerComponent](wCtx, id)
			if err != nil {
				return err
			}
			p.Power++
			return SetComponent[onePowerComponent](wCtx, id, p)
		},
	)
	assert.NilError(t, err)

	// Loading a game state with the fixed system should automatically finish the previous tick.
	go func() {
		err = world2.StartGame()
		assert.NilError(t, err)
	}()
	<-world2.worldStage.NotifyOnStage(worldstage.Running)

	world2Ctx := NewWorldContext(world2)
	p, err := GetComponent[onePowerComponent](world2Ctx, id)
	assert.NilError(t, err)
	assert.Equal(t, 3, p.Power)

	// Just for fun, tick one last time to make sure power is still being incremented.
	world2.tickTheEngine(ctx, nil)
	p1, err := GetComponent[onePowerComponent](world2Ctx, id)
	assert.NilError(t, err)
	assert.Equal(t, 4, p1.Power)

	assert.NilError(t, world2.Shutdown())
}

type Foo struct{}

func (Foo) Name() string { return "foo" }

type Bar struct{}

func (Bar) Name() string { return "bar" }

type Qux struct{}

func (Qux) Name() string { return "qux" }

// TestSystemsPanicOnRedisError ensures systems panic when there is a problem connecting to redis. In general, Systems
// should panic on ANY fatal error, but this connection problem is how we'll simulate a non ecs state related error.
func TestSystemsPanicOnRedisError(t *testing.T) {
	testCases := []struct {
		name string
		// the failFn will be called at a time when the ECB is empty of cached data and redis is down.
		failFn func(wCtx engine.Context, goodID types.EntityID)
	}{
		{
			name: "AddComponentTo",
			failFn: func(wCtx engine.Context, goodID types.EntityID) {
				_ = AddComponentTo[Qux](wCtx, goodID)
			},
		},
		{
			name: "RemoveComponentFrom",
			failFn: func(wCtx engine.Context, goodID types.EntityID) {
				_ = RemoveComponentFrom[Bar](wCtx, goodID)
			},
		},
		{
			name: "GetComponent",
			failFn: func(wCtx engine.Context, goodID types.EntityID) {
				_, _ = GetComponent[Foo](wCtx, goodID)
			},
		},
		{
			name: "SetComponent",
			failFn: func(wCtx engine.Context, goodID types.EntityID) {
				_ = SetComponent[Foo](wCtx, goodID, &Foo{})
			},
		},
		{
			name: "UpdateComponent",
			failFn: func(wCtx engine.Context, goodID types.EntityID) {
				_ = UpdateComponent[Foo](wCtx, goodID, func(f *Foo) *Foo {
					return f
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			miniRedis := miniredis.RunT(t)
			t.Setenv("REDIS_ADDRESS", miniRedis.Addr())

			ctx := context.Background()

			world, err := NewWorld(WithPort(getOpenPort(t)))
			assert.NilError(t, err)

			assert.NilError(t, RegisterComponent[Foo](world))
			assert.NilError(t, RegisterComponent[Bar](world))
			assert.NilError(t, RegisterComponent[Qux](world))

			// This system will be called 2 times. The first time, a single entity is Created. The second time,
			// the previously Created entity is fetched, and then miniRedis is closed. Subsequent attempts to access
			// data should panic.
			assert.NilError(t, RegisterSystems(world, func(wCtx engine.Context) error {
				// Set up the entity in the first tick
				if wCtx.CurrentTick() == 0 {
					_, err := Create(wCtx, Foo{}, Bar{})
					assert.Check(t, err == nil)
					return nil
				}
				// Get the valid entity for the second tick
				id, err := NewSearch(wCtx, filter.Exact(Foo{}, Bar{})).First()
				assert.Check(t, err == nil)
				assert.Check(t, id != iterators.BadID)

				// Shut down redis. The testCase's failure function will now be able to fail
				miniRedis.Close()

				// Only set up this panic/recover expectation if we're in the second tick.
				defer func() {
					err := recover()
					assert.Check(t, err != nil, "expected panic")
				}()

				tc.failFn(wCtx, id)
				assert.Check(t, false, "should never reach here")
				return nil
			}))

			go func() {
				err = world.StartGame()
				assert.NilError(t, err)
			}()
			<-world.worldStage.NotifyOnStage(worldstage.Running)
			defer func() {
				assert.NilError(t, world.Shutdown())
			}()

			// The first tick sets up the entity
			world.tickTheEngine(ctx, nil)
			// The second tick calls the test case's failure function.
			err = doTickCapturePanic(ctx, world)
			assert.IsError(t, err)
		})
	}
}

func findLastJSON(buf []byte) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(buf))
	var lastVal json.RawMessage
	for {
		if err := dec.Decode(&lastVal); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
	}
	if lastVal == nil {
		return nil, fmt.Errorf("no JSON value found")
	}
	return lastVal, nil
}

func doTickCapturePanic(ctx context.Context, world *World) (err error) {
	defer func() {
		if panicValue := recover(); panicValue != nil {
			err = fmt.Errorf(panicValue.(string))
		}
	}()
	world.tickTheEngine(ctx, nil)

	return nil
}

func getMessage[In any, Out any](wCtx engine.Context) (*message.MessageType[In, Out], error) {
	var msg message.MessageType[In, Out]
	msgType := reflect.TypeOf(msg)
	tempRes, ok := wCtx.GetMessageByType(msgType)
	if !ok {
		return &msg, eris.Errorf("Could not find %s, Message may not be registered.", msg.Name())
	}
	var _ types.Message = &msg
	res, ok := tempRes.(*message.MessageType[In, Out])
	if !ok {
		return &msg, eris.New("wrong type")
	}
	return res, nil
}

func getOpenPort(t testing.TB) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	defer func() {
		assert.NilError(t, l.Close())
	}()

	assert.NilError(t, err)
	tcpAddr, err := net.ResolveTCPAddr(l.Addr().Network(), l.Addr().String())
	assert.NilError(t, err)
	return fmt.Sprintf("%d", tcpAddr.Port)
}
