package cardinal

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

type SystemDelayManager interface {
	InitializeSystemDelayer(*World) error
	RegisterTask(string, System) error
	CallTask(WorldContext, string, int) error
	AmountOfTasks(WorldContext) (int, error)
	ClearTasks(WorldContext) error
}

// Task represents a task that can be added to a task queue.
type Task struct {
	Delay    int
	TaskName string
}

func (t Task) Name() string {
	return "taskComponents"
}

// SystemDelayer represents a type that manages a queue of tasks to be executed with a specified Delay.
type SystemDelayer struct {
	storedTasks map[string]System
}

// newSystemDelayer returns a new instance of SystemDelayer.
func newSystemDelayer() *SystemDelayer {
	return &SystemDelayer{
		storedTasks: map[string]System{},
	}
}

func (s *SystemDelayer) InitializeSystemDelayer(w *World) error {
	err := RegisterComponent[Task](w)
	if err != nil {
		return err
	}
	return RegisterSystems(w, s.delayedTaskSystem)
}

// RegisterTask adds a task to storage that can be called to execute later.
// This is needed because we cannot just call a delayed task as a closure
// The task as code needs to be registered.
func (s *SystemDelayer) RegisterTask(name string, system System) error {
	_, ok := s.storedTasks[name]
	if ok {
		return eris.New("duplicated task")
	}
	s.storedTasks[name] = system
	return nil
}

// CallTask adds a task to the task queue with the specified name and Delay.
// The Delay parameter represents the number of seconds to wait before executing the task.
// Each time this method is called, the Delay is decremented by one.
// If the Delay becomes zero, the task is executed.
// If the task name does not exist in the stored tasks, an error is returned.
// The method appends a new task to the tasks slice with the updated Delay and task name.
//
// Parameters:
// - TaskName: a string representing the name of the task to be added.
// - Delay: an integer representing the number of seconds to Delay the task.
//
// Returns:
//   - error: an error object if encountered during task execution or if the task name does not exist,
//     nil otherwise.
func (s *SystemDelayer) CallTask(wCtx WorldContext, taskName string, delay int) error {
	delay--
	if delay < 0 {
		return eris.New("cannot Delay zero seconds.")
	}
	_, ok := s.storedTasks[taskName]
	if !ok {
		return eris.Errorf("task with name: %s not found", taskName)
	}
	currentTask := Task{
		Delay:    delay,
		TaskName: taskName,
	}
	_, err := Create(wCtx, currentTask)
	if err != nil {
		return err
	}
	//err = SetComponent[Task](wCtx, id, &currentTask)
	//if err != nil {
	//	return err
	//}
	//comp, err := GetComponent[Task](wCtx, id)
	//if err != nil {
	//	return err
	//}
	//wCtx.Logger().Debug().Msgf("task Delay: %d", comp.Delay)
	//wCtx.Logger().Debug().Msgf("task name %s", comp.TaskName)
	return err

}

// AmountOfTasks returns the number of tasks in the task queue.
// The task queue length is determined by the length of the slice holding the tasks.
// Returns:
// - int: the number of tasks in the queue.
func (s *SystemDelayer) AmountOfTasks(wCtx WorldContext) (int, error) {
	count, err := NewSearch().Entity(filter.Exact(filter.Component[Task]())).Count(wCtx)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ClearTasks removes all pending tasks.
func (s *SystemDelayer) ClearTasks(wCtx WorldContext) error {
	ids, err := NewSearch().Entity(filter.Exact(filter.Component[Task]())).Collect(wCtx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		err = Remove(wCtx, id)
		if err != nil {
			return err
		}
	}
	return nil
}

// delayedTaskSystem updates the Delay of tasks in the task queue.
// It decrements the Delay of each task by one.
// If the Delay becomes zero, the task is executed using the provided WorldContext.
// After updating the delays and executing the tasks, a new task queue is created
// based on the indexes that were kept during the iteration. This is supposed to be Registered as a system in cardinal.
//
// Parameters:
// - wCtx: a cardinal.WorldContext object representing the context in which the tasks are executed.
//
// Returns:
// - error: an error object if encountered during task execution, nil otherwise.
func (s *SystemDelayer) delayedTaskSystem(wCtx WorldContext) error {
	tasksToRemove := make([]types.EntityID, 0, 0)
	var internalErr error
	err := NewSearch().Entity(filter.Exact(filter.Component[Task]())).Each(wCtx, func(id types.EntityID) bool {
		var currentTask *Task
		currentTask, internalErr = GetComponent[Task](wCtx, id)
		if internalErr != nil {
			return false
		}

		if currentTask.Delay == 0 {
			proc, ok := s.storedTasks[currentTask.TaskName]
			if !ok {
				internalErr = eris.Errorf("no such task with name %s was registered", currentTask.TaskName)
				return false
			}
			// task execution occurs here
			internalErr = proc(wCtx)

			if internalErr != nil {
				return false
			}

			// queue task for removal if executed.
			tasksToRemove = append(tasksToRemove, id)

		} else {
			// task not ready for execution, decrement counter.
			currentTask.Delay--
			internalErr = SetComponent[Task](wCtx, id, currentTask)
			if internalErr != nil {
				return false
			}
		}

		return true
	})
	if internalErr != nil {
		return internalErr
	}
	if err != nil {
		return err
	}

	// remove all tasks that are queued.
	for _, id := range tasksToRemove {
		err = Remove(wCtx, id)
		if err != nil {
			return err
		}
	}

	return nil
}
