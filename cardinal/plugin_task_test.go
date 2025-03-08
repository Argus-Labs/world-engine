package cardinal_test

import (
	"testing"
	"time"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
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

var _ cardinal.Task = (*StorageSetterTask)(nil)

func (StorageSetterTask) Name() string {
	return "StorageSetterTask"
}

func (t StorageSetterTask) Handle(w cardinal.WorldContext) error {
	w.Logger().Info().Msgf("Executing task %v", t)

	id, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Storage]())).First(w)
	if err != nil {
		return eris.Wrap(err, "failed to get Storage entity")
	}

	err = cardinal.UpdateComponent[Storage](w, id, func(c *Storage) *Storage {
		c.Storage = t.Payload
		return c
	})
	if err != nil {
		return eris.Wrap(err, "failed to update Storage entity")
	}

	return nil
}

type CounterTask struct{}

var _ cardinal.Task = (*CounterTask)(nil)

func (CounterTask) Name() string {
	return "CounterTask"
}

func (CounterTask) Handle(w cardinal.WorldContext) error {
	w.Logger().Info().Msgf("Executing task %v", CounterTask{})

	id, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Counter]())).First(w)
	if err != nil {
		return eris.Wrap(err, "failed to get Counter entity")
	}

	err = cardinal.UpdateComponent[Counter](w, id, func(c *Counter) *Counter {
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
		task  cardinal.Task
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
			tf := cardinal.NewTestFixture(t, nil)
			world := tf.World

			assert.NilError(t, cardinal.RegisterComponent[Storage](world))
			assert.NilError(t, cardinal.RegisterComponent[Counter](world))
			assert.NilError(t, cardinal.RegisterTask[StorageSetterTask](world))
			assert.NilError(t, cardinal.RegisterTask[CounterTask](world))

			// Register an init system that creates a Storage and Counter entity and schedules tasks
			err := cardinal.RegisterInitSystems(world, func(wCtx cardinal.WorldContext) error {
				// Create a Storage entity
				_, err := cardinal.Create(wCtx, Storage{})
				assert.NilError(t, err)

				// Create a Counter entity
				_, err = cardinal.Create(wCtx, Counter{})
				assert.NilError(t, err)

				// Schedule tasks
				for _, testTask := range tc.testTasks {
					assert.NilError(t, wCtx.ScheduleTimeTask(testTask.delay, testTask.task))
				}

				return nil
			})
			assert.NilError(t, err)

			// Execute the init system
			tf.DoTick()

			for _, expected := range tc.expected {
				// Wait for the task to be executed
				time.Sleep(expected.wait)
				// Execute the task
				tf.DoTick()

				// Fetch the storage and counter entities
				wCtx := cardinal.NewWorldContext(world)
				storageID, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Storage]())).First(wCtx)
				assert.NilError(t, err)
				counterID, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Counter]())).First(wCtx)
				assert.NilError(t, err)

				// Assert the storage value
				gotStorage, err := cardinal.GetComponent[Storage](wCtx, storageID)
				assert.NilError(t, err)
				assert.Equal(t, gotStorage.Storage, expected.storage)

				// Assert the counter value
				gotCounter, err := cardinal.GetComponent[Counter](wCtx, counterID)
				assert.NilError(t, err)
				assert.Equal(t, gotCounter.Count, expected.count)
			}
		})
	}
}

// TestPluginTask_ScheduleTimeTask_Recovery tests that the task is recovered after a world restart.
func TestPluginTask_ScheduleTimeTask_Recovery(t *testing.T) {
	tf1 := cardinal.NewTestFixture(t, nil)
	world1 := tf1.World

	assert.NilError(t, cardinal.RegisterComponent[Storage](world1))
	assert.NilError(t, cardinal.RegisterTask[StorageSetterTask](world1))

	// Register an init system that creates a Storage and Counter entity and schedules tasks
	err := cardinal.RegisterInitSystems(world1, func(wCtx cardinal.WorldContext) error {
		// Create a Storage entity
		_, err := cardinal.Create(wCtx, Storage{})
		assert.NilError(t, err)

		// Schedule tasks
		err = wCtx.ScheduleTimeTask(10*time.Millisecond, StorageSetterTask{Payload: "test"})
		assert.NilError(t, err)

		return nil
	})
	assert.NilError(t, err)

	// Execute the init system
	tf1.DoTick()

	// Create a new test fixture with the same redis DB
	tf2 := cardinal.NewTestFixture(t, tf1.Redis)
	world2 := tf2.World

	assert.NilError(t, cardinal.RegisterComponent[Storage](world2))
	assert.NilError(t, cardinal.RegisterTask[StorageSetterTask](world2))

	// Wait until the task is ready to be executed
	time.Sleep(20 * time.Millisecond)

	// Execute the task
	tf2.DoTick()

	// Fetch the storage and counter entities
	wCtx := cardinal.NewWorldContext(world2)
	storageID, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Storage]())).First(wCtx)
	assert.NilError(t, err)

	// Assert the storage value
	gotStorage, err := cardinal.GetComponent[Storage](wCtx, storageID)
	assert.NilError(t, err)
	assert.Equal(t, gotStorage.Storage, "test")
}

