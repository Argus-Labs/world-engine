package kernel

import (
	"errors"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// only admits the players it names.
type only map[string]bool

func (o only) Admits(player string, _ any) bool { return o[player] }

func TestRunner_RunsHooksInRegistrationOrder(t *testing.T) {
	t.Parallel()
	r := New(ecs.NewWorld())
	var ran []string
	for _, name := range []string{"first", "second"} {
		r.Register(Hooks{
			Name:   name,
			Begin:  func() { ran = append(ran, name+" begin") },
			Admit:  func(command.Command) Verdict { ran = append(ran, name+" admit"); return Verdict{} },
			Emit:   func(*Emit) { ran = append(ran, name+" emit") },
			Commit: func() { ran = append(ran, name+" commit") },
			Settle: func() { ran = append(ran, name+" settle") },
		})
	}
	r.Register(Hooks{Name: "third"}) // registers no hook, so nothing of it runs

	r.Begin()
	r.Admit(command.Command{Name: "move"})
	r.Emit(Broadcast, new(int), "")
	r.Commit()
	r.Settle()

	assert.Equal(t, []string{
		"first begin", "second begin",
		"first admit", "second admit",
		"first emit", "second emit",
		"first commit", "second commit",
		"first settle", "second settle",
	}, ran)
}

func TestRunner_AdmitStopsAtTheFirstVerdictThatDoesNotAdmit(t *testing.T) {
	t.Parallel()
	for name, verdict := range map[string]Verdict{
		"rejected": {Outcome: Rejected, Reason: "no_authority", CommandID: 7},
		"dropped":  {Outcome: Dropped},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := New(ecs.NewWorld())
			var asked []string
			rule := func(name string, verdict Verdict) Hooks {
				return Hooks{Name: name, Admit: func(command.Command) Verdict {
					asked = append(asked, name)
					return verdict
				}}
			}
			r.Register(rule("admits", Verdict{}))
			r.Register(rule("refuses", verdict))
			r.Register(rule("never asked", Verdict{Outcome: Rejected, Reason: "too_late"}))

			got := r.Admit(command.Command{Name: "move"})

			assert.Equal(t, verdict, got)
			assert.Equal(t, []string{"admits", "refuses"}, asked)
		})
	}
}

func TestRunner_AdmitsACommandNoHookRefuses(t *testing.T) {
	t.Parallel()
	r := New(ecs.NewWorld())
	r.Register(Hooks{Name: "lenient", Admit: func(command.Command) Verdict { return Verdict{} }})

	assert.Equal(t, Verdict{Outcome: Admitted}, r.Admit(command.Command{Name: "move"}))
}

func TestRunner_EmitHandsHooksTheEvent(t *testing.T) {
	t.Parallel()
	r := New(ecs.NewWorld())
	var seen Emit
	r.Register(Hooks{Name: "stamper", Emit: func(e *Emit) {
		seen = Emit{Kind: e.Kind, Payload: e.Payload, Recipient: e.Recipient}
		*e.Payload.(*string) += " (stamped)" //nolint:forcetypeassert // the test passes a *string
	}})
	payload := "hit"

	audience := r.Emit(Targeted, &payload, "alice")

	assert.Equal(t, Emit{Kind: Targeted, Payload: &payload, Recipient: "alice"}, seen)
	assert.Equal(t, "hit (stamped)", payload, "a hook completes the event through the pointer")
	assert.Nil(t, audience, "an event no hook narrows keeps the default delivery")
}

func TestRunner_NarrowedEventNeedsEveryAudience(t *testing.T) {
	t.Parallel()
	r := New(ecs.NewWorld())
	r.Register(Hooks{Name: "team", Emit: func(e *Emit) { e.Narrow(only{"alice": true, "bob": true}) }})
	r.Register(Hooks{Name: "quiet", Emit: func(e *Emit) { e.Narrow(nil) }}) // narrows nothing
	r.Register(Hooks{Name: "nearby", Emit: func(e *Emit) { e.Narrow(only{"bob": true, "carol": true}) }})

	audience := r.Emit(Broadcast, new(int), "")

	require.NotNil(t, audience)
	assert.False(t, audience.Admits("alice", nil), "only the first audience admits alice")
	assert.True(t, audience.Admits("bob", nil), "both audiences admit bob")
	assert.False(t, audience.Admits("carol", nil), "only the second audience admits carol")
	assert.False(t, audience.Admits("dave", nil))

	assert.Nil(t, r.Emit(SystemEvent, new(int), ""), "a system event has no audience to narrow")
}

