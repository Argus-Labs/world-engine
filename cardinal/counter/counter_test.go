package counter_test

import (
	"sync"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/counter"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

type Alpha struct{}

func (Alpha) Name() string { return "alpha" }

type Beta struct{}

func (Beta) Name() string { return "beta" }

type Gamma struct{}

func (Gamma) Name() string { return "gamma" }

func TestCounter(t *testing.T) {
	counterStore := counter.NewCounter()
	wg := sync.WaitGroup{}
	adder := func(key string, wg *sync.WaitGroup) {
		wg.Add(1)
		go func() {
			counterStore.Add(key)
			wg.Done()
		}()
	}
	testKeys := []string{
		"hello",
		"world",
		"blah",
		"world",
		"hello",
		"blah",
		"world",
		"blah",
		"world",
	}
	for _, key := range testKeys {
		adder(key, &wg)
	}
	wg.Wait()
	results, err := counterStore.GetAllCounts()
	assert.NilError(t, err)
	v, ok := results["hello"]
	assert.Assert(t, ok)
	assert.Equal(t, v, uint64(2))
	v, ok = results["world"]
	assert.Assert(t, ok)
	assert.Equal(t, v, uint64(4))
	v, ok = results["blah"]
	assert.Assert(t, ok)
	assert.Equal(t, v, uint64(3))
}

func TestCounterWithEventHub(t *testing.T) {
	counterStore := counter.NewCounter()
	world, _ := testutils.MakeWorldAndTicker(t, cardinal.WithMetricCounter(&counterStore))
	testKeys := []string{
		"hello",
		"world",
		"blah",
		"world",
		"hello",
		"blah",
		"world",
		"blah",
		"world",
	}
	for _, key := range testKeys {
		world.Instance().Count(&events.Event{
			Key:     key,
			Message: "",
		})
	}
	results, err := counterStore.GetAllCounts()
	assert.NilError(t, err)
	v, ok := results["hello"]
	assert.Assert(t, ok)
	assert.Equal(t, v, uint64(2))
	v, ok = results["world"]
	assert.Assert(t, ok)
	assert.Equal(t, v, uint64(4))
	v, ok = results["blah"]
	assert.Assert(t, ok)
	assert.Equal(t, v, uint64(3))
}
