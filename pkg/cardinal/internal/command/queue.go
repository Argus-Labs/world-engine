package command

import (
	"sync"

	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
)

// Queue defines the interface for command queuing operations.
// It provides methods to enqueue commands and drain all queued commands.
type Queue interface {
	enqueue(*iscv1.Command) error
	drain(target *[]Command)
}

var _ Queue = &queue[CommandPayload]{}

// TODO: figure out whether to make this configurable.
// initialQueueCapacity is the starting capacity of queue.
const initialQueueCapacity = 1024

// queue is a generic unbounded queue for handling commands of a specific type.
// It implements the Queue interface and provides type-safe command processing.
type queue[T CommandPayload] struct {
	commands []Command
	mu       sync.Mutex
}

// NewQueue creates a new command queue with an initial buffer capacity.
func NewQueue[T CommandPayload]() *queue[T] {
	return &queue[T]{
		commands: make([]Command, 0, initialQueueCapacity),
	}
}

// enqueue validates and adds a command to the queue. It performs type checking to ensure the
// command matches the expected type T, unmarshals the command payload, and appends it to the queue.
// Returns an error if validation fails or marshaling/unmarshaling operations fail.
func (q *queue[T]) enqueue(command *iscv1.Command) error {
	var zero T

	if command.GetName() != zero.Name() {
		return eris.Errorf("mismatched command name, expected %s, actual %s", zero.Name(), command.GetName())
	}

	jsonBytes, err := command.GetPayload().MarshalJSON()
	if err != nil {
		return eris.Wrap(err, "failed to marshal command payload to json")
	}

	if err := json.Unmarshal(jsonBytes, &zero); err != nil {
		return eris.Wrap(err, "failed to unmarshal to command")
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

// drain returns all queued commands to the target slice and resets the queue.
func (q *queue[T]) drain(target *[]Command) {
	q.mu.Lock()
	defer q.mu.Unlock()

	*target = append(*target, q.commands...)
	q.commands = q.commands[:0]
}
