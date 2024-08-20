package cardinal

import (
	"fmt"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

// -----------------------------------------------------------------------------
// Public API accessible via cardinal.<function_name>
// -----------------------------------------------------------------------------

// RegisterTask registers a Task definition with the World. A Task definition is a special type of component that
// has a Handle(WorldContext) method that is called when the task is triggered. The Handle method is responsible
// for executing the task and returning an error if any occurred.
func RegisterTask[T Task](w *World) error {
	if err := RegisterComponent[T](w); err != nil {
		return eris.Wrap(err, "failed to register timestamp task component")
	}

	var t T
	systemName := fmt.Sprintf("task_system_%s", t.Name())
	if err := w.SystemManager.registerSystem(false, systemName, taskSystem[T]); err != nil {
		return eris.Wrap(err, "failed to register timestamp task system")
	}

	return nil
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
	Handle(WorldContext) error
}

// taskMetadata is an internal component that is used to store the trigger condition for a task.
// It implements the types.Component interface along with an isTriggered method that returns true if the task
// should be triggered based on the current tick or timestamp.
type taskMetadata struct {
	TriggerAtTick      *uint64
	TriggerAtTimestamp *uint64
}

func (taskMetadata) Name() string {
	return "taskMetadata"
}

// Task will be triggered when the current tick is greater than designated trigger tick OR when the current timestamp
// is greater than designated trigger timestamp. A task can only have one trigger condition, either tick or timestamp.
// The task should have been trigger at exactly the designated trigger tick, but we make it >= to be safe.
func (t taskMetadata) isTriggered(tick uint64, timestamp uint64) bool {
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
func taskSystem[T Task](wCtx WorldContext) error {
	var internalErr error
	err := NewSearch().Entity(filter.Contains(filter.Component[T](), filter.Component[taskMetadata]())).Each(wCtx,
		func(id types.EntityID) bool {
			taskMetadata, err := GetComponent[taskMetadata](wCtx, id)
			if err != nil {
				internalErr = err
				return false
			}

			if taskMetadata.isTriggered(wCtx.CurrentTick(), wCtx.Timestamp()) {
				task, err := GetComponent[T](wCtx, id)
				if err != nil {
					internalErr = err
					return false
				}

				if err = (*task).Handle(wCtx); err != nil {
					internalErr = err
					return false
				}

				if err = Remove(wCtx, id); err != nil {
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
func createTickTask(wCtx WorldContext, tick uint64, task Task) error {
	_, err := Create(wCtx, task, taskMetadata{TriggerAtTick: &tick})
	if err != nil {
		return eris.Wrap(err, "failed to create tick task entity")
	}
	return nil
}

// createTimestampTask creates a task entity that will be executed by taskSystem at the designated timestamp.
func createTimestampTask(wCtx WorldContext, timestamp uint64, task Task) error {
	_, err := Create(wCtx, task, taskMetadata{TriggerAtTimestamp: &timestamp})
	if err != nil {
		return eris.Wrap(err, "failed to create timestamp task entity")
	}
	return nil
}

// -----------------------------------------------------------------------------
// Plugin Definition
// -----------------------------------------------------------------------------

var _ Plugin = (*TaskPlugin)(nil)

// TaskPlugin defines a plugin that handles task scheduling and execution.
type TaskPlugin struct{}

func newFutureTaskPlugin() *TaskPlugin {
	return &TaskPlugin{}
}

func (*TaskPlugin) Register(w *World) error {
	err := RegisterComponent[taskMetadata](w)
	if err != nil {
		return eris.Wrap(err, "failed to register task entry component")
	}
	return nil
}
