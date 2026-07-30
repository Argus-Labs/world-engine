package command

import (
	"sync"

	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
)

// Queue defines the interface for command queuing operations.
// It provides methods to enqueue commands and drain all queued commands.
type Queue interface {
	Validate(*iscv1.Command) error
	Enqueue(*iscv1.Command) error
	Drain(target *[]Command)
	Len() int
	Zero() Payload
}

// TODO: figure out whether to make this configurable.
// initialQueueCapacity is the starting capacity of queue.
const initialQueueCapacity = 1024

// sliceQueue is a generic unbounded queue for a single command type T. It decodes an incoming payload
// by calling the command type's own UnmarshalWire (a value-receiver decode factory) under the static
// type — there is no codec registry to look up. The decoded value is stored directly as a Payload.
type sliceQueue[T Payload] struct {
	commands []Command
	mu       sync.Mutex
}

// NewQueue creates a new command queue with an initial buffer capacity.
func NewQueue[T Payload]() Queue {
	return &sliceQueue[T]{
		commands: make([]Command, 0, initialQueueCapacity),
	}
}

func (q *sliceQueue[T]) decode(cmd *iscv1.Command) (T, error) {
	var zero T

	if cmd.GetName() != zero.Name() {
		return zero, eris.Errorf("mismatched command name, expected %s, actual %s", zero.Name(), cmd.GetName())
	}

	decoded, err := zero.UnmarshalWire(cmd.GetPayload())
	if err != nil {
		return zero, eris.Wrapf(err, "failed to decode command payload for %q", zero.Name())
	}

	concrete, ok := decoded.(T)
	if !ok {
		return zero, eris.Errorf("decoded command %q has unexpected type %T", zero.Name(), decoded)
	}
	return concrete, nil
}

// Validate checks the command's name and wire payload without mutating the queue.
func (q *sliceQueue[T]) Validate(cmd *iscv1.Command) error {
	_, err := q.decode(cmd)
	return err
}

// Enqueue validates, decodes, and adds a command to the queue.
func (q *sliceQueue[T]) Enqueue(cmd *iscv1.Command) error {
	payload, err := q.decode(cmd)
	if err != nil {
		return err
	}

	q.mu.Lock()
	q.commands = append(q.commands, Command{
		Name:    cmd.GetName(),
		Address: cmd.GetAddress(),
		Persona: cmd.GetPersona().GetId(),
		Payload: payload,
	})
	q.mu.Unlock()
	return nil
}

// Drain returns all queued commands to the target slice and resets the queue.
func (q *sliceQueue[T]) Drain(target *[]Command) {
	q.mu.Lock()
	defer q.mu.Unlock()

	*target = append(*target, q.commands...)
	q.commands = q.commands[:0]
}

// Len returns the length of the queue.
func (q *sliceQueue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.commands)
}

// Zero returns a zero-value payload for the queue's command type.
func (q *sliceQueue[T]) Zero() Payload {
	var zero T
	return zero
}
