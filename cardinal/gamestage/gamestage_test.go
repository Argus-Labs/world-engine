package gamestage

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestCanOperateOnZeroValue(t *testing.T) {
	atomicGameStage := NewAtomic()
	gotStage := atomicGameStage.Load()
	assert.Equal(t, StagePreStart, gotStage)

	gotStage = atomicGameStage.Swap(StageShutDown)
	assert.Equal(t, StagePreStart, gotStage)
}

func TestCanCompareAndSwapOnZeroValue(t *testing.T) {
	atomicGameStage := NewAtomic()
	ok := atomicGameStage.CompareAndSwap(StageShutDown, StageShutDown)
	assert.Check(t, !ok, "zero value should be StagePreStart")

	ok = atomicGameStage.CompareAndSwap(StagePreStart, StageShutDown)
	assert.Check(t, ok, "compare and swap should succeed with correct old value")

	assert.Equal(t, StageShutDown, atomicGameStage.Load())
}

func TestOnlyOneCompareAndSwapSuccess(t *testing.T) {
	successCh := make(chan bool)
	atomicGameStage := NewAtomic()

	for i := 0; i < 10; i++ {
		go func() {
			ok := atomicGameStage.CompareAndSwap(StagePreStart, StageShutDown)
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
