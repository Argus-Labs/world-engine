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
	getTaskByName(string) (System, bool)
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

// ComponentTask represents a sub-interface of types.Component that is used to define tasks that can be executed in
// the future
// It defines three methods: shouldExecute, getTaskName, and Name (inherited from types.Component).
//
// Methods:
// - shouldExecute: determines if the task should be executed based on the WorldContext provided as a parameter.
// - getTaskName: returns the name of the task.
// - Name: returns the name of the component.
type ComponentTask interface {
	types.Component
	shouldExecute(WorldContext) bool
	getTaskName() string
}

func (t TaskAtTimestamp) Name() string {
	return "TaskAtTimestamp"
}

func (t TaskAtTick) Name() string {
	return "TaskAtTick"
}

func (t TaskAtTimestamp) getTaskName() string {
	return t.TaskName
}

func (t TaskAtTick) getTaskName() string {
	return t.TaskName
}

func (t TaskAtTick) shouldExecute(wCtx WorldContext) bool {
	return wCtx.CurrentTick() >= t.Tick
}

func (t TaskAtTimestamp) shouldExecute(wCtx WorldContext) bool {
	return wCtx.Timestamp() >= t.Timestamp
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

// getTaskByName returns the System associated with the given task name from the storedTasks map.
// It also returns a boolean value to indicate whether the task was found or not.
//
// Parameters:
// - name: a string representing the name of the task to search for.
//
// Returns:
// - sys: a System object mapped to the given name, if found.
// - ok: a boolean value indicating whether the task was found or not.
func (s *futureTaskStorage) getTaskByName(name string) (System, bool) {
	sys, ok := s.storedTasks[name]
	return sys, ok
}

// registerTask adds a task to storage that can be called to execute later.
// This method is required because we cannot just call a delayed task as a closure.
// The task needs to be saved to state, and we cannot retrieve a function/logic from state (redis).
// We can, however, store a function as a registered function in memory and save an associated
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

// removeTasks simply removes the given entity ids in the parameter. It's a helper function intended to
// remove entity ids that represent tasks that are to be removed.
func removeTasks(wCtx WorldContext, tasksIDs ...types.EntityID) error {
	for _, id := range tasksIDs {
		err := Remove(wCtx, id)
		if err != nil {
			return err
		}
	}
	return nil
}

// createSystem is a generic function that creates a system for executing component tasks (ComponentTask).
// It takes a futureTaskManager as a parameter and returns a System function.
// The System function iterates over entities with the specified component type and executes the corresponding task.
// If an error occurs during execution, the System function returns the error.
// After execution, the System function removes the tasks that are queued for removal.
//
// Parameters:
// - s: a futureTaskManager that manages the tasks to be executed.
//
// Returns:
// - System: a function that executes the component tasks.
func createSystem[T ComponentTask](s futureTaskManager) System {
	return func(wCtx WorldContext) error {
		tasksToRemove := make([]types.EntityID, 0)
		var internalErr error
		err := NewSearch().Entity(filter.Exact(filter.Component[T]())).Each(wCtx, func(id types.EntityID) bool {
			var currentTask *T
			currentTask, internalErr = GetComponent[T](wCtx, id)
			if internalErr != nil {
				return false
			}

			if (*currentTask).shouldExecute(wCtx) {
				proc, ok := s.getTaskByName((*currentTask).getTaskName())
				if !ok {
					internalErr = eris.Errorf("no such task with name %s was registered", (*currentTask).getTaskName())
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
		return removeTasks(wCtx, tasksToRemove...)
	}
}

// taskAtTimestampSystem is a system function that is used to handle tasks that are scheduled
// to be executed at a specific timestamp. It calls the createSystem function passing in the
// TaskAtTimestamp component type and the futureTaskStorage instance, and then executes the
// system on the provided WorldContext.
// Parameters:
// - wCtx: the WorldContext in which the system is executed.
// Returns:
// - error: an error object if encountered during system execution, nil otherwise.
func (s *futureTaskStorage) taskAtTimestampSystem(wCtx WorldContext) error {
	return createSystem[TaskAtTimestamp](s)(wCtx)
}

// taskDelayByTicksSystem is a system function that is used to handle tasks that are delayed by a certain number of
// ticks.
// It calls the createSystem function passing in the TaskAtTick component type and the futureTaskStorage instance,
// and then executes the system on the provided WorldContext.
//
// Parameters:
// - wCtx: the WorldContext in which the system is executed.
//
// Returns:
// - error: an error object if encountered during system execution, nil otherwise.
func (s *futureTaskStorage) taskDelayByTicksSystem(wCtx WorldContext) error {
	return createSystem[TaskAtTick](s)(wCtx)
}