// A hook that sends an event of its own still reads the event it was called for afterwards.
func TestRunner_EmitHookThatEmitsKeepsItsEvent(t *testing.T) {
	t.Parallel()
	r := New(ecs.NewWorld())
	outer, inner := "outer", "inner"
	var after []any
	r.Register(Hooks{Name: "relay", Emit: func(e *Emit) {
		if e.Payload == &outer {
			assert.Nil(t, r.Emit(Targeted, &inner, "bob"))
			e.Narrow(only{"alice": true})
		}
		after = append(after, e.Payload, e.Recipient)
	}})

	audience := r.Emit(Targeted, &outer, "alice")

	assert.Equal(t, []any{&inner, "bob", &outer, "alice"}, after)
	require.NotNil(t, audience, "the outer event keeps the audience its hook gave it")
	assert.True(t, audience.Admits("alice", nil))
}

func TestRunner_HookPanicNamesTheHook(t *testing.T) {
	t.Parallel()
	errBroken := errors.New("broken")
	hooks := map[string]func(fn func()) Hooks{
		"Begin":  func(fn func()) Hooks { return Hooks{Begin: fn} },
		"Admit":  func(fn func()) Hooks { return Hooks{Admit: func(command.Command) Verdict { fn(); return Verdict{} }} },
		"Emit":   func(fn func()) Hooks { return Hooks{Emit: func(*Emit) { fn() }} },
		"Commit": func(fn func()) Hooks { return Hooks{Commit: fn} },
		"Settle": func(fn func()) Hooks { return Hooks{Settle: fn} },
	}
	for hook, build := range hooks {
		t.Run(hook, func(t *testing.T) {
			t.Parallel()
			world := ecs.NewWorld()
			r := New(world)
			h := build(func() { panic(errBroken) })
			h.Name = "physics"
			r.Register(h)

			err := recovered(t, func() { runAll(r) })

			var panicked *PanicError
			require.ErrorAs(t, err, &panicked)
			assert.Equal(t, "physics's "+hook+" hook", panicked.Hook)
			require.ErrorIs(t, err, errBroken, "the hook's own error stays reachable")
			require.EqualError(t, err, "cardinal: physics's "+hook+" hook panicked: broken")
			assert.NotPanics(t, func() { world.Create() }, "the world takes writes again after the panic")
		})
	}
}

func TestRunner_HooksCannotWriteToTheWorld(t *testing.T) {
	t.Parallel()
	world := ecs.NewWorld()
	r := New(world)
	r.Register(Hooks{Name: "cheat", Commit: func() { world.Create() }})

	err := recovered(t, func() { r.Commit() })

	var panicked *PanicError
	require.ErrorAs(t, err, &panicked)
	assert.Equal(t, "cardinal: cheat's Commit hook must not change the world, and it called Create", panicked.Value)
	assert.Empty(t, world.LiveEntityIDs(), "the refused write changed nothing")
	assert.NotPanics(t, func() { world.Create() }, "code outside a hook still writes")
}

func TestRunner_NilRunnerHasNoHooks(t *testing.T) {
	t.Parallel()
	var r *Runner

	assert.NotPanics(t, func() { runAll(r) })
	assert.Equal(t, Verdict{}, r.Admit(command.Command{Name: "move"}))
	assert.Nil(t, r.Emit(Broadcast, new(int), ""))
}

func TestRunner_RegisterNeedsAName(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { New(ecs.NewWorld()).Register(Hooks{Commit: func() {}}) })
}

// runAll runs every hook once.
func runAll(r *Runner) {
	r.Begin()
	r.Admit(command.Command{Name: "move"})
	r.Emit(Broadcast, new(int), "")
	r.Commit()
	r.Settle()
}

// recovered runs fn and returns the error it panicked with.
func recovered(t *testing.T, fn func()) (err error) {
	t.Helper()
	defer func() {
		value := recover()
		require.NotNil(t, value, "expected a panic")
		var ok bool
		err, ok = value.(error)
		require.True(t, ok, "panicked with %T, want an error", value)
	}()
	fn()
	return nil
}

// BenchmarkRunner is what the runner adds to one hook call: the write guard, the panic naming,
// and the call itself. The hooks do nothing.
func BenchmarkRunner(b *testing.B) {
	r := New(ecs.NewWorld())
	r.Register(Hooks{
		Name:   "idle",
		Begin:  func() {},
		Admit:  func(command.Command) Verdict { return Verdict{} },
		Emit:   func(*Emit) {},
		Commit: func() {},
		Settle: func() {},
	})
	cmd := command.Command{Name: "move", Sender: command.PlayerSender("alice")}
	payload := new(int)
	b.Run("Commit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			r.Commit()
		}
	})
	b.Run("Admit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			r.Admit(cmd)
		}
	})
	b.Run("Emit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			r.Emit(Broadcast, payload, "")
		}
	})
}
