package ecs

import (
	"sync"
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventManager_EnqueueAndGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFn    func(*testing.T, *eventManager)
		validateFn func(*testing.T, []RawEvent)
	}{
		{
			name:    "empty manager returns empty slice",
			setupFn: func(t *testing.T, em *eventManager) {},
			validateFn: func(t *testing.T, events []RawEvent) {
				assert.Empty(t, events)
				assert.NotNil(t, events)
			},
		},
		{
			name: "single event type",
			setupFn: func(t *testing.T, em *eventManager) {
				for i := range 100 {
					em.enqueue(EventKindDefault, PlayerDeathEvent{Value: i})
				}
			},
			validateFn: func(t *testing.T, events []RawEvent) {
				require.Len(t, events, 100)

				for i := range 100 {
					assert.Equal(t, EventKindDefault, events[i].Kind)

					event, ok := events[i].Payload.(PlayerDeathEvent)
					assert.True(t, ok)
					assert.Equal(t, i, event.Value)
				}
			},
		},
		{
			name: "multiple event types",
			setupFn: func(t *testing.T, em *eventManager) {
				// Enqueue alternating event types
				for i := range 50 {
					em.enqueue(EventKindDefault, PlayerDeathEvent{Value: i})
					em.enqueue(EventKindDefault, ItemDropEvent{Value: i + 100})
				}
			},
			validateFn: func(t *testing.T, events []RawEvent) {
				require.Len(t, events, 100)

				for i := range 50 {
					// Check PlayerDeathEvent
					assert.Equal(t, EventKindDefault, events[i*2].Kind)
					death, ok := events[i*2].Payload.(PlayerDeathEvent)
					assert.True(t, ok)
					assert.Equal(t, i, death.Value)

					// Check ItemDropEvent
					assert.Equal(t, EventKindDefault, events[i*2+1].Kind)
					drop, ok := events[i*2+1].Payload.(ItemDropEvent)
					assert.True(t, ok)
					assert.Equal(t, i+100, drop.Value)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			em := newEventManager()
			tt.setupFn(t, &em)

			events := em.getEvents()

			tt.validateFn(t, events)
		})
	}
}

func TestEventManager_Clear(t *testing.T) {
	t.Parallel()

	em := newEventManager()

	for i := range 50 {
		em.enqueue(EventKindDefault, PlayerDeathEvent{Value: i})
	}

	// Verify events were enqueued
	events := em.getEvents()
	require.Len(t, events, 50)

	// Clear and verify buffer is empty
	em.clear()
	assert.Empty(t, em.buffer)

	// Verify empty channel returns empty slice
	emptyEvents := em.getEvents()
	assert.Empty(t, emptyEvents)

	// Verify clear still works with empty event manager
	em.clear()
	assert.Empty(t, em.buffer)
}

func TestEventManager_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	em := newEventManager()

	// Test concurrent enqueueing of events
	const numGoroutines = 10
	const eventsPerGoroutine = 100
	totalEvents := numGoroutines * eventsPerGoroutine

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			for j := range eventsPerGoroutine {
				if j%2 == 0 {
					// Even indices: use PlayerDeathEvent
					em.enqueue(EventKindDefault, PlayerDeathEvent{Value: id*1000 + j})
				} else {
					// Odd indices: use ItemDropEvent
					em.enqueue(EventKindDefault, ItemDropEvent{Value: id*1000 + j})
				}
			}
		}(i)
	}
	wg.Wait()

	// Verify we got the expected number of events
	events := em.getEvents()
	assert.Len(t, events, totalEvents)

	// Verify we have both event types
	hasDeathEvent := false
	hasDropEvent := false

	for _, evt := range events {
		switch evt.Payload.(type) {
		case PlayerDeathEvent:
			hasDeathEvent = true
		case ItemDropEvent:
			hasDropEvent = true
		}

		// Once we've found both types, we can break early
		if hasDeathEvent && hasDropEvent {
			break
		}
	}

	assert.True(t, hasDeathEvent)
	assert.True(t, hasDropEvent)
}
