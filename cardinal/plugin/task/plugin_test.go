package task_test

import (
	"testing"
	"time"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/plugin/task"
	"pkg.world.dev/world-engine/cardinal/world"
)

// -----------------------------------------------------------------------------
// Test components
// -----------------------------------------------------------------------------

// Storage is a test component that stores a string and a counter.
type Storage struct {
	Storage string
}

func (Storage) Name() string {
	return "Storage"
}

type Counter struct {
	Count int
}

func (Counter) Name() string {
	return "Counter"
}

// -----------------------------------------------------------------------------
// Test tasks
// -----------------------------------------------------------------------------

// StorageSetterTask is a task that sets the storage value to the corresponding payload.
type StorageSetterTask struct {
	Payload string
}

var _ task.Task = (*StorageSetterTask)(nil)

func (StorageSetterTask) Name() string {
	return "StorageSetterTask"
}

func (t StorageSetterTask) Handle(w world.WorldContext) error {
	w.Logger().Info().Msgf("Executing task %v", t)

	id, err := w.Search(filter.Contains(Storage{})).First()
	if err != nil {
		return eris.Wrap(err, "failed to get Storage entity")
	}

	err = world.UpdateComponent[Storage](w, id, func(c *Storage) *Storage {
		c.Storage = t.Payload
		return c
	})
	if err != nil {
		return eris.Wrap(err, "failed to update Storage entity")
	}

	return nil
}

type CounterTask struct{}

var _ task.Task = (*CounterTask)(nil)

func (CounterTask) Name() string {
	return "CounterTask"
}

func (CounterTask) Handle(w world.WorldContext) error {
	w.Logger().Info().Msgf("Executing task %v", CounterTask{})

	id, err := w.Search(filter.Contains(Counter{})).First()
	if err != nil {
		return eris.Wrap(err, "failed to get Counter entity")
	}

	err = world.UpdateComponent[Counter](w, id, func(c *Counter) *Counter {
		c.Count++
		return c
	})
	if err != nil {
		return eris.Wrap(err, "failed to update Counter entity")
	}

	return nil
}

// -----------------------------------------------------------------------------
// ScheduleTimeTask tests
// -----------------------------------------------------------------------------

func TestPluginTask_ScheduleTimeTask(t *testing.T) {
	type testTask struct {
		delay time.Duration
		task  task.Task
	}

	type testExpected struct {
		wait    time.Duration
		storage string
		count   int
	}

	tests := []struct {
		name      string
		testTasks []testTask
		expected  []testExpected
	}{
		{
			name: "Task executed after the specified duration",
			testTasks: []testTask{
				{
					delay: 1 * time.Millisecond,
					task:  StorageSetterTask{Payload: "test"},
				},
			},
			expected: []testExpected{
				{
					wait:    10 * time.Millisecond,
					storage: "test",
					count:   0,
				},
			},
		},
		{
			name: "Task executed in the correct order",
			testTasks: []testTask{
				{
					delay: 1 * time.Millisecond,
					task:  StorageSetterTask{Payload: "test"},
				},
				{
					delay: 2 * time.Millisecond,
					task:  StorageSetterTask{Payload: "test2"},
				},
			},
			expected: []testExpected{
				{
					wait:    10 * time.Millisecond,
					storage: "test2",
					count:   0,
				},
			},
		},
		{
			name: "Task not prematurely executed",
			testTasks: []testTask{
				{
					delay: 10 * time.Second,
					task:  StorageSetterTask{Payload: "test"},
				},
			},
			expected: []testExpected{
				{
					wait:    10 * time.Millisecond,
					storage: "",
					count:   0,
				},
			},
		},
		{
			name: "Task executed only once",
			testTasks: []testTask{
				{
					delay: 1 * time.Millisecond,
					task:  CounterTask{},
				},
			},
			expected: []testExpected{
				{
					wait:    10 * time.Millisecond,
					storage: "",
					count:   1,
				},
				{
					wait:    10 * time.Millisecond,
					storage: "",
					count:   1,
				},
				{
					wait:    10 * time.Millisecond,
					storage: "",
					count:   1,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tf := cardinal.NewTestCardinal(t, nil)

			assert.NilError(t, world.RegisterComponent[Storage](tf.World()))
			assert.NilError(t, world.RegisterComponent[Counter](tf.World()))
			assert.NilError(t, task.RegisterTask[StorageSetterTask](tf.World()))
			assert.NilError(t, task.RegisterTask[CounterTask](tf.World()))

			// Register an init system that creates a Storage and Counter entity and schedules tasks
			err := world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
				// Create a Storage entity
				_, err := world.Create(wCtx, Storage{})
				assert.NilError(t, err)

				// Create a Counter entity
				_, err = world.Create(wCtx, Counter{})
				assert.NilError(t, err)

				// Schedule tasks
				for _, testTask := range tc.testTasks {
					assert.NilError(t, task.ScheduleTimeTask(wCtx, testTask.delay, testTask.task))
				}

				return nil
			})
			assert.NilError(t, err)

			// Execute the init system
			tf.DoTick()

			tf.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
				for _, expected := range tc.expected {
					// Wait for the task to be executed
					time.Sleep(expected.wait)
					// Execute the task
					tf.DoTick()

					// Fetch the storage and counter entities
					storageID, err := wCtx.Search(filter.Contains(Storage{})).First()
					assert.NilError(t, err)
					counterID, err := wCtx.Search(filter.Contains(Counter{})).First()
					assert.NilError(t, err)

					// Assert the storage value
					gotStorage, err := world.GetComponent[Storage](wCtx, storageID)
					assert.NilError(t, err)
					assert.Equal(t, gotStorage.Storage, expected.storage)

					// Assert the counter value
					gotCounter, err := world.GetComponent[Counter](wCtx, counterID)
					assert.NilError(t, err)
					assert.Equal(t, gotCounter.Count, expected.count)
				}
				return nil
			})
		})
	}
}

