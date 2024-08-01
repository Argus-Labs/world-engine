package cardinal

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/filter"
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

// futureTaskStorage represents a type that stores a series of tasks that can be executed in the future.
type futureTaskStorage struct {
	storedTasks map[string]System
}

// TaskAtTick represents a task that can be delayed by a certain tick amount
type TaskAtTick struct {
	Tick     uint64
	TaskName string
}

// TaskAtTimestamp represents a task to be exectuted at a certain timestamp
type TaskAtTimestamp struct {
	Timestamp uint64
	TaskName  string
}

func (t TaskAtTimestamp) Name() string {
	return "TaskAtTimestamp"
}

func (t TaskAtTick) Name() string {
	return "TaskAtTick"
}

// newFutureTaskStorage returns a new instance of futureTaskStorage.
func newFutureTaskStorage() *futureTaskStorage {
	return &futureTaskStorage{
		storedTasks: map[string]System{},
	}
}

// initializeFutureTaskStorage initializes the future task storage by registering the relevant components
// and systems to the provided World. This method is called during initialization of the storage.
// The function first calls RegisterComponent[TaskAtTick] on the World context.
// If an error occurs, it is returned immediately. Next, it calls RegisterComponent[TaskAtTimestamp].
// If an error occurs, it is returned immediately. Finally, it calls RegisterSystems on the World context,
// passing in the taskDelayByTicksSystem and taskAtTimestampSystem functions as the systems to register.
//
// Parameters:
// - w: a pointer to the World context in which the components and systems are registered.
//
// Returns:
// - error: an error object if encountered during registration, nil otherwise.
func (s *futureTaskStorage) initializeFutureTaskStorage(w *World) error {
	err := RegisterComponent[TaskAtTick](w)
	if err != nil {
		return err
	}
	err = RegisterComponent[TaskAtTimestamp](w)
	if err != nil {
		return err
	}
	return RegisterSystems(w, s.taskDelayByTicksSystem, s.taskAtTimestampSystem)
}

// registerTask adds a task to storage that can be called to execute later.
// This is needed because we cannot just call a delayed task as a closure
// The task needs to be saved to state and we cannot retrieve a function from state
// we can however store a function as a registered function in memory and save an associated
// name of the function to state.
func (s *futureTaskStorage) registerTask(name string, system System) error {
	_, ok := s.storedTasks[name]
	if ok {
		return eris.New("duplicated task")
	}
	s.storedTasks[name] = system
	return nil
}

// delayTaskByTicks delays a task execution by a specified number of ticks.
// It decrements the delay by 1 and checks if the delay is less than 0.
// If the delay is negative, it returns an error. Otherwise, it retrieves the
// stored task with the given taskName from the futureTaskStorage and creates
// a TaskAtTick object with the updated delay and taskName.
// The TaskAtTick object is then created using the Create function with
// the provided WorldContext.
//
// Parameters:
// - wCtx: a WorldContext object representing the context in which the task is delayed.
// - taskName: a string containing the name of the task to be delayed.
// - delay: an integer representing the number of ticks to delay the task execution.
//
// Returns:
// - error: an error object if encountered during the delay or task creation, nil otherwise.
func (s *futureTaskStorage) delayTaskByTicks(wCtx WorldContext, taskName string, delay int) error {
	task, ok := s.storedTasks[taskName]
	if !ok {
		return eris.Errorf("task with name: %s not found", taskName)
	}
	if delay == 0 {
		return task(wCtx)
	} else if delay < 0 {
		return eris.New("cannot Delay less than zero seconds.")
	}

	currentTask := TaskAtTick{
		Tick:     uint64(delay) + wCtx.CurrentTick(),
		TaskName: taskName,
	}
	_, err := Create(wCtx, currentTask)
	return err
}

// callTaskAtTimestamp calls a task at the specified timestamp by creating a TaskAtTimestamp component and
// adding it to the World context. First, the method checks if the specified timestamp is earlier than the
// current time. If so, an error is returned. Then, it checks if a stored task with the specified task name
// exists in the future task storage. If not, an error is returned. Finally, it creates a TaskAtTimestamp
// object with the given timestamp and task name, and adds it to the World context using the Create method.
//
// Parameters:
// - wCtx: a WorldContext representing the current state of the game world.
// - taskName: the name of the task to be called.
// - timestamp: the timestamp at which the task should be called.
//
// Returns:
// - error: an error object if encountered during execution, nil otherwise.
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
	currentTask := TaskAtTimestamp{
		Timestamp: timestamp,
		TaskName:  taskName,
	}
	_, err := Create(wCtx, currentTask)
	return err
}

