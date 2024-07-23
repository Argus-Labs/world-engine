package cardinal

import (
	"github.com/rotisserie/eris"
)

type SystemDelayManager interface {
	AddTask(func(ctx WorldContext) error, int) error
	AmountOfTasks() int
	ClearTasks()
	Register(*World) error
}

// task represents a task that can be added to a task queue.
type task struct {
	delay int
	task  func(ctx WorldContext) error
}

// New returns a new instance of SystemDelayer.
func newSystemDelayer() *SystemDelayer {
	return &SystemDelayer{
		tasks: make([]*task, 0),
	}
}

// SystemDelayer represents a type that manages a queue of tasks to be executed with a specified delay.
type SystemDelayer struct {
	tasks []*task
}

// AddTask adds a new task to the task queue with the given system and delay.
// The delay is decremented by one, and if it becomes less than zero, an error is returned.
// The task is appended to the tasks slice with the specified delay and system.
// Parameters:
// - system: a cardinal.System object representing the system to be executed as a task.
// - delay: an unsigned integer representing the delay in seconds before the task is executed.
// Returns:
// - error: an error object if the delay is less than zero, nil otherwise.
func (s *SystemDelayer) AddTask(taskFunc func(ctx WorldContext) error, delay int) error {
	delay--
	if delay < 0 {
		return eris.New("cannot delay zero seconds.")
	}
	s.tasks = append(s.tasks, &task{
		delay: delay,
		task:  taskFunc,
	})
	return nil
}

// AmountOfTasks returns the number of tasks in the task queue.
// The task queue length is determined by the length of the slice holding the tasks.
// Returns:
// - int: the number of tasks in the queue.
func (s *SystemDelayer) AmountOfTasks() int {
	return len(s.tasks)
}

func (s *SystemDelayer) ClearTasks() {
	s.tasks = []*task{}
}

// createNewTaskQueueFromIndexes creates a new task queue from the specified indexes.
// It iterates over the indexes and appends the corresponding tasks from the current task queue
// to the newTasks slice. Then it assigns newTasks to the tasks slice of the SystemDelayer.
//
// Parameters:
//   - indexes: a variadic parameter representing the indexes of the tasks that should be included
//     in the new task queue.
//
// Returns: none
func (s *SystemDelayer) createNewTaskQueueFromIndexes(indexes ...int) {
	newTasks := make([]*task, 0, len(indexes))
	for _, index := range indexes {
		newTasks = append(newTasks, s.tasks[index])
	}
	s.tasks = newTasks
}

// DelayedTaskSystem updates the delay of tasks in the task queue.
// It decrements the delay of each task by one.
// If the delay becomes zero, the task is executed using the provided WorldContext.
// After updating the delays and executing the tasks, a new task queue is created
// based on the indexes that were kept during the iteration. This is supposed to be Registered as a system in cardinal.
//
// Parameters:
// - wCtx: a cardinal.WorldContext object representing the context in which the tasks are executed.
//
// Returns:
// - error: an error object if encountered during task execution, nil otherwise.
func (s *SystemDelayer) delayedTaskSystem(wCtx WorldContext) error {
	taskIndexToKeep := make([]int, 0)
	for i, task := range s.tasks {
		if task.delay > 0 {
			task.delay--
			taskIndexToKeep = append(taskIndexToKeep, i)
		} else {
			err := task.task(wCtx)
			if err != nil {
				return err
			}
		}
	}
	s.createNewTaskQueueFromIndexes(taskIndexToKeep...)
	return nil
}

func (s *SystemDelayer) Register(w *World) error {
	return RegisterSystems(w, s.delayedTaskSystem)
}
