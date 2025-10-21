package ecs

import (
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemEventManager_Register(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		systemEvent SystemEvent
		wantErr     bool
	}{
		{
			name:        "successful registration",
			systemEvent: PlayerDeathSystemEvent{},
		},
		{
			name:        "empty system event name",
			systemEvent: InvalidEmptyCommand{}, // Reusing this for empty Name() implementation
			wantErr:     true,
		},
		{
			name:        "duplicate system event name returns same ID",
			systemEvent: PlayerDeathSystemEvent{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sem := newSystemEventManager()

			if tt.name == "duplicate system event name returns same ID" {
				_, err := sem.register(tt.systemEvent.Name())
				require.NoError(t, err)
			}

			firstID, err := sem.register(tt.systemEvent.Name())
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			storedID, exists := sem.registry[tt.systemEvent.Name()]

			assert.True(t, exists, "Event should be registered")
			assert.Equal(t, firstID, storedID, "ID in registry should match returned ID")
			assert.Len(t, sem.events, int(sem.nextID), "Events slice should match nextID")
		})
	}
}

func TestSystemEventManager_EnqueueAndGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFn    func(*testing.T, *systemEventManager)
		names      []string
		validateFn func(*testing.T, []any)
		wantErr    bool
	}{
		{
			name: "enqueue and get single event type",
			setupFn: func(t *testing.T, sem *systemEventManager) {
				_, err := sem.register(PlayerDeathSystemEvent{}.Name())
				require.NoError(t, err)

				for i := range 10 {
					err := sem.enqueue(PlayerDeathSystemEvent{}.Name(), PlayerDeathSystemEvent{Value: i})
					require.NoError(t, err)
				}
			},
			names: []string{PlayerDeathSystemEvent{}.Name()},
			validateFn: func(t *testing.T, events []any) {
				require.Len(t, events, 10)
				for i, evt := range events {
					event, ok := evt.(PlayerDeathSystemEvent)
					assert.True(t, ok)
					assert.Equal(t, i, event.Value)
				}
			},
		},
		{
			name: "enqueue and get multiple event types",
			setupFn: func(t *testing.T, sem *systemEventManager) {
				_, err := sem.register(PlayerDeathSystemEvent{}.Name())
				require.NoError(t, err)

				_, err = sem.register(ItemDropSystemEvent{}.Name())
				require.NoError(t, err)

				for i := range 10 {
					err := sem.enqueue(PlayerDeathSystemEvent{}.Name(), PlayerDeathSystemEvent{Value: i})
					require.NoError(t, err)

					err = sem.enqueue(ItemDropSystemEvent{}.Name(), ItemDropSystemEvent{Value: i + 100})
					require.NoError(t, err)
				}
			},
			names: []string{PlayerDeathSystemEvent{}.Name(), ItemDropSystemEvent{}.Name()},
			validateFn: func(t *testing.T, events []any) {
				require.Len(t, events, 20)
				for i, evt := range events[:10] {
					event, ok := evt.(PlayerDeathSystemEvent)
					assert.True(t, ok)
					assert.Equal(t, i, event.Value)
				}
				for i, evt := range events[10:] {
					event, ok := evt.(ItemDropSystemEvent)
					assert.True(t, ok)
					assert.Equal(t, i+100, event.Value)
				}
			},
		},
		{
			name: "get unregistered event returns error",
			setupFn: func(t *testing.T, sem *systemEventManager) {
				// Don't register anything
			},
			names:   []string{PlayerDeathSystemEvent{}.Name()},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sem := newSystemEventManager()
			tt.setupFn(t, &sem)

			var events []any
			for _, name := range tt.names {
				evts, err := sem.get(name)
				if tt.wantErr {
					assert.Error(t, err)
					return
				}
				events = append(events, evts...)
			}

			tt.validateFn(t, events)
		})
	}
}

func TestSystemEventManager_Clear(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setupFn func(*testing.T, *systemEventManager) []string
		testFn  func(*testing.T, *systemEventManager, []string)
	}{
		{
			name: "clears multiple event types",
			setupFn: func(t *testing.T, sem *systemEventManager) []string {
				_, err := sem.register(PlayerDeathSystemEvent{}.Name())
				require.NoError(t, err)

				_, err = sem.register(ItemDropSystemEvent{}.Name())
				require.NoError(t, err)

				for i := range 20 {
					err := sem.enqueue(PlayerDeathSystemEvent{}.Name(), PlayerDeathSystemEvent{Value: i})
					require.NoError(t, err)

					err = sem.enqueue(ItemDropSystemEvent{}.Name(), ItemDropSystemEvent{Value: i})
					require.NoError(t, err)
				}

				return []string{
					PlayerDeathSystemEvent{}.Name(),
					ItemDropSystemEvent{}.Name(),
				}
			},
			testFn: func(t *testing.T, sem *systemEventManager, eventNames []string) {
				for _, name := range eventNames {
					events, err := sem.get(name)
					require.NoError(t, err)
					assert.Empty(t, events, "Events for %s should be empty after clear", name)
				}
			},
		},
		{
			name: "can add events after clearing",
			setupFn: func(t *testing.T, sem *systemEventManager) []string {
				_, err := sem.register(PlayerDeathSystemEvent{}.Name())
				require.NoError(t, err)

				for i := range 10 {
					err := sem.enqueue(PlayerDeathSystemEvent{}.Name(), PlayerDeathSystemEvent{Value: i})
					require.NoError(t, err)
				}

				return []string{PlayerDeathSystemEvent{}.Name()}
			},
			testFn: func(t *testing.T, sem *systemEventManager, eventNames []string) {
				// Add new events after clear
				err := sem.enqueue(eventNames[0], PlayerDeathSystemEvent{Value: 100})
				require.NoError(t, err)

				// Verify new events were added
				events, err := sem.get(eventNames[0])
				require.NoError(t, err)
				require.Len(t, events, 1)

				event, ok := events[0].(PlayerDeathSystemEvent)
				assert.True(t, ok)
				assert.Equal(t, 100, event.Value)
			},
		},
		{
			name: "clear works on empty event lists",
			setupFn: func(t *testing.T, sem *systemEventManager) []string {
				_, err := sem.register(PlayerDeathSystemEvent{}.Name())
				require.NoError(t, err)
				return []string{PlayerDeathSystemEvent{}.Name()}
			},
			testFn: func(t *testing.T, sem *systemEventManager, eventNames []string) {
				events, err := sem.get(eventNames[0])
				require.NoError(t, err)
				assert.Empty(t, events)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sem := newSystemEventManager()
			eventNames := tt.setupFn(t, &sem)

			sem.clear()

			tt.testFn(t, &sem, eventNames)
		})
	}
}
