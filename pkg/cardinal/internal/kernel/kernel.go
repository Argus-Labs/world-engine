// Package kernel runs the hooks Cardinal calls at fixed points of a tick. A hook reads the world
// and decides what the tick admits, sends and publishes; it never writes to the world.
//
// Hooks run on the tick goroutine, in the order their registrants were registered.
package kernel

import (
	"fmt"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
)

// Hooks is what one registrant runs at each point of a tick. A nil hook is skipped.
type Hooks struct {
	Name string // Who registered them. A hook's panic and a refused write name it.

	Begin  func()                        // Start of the tick, before any command is judged.
	Admit  func(command.Command) Verdict // Each drained command, before any system reads it.
	Emit   func(*Emit)                   // Each client event or system event, before it is queued.
	Commit func()                        // After the last system, before the tick's events are dispatched.
	Settle func()                        // After the tick's events are dispatched.
}

// Outcome is what becomes of a command.
type Outcome uint8

const (
	Admitted Outcome = iota // It reaches its handlers.
	Dropped                 // It is removed, and its sender is told nothing.
	Rejected                // It is removed, and its sender is told why.
)

// Verdict is a hook's ruling on one command. The zero Verdict admits it.
type Verdict struct {
	Outcome   Outcome
	Reason    string // Why a Rejected command was removed.
	CommandID uint64 // The ID the rejection names; 0 if the command had none.
}

// EmitKind is what a system is sending.
type EmitKind uint8

const (
	Broadcast   EmitKind = iota // A client event for every subscribed stream.
	Targeted                    // A client event for one player.
	SystemEvent                 // An event for systems later in the tick. It has no audience.
)

// Emit is one event on its way to the queue. It is valid only during the hook's call.
type Emit struct {
	Kind      EmitKind
	Payload   any    // Points at the event, so a hook can complete it before it is queued.
	Recipient string // The player a Targeted event is for.

	audience event.Audience
}

// Narrow limits who receives a client event. An audience can only remove recipients: when hooks
// narrow one event more than once, a recipient must be admitted by every audience.
func (e *Emit) Narrow(audience event.Audience) {
	switch {
	case audience == nil || e.Kind == SystemEvent:
	case e.audience == nil:
		e.audience = audience
	default:
		e.audience = both{e.audience, audience}
	}
}

// both admits a recipient that each of its two audiences admits.
type both struct{ first, second event.Audience }

func (b both) Admits(player string, stream any) bool {
	return b.first.Admits(player, stream) && b.second.Admits(player, stream)
}

// PanicError is what a hook's panic becomes. It stops the tick like any other panic, and names the
// hook.
type PanicError struct {
	Hook  string // The registrant and the hook, such as "sync's Commit hook".
	Value any    // What the hook panicked with.
}

func (p *PanicError) Error() string { return fmt.Sprintf("cardinal: %s panicked: %v", p.Hook, p.Value) }

// Unwrap returns the hook's panic value if it was an error.
func (p *PanicError) Unwrap() error {
	err, _ := p.Value.(error)
	return err
}

// registrant is one registration: its hooks, and the name each one runs under.
type registrant struct {
	Hooks

	begin, admit, emit, commit, settle string
}

// Runner calls the registered hooks. A nil Runner has none, and every method is safe on it.
type Runner struct {
	world       *ecs.World
	registrants []registrant
	emit        Emit // The event Emit is running hooks for.
}

// New returns a runner whose hooks may read world and may not write to it.
func New(world *ecs.World) *Runner {
	return &Runner{world: world}
}

// Register adds hooks after every registration before it. Register before the first tick.
func (r *Runner) Register(hooks Hooks) {
	if hooks.Name == "" {
		panic("cardinal: hooks need the name of who registers them")
	}
	label := func(hook string) string { return hooks.Name + "'s " + hook + " hook" }
	r.registrants = append(r.registrants, registrant{
		Hooks: hooks,
		begin: label("Begin"), admit: label("Admit"), emit: label("Emit"),
		commit: label("Commit"), settle: label("Settle"),
	})
}

// Begin runs the Begin hooks.
func (r *Runner) Begin() {
	if r == nil {
		return
	}
	for i := range r.registrants {
		if g := &r.registrants[i]; g.Begin != nil {
			r.run(g.begin, g.Begin)
		}
	}
}

// Admit asks the Admit hooks about one command and returns the first verdict that does not admit
// it. Hooks registered after that one are not asked.
func (r *Runner) Admit(cmd command.Command) Verdict {
	if r == nil {
		return Verdict{}
	}
	for i := range r.registrants {
		g := &r.registrants[i]
		if g.Admit == nil {
			continue
		}
		if verdict := r.admit(g, cmd); verdict.Outcome != Admitted {
			return verdict
		}
	}
	return Verdict{}
}

// Emit runs the Emit hooks on one event and returns who may receive it; nil is the default
// delivery. payload points at the event.
func (r *Runner) Emit(kind EmitKind, payload any, recipient string) event.Audience {
	if r == nil {
		return nil
	}
	// A hook holds a pointer to r.emit for the length of its call. Putting the outer event back
	// keeps that true when a hook sends an event of its own.
	outer := r.emit
	r.emit = Emit{Kind: kind, Payload: payload, Recipient: recipient}
	for i := range r.registrants {
		if g := &r.registrants[i]; g.Emit != nil {
			r.emitTo(g)
		}
	}
	audience := r.emit.audience
	r.emit = outer
	return audience
}

// Commit runs the Commit hooks.
func (r *Runner) Commit() {
	if r == nil {
		return
	}
	for i := range r.registrants {
		if g := &r.registrants[i]; g.Commit != nil {
			r.run(g.commit, g.Commit)
		}
	}
}

// Settle runs the Settle hooks.
func (r *Runner) Settle() {
	if r == nil {
		return
	}
	for i := range r.registrants {
		if g := &r.registrants[i]; g.Settle != nil {
			r.run(g.settle, g.Settle)
		}
	}
}

func (r *Runner) run(hook string, fn func()) {
	defer r.leave(hook, r.world.ReadOnly(hook))
	fn()
}

func (r *Runner) admit(g *registrant, cmd command.Command) Verdict {
	defer r.leave(g.admit, r.world.ReadOnly(g.admit))
	return g.Admit(cmd)
}

func (r *Runner) emitTo(g *registrant) {
	defer r.leave(g.emit, r.world.ReadOnly(g.emit))
	g.Emit(&r.emit)
}

// leave ends a hook's call: it makes the world writable again for whoever held it before, and
// turns the hook's panic, if it had one, into a PanicError that names the hook.
func (r *Runner) leave(hook, previous string) {
	r.world.ReadOnly(previous)
	if value := recover(); value != nil {
		if _, named := value.(*PanicError); !named {
			value = &PanicError{Hook: hook, Value: value}
		}
		panic(value)
	}
}
