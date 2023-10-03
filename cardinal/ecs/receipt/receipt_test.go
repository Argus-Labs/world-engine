package receipt

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/google/uuid"
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

func txHash(t *testing.T) interfaces.TxHash {
	id, err := uuid.NewUUID()
	assert.NilError(t, err)
	return interfaces.TxHash(id.String())
}

func TestCanSaveAndGetAnError(t *testing.T) {
	rh := NewHistory(100, 10)
	hash := txHash(t)
	wantError := errors.New("some error")

	rh.AddError(hash, wantError)

	rec, ok := rh.GetReceipt(hash)
	assert.Check(t, ok)
	assert.Equal(t, 1, len(rec.GetErrors()))
	assert.ErrorIs(t, wantError, rec.GetErrors()[0])
	assert.Equal(t, nil, rec.GetResult())
}

func TestCanSaveAndGetManyErrors(t *testing.T) {
	rh := NewHistory(99, 5)
	hash := txHash(t)
	errA, errB := errors.New("a error"), errors.New("b error")
	rh.AddError(hash, errA)
	rh.AddError(hash, errB)
	rec, ok := rh.GetReceipt(hash)
	assert.Check(t, ok)
	assert.Equal(t, 2, len(rec.GetErrors()))
	assert.ErrorIs(t, errA, rec.GetErrors()[0])
	assert.ErrorIs(t, errB, rec.GetErrors()[1])
	assert.Equal(t, nil, rec.GetResult())
}

func TestCanSaveAndGetResult(t *testing.T) {
	type myStruct struct {
		X string
		Y int
	}

	rh := NewHistory(99, 5)
	hash := txHash(t)
	wantStruct := myStruct{"woo", 100}
	rh.SetResult(hash, wantStruct)

	rec, ok := rh.GetReceipt(hash)
	assert.Check(t, ok)
	assert.Equal(t, 0, len(rec.GetErrors()))
	assert.Check(t, rec.GetResult() != nil)
	gotStruct, ok := rec.GetResult().(myStruct)
	assert.Check(t, ok)
	assert.Equal(t, wantStruct, gotStruct)
}

func TestCanReplaceResult(t *testing.T) {
	type toBeReplaced struct {
		Name string
	}

	rh := NewHistory(99, 5)
	hash := txHash(t)

	doNotWant := toBeReplaced{"replaceme"}
	rh.SetResult(hash, doNotWant)

	want := toBeReplaced{"I actually want this result"}
	rh.SetResult(hash, want)

	rec, ok := rh.GetReceipt(hash)
	assert.Check(t, ok)
	assert.Equal(t, 0, len(rec.GetErrors()))
	assert.Check(t, rec.GetResult() != nil)

	got, ok := rec.GetResult().(toBeReplaced)
	assert.Check(t, ok)
	assert.Equal(t, want, got)

}

func TestMissingHashReturnsNotOK(t *testing.T) {
	rh := NewHistory(99, 5)
	hash := txHash(t)

	_, ok := rh.GetReceipt(hash)
	assert.Check(t, !ok)
}

func TestErrorWhenGettingReceiptsInNonFinishedTick(t *testing.T) {
	currTick := uint64(99)
	rh := NewHistory(currTick, 5)

	_, err := rh.GetReceiptsForTick(currTick)
	assert.ErrorIs(t, ErrorTickHasNotBeenProcessed, err)

	rh.NextTick()

	recs, err := rh.GetReceiptsForTick(currTick)
	assert.NilError(t, err)
	assert.Equal(t, 0, len(recs))
}

func TestOldTicksAreDiscarded(t *testing.T) {
	type MyStruct struct {
		Number int
	}

	tickToGet := uint64(99)
	historyLength := 3
	// ticksToStore is 3, so at most 3 ticks from the past will be remembered.
	rh := NewHistory(tickToGet, historyLength)
	hash := txHash(t)
	wantResult := MyStruct{1234}
	wantError := errors.New("some error")
	rh.SetResult(hash, wantResult)
	rh.AddError(hash, wantError)

	// We should be able to call NextTick 3 times and still be able to get the relevant tick
	for i := 0; i < historyLength; i++ {
		rh.NextTick()
		recs, err := rh.GetReceiptsForTick(tickToGet)
		assert.NilError(t, err)
		assert.Equal(t, 1, len(recs), "failed to get receipts in step %d", i)
		rec := recs[0]
		assert.Equal(t, 1, len(rec.GetErrors()))
		assert.ErrorIs(t, wantError, rec.GetErrors()[0])
		gotResult, ok := rec.GetResult().(MyStruct)
		assert.Check(t, ok)
		assert.Equal(t, wantResult, gotResult)
	}

	// tickToGet is now 4 ticks in the past, and since our historyLength is only 3, the tick
	// should no longer be stored
	rh.NextTick()
	_, err := rh.GetReceiptsForTick(tickToGet)
	assert.ErrorIs(t, ErrorOldTickHasBeenDiscarded, err)
}
