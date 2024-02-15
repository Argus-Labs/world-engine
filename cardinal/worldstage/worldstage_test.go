package worldstage

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
)

func TestCanOperateOnZeroValue(t *testing.T) {
	atomicGameStage := NewManager()
	gotStage := atomicGameStage.Current()
	assert.Equal(t, Init, gotStage)

	gotStage = atomicGameStage.Swap(ShutDown)
	assert.Equal(t, Init, gotStage)
}

func TestCanCompareAndSwapOnZeroValue(t *testing.T) {
	atomicGameStage := NewManager()
	ok := atomicGameStage.CompareAndSwap(ShutDown, ShutDown)
	assert.Check(t, !ok, "zero value should be StagePreStart")

	ok = atomicGameStage.CompareAndSwap(Init, ShutDown)
	assert.Check(t, ok, "compare and swap should succeed with correct old value")

	assert.Equal(t, ShutDown, atomicGameStage.Current())
}

func TestOnlyOneCompareAndSwapSuccess(t *testing.T) {
	successCh := make(chan bool)
	atomicGameStage := NewManager()

	for i := 0; i < 10; i++ {
		go func() {
			ok := atomicGameStage.CompareAndSwap(Init, ShutDown)
			successCh <- ok
		}()
	}

	successCount := 0
	failureCount := 0
	for i := 0; i < 10; i++ {
		if <-successCh {
			successCount++
		} else {
			failureCount++
		}
	}
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 9, failureCount)
}
