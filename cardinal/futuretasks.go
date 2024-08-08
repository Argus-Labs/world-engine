package cardinal

import (
	"errors"

	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

// Task is an interface that represents a task to be executed in a system.
//
// Handle executes the task in the specified WorldContext and returns an error if any occurred.
// It also implements the types.Component interface, which provides the Name method to get the name of the task.
type Task interface {
	Handle(WorldContext) error
	types.Component
}

// TickTask is a type that represents a component for scheduling tasks based on ticks.
// It implements the types.Component interface. The Tick field represents the tick at
// which the task should be executed.
//
// Example usage:
//
//	tickStore := TickTask{Tick: wCtx.CurrentTick() + delay}
//	_, err := Create(wCtx, task, tickStore)
type TickTask struct {
	types.Component
	Tick uint64
}

// TimestampTask represents a component that stores a timestamp value. It implements the types.Component interface.
//
// Example usage:
//
//	timestampstore := TimestampTask{Timestamp: timestamp}
//	_, err := Create(wCtx, task, timestampstore)
type TimestampTask struct {
	types.Component
	Timestamp uint64
}

// FutureTaskPlugin is a type that represents a plugin for handling future tasks in a system.
type FutureTaskPlugin struct{}

func (TickTask) Name() string {
	return "TickTask"
}

func (TimestampTask) Name() string {
	return "TimestampTask"
}

func (FutureTaskPlugin) Register(w *World) error {
	return errors.Join(
		RegisterComponent[TickTask](w),
		RegisterComponent[TimestampTask](w),
	)
}

func newFutureTaskPlugin() *FutureTaskPlugin {
	return &FutureTaskPlugin{}
}

// RegisterTimestampTask registers a timestamp task in the given World.
// It first calls RegisterComponent[T](w) to register the task component.
// If it returns an error, the error is returned.
// Otherwise, it calls RegisterSystems(w, futureTaskSystemTimestamp[T])
// to register the system. The error returned by RegisterSystems is returned.
func RegisterTimestampTask[T Task](w *World) error {
	err := RegisterComponent[T](w)
	if err != nil {
		return err
	}
	return RegisterSystems(w, futureTaskSystemTimestamp[T])
}

// RegisterTickTask registers a tick task in the specified World. It first calls RegisterComponent[T]
// to register the task's component, then calls RegisterSystems to register the tick task system
// futureTaskSystemTick[T]. Returns an error if any occurred.
func RegisterTickTask[T Task](w *World) error {
	err := RegisterComponent[T](w)
	if err != nil {
		return err
	}
	return RegisterSystems(w, futureTaskSystemTick[T])
}

// delayTaskByTicks schedules a task to be executed after a certain number of ticks.
// It creates a TickTask with the specified delay and calls the Create function to create the task entity.
// The TickTask represents a component for scheduling tasks based on ticks.
// The delay parameter is added to the current tick to determine the tick at which the task should be executed.
// The WorldContext parameter represents the context of the world in which the task is executed.
// The Task parameter represents the task to be executed.
// It returns an error if any occurred during the task creation.

func delayTaskByTicks(wCtx WorldContext, task Task, delay uint64) error {
	tickStore := TickTask{Tick: wCtx.CurrentTick() + delay}
	_, err := Create(wCtx, task, tickStore)
	return err
}

// executeTaskAtTime executes a task at a specified timestamp in the given WorldContext.
// It creates a TimestampTask component with the provided timestamp and calls the Create function to create
// the task entity. If an error occurs during the creation of the entity, it is returned.
func executeTaskAtTime(wCtx WorldContext, task Task, timestamp uint64) error {
	timestampstore := TimestampTask{Timestamp: timestamp}
	_, err := Create(wCtx, task, timestampstore)
	return err
}

// futureTaskSystemTick is a function that executes tick tasks in the specified WorldContext.
// It iterates over all entities that have a TickTask component and checks if the current tick is
// greater than or equal to the tick specified in the TickTask component. If it is, it retrieves the
// task component and calls the Handle method on it. If there is an error during the execution of the task,
// it returns the error and stops iterating. After executing the task, it saves the task component
// back to the entity. It returns any internal error that occurred during the iteration or execution
// of the tasks.
func futureTaskSystemTick[T Task](wCtx WorldContext) error {
	var internalErr error
	err := NewSearch().Entity(filter.Contains(filter.Component[TickTask]())).Each(wCtx, func(id types.EntityID) bool {
		tickstore, err := GetComponent[TickTask](wCtx, id)
		if err != nil {
			internalErr = err
			return false
		}
		if wCtx.CurrentTick() >= tickstore.Tick {
			taskObj, err := GetComponent[T](wCtx, id)
			if err != nil {
				internalErr = err
				return false
			}
			err = (*taskObj).Handle(wCtx)
			if err != nil {
				internalErr = err
				return false
			}
			err = Remove(wCtx, id)
			if err != nil {
				internalErr = err
				return false
			}
		}
		return true
	})
	if internalErr != nil {
		return internalErr
	}
	return err
}

// futureTaskSystemTimestamp is a function that executes the tasks of type T whose timestamps
// have been reached in the system. It retrieves all the entities that have a component of
// type TimestampTask and compares their timestamps with the current system timestamp obtained
// from the WorldContext. If the system timestamp is greater than or equal to the entity's timestamp,
// the task of type T associated with the entity is executed using the Handle method. If the execution
// is successful, the state of the task object may have changed, so it is saved back to the entity using
// the SetComponent method. The function returns an error if any occurred during the execution.
// The function takes a WorldContext as an argument, which provides the necessary context for
// executing the tasks.
//
// The function uses the NewSearch function to create a search object. It then filters the entities
// using the Contains method of filter package to find entities that have the TimestampTask component.
// In each iteration of the Each method, it retrieves the TimestampTask component and compares its
// timestamp with the system timestamp. If the condition is true, it retrieves the task object of type T
// associated with the entity using the GetComponent method. It then executes the task using the Handle
// method and checks for any error. If successful, it saves the task object back to the entity using
// the SetComponent method. The function returns the error occurred during execution, if any.
//
// The function does not return anything.
func futureTaskSystemTimestamp[T Task](wCtx WorldContext) error {
	var internalErr error
	err := NewSearch().Entity(filter.Contains(filter.Component[TimestampTask]())).Each(wCtx, func(id types.EntityID) bool {
		timestampStore, err := GetComponent[TimestampTask](wCtx, id)
		if err != nil {
			internalErr = err
			return false
		}
		if wCtx.Timestamp() >= timestampStore.Timestamp {
			taskObj, err := GetComponent[T](wCtx, id)
			if err != nil {
				internalErr = err
				return false
			}
			err = (*taskObj).Handle(wCtx)
			if err != nil {
				internalErr = err
				return false
			}

			// task only executes once.
			err = Remove(wCtx, id)
			if err != nil {
				internalErr = err
				return false
			}
		}
		return true
	})
	if internalErr != nil {
		return internalErr
	}
	return err
}
