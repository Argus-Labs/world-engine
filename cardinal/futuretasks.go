package cardinal

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

type futureTaskManager interface {
	initializeFutureTaskStorage(*World) error
	registerTask(string, System) error
	delayTaskByTicks(WorldContext, string, int) error
	callTaskAtTimestamp(WorldContext, string, uint64) error
	amountOfDelayedTasksByTick(WorldContext) (int, error)
	amountOfTasksAtTimestamp(wCtx WorldContext) (int, error)
	clearTasks(WorldContext) error
}

// futureTaskStorage represents a type that manages a queue of tasks to be executed with a specified Delay.
type futureTaskStorage struct {
	storedTasks map[string]System
}

// DelayedTaskByTick represents a task that can be added to a task queue.
type DelayedTaskByTick struct {
	Delay    int
	TaskName string
}

type TaskAtTimeSamp struct {
	Timestamp uint64
	TaskName  string
}

func (t TaskAtTimeSamp) Name() string {
	return "TaskAtTimeSamp"
}

func (t DelayedTaskByTick) Name() string {
	return "DelayedTaskByTick"
}

// newFutureTaskStorage returns a new instance of futureTaskStorage.
func newFutureTaskStorage() *futureTaskStorage {
	return &futureTaskStorage{
		storedTasks: map[string]System{},
	}
}

func (s *futureTaskStorage) initializeFutureTaskStorage(w *World) error {
	err := RegisterComponent[DelayedTaskByTick](w)
	if err != nil {
		return err
	}
	err = RegisterComponent[TaskAtTimeSamp](w)
	if err != nil {
		return err
	}
	return RegisterSystems(w, s.taskDelayByTicksSystem, s.taskAtTimestampSystem)
}

// registerTask adds a task to storage that can be called to execute later.
// This is needed because we cannot just call a delayed task as a closure
// The task as code needs to be registered.
func (s *futureTaskStorage) registerTask(name string, system System) error {
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
func (s *futureTaskStorage) delayTaskByTicks(wCtx WorldContext, taskName string, delay int) error {
	delay--
	if delay < 0 {
		return eris.New("cannot Delay zero seconds.")
	}
	_, ok := s.storedTasks[taskName]
	if !ok {
		return eris.Errorf("task with name: %s not found", taskName)
	}
	currentTask := DelayedTaskByTick{
		Delay:    delay,
		TaskName: taskName,
	}
	_, err := Create(wCtx, currentTask)
	return err
}

func (s *futureTaskStorage) callTaskAtTimestamp(wCtx WorldContext, taskName string, timestamp uint64) error {
	if timestamp < wCtx.Timestamp() {
		return eris.Errorf(
			"cannot call task at timestamp %d as it is earlier then the current time: %d",
			timestamp, wCtx.Timestamp())
	}
	_, ok := s.storedTasks[taskName]
	if !ok {
		return eris.Errorf("task with name: %s not found", taskName)
	}
	currentTask := TaskAtTimeSamp{
		Timestamp: timestamp,
		TaskName:  taskName,
	}
	_, err := Create(wCtx, currentTask)
	return err
}

// AmountOfTasks returns the number of tasks in the task queue.
// The task queue length is determined by the length of the slice holding the tasks.
// Returns:
// - int: the number of tasks in the queue.
func (s *futureTaskStorage) amountOfDelayedTasksByTick(wCtx WorldContext) (int, error) {
	count, err := NewSearch().Entity(filter.Exact(filter.Component[DelayedTaskByTick]())).Count(wCtx)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *futureTaskStorage) amountOfTasksAtTimestamp(wCtx WorldContext) (int, error) {
	count, err := NewSearch().Entity(filter.Exact(filter.Component[TaskAtTimeSamp]())).Count(wCtx)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ClearTasks removes all pending tasks.
func (s *futureTaskStorage) clearTasks(wCtx WorldContext) error {
	ids, err := NewSearch().Entity(filter.Exact(filter.Component[DelayedTaskByTick]())).Collect(wCtx)
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

// taskAtTimestampSystem processes tasks scheduled to be executed at a specific timestamp.
// It retrieves all TaskAtTimeSamp entities from the WorldContext and checks if their
// timestamp is greater than or equal to the current timestamp. If so, it retrieves the corresponding
// stored task and executes it using the WorldContext. After execution, the TaskAtTimeSamp
// entity is marked for removal. Finally, all marked entities are removed from the WorldContext.
//
// Parameters:
// - wCtx: a WorldContext object representing the context in which the tasks are executed.
//
// Returns:
// - error: an error object if encountered during task execution or removal, nil otherwise.
func (s *futureTaskStorage) taskAtTimestampSystem(wCtx WorldContext) error {
	tasksToRemove := make([]types.EntityID, 0)
	var internalErr error
	err := NewSearch().Entity(filter.Exact(filter.Component[TaskAtTimeSamp]())).Each(wCtx, func(id types.EntityID) bool {
		var currentTask *TaskAtTimeSamp
		currentTask, internalErr = GetComponent[TaskAtTimeSamp](wCtx, id)
		if internalErr != nil {
			return false
		}

		if currentTask.Timestamp <= wCtx.Timestamp() {
			proc, ok := s.storedTasks[currentTask.TaskName]
			if !ok {
				internalErr = eris.Errorf("no such task with name %s was registered", currentTask.TaskName)
				return false
			}
			internalErr = proc(wCtx)
			if internalErr != nil {
				return false
			}
			tasksToRemove = append(tasksToRemove, id)
		}
		return true
	})
	if internalErr != nil {
		return internalErr
	}
	if err != nil {
		return err
	}

	// remove all tasks that are queued for removal.
	for _, id := range tasksToRemove {
		err = Remove(wCtx, id)
		if err != nil {
			return err
		}
	}
	return nil
}

// taskDelayByTicksSystem updates the Delay of tasks in the task queue.
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
func (s *futureTaskStorage) taskDelayByTicksSystem(wCtx WorldContext) error {
	tasksToRemove := make([]types.EntityID, 0)
	var internalErr error
	err := NewSearch().Entity(
		filter.Exact(
			filter.Component[DelayedTaskByTick]())).Each(wCtx, func(id types.EntityID) bool {
		var currentTask *DelayedTaskByTick
		currentTask, internalErr = GetComponent[DelayedTaskByTick](wCtx, id)
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
			internalErr = SetComponent[DelayedTaskByTick](wCtx, id, currentTask)
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

	// remove all tasks that are queued for removal.
	for _, id := range tasksToRemove {
		err = Remove(wCtx, id)
		if err != nil {
			return err
		}
	}

	return nil
}
