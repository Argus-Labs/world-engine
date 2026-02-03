package command

import (
	"sync"

	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/shamaton/msgpack/v3"
)

// Queue defines the interface for command queuing operations.
// It provides methods to enqueue commands and drain all queued commands.
type Queue interface {
	Enqueue(*iscv1.Command) error
	Drain(target *[]Command)
	Len() int
}

var _ Queue = &sliceQueue[Payload]{}

// TODO: figure out whether to make this configurable.
// initialQueueCapacity is the starting capacity of queue.
const initialQueueCapacity = 1024

// sliceQueue is a generic unbounded sliceQueue for handling commands of a specific type.
// It implements the Queue interface and provides type-safe command processing.
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

// Enqueue validates and adds a command to the queue. It performs type checking to ensure the
// command matches the expected type T, unmarshals the command payload, and appends it to the queue.
// Returns an error if validation fails or marshaling/unmarshaling operations fail.
func (q *sliceQueue[T]) Enqueue(command *iscv1.Command) error {
	var zero T

	if command.GetName() != zero.Name() {
		return eris.Errorf("mismatched command name, expected %s, actual %s", zero.Name(), command.GetName())
	}

	if err := msgpack.Unmarshal(command.GetPayload(), &zero); err != nil {
		return eris.Wrap(err, "failed to unmarshal command payload")
	}

	q.mu.Lock()
	q.commands = append(q.commands, Command{
		Name:    zero.Name(),
		Address: command.GetAddress(),
		Persona: command.GetPersona().GetId(),
		Payload: zero,
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
