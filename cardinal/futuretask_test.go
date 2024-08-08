package cardinal_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
)

// MyTask is a struct representing a task. It has a `TestValue` field which is an integer.
// It also implements the `Name` method which returns the name of the task, and the `Handle` method
// which executes the task logic.
type MyTask struct {
	TestValue int
}

func (*MyTask) Name() string {
	return "MyTask"
}

func (mt *MyTask) Handle(_ cardinal.WorldContext) error {
	mt.TestValue++
	return nil
}

// TestCallTasksAt is a unit test function that tests the functionality of calling tasks
// at a specific timestamp using the CallTaskAt method in the Cardinal library. It initializes
// a test fixture and a world context, and registers and runs test systems to test the CallTaskAt
// functionality. The test logic checks if the task is registered and executed correctly at the
// specified timestamp, and validates the result of the task execution.
func TestCallTasksAt(t *testing.T) {
	// initialize test fixture
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World

	// Register the relevant task for getting called at a timestamp
	err := cardinal.RegisterTimestampTask[*MyTask](world)
	assert.NilError(t, err)

	// variables for testing
	wasInitializeTestSystemCall := false
	var startTime uint64
	var futureDelay uint64 = 1000
	endTestIfThisTrue := false

	// This system calls CallTaskAt, The logic is run once.
	initializeTestsSystem := func(context cardinal.WorldContext) error {
		var res error
		if !wasInitializeTestSystemCall {
			startTime = context.Timestamp()
			res = context.CallTaskAt(&MyTask{TestValue: 0}, startTime+futureDelay)
		} else {
			res = nil
		}
		wasInitializeTestSystemCall = true
		return res
	}

	// This system tests if CallTaskAt's functionality was executed successfully
	testIfCallTaskAtIsSuccessfulSystem := func(ctx cardinal.WorldContext) error {
		if wasInitializeTestSystemCall {
			if ctx.Timestamp() >= startTime+futureDelay {
				endTestIfThisTrue = true
				count, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[*MyTask]())).Count(ctx)
				assert.NilError(t, err)
				assert.Equal(t, count, 0)
				return nil
			}
			id, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[*MyTask]())).First(ctx)
			assert.NilError(t, err)
			task, err := cardinal.GetComponent[*MyTask](ctx, id)
			if err != nil {
				return err
			}
			assert.Equal(t, (*task).TestValue, 0)
		}
		return nil
	}

	// Run and register test systems
	err = cardinal.RegisterSystems(world, initializeTestsSystem, testIfCallTaskAtIsSuccessfulSystem)
	assert.NilError(t, err)
	for !endTestIfThisTrue {
		tf.DoTick()
	}
}

// TestDelayedTask is a test function that simulates the delayed execution of a task.
// It registers a task for tick delay and runs the tick function multiple times until the delay is complete.
// It verifies if the delay task logic executes successfully by checking the test value after each tick.
// The test passes if the test value is 0 at the beginning and becomes 1 after the delay.
func TestDelayedTask(t *testing.T) {
	// initialize test fixture
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World

	// register task for tick delay
	err := cardinal.RegisterTickTask[*MyTask](world)
	assert.NilError(t, err)

	// variables
	var delay uint64 = 20
	var testValue int
	isInitializeTestSystemCalled := false

	// this task initializes the first delay task call. The logic runs once.
	initializeTestsSystem := func(context cardinal.WorldContext) error {
		var res error
		if !isInitializeTestSystemCalled {
			res = context.DelayTask(&MyTask{TestValue: 0}, delay)
		} else {
			res = nil
		}
		isInitializeTestSystemCalled = true
		return res
	}

	// This system tests if the DelayTask logic executes successfully.
	var counter uint64
	testIfDelayTaskIsSuccessfulSystem := func(ctx cardinal.WorldContext) error {
		if isInitializeTestSystemCalled {
			if counter < delay {
				id, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[*MyTask]())).First(ctx)
				assert.NilError(t, err)
				task, err := cardinal.GetComponent[*MyTask](ctx, id)
				if err != nil {
					return err
				}
				testValue = (*task).TestValue
			} else {
				count, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[*MyTask]())).Count(ctx)
				assert.NilError(t, err)
				assert.Equal(t, count, 0)
			}
		}
		counter++
		return nil
	}

	// Register the relevant systems
	err = cardinal.RegisterSystems(world, initializeTestsSystem, testIfDelayTaskIsSuccessfulSystem)
	assert.NilError(t, err)

	// run ticks.
	var i uint64
	for i = 0; i < delay+1; i++ {
		assert.Equal(t, testValue, 0)
		tf.DoTick()
	}
}

// TestNotRegisteringOfDelayedTask tests the behavior when a delayed task is not registered in the system.
func TestNotRegisteringOfDelayedTask(t *testing.T) {
	// initialize test fixture
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World

	// task deliberately not registered.
	// err := cardinal.RegisterTimestampTask[*MyTask](world)
	// assert.NilError(t, err)

	// variables
	var delay uint64 = 20

	// this task initializes the first delay task call. It should fail because the component wasn't Registered.
	initializeTestsSystem := func(context cardinal.WorldContext) error {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Recovered from panic: %v", r)
			} else {
				t.Errorf("Expected a panic, but the function did not panic")
			}
		}()
		_ = context.DelayTask(&MyTask{TestValue: 0}, delay)

		return nil
	}

	// Register the relevant systems
	err := cardinal.RegisterSystems(world, initializeTestsSystem)
	assert.NilError(t, err)

	// run ticks. Should tail every time.
	var i uint64
	for i = 0; i < delay+1; i++ {
		tf.DoTick()
	}
}

// TestNotRegisteringOfTimestampTask tests the scenario where a timestamp task is not registered.
// It initializes a test fixture with a world and deliberately does not register the task.
// The task should fail because it is not registered, and a panic is expected.
// The test verifies that a panic occurs and logs the recovered value if any.
// The initializeTestsSystem function is used to initialize the first delay task call.
// The relevant systems are registered using cardinal.RegisterSystems.
// A loop is executed to run ticks, and each tick should fail because the task is not registered.
// The test ensures that the expected panic occurs when the task is called without being registered.
func TestNotRegisteringOfTimestampTask(t *testing.T) {
	// initialize test fixture
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World

	// task deliberately not registered.
	// err := cardinal.RegisterTickTask[*MyTask](world)
	// assert.NilError(t, err)

	// variables
	var delay uint64 = 20

	// this task initializes the first delay task call. It should fail because the component wasn't Registered.
	initializeTestsSystem := func(context cardinal.WorldContext) error {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Recovered from panic: %v", r)
			} else {
				t.Errorf("Expected a panic, but the function did not panic")
			}
		}()
		_ = context.CallTaskAt(&MyTask{TestValue: 0}, delay)

		return nil
	}

	// Register the relevant systems
	err := cardinal.RegisterSystems(world, initializeTestsSystem)
	assert.NilError(t, err)

	// run ticks. Should tail every time.
	var i uint64
	for i = 0; i < delay+1; i++ {
		tf.DoTick()
	}
}
