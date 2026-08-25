package snapshot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run all tests in this file with -race. Each test uses the caller and worker goroutines.

// TestAsyncWriterWriteDoesNotBlock verifies that storage does not block Write.
func TestAsyncWriterWriteDoesNotBlock(t *testing.T) {
	t.Parallel()

	storage := newBlockingWriterStorage()
	writer := newTestAsyncWriter(t, storage)
	snap := randomSnapshot(testutils.NewRand(t))

	written := make(chan struct{})
	go func() {
		writer.Write(snap)
		close(written)
	}()

	storage.waitEntered(t)
	select {
	case <-written:
	case <-time.After(snapshotWriteTimeout / 2):
		t.Fatal("Write waited for snapshot storage")
	}
	select {
	case <-storage.returned:
		t.Fatal("storage returned before Write")
	default:
	}

	storage.release <- struct{}{}
	require.NoError(t, writer.Drain(t.Context()))
}

// TestAsyncWriterLatestWins verifies that a new snapshot replaces the pending snapshot.
func TestAsyncWriterLatestWins(t *testing.T) {
	t.Parallel()
	storage := newBlockingWriterStorage()
	writer := newTestAsyncWriter(t, storage)
	prng := testutils.NewRand(t)

	first := randomSnapshot(prng)
	second := randomSnapshot(prng)
	third := randomSnapshot(prng)
	first.TickHeight = 1
	second.TickHeight = 2
	third.TickHeight = 3

	writer.Write(first)
	require.Equal(t, first.GetTickHeight(), storage.waitEntered(t))
	writer.Write(second)
	writer.Write(third)

	storage.release <- struct{}{}
	require.Equal(t, third.GetTickHeight(), storage.waitEntered(t))
	storage.release <- struct{}{}

	require.NoError(t, writer.Drain(t.Context()))
	assert.Equal(t, []uint64{first.GetTickHeight(), third.GetTickHeight()}, storage.storedTicks())
}

// TestAsyncWriterDrainWaitsForFinalSnapshot verifies that Drain waits for storage.
func TestAsyncWriterDrainWaitsForFinalSnapshot(t *testing.T) {
	t.Parallel()
	storage := newBlockingWriterStorage()
	writer := newTestAsyncWriter(t, storage)
	snap := randomSnapshot(testutils.NewRand(t))

	writer.Write(snap)
	storage.waitEntered(t)

	drainCtx := newObservedWriterContext()
	drained := make(chan error, 1)
	go func() { drained <- writer.Drain(drainCtx) }()
	select {
	case <-drainCtx.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Drain did not start")
	}
	select {
	case <-drained:
		t.Fatal("Drain returned before storage finished")
	case <-time.After(100 * time.Millisecond):
	}

	storage.release <- struct{}{}
	select {
	case err := <-drained:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Drain did not return after storage finished")
	}
	assert.Equal(t, []uint64{snap.GetTickHeight()}, storage.storedTicks())
}

// TestAsyncWriterDrainReportsStoreFailure verifies that Drain returns the final storage error.
func TestAsyncWriterDrainReportsStoreFailure(t *testing.T) {
	t.Parallel()
	firstErr := errors.New("first storage error")
	finalErr := errors.New("final storage error")
	storage := newSequencedErrorWriterStorage(firstErr, finalErr)
	writer := newTestAsyncWriter(t, storage)
	prng := testutils.NewRand(t)

	writer.Write(randomSnapshot(prng))
	require.Equal(t, 0, storage.waitEntered(t))
	writer.Write(randomSnapshot(prng))
	storage.release <- struct{}{}
	require.Equal(t, 1, storage.waitEntered(t))
	storage.release <- struct{}{}

	err := writer.Drain(t.Context())
	require.Error(t, err)
	require.ErrorIs(t, err, finalErr)
	assert.NotErrorIs(t, err, firstErr)
}

// TestAsyncWriterContinuesAfterAcceptedDrainIsCanceled verifies that an abandoned response does not
// block the worker.
func TestAsyncWriterContinuesAfterAcceptedDrainIsCanceled(t *testing.T) {
	t.Parallel()
	storage := newBlockingWriterStorage()
	writer := newTestAsyncWriter(t, storage)
	prng := testutils.NewRand(t)

	// Set the state that exists after Write updates pending and before it sends wake.
	first := randomSnapshot(prng)
	first.TickHeight = 1
	writer.mu.Lock()
	writer.pending = first
	writer.mu.Unlock()

	drainCtx, cancel := context.WithCancel(context.Background())
	drained := make(chan error, 1)
	go func() { drained <- writer.Drain(drainCtx) }()
	storage.waitEntered(t)
	cancel()
	select {
	case err := <-drained:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("canceled Drain did not return")
	}

	storage.release <- struct{}{}
	second := randomSnapshot(prng)
	second.TickHeight = 2
	writer.Write(second)
	require.Equal(t, second.GetTickHeight(), storage.waitEntered(t))
	storage.release <- struct{}{}
	require.NoError(t, writer.Drain(t.Context()))
	assert.Equal(t, []uint64{first.GetTickHeight(), second.GetTickHeight()}, storage.storedTicks())
}

