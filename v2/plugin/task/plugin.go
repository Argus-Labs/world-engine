package task

import (
	"time"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/v2/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/v2/types"
	"pkg.world.dev/world-engine/cardinal/v2/world"
)

// -----------------------------------------------------------------------------
// Public API accessible via task.<function_name>
// -----------------------------------------------------------------------------

// RegisterTask registers a Task definition with the World. A Task definition is a special type of component that
// has a Handle(WorldContext) method that is called when the task is triggered. The Handle method is responsible
// for executing the task and returning an error if any occurred.
func RegisterTask[T Task](w *world.World) error {
	if err := world.RegisterComponent[T](w); err != nil {
		return eris.Wrap(err, "failed to register timestamp task component")
	}

	if err := world.RegisterSystems(w, taskSystem[T]); err != nil {
		return eris.Wrap(err, "failed to register timestamp task system")
	}

	return nil
}

func ScheduleTickTask(w world.WorldContext, tickDelay int64, task Task) error {
	triggerAtTick := w.CurrentTick() + tickDelay
	return createTickTask(w, triggerAtTick, task)
}

func ScheduleTimeTask(w world.WorldContext, duration time.Duration, task Task) error {
	if duration.Milliseconds() < 0 {
		return eris.New("duration value must be positive")
	}

	triggerAtTimestamp := w.Timestamp() + duration.Milliseconds()
	return createTimestampTask(w, triggerAtTimestamp, task)
}

// -----------------------------------------------------------------------------
// Components
// -----------------------------------------------------------------------------

// Task is a user-facing special component interface that is used to define a task that can be scheduled to be executed.
// It implements the types.Component interface along with a Handle method that is called when the task is triggered.
// This method is not to be confused with taskMetadata, which is an internal component type used to store the trigger
// condition for a task.
type Task interface {
	types.Component
	Handle(world.WorldContext) error
}

// taskMetadata is an internal component that is used to store the trigger condition for a task.
// It implements the types.Component interface along with an isTriggered method that returns true if the task
// should be triggered based on the current tick or timestamp.
type taskMetadata struct {
	TriggerAtTick      *int64
	TriggerAtTimestamp *int64
}

func (taskMetadata) Name() string {
	return "taskMetadata"
}

// Task will be triggered when the current tick is greater than designated trigger tick OR when the current timestamp
// is greater than designated trigger timestamp. A task can only have one trigger condition, either tick or timestamp.
// The task should have been trigger at exactly the designated trigger tick, but we make it >= to be safe.
func (t taskMetadata) isTriggered(tick int64, timestamp int64) bool {
	if t.TriggerAtTick != nil {
		return tick >= *t.TriggerAtTick
	}
	return timestamp >= *t.TriggerAtTimestamp
}

// -----------------------------------------------------------------------------
// Systems
// -----------------------------------------------------------------------------

// taskSystem is a system that is registered when RegisterTask is called. It is responsible for iterating through all
// entities with the Task type T and executing the task if the trigger condition is met.
func taskSystem[T Task](wCtx world.WorldContext) error {
	var t T
	var internalErr error
	err := wCtx.Search(filter.Contains(t, filter.Component[taskMetadata]())).Each(
		func(id types.EntityID) bool {
			taskMetadata, err := world.GetComponent[taskMetadata](wCtx, id)
			if err != nil {
				internalErr = err
				return false
			}

			if taskMetadata.isTriggered(wCtx.CurrentTick(), wCtx.Timestamp()) {
				task, err := world.GetComponent[T](wCtx, id)
				if err != nil {
					internalErr = err
					return false
				}

				if err = (*task).Handle(wCtx); err != nil {
					internalErr = err
					return false
				}

				if err = world.Remove(wCtx, id); err != nil {
					internalErr = err
					return false
				}
			}
			return true
		},
	)
	if internalErr != nil {
		return eris.Wrap(internalErr, "encountered an error while executing a task")
	}
	if err != nil {
		return eris.Wrap(err, "encountered an error while iterating over tasks")
	}

	return nil
}

// -----------------------------------------------------------------------------
// Internal functions used by WorldContext to schedule tasks
// -----------------------------------------------------------------------------

// createTickTask creates a task entity that will be executed by taskSystem at the designated tick.
func createTickTask(wCtx world.WorldContext, tick int64, task Task) error {
	_, err := world.Create(wCtx, task, taskMetadata{TriggerAtTick: &tick})
	if err != nil {
		return eris.Wrap(err, "failed to create tick task entity")
	}
	return nil
}

// createTimestampTask creates a task entity that will be executed by taskSystem at the designated timestamp.
func createTimestampTask(wCtx world.WorldContext, timestamp int64, task Task) error {
	_, err := world.Create(wCtx, task, taskMetadata{TriggerAtTimestamp: &timestamp})
	if err != nil {
		return eris.Wrap(err, "failed to create timestamp task entity")
	}
	return nil
}

// -----------------------------------------------------------------------------
// Plugin Definition
// -----------------------------------------------------------------------------

// plugin defines a plugin that handles task scheduling and execution.
type plugin struct{}

var _ world.Plugin = (*plugin)(nil)

func NewPlugin() *plugin {
	return &plugin{}
}

func (*plugin) Register(w *world.World) error {
	return world.RegisterComponent[taskMetadata](w)
}