// -----------------------------------------------------------------------------
// ScheduleTickTask tests
// -----------------------------------------------------------------------------

func TestPluginTask_ScheduleTickTask(t *testing.T) {
	type testTask struct {
		delay uint64
		task  cardinal.Task
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
			tf := cardinal.NewTestFixture(t, nil)
			world := tf.World

			assert.NilError(t, cardinal.RegisterComponent[Storage](world))
			assert.NilError(t, cardinal.RegisterComponent[Counter](world))
			assert.NilError(t, cardinal.RegisterTask[StorageSetterTask](world))
			assert.NilError(t, cardinal.RegisterTask[CounterTask](world))

			// Register an init system that creates a Storage and Counter entity and schedules tasks
			err := cardinal.RegisterInitSystems(world, func(wCtx cardinal.WorldContext) error {
				// Create a Storage entity
				_, err := cardinal.Create(wCtx, Storage{})
				assert.NilError(t, err)

				// Create a Counter entity
				_, err = cardinal.Create(wCtx, Counter{})
				assert.NilError(t, err)

				// Schedule tasks
				for _, testTask := range tc.testTasks {
					assert.NilError(t, wCtx.ScheduleTickTask(testTask.delay, testTask.task))
				}

				return nil
			})
			assert.NilError(t, err)

			// Execute the init system
			tf.DoTick()

			for _, expected := range tc.expected {
				for i := 0; i < expected.tickToRun; i++ {
					// Fast forward the tick
					tf.DoTick()
				}

				// Fetch the storage and counter entities
				wCtx := cardinal.NewWorldContext(world)
				storageID, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Storage]())).First(wCtx)
				assert.NilError(t, err)
				counterID, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Counter]())).First(wCtx)
				assert.NilError(t, err)

				// Assert the storage value
				gotStorage, err := cardinal.GetComponent[Storage](wCtx, storageID)
				assert.NilError(t, err)
				assert.Equal(t, gotStorage.Storage, expected.storage)

				// Assert the counter value
				gotCounter, err := cardinal.GetComponent[Counter](wCtx, counterID)
				assert.NilError(t, err)
				assert.Equal(t, gotCounter.Count, expected.count)
			}
		})
	}
}

// TestPluginTask_ScheduleTickTask_Recovery tests that the task is recovered after a world restart.
func TestPluginTask_ScheduleTickTask_Recovery(t *testing.T) {
	tf1 := cardinal.NewTestFixture(t, nil)
	world1 := tf1.World

	assert.NilError(t, cardinal.RegisterComponent[Storage](world1))
	assert.NilError(t, cardinal.RegisterTask[StorageSetterTask](world1))

	// Register an init system that creates a Storage and Counter entity and schedules tasks
	err := cardinal.RegisterInitSystems(world1, func(wCtx cardinal.WorldContext) error {
		// Create a Storage entity
		_, err := cardinal.Create(wCtx, Storage{})
		assert.NilError(t, err)

		// Schedule tasks
		err = wCtx.ScheduleTickTask(2, StorageSetterTask{Payload: "test"})
		assert.NilError(t, err)

		return nil
	})
	assert.NilError(t, err)

	// Execute the init system
	tf1.DoTick()

	// Create a new test fixture with the same redis DB
	tf2 := cardinal.NewTestFixture(t, tf1.Redis)
	world2 := tf2.World

	assert.NilError(t, cardinal.RegisterComponent[Storage](world2))
	assert.NilError(t, cardinal.RegisterTask[StorageSetterTask](world2))

	// Wait until the task is ready to be executed
	time.Sleep(20 * time.Millisecond)

	// Execute the task
	tf2.DoTick()
	tf2.DoTick()

	// Fetch the storage and counter entities
	wCtx := cardinal.NewWorldContext(world2)
	storageID, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Storage]())).First(wCtx)
	assert.NilError(t, err)

	// Assert the storage value
	gotStorage, err := cardinal.GetComponent[Storage](wCtx, storageID)
	assert.NilError(t, err)
	assert.Equal(t, gotStorage.Storage, "test")
}