// TestAsyncWriterSurvivesStoragePanic verifies that a storage panic does not stop the worker.
func TestAsyncWriterSurvivesStoragePanic(t *testing.T) {
	t.Parallel()
	storage := &panickingWriterStorage{panics: 1}
	writer := newTestAsyncWriter(t, storage)
	prng := testutils.NewRand(t)

	first := randomSnapshot(prng)
	first.TickHeight = 1
	writer.Write(first)
	err := writer.Drain(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "panicked")

	second := randomSnapshot(prng)
	second.TickHeight = 2
	writer.Write(second)
	require.NoError(t, writer.Drain(t.Context()))
	assert.Equal(t, []uint64{second.GetTickHeight()}, storage.storedTicks())
}

// -------------------------------------------------------------------------------------------------
// Test fixtures
// -------------------------------------------------------------------------------------------------

type blockingWriterStorage struct {
	entered  chan uint64
	returned chan uint64
	release  chan struct{}

	mu     sync.Mutex
	stored []uint64
}

var _ Storage = (*blockingWriterStorage)(nil)

func newBlockingWriterStorage() *blockingWriterStorage {
	return &blockingWriterStorage{
		entered:  make(chan uint64, 16),
		returned: make(chan uint64, 16),
		release:  make(chan struct{}, 16),
	}
}

func (s *blockingWriterStorage) Store(ctx context.Context, snap *cardinalv1.Snapshot) error {
	s.entered <- snap.GetTickHeight()
	defer func() { s.returned <- snap.GetTickHeight() }()
	select {
	case <-s.release:
	case <-ctx.Done():
		return ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.stored = append(s.stored, snap.GetTickHeight())
	return nil
}

func (*blockingWriterStorage) Load(context.Context) (*cardinalv1.Snapshot, error) {
	return nil, ErrSnapshotNotFound
}

func (s *blockingWriterStorage) waitEntered(t *testing.T) uint64 {
	t.Helper()
	select {
	case tickHeight := <-s.entered:
		return tickHeight
	case <-time.After(5 * time.Second):
		t.Fatal("no snapshot storage operation started")
		return 0
	}
}

func (s *blockingWriterStorage) storedTicks() []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint64(nil), s.stored...)
}

func newTestAsyncWriter(t *testing.T, storage Storage) *AsyncWriter {
	t.Helper()
	tel := &telemetry.Telemetry{Logger: zerolog.Nop()}
	writer := NewAsyncWriter(storage, tel)
	t.Cleanup(func() { writer.Stop(context.Background()) })
	return writer
}

type observedWriterContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func newObservedWriterContext() *observedWriterContext {
	return &observedWriterContext{
		Context: context.Background(),
		entered: make(chan struct{}),
	}
}

func (c *observedWriterContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

type sequencedErrorWriterStorage struct {
	errors  []error
	entered chan int
	release chan struct{}

	mu   sync.Mutex
	next int
}

func newSequencedErrorWriterStorage(errs ...error) *sequencedErrorWriterStorage {
	return &sequencedErrorWriterStorage{
		errors:  errs,
		entered: make(chan int, len(errs)),
		release: make(chan struct{}, len(errs)),
	}
}

func (s *sequencedErrorWriterStorage) Store(ctx context.Context, _ *cardinalv1.Snapshot) error {
	s.mu.Lock()
	index := s.next
	s.next++
	err := s.errors[index]
	s.mu.Unlock()

	s.entered <- index
	select {
	case <-s.release:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (*sequencedErrorWriterStorage) Load(context.Context) (*cardinalv1.Snapshot, error) {
	return nil, ErrSnapshotNotFound
}

func (s *sequencedErrorWriterStorage) waitEntered(t *testing.T) int {
	t.Helper()
	select {
	case index := <-s.entered:
		return index
	case <-time.After(5 * time.Second):
		t.Fatal("no snapshot storage operation started")
		return 0
	}
}

type panickingWriterStorage struct {
	mu     sync.Mutex
	panics int
	stored []uint64
}

func (s *panickingWriterStorage) Store(_ context.Context, snap *cardinalv1.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.panics > 0 {
		s.panics--
		panic("storage failed")
	}
	s.stored = append(s.stored, snap.GetTickHeight())
	return nil
}

func (*panickingWriterStorage) Load(context.Context) (*cardinalv1.Snapshot, error) {
	return nil, ErrSnapshotNotFound
}

func (s *panickingWriterStorage) storedTicks() []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint64(nil), s.stored...)
}