// TestPluginTask_ScheduleTimeTask_Recovery tests that the task is recovered after a world restart
func TestPluginTask_ScheduleTimeTask_Recovery(t *testing.T) {
	tf1 := cardinal.NewTestCardinal(t, nil)

	assert.NilError(t, world.RegisterComponent[Storage](tf1.World()))
	assert.NilError(t, task.RegisterTask[StorageSetterTask](tf1.World()))

	// Register an init system that creates a Storage and Counter entity and schedules tasks
	err := world.RegisterInitSystems(tf1.World(), func(wCtx world.WorldContext) error {
		// Create a Storage entity
		_, err := world.Create(wCtx, Storage{})
		assert.NilError(t, err)

		// Schedule tasks
		err = task.ScheduleTimeTask(wCtx, 10*time.Millisecond, StorageSetterTask{Payload: "test"})
		assert.NilError(t, err)

		return nil
	})
	assert.NilError(t, err)

	// Execute the init system
	tf1.DoTick()

	// Create a new test fixture with the same redis DB
	tf2 := cardinal.NewTestCardinal(t, tf1.Redis)

	assert.NilError(t, world.RegisterComponent[Storage](tf2.World()))
	assert.NilError(t, task.RegisterTask[StorageSetterTask](tf2.World()))

	// Wait until the task is ready to be executed
	time.Sleep(20 * time.Millisecond)

	// Execute the task
	tf2.DoTick()

	// Fetch the storage and counter entities
	tf2.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
		storageID, err := wCtx.Search(filter.Contains(Storage{})).First()
		assert.NilError(t, err)

		// Assert the storage value
		gotStorage, err := world.GetComponent[Storage](wCtx, storageID)
		assert.NilError(t, err)
		assert.Equal(t, gotStorage.Storage, "test")
		return nil
	})
}

// -----------------------------------------------------------------------------
// ScheduleTickTask tests
// -----------------------------------------------------------------------------

