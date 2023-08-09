package receipt

import (
	"errors"
	"testing"

	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
	"gotest.tools/v3/assert"
)

func txID(name string, index uint64) transaction.TxID {
	return transaction.TxID{
		PersonaTag: name,
		Index:      index,
	}
}

func TestCanSaveAndGetAnError(t *testing.T) {
	rh := NewHistory(100, 10)
	id := txID("foobar", 99)
	wantError := errors.New("some error")

	rh.AddError(id, wantError)

	rec, ok := rh.GetReceipt(id)
	assert.Check(t, ok)
	assert.Equal(t, 1, len(rec.Errs))
	assert.ErrorIs(t, wantError, rec.Errs[0])
	assert.Equal(t, nil, rec.Result)
}

func TestCanSaveAndGetManyErrors(t *testing.T) {
	rh := NewHistory(99, 5)
	id := txID("xyzzy", 10)
	errA, errB := errors.New("a error"), errors.New("b error")
	rh.AddError(id, errA)
	rh.AddError(id, errB)
	rec, ok := rh.GetReceipt(id)
	assert.Check(t, ok)
	assert.Equal(t, 2, len(rec.Errs))
	assert.ErrorIs(t, errA, rec.Errs[0])
	assert.ErrorIs(t, errB, rec.Errs[1])
	assert.Equal(t, nil, rec.Result)
}

func TestCanSaveAndGetResult(t *testing.T) {
	type myStruct struct {
		X string
		Y int
	}

	rh := NewHistory(99, 5)
	id := txID("xyzzy", 10)
	wantStruct := myStruct{"woo", 100}
	rh.SetResult(id, wantStruct)

	rec, ok := rh.GetReceipt(id)
	assert.Check(t, ok)
	assert.Equal(t, 0, len(rec.Errs))
	assert.Check(t, rec.Result != nil)
	gotStruct, ok := rec.Result.(myStruct)
	assert.Check(t, ok)
	assert.Equal(t, wantStruct, gotStruct)
}

func TestCanReplaceResult(t *testing.T) {
	type toBeReplaced struct {
		Name string
	}

	rh := NewHistory(99, 5)
	id := txID("xyzzy", 10)

	doNotWant := toBeReplaced{"replaceme"}
	rh.SetResult(id, doNotWant)

	want := toBeReplaced{"I actually want this result"}
	rh.SetResult(id, want)

	rec, ok := rh.GetReceipt(id)
	assert.Check(t, ok)
	assert.Equal(t, 0, len(rec.Errs))
	assert.Check(t, rec.Result != nil)

	got, ok := rec.Result.(toBeReplaced)
	assert.Check(t, ok)
	assert.Equal(t, want, got)

}

func TestMissingIDReturnsNotOK(t *testing.T) {
	rh := NewHistory(99, 5)
	id := txID("does not exist", 10)

	_, ok := rh.GetReceipt(id)
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
	id := txID("an-old-id", 10)
	wantResult := MyStruct{1234}
	wantError := errors.New("some error")
	rh.SetResult(id, wantResult)
	rh.AddError(id, wantError)

	// We should be able to call NextTick 3 times and still be able to get the relevant tick
	for i := 0; i < historyLength; i++ {
		rh.NextTick()
		recs, err := rh.GetReceiptsForTick(tickToGet)
		assert.NilError(t, err)
		assert.Equal(t, 1, len(recs), "failed to get receipts in step %d", i)
		rec := recs[0]
		assert.Equal(t, 1, len(rec.Errs))
		assert.ErrorIs(t, wantError, rec.Errs[0])
		gotResult, ok := rec.Result.(MyStruct)
		assert.Check(t, ok)
		assert.Equal(t, wantResult, gotResult)
	}

	// tickToGet is now 4 ticks in the past, and since our historyLength is only 3, the tick
	// should no longer be stored
	rh.NextTick()
	_, err := rh.GetReceiptsForTick(tickToGet)
	assert.ErrorIs(t, ErrorOldTickHasBeenDiscarded, err)
}