// amountOfDelayedTasksByTick returns the number of delayed tasks with the `TaskAtTick`
// component in the provided WorldContext. It uses a search query to count the number of entities with the
// `TaskAtTick` component.
//
// Parameters:
// - wCtx: a WorldContext representing the context in which to search for delayed tasks.
//
// Returns:
// - int: the number of delayed tasks with the `TaskAtTick` component.
// - error: an error object if encountered during the count, nil otherwise.
func (s *futureTaskStorage) amountOfDelayedTasksByTick(wCtx WorldContext) (int, error) {
	count, err := NewSearch().Entity(filter.Exact(filter.Component[TaskAtTick]())).Count(wCtx)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// amountOfTasksAtTimestamp returns the number of tasks assigned a timestamp by
// performing a search using the NewSearch function. It filters entities that have
// the TaskAtTimestamp component and counts the number of matching entities. If an
// error occurs during the search, it is returned immediately. The function returns
// the count of tasks and a nil error object if the operation is successful.
// Parameters:
// - wCtx: the WorldContext in which the search is performed.
// Returns:
// - int: the number of tasks assigned a timestamp.
// - error: an error object if encountered during the search, nil otherwise.
func (s *futureTaskStorage) amountOfTasksAtTimestamp(wCtx WorldContext) (int, error) {
	count, err := NewSearch().Entity(filter.Exact(filter.Component[TaskAtTimestamp]())).Count(wCtx)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// clearTasks removes all tasks stored in the futureTaskStorage that have the TaskAtTick and TaskAtTimestamp
// components. It first collects the entity IDs of all tasks with TaskAtTick component, and then collects the
// entity IDs of all tasks with TaskAtTimestamp component. It then iterates over the collected entity IDs and removes
// each task from the WorldContext by calling the Remove function. If an error occurs during the removal process, it
// is returned immediately. After all tasks are removed, nil is returned.
// Parameters:
// - wCtx: a WorldContext object representing the context in which the tasks are stored.
// Returns:
// - error: an error object if encountered during task removal, nil otherwise.
func (s *futureTaskStorage) clearTasks(wCtx WorldContext) error {
	ids1, err := NewSearch().Entity(filter.Exact(filter.Component[TaskAtTick]())).Collect(wCtx)
	if err != nil {
		return err
	}
	ids2, err := NewSearch().Entity(filter.Exact(filter.Component[TaskAtTimestamp]())).Collect(wCtx)
	if err != nil {
		return err
	}
	for _, id := range append(ids1, ids2...) {
		err = Remove(wCtx, id)
		if err != nil {
			return err
		}
	}
	return nil
}

// taskAtTimestampSystem processes tasks scheduled to be executed at a specific timestamp.
// It retrieves all TaskAtTimestamp entities from the WorldContext and checks if their
// timestamp is greater than or equal to the current timestamp. If so, it retrieves the corresponding
// stored task and executes it using the WorldContext. After execution, the TaskAtTimestamp
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
	err := NewSearch().Entity(filter.Exact(filter.Component[TaskAtTimestamp]())).Each(wCtx, func(id types.EntityID) bool {
		var currentTask *TaskAtTimestamp
		currentTask, internalErr = GetComponent[TaskAtTimestamp](wCtx, id)
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

// taskDelayByTicksSystem is a system function that processes delayed tasks by ticking.
// It takes a WorldContext as a parameter and returns an error object if encountered during processing, nil otherwise.
// The function starts by creating an empty slice to store the IDs of tasks to be removed.
// It then initializes the internalErr variable to handle internal errors.
// The function performs a search for entities with the TaskAtTick component using the NewSearch function.
// For each entity found, it retrieves the TaskAtTick component and assigns it to the currentTask variable.
// If there is an error retrieving the component, the function returns false and stops iterating.
// If the currentTask.Tick is zero, it checks if the corresponding task procedure is registered in s.storedTasks.
// If it is not registered, an error is returned indicating that the task does not exist.
// Otherwise, the task procedure is executed and any internal error encountered is assigned to internalErr.
// If there is an internal error, the function returns false and stops iterating.
// The task is then appended to the tasksToRemove slice.
// If the currentTask.Tick is non-zero, the counter is decremented and the updated component is set using SetComponent.
// If there is an error setting the component, the function returns false and stops iterating.
// After all entities have been processed, the function checks for any internal error or search error encountered.
// If there is an internal error, it is returned.
// If there is a search error, it is returned.
// Finally, the function iterates through the tasksToRemove slice and removes each task from the World using the Remove function.
// If there is an error removing a task, it is returned.
// If no errors occur, the function returns nil.
// Parameters:
// - wCtx: a WorldContext object representing the context in which the system is executed.
// Returns:
// - error: an error object if encountered during processing, nil otherwise.
func (s *futureTaskStorage) taskDelayByTicksSystem(wCtx WorldContext) error {
	tasksToRemove := make([]types.EntityID, 0)
	var internalErr error
	err := NewSearch().Entity(
		filter.Exact(
			filter.Component[TaskAtTick]())).Each(wCtx, func(id types.EntityID) bool {
		var currentTask *TaskAtTick
		currentTask, internalErr = GetComponent[TaskAtTick](wCtx, id)
		if internalErr != nil {
			return false
		}

		if currentTask.Tick == wCtx.CurrentTick() {
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