func TestPluginTask_ScheduleTickTask(t *testing.T) {
	type testTask struct {
		delay int64
		task  task.Task
	}

	type testExpected struct {
		tickToRun int // how many ticks to run before test is asserted
		storage   string
		count     int
	}

	tests := []struct {
		name      string
		testTasks []testTask
		expected  []testExpected
	}{
		{
			name: "Task executed after the specified duration",
			testTasks: []testTask{
				{
					delay: 1,
					task:  StorageSetterTask{Payload: "test"},
				},
			},
			expected: []testExpected{
				{
					tickToRun: 1,
					storage:   "test",
					count:     0,
				},
			},
		},
		{
			name: "Task executed in the correct order",
			testTasks: []testTask{
				{
					delay: 1,
					task:  StorageSetterTask{Payload: "test"},
				},
				{
					delay: 2,
					task:  StorageSetterTask{Payload: "test2"},
				},
			},
			expected: []testExpected{
				{
					tickToRun: 2,
					storage:   "test2",
					count:     0,
				},
			},
		},
		{
			name: "Task not prematurely executed",
			testTasks: []testTask{
				{
					delay: 10,
					task:  StorageSetterTask{Payload: "test"},
				},
			},
			expected: []testExpected{
				{
					tickToRun: 1,
					storage:   "",
					count:     0,
				},
			},
		},
		{
			name: "Task executed only once",
			testTasks: []testTask{
				{
					delay: 1,
					task:  CounterTask{},
				},
			},
			expected: []testExpected{
				{
					tickToRun: 1,
					storage:   "",
					count:     1,
				},
				{
					tickToRun: 1,
					storage:   "",
					count:     1,
				},
				{
					tickToRun: 1,
					storage:   "",
					count:     1,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tf := cardinal.NewTestCardinal(t, nil)

			assert.NilError(t, world.RegisterComponent[Storage](tf.World()))
			assert.NilError(t, world.RegisterComponent[Counter](tf.World()))
			assert.NilError(t, task.RegisterTask[StorageSetterTask](tf.World()))
			assert.NilError(t, task.RegisterTask[CounterTask](tf.World()))

			// Register an init system that creates a Storage and Counter entity and schedules tasks
			err := world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
				// Create a Storage entity
				_, err := world.Create(wCtx, Storage{})
				assert.NilError(t, err)

				// Create a Counter entity
				_, err = world.Create(wCtx, Counter{})
				assert.NilError(t, err)

				// Schedule tasks
				for _, testTask := range tc.testTasks {
					assert.NilError(t, task.ScheduleTickTask(wCtx, testTask.delay, testTask.task))
				}

				return nil
			})
			assert.NilError(t, err)

			// Execute the init system
			tf.DoTick()

			err = tf.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
				for _, expected := range tc.expected {
					for i := 0; i < expected.tickToRun; i++ {
						// Fast forward the tick
						tf.DoTick()
					}

					// Fetch the storage and counter entities
					storageID, err := wCtx.Search(filter.Contains(Storage{})).First()
					assert.NilError(t, err)
					counterID, err := wCtx.Search(filter.Contains(Counter{})).First()
					assert.NilError(t, err)

					// Assert the storage value
					gotStorage, err := world.GetComponent[Storage](wCtx, storageID)
					assert.NilError(t, err)
					assert.Equal(t, gotStorage.Storage, expected.storage)

					// Assert the counter value
					gotCounter, err := world.GetComponent[Counter](wCtx, counterID)
					assert.NilError(t, err)
					assert.Equal(t, gotCounter.Count, expected.count)
				}
				return nil
			})
			assert.NilError(t, err)
		})
	}
}

// TestPluginTask_ScheduleTickTask_Recovery tests that the task is recovered after a world restart
func TestPluginTask_ScheduleTickTask_Recovery(t *testing.T) {
	tf1 := cardinal.NewTestCardinal(t, nil)

	assert.NilError(t, world.RegisterComponent[Storage](tf1.World()))
	assert.NilError(t, task.RegisterTask[StorageSetterTask](tf1.World()))

	// Register an init system that creates a Storage and Counter entity and schedules tasks
	err := world.RegisterInitSystems(tf1.World(), func(wCtx world.WorldContext) error {
		// Create a Storage entity
		_, err := world.Create(wCtx, Storage{})
		assert.NilError(t, err)

		// Schedule tasks
		err = task.ScheduleTickTask(wCtx, 2, StorageSetterTask{Payload: "test"})
		assert.NilError(t, err)

		return nil
	})
	assert.NilError(t, err)

	// Execute the init system
	tf1.DoTick()

	// Create a new test fixture with the same redis DB
	tf2 := cardinal.NewTestCardinal(t, tf1.Redis)

	assert.NilError(t, world.RegisterComponent[Storage](tf2.World()))
	assert.NilError(t, task.RegisterTask[StorageSetterTask](tf2.World()))

	// Execute the task
	tf2.DoTick()
	tf2.DoTick()

	// Fetch the storage and counter entities
	err = tf2.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
		storageID, err := wCtx.Search(filter.Contains(Storage{})).First()
		assert.NilError(t, err)

		// Assert the storage value
		gotStorage, err := world.GetComponent[Storage](wCtx, storageID)
		assert.NilError(t, err)
		assert.Equal(t, gotStorage.Storage, "test")
		return nil
	})
	assert.NilError(t, err)
}
