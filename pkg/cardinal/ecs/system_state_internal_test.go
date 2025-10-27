package ecs

import (
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithCommand_Init(t *testing.T) {
	t.Parallel()

	t.Run("successful initialization", func(t *testing.T) {
		t.Parallel()
		w := NewWorld()

		wc := &WithCommand[AttackPlayerCommand]{}
		_, err := wc.init(w)

		require.NoError(t, err)
		assert.Equal(t, w, wc.world, "Command field should store the world reference")
	})

	t.Run("invalid command name", func(t *testing.T) {
		t.Parallel()

		t.Run("empty command name", func(t *testing.T) {
			t.Parallel()
			w := NewWorld()

			wc := &WithCommand[InvalidEmptyCommand]{}
			_, err := wc.init(w)

			require.Error(t, err)
		})

		t.Run("automatically registers unregistered commands", func(t *testing.T) {
			t.Parallel()
			w := NewWorld()

			// Before: check that command is not already registered
			commandName := AttackPlayerCommand{}.Name()
			_, exists := w.commands.registry[commandName]
			assert.False(t, exists, "Command should not be registered initially")

			// Register command through init
			wc := &WithCommand[AttackPlayerCommand]{}
			_, err := wc.init(w)

			// Check results
			require.NoError(t, err, "Should automatically register the command")
			_, exists = w.commands.registry[commandName]
			assert.True(t, exists, "Command should be registered after init")
		})

		t.Run("handles duplicate registrations", func(t *testing.T) {
			t.Parallel()
			w := NewWorld()

			// First registration through init
			wc1 := &WithCommand[AttackPlayerCommand]{}
			_, err := wc1.init(w)
			require.NoError(t, err, "First registration should succeed")

			// Second registration through init
			wc2 := &WithCommand[AttackPlayerCommand]{}
			_, err = wc2.init(w)

			// Verify results
			require.NoError(t, err, "Second registration should not error")
			_, exists := w.commands.registry[AttackPlayerCommand{}.Name()]
			assert.True(t, exists, "Command should still be registered")
		})
	})
}

func TestWithCommand_Iter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFn    func(*testing.T, *World)
		validateFn func(*testing.T, []AttackPlayerCommand)
	}{
		{
			name:    "empty - no commands",
			setupFn: func(t *testing.T, w *World) {},
			validateFn: func(t *testing.T, results []AttackPlayerCommand) {
				assert.Empty(t, results, "Should return empty results when no commands exist")
			},
		},
		{
			name: "basic iteration",
			setupFn: func(t *testing.T, w *World) {
				var commands []micro.Command
				for i := range 5 {
					cmd := AttackPlayerCommand{Value: i}
					commands = append(commands, micro.Command{Command: micro.CommandRaw{Body: micro.CommandBody{
						Name: cmd.Name(), Payload: cmd}}})
				}
				err := w.commands.receiveCommands(commands)
				require.NoError(t, err)
			},
			validateFn: func(t *testing.T, results []AttackPlayerCommand) {
				require.Len(t, results, 5, "Should iterate all commands")
				for i, cmd := range results {
					assert.Equal(t, i, cmd.Value, "Command values should match")
				}
			},
		},
		{
			name: "mixed command types",
			setupFn: func(t *testing.T, w *World) {
				// Register the additional command type.
				wc := &WithCommand[CreatePlayerCommand]{}
				_, err := wc.init(w)
				require.NoError(t, err)

				var commands []micro.Command
				for i := range 10 { // Add attack commands
					cmd := AttackPlayerCommand{Value: i}
					commands = append(commands, micro.Command{Command: micro.CommandRaw{Body: micro.CommandBody{
						Name: cmd.Name(), Payload: cmd}}})
				}
				for i := range 5 { // Add create commands (should be filtered out)
					cmd := CreatePlayerCommand{Value: i}
					commands = append(commands, micro.Command{Command: micro.CommandRaw{Body: micro.CommandBody{
						Name: cmd.Name(), Payload: cmd}}})
				}
				err = w.commands.receiveCommands(commands)
				require.NoError(t, err)
			},
			validateFn: func(t *testing.T, results []AttackPlayerCommand) {
				require.Len(t, results, 10, "Should only get the correct command type")
				for i, cmd := range results {
					assert.Equal(t, i, cmd.Value)
				}
			},
		},
		{
			name: "early termination",
			setupFn: func(t *testing.T, w *World) {
				var commands []micro.Command
				for i := range 10 {
					cmd := AttackPlayerCommand{Value: i}
					commands = append(commands, micro.Command{Command: micro.CommandRaw{Body: micro.CommandBody{
						Name: cmd.Name(), Payload: cmd}}})
				}
				err := w.commands.receiveCommands(commands)
				require.NoError(t, err)
			},
			validateFn: func(t *testing.T, results []AttackPlayerCommand) {
				require.Len(t, results, 3, "Should only get first 3 commands")
				for i, cmd := range results {
					assert.Equal(t, i, cmd.Value)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := NewWorld()

			// Initialize WithCommand to register the command with the world.
			wc := &WithCommand[AttackPlayerCommand]{}
			_, err := wc.init(w)
			require.NoError(t, err)

			tt.setupFn(t, w)

			var results []AttackPlayerCommand

			if tt.name == "early termination" {
				iter := wc.Iter()
				count := 0
				iter(func(ctx CommandContext[AttackPlayerCommand]) bool {
					results = append(results, ctx.Payload())
					count++
					return count < 3 // Return false after 3 items
				})
			} else {
				for ctx := range wc.Iter() {
					results = append(results, ctx.Payload())
				}
			}
			tt.validateFn(t, results)
		})
	}
}

func TestWithEvent_Init(t *testing.T) {
	t.Parallel()

	w := NewWorld()
	we := &WithEvent[PlayerDeathEvent]{}

	// Initialize the WithEvent field
	_, err := we.init(w)
	require.NoError(t, err)

	// Verify world reference is set
	assert.Equal(t, w, we.world)
}

func TestWithEvent_Emit(t *testing.T) {
	t.Parallel()

	w := NewWorld()
	we := &WithEvent[PlayerDeathEvent]{}
	_, err := we.init(w)
	require.NoError(t, err)

	for i := range 5 {
		we.Emit(PlayerDeathEvent{Value: i})
	}

	// Get and verify events
	events := w.events.getEvents()
	require.Len(t, events, 5)

	for i, event := range events {
		assert.Equal(t, EventKindDefault, event.Kind)

		payload, ok := event.Payload.(PlayerDeathEvent)
		assert.True(t, ok)
		assert.Equal(t, i, payload.Value)
	}
}

func TestWithSystemEventReceiver_Init(t *testing.T) {
	t.Parallel()

	t.Run("auto-registers new system event", func(t *testing.T) {
		t.Parallel()
		w := NewWorld()

		// Record initial event count
		beforeCount := len(w.systemEvents.registry)

		ser := &WithSystemEventReceiver[PlayerDeathSystemEvent]{}
		_, err := ser.init(w)

		require.NoError(t, err)
		assert.Equal(t, w, ser.world, "Should store world reference")

		// Verify event was registered
		afterCount := len(w.systemEvents.registry)
		assert.Equal(t, beforeCount+1, afterCount, "System event should be registered")
	})

	t.Run("reuses already registered event", func(t *testing.T) {
		t.Parallel()
		w := NewWorld()

		// Pre-register the event
		_, err := w.systemEvents.register(PlayerDeathSystemEvent{}.Name())
		require.NoError(t, err)

		beforeCount := len(w.systemEvents.registry)

		ser := &WithSystemEventReceiver[PlayerDeathSystemEvent]{}
		_, err = ser.init(w)

		require.NoError(t, err)
		assert.Equal(t, w, ser.world, "Should store world reference")

		// Verify no new event was registered
		afterCount := len(w.systemEvents.registry)
		assert.Equal(t, beforeCount, afterCount, "No new event should be registered")
	})

	t.Run("fails with empty event name", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()

		ser := &WithSystemEventReceiver[InvalidEmptyCommand]{}
		_, err := ser.init(w)
		require.Error(t, err)
	})
}

func TestWithSystemEventReceiver_Iter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFn    func(*testing.T, *World)
		validateFn func(*testing.T, []PlayerDeathSystemEvent)
	}{
		{
			name: "empty - no events",
			setupFn: func(t *testing.T, w *World) {
				// Register but don't add any events
				_, err := w.systemEvents.register(PlayerDeathSystemEvent{}.Name())
				require.NoError(t, err)
			},
			validateFn: func(t *testing.T, results []PlayerDeathSystemEvent) {
				assert.Empty(t, results, "Should return empty results when no events exist")
			},
		},
		{
			name: "basic iteration",
			setupFn: func(t *testing.T, w *World) {
				id, err := w.systemEvents.register(PlayerDeathSystemEvent{}.Name())
				require.NoError(t, err)

				// Add some test events
				for i := range 5 {
					w.systemEvents.events[id] = append(w.systemEvents.events[id], PlayerDeathSystemEvent{Value: i})
				}
			},
			validateFn: func(t *testing.T, results []PlayerDeathSystemEvent) {
				require.Len(t, results, 5, "Should iterate all events")
				for i, event := range results {
					assert.Equal(t, i, event.Value, "Event values should match")
				}
			},
		},
		{
			name: "mixed event types",
			setupFn: func(t *testing.T, w *World) {
				deathID, err := w.systemEvents.register(PlayerDeathSystemEvent{}.Name())
				require.NoError(t, err)
				dropID, err := w.systemEvents.register(ItemDropSystemEvent{}.Name())
				require.NoError(t, err)

				// Add death events
				for i := range 100 {
					w.systemEvents.events[deathID] = append(w.systemEvents.events[deathID],
						PlayerDeathSystemEvent{Value: i})
				}

				// Add drop events (should be filtered out by iterator)
				for i := range 50 {
					w.systemEvents.events[dropID] = append(w.systemEvents.events[dropID],
						ItemDropSystemEvent{Value: i})
				}
			},
			validateFn: func(t *testing.T, results []PlayerDeathSystemEvent) {
				require.Len(t, results, 100, "Should only get the correct event type")
				for i, event := range results {
					assert.Equal(t, i, event.Value, "Event values should match")
				}
			},
		},
		{
			name: "early termination",
			setupFn: func(t *testing.T, w *World) {
				id, err := w.systemEvents.register(PlayerDeathSystemEvent{}.Name())
				require.NoError(t, err)

				// Add many events
				for i := range 10 {
					w.systemEvents.events[id] = append(w.systemEvents.events[id],
						PlayerDeathSystemEvent{Value: i})
				}
			},
			validateFn: func(t *testing.T, results []PlayerDeathSystemEvent) {
				require.Len(t, results, 3, "Should only get first 3 events")
				for i, event := range results {
					assert.Equal(t, i, event.Value, "Event values should match")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := NewWorld()
			tt.setupFn(t, w)

			ser := &WithSystemEventReceiver[PlayerDeathSystemEvent]{}
			_, err := ser.init(w)
			require.NoError(t, err)

			var results []PlayerDeathSystemEvent

			if tt.name == "early termination" {
				iter := ser.Iter()
				count := 0
				iter(func(event PlayerDeathSystemEvent) bool {
					results = append(results, event)
					count++
					return count < 3 // Return false after 3 items
				})
			} else {
				for event := range ser.Iter() {
					results = append(results, event)
				}
			}
			tt.validateFn(t, results)
		})
	}
}

func TestWithSystemEventEmitter_Init(t *testing.T) {
	t.Parallel()

	// Test successful initialization with auto-registration
	t.Run("auto-registers new system event", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()

		// Record initial event count
		beforeCount := len(w.systemEvents.registry)

		see := &WithSystemEventEmitter[PlayerDeathSystemEvent]{}
		_, err := see.init(w)

		require.NoError(t, err)
		assert.Equal(t, w, see.world, "Should store world reference")

		// Verify event was registered
		afterCount := len(w.systemEvents.registry)
		assert.Equal(t, beforeCount+1, afterCount, "System event should be registered")

		// Verify event name is in registry
		_, exists := w.systemEvents.registry[PlayerDeathSystemEvent{}.Name()]
		assert.True(t, exists, "Event should be registered with correct name")
	})

	// Test with already registered event
	t.Run("reuses already registered event", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()

		// Pre-register the event
		_, err := w.systemEvents.register(PlayerDeathSystemEvent{}.Name())
		require.NoError(t, err)

		beforeCount := len(w.systemEvents.registry)

		see := &WithSystemEventEmitter[PlayerDeathSystemEvent]{}
		_, err = see.init(w)

		require.NoError(t, err)
		assert.Equal(t, w, see.world, "Should store world reference")

		// Verify no new event was registered
		afterCount := len(w.systemEvents.registry)
		assert.Equal(t, beforeCount, afterCount, "No new event should be registered")
	})

	// Test with empty event name
	t.Run("fails with empty event name", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()

		see := &WithSystemEventEmitter[InvalidEmptyCommand]{}
		_, err := see.init(w)
		require.Error(t, err)
	})
}

func TestWithSystemEventEmitter_Emit(t *testing.T) {
	t.Parallel()

	w := NewWorld()

	see := &WithSystemEventEmitter[PlayerDeathSystemEvent]{world: w}
	_, err := see.init(w)
	require.NoError(t, err)

	for i := range 5 {
		see.Emit(PlayerDeathSystemEvent{Value: i})
	}

	events, err := w.systemEvents.get(PlayerDeathSystemEvent{}.Name())
	require.NoError(t, err)
	require.Len(t, events, 5)

	for i, event := range events {
		evt, ok := event.(PlayerDeathSystemEvent)
		assert.True(t, ok)
		assert.Equal(t, i, evt.Value)
	}
}

type initSystemState struct {
	Position       Exact[struct{ Ref[Position] }]
	Health         Exact[struct{ Ref[Health] }]
	Velocity       Exact[struct{ Ref[Velocity] }]
	PositionHealth Exact[struct {
		Position Ref[Position]
		Health   Ref[Health]
	}]
	PositionVelocity Exact[struct {
		Position Ref[Position]
		Velocity Ref[Velocity]
	}]
	HealthVelocity Exact[struct {
		Health   Ref[Health]
		Velocity Ref[Velocity]
	}]
}

func TestContains_Iter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		search     systemStateField
		setupFn    func(*initSystemState) error
		getResult  func(systemStateField) []any
		validateFn func(*testing.T, []any)
	}{
		{
			name:    "empty world",
			search:  &Contains[struct{ Ref[Position] }]{},
			setupFn: func(state *initSystemState) error { return nil },
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Contains[struct{ Ref[Position] }])
				for _, pos := range search.Iter() {
					result = append(result, pos.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, result []any) {
				assert.Empty(t, result)
			},
		},
		{
			name:   "no matching components",
			search: &Contains[struct{ Ref[Position] }]{},
			setupFn: func(state *initSystemState) error {
				_, velocity := state.Velocity.Create()
				velocity.Set(Velocity{})

				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Contains[struct{ Ref[Position] }])
				for _, pos := range search.Iter() {
					result = append(result, pos.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, result []any) {
				assert.Empty(t, result)
			},
		},
		{
			name:   "matching components",
			search: &Contains[struct{ Ref[Position] }]{},
			setupFn: func(state *initSystemState) error {
				for i := range 20 {
					if i%2 == 0 {
						_, position := state.Position.Create()
						position.Set(Position{X: i, Y: i})
					} else {
						_, velocity := state.Velocity.Create()
						velocity.Set(Velocity{X: i, Y: i})
					}
				}
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Contains[struct{ Ref[Position] }])
				for _, pos := range search.Iter() {
					result = append(result, pos.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, results []any) {
				assert.Len(t, results, 10)
				for i, result := range results {
					pos, ok := result.(Position)
					assert.True(t, ok)
					assert.Equal(t, Position{X: i * 2, Y: i * 2}, pos)
				}
			},
		},
		{
			name:   "early return",
			search: &Contains[struct{ Ref[Position] }]{},
			setupFn: func(state *initSystemState) error {
				for i := range 10 {
					_, position := state.Position.Create()
					position.Set(Position{X: i, Y: i})
				}
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				count := 0

				search := s.(*Contains[struct{ Ref[Position] }])
				iter := search.Iter()
				iter(func(_ EntityID, pos struct{ Ref[Position] }) bool {
					result = append(result, pos.Get())
					count++
					return count < 3 // Stop after collecting 3 items
				})

				return result
			},
			validateFn: func(t *testing.T, results []any) {
				assert.Len(t, results, 3, "Should only get first 3 positions")
				for i, result := range results {
					pos := result.(Position)
					assert.Equal(t, i, pos.X, "Position X should match index")
					assert.Equal(t, i, pos.Y, "Position Y should match index")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := NewWorld()

			_, err := tt.search.init(w)
			require.NoError(t, err)

			RegisterSystem(w, tt.setupFn, WithHook(Init))

			w.InitSchedulers()

			err = w.InitSystems()
			require.NoError(t, err)

			results := tt.getResult(tt.search)

			tt.validateFn(t, results)
		})
	}
}

func TestContains_Iter2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		search     systemStateField
		setupFn    func(*initSystemState) error
		getResult  func(systemStateField) []any
		validateFn func(*testing.T, []any)
	}{
		{
			name: "empty world",
			search: &Contains[struct {
				Position Ref[Position]
				Velocity Ref[Velocity]
			}]{},
			setupFn: func(state *initSystemState) error { return nil },
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Contains[struct {
					Position Ref[Position]
					Velocity Ref[Velocity]
				}])
				for _, comps := range search.Iter() {
					result = append(result, comps.Position.Get(), comps.Velocity.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, result []any) {
				assert.Empty(t, result)
			},
		},
		{
			name: "no matching components",
			search: &Contains[struct {
				Position Ref[Position]
				Velocity Ref[Velocity]
			}]{},
			setupFn: func(state *initSystemState) error {
				_, position := state.Position.Create()
				position.Set(Position{X: 1, Y: 1})

				_, velocity := state.Velocity.Create()
				velocity.Set(Velocity{X: 2, Y: 2})
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Contains[struct {
					Position Ref[Position]
					Velocity Ref[Velocity]
				}])
				for _, comps := range search.Iter() {
					result = append(result, comps.Position.Get(), comps.Velocity.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, result []any) {
				assert.Empty(t, result)
			},
		},
		{
			name: "matching components",
			search: &Contains[struct {
				Position Ref[Position]
				Velocity Ref[Velocity]
			}]{},
			setupFn: func(state *initSystemState) error {
				for i := range 10 {
					if i%2 == 0 {
						_, posVel := state.PositionVelocity.Create()
						posVel.Position.Set(Position{X: i, Y: i})
						posVel.Velocity.Set(Velocity{X: i * 10, Y: i * 10})
					} else {
						_, position := state.Position.Create()
						position.Set(Position{X: i, Y: i})
					}
				}
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Contains[struct {
					Position Ref[Position]
					Velocity Ref[Velocity]
				}])
				for _, comps := range search.Iter() {
					result = append(result, comps.Position.Get(), comps.Velocity.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, results []any) {
				assert.Len(t, results, 10) // 5 entities * 2 components
				for i := 0; i < len(results); i += 2 {
					pos := results[i].(Position)
					vel := results[i+1].(Velocity)
					idx := i / 2 * 2 // Convert to the original index (0, 2, 4, 6, 8)
					assert.Equal(t, idx, pos.X, "Position X should match index")
					assert.Equal(t, idx, pos.Y, "Position Y should match index")
					assert.Equal(t, idx*10, vel.X, "Velocity X should match index*10")
					assert.Equal(t, idx*10, vel.Y, "Velocity Y should match index*10")
				}
			},
		},
		{
			name: "early return",
			search: &Contains[struct {
				Position Ref[Position]
				Velocity Ref[Velocity]
			}]{},
			setupFn: func(state *initSystemState) error {
				for i := range 10 {
					_, posVel := state.PositionVelocity.Create()
					posVel.Position.Set(Position{X: i, Y: i})
					posVel.Velocity.Set(Velocity{X: i * 10, Y: i * 10})
				}
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				count := 0

				search := s.(*Contains[struct {
					Position Ref[Position]
					Velocity Ref[Velocity]
				}])
				iter := search.Iter()
				iter(func(_ EntityID, comps struct {
					Position Ref[Position]
					Velocity Ref[Velocity]
				}) bool {
					result = append(result, comps.Position.Get(), comps.Velocity.Get())
					count++
					return count < 3 // Stop after collecting 3 items
				})

				return result
			},
			validateFn: func(t *testing.T, results []any) {
				assert.Len(t, results, 6, "Should get 3 entities with 2 components each")
				for i := 0; i < len(results); i += 2 {
					pos := results[i].(Position)
					vel := results[i+1].(Velocity)
					idx := i / 2
					assert.Equal(t, idx, pos.X, "Position X should match index")
					assert.Equal(t, idx, pos.Y, "Position Y should match index")
					assert.Equal(t, idx*10, vel.X, "Velocity X should match index*10")
					assert.Equal(t, idx*10, vel.Y, "Velocity Y should match index*10")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := NewWorld()

			_, err := tt.search.init(w)
			require.NoError(t, err)

			RegisterSystem(w, tt.setupFn, WithHook(Init))

			w.InitSchedulers()

			err = w.InitSystems()
			require.NoError(t, err)

			results := tt.getResult(tt.search)

			tt.validateFn(t, results)
		})
	}
}

func TestExact_Iter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		search     systemStateField
		setupFn    func(*initSystemState) error
		getResult  func(systemStateField) []any
		validateFn func(*testing.T, []any)
	}{
		{
			name:    "empty world",
			search:  &Exact[struct{ Ref[Position] }]{},
			setupFn: func(state *initSystemState) error { return nil },
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Exact[struct{ Ref[Position] }])
				for _, pos := range search.Iter() {
					result = append(result, pos.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, result []any) {
				assert.Empty(t, result)
			},
		},
		{
			name:   "no matching components",
			search: &Exact[struct{ Ref[Position] }]{},
			setupFn: func(state *initSystemState) error {
				_, velocity := state.Velocity.Create()
				velocity.Set(Velocity{})

				_, posVel := state.PositionVelocity.Create()
				posVel.Position.Set(Position{X: 5, Y: 5})
				posVel.Velocity.Set(Velocity{})
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Exact[struct{ Ref[Position] }])
				for _, pos := range search.Iter() {
					result = append(result, pos.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, result []any) {
				assert.Empty(t, result)
			},
		},
		{
			name:   "matching components",
			search: &Exact[struct{ Ref[Position] }]{},
			setupFn: func(state *initSystemState) error {
				for i := range 20 {
					if i%2 == 0 {
						_, position := state.Position.Create()
						position.Set(Position{X: i, Y: i})
					} else {
						_, velocity := state.Velocity.Create()
						velocity.Set(Velocity{X: i, Y: i})
					}
				}
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Exact[struct{ Ref[Position] }])
				for _, pos := range search.Iter() {
					result = append(result, pos.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, results []any) {
				assert.Len(t, results, 10)
				for i, result := range results {
					pos := result.(Position)
					expectedX := i * 2 // Only even indices should match
					assert.Equal(t, expectedX, pos.X, "Position X should match index")
					assert.Equal(t, expectedX, pos.Y, "Position Y should match index")
				}
			},
		},
		{
			name:   "early return",
			search: &Exact[struct{ Ref[Position] }]{},
			setupFn: func(state *initSystemState) error {
				for i := range 10 {
					_, position := state.Position.Create()
					position.Set(Position{X: i, Y: i})
				}
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				count := 0

				search := s.(*Exact[struct{ Ref[Position] }])
				iter := search.Iter()
				iter(func(_ EntityID, pos struct{ Ref[Position] }) bool {
					result = append(result, pos.Get())
					count++
					return count < 3 // Stop after collecting 3 items
				})

				return result
			},
			validateFn: func(t *testing.T, results []any) {
				assert.Len(t, results, 3, "Should only get first 3 positions")
				for i, result := range results {
					pos := result.(Position)
					assert.Equal(t, i, pos.X, "Position X should match index")
					assert.Equal(t, i, pos.Y, "Position Y should match index")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := NewWorld()

			_, err := tt.search.init(w)
			require.NoError(t, err)

			RegisterSystem(w, tt.setupFn, WithHook(Init))

			w.InitSchedulers()

			err = w.InitSystems()
			require.NoError(t, err)

			results := tt.getResult(tt.search)

			tt.validateFn(t, results)
		})
	}
}

func TestExact_Iter2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		search     systemStateField
		setupFn    func(*initSystemState) error
		getResult  func(systemStateField) []any
		validateFn func(*testing.T, []any)
	}{
		{
			name: "empty world",
			search: &Exact[struct {
				Position Ref[Position]
				Velocity Ref[Velocity]
			}]{},
			setupFn: func(state *initSystemState) error { return nil },
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Exact[struct {
					Position Ref[Position]
					Velocity Ref[Velocity]
				}])
				for _, comps := range search.Iter() {
					result = append(result, comps.Position.Get(), comps.Velocity.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, result []any) {
				assert.Empty(t, result)
			},
		},
		{
			name: "no matching components",
			search: &Exact[struct {
				Position Ref[Position]
				Velocity Ref[Velocity]
			}]{},
			setupFn: func(state *initSystemState) error {
				_, position := state.Position.Create()
				position.Set(Position{X: 1, Y: 1})

				_, velocity := state.Velocity.Create()
				velocity.Set(Velocity{X: 2, Y: 2})
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Exact[struct {
					Position Ref[Position]
					Velocity Ref[Velocity]
				}])
				for _, comps := range search.Iter() {
					result = append(result, comps.Position.Get(), comps.Velocity.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, result []any) {
				assert.Empty(t, result)
			},
		},
		{
			name: "matching components",
			search: &Exact[struct {
				Position Ref[Position]
				Velocity Ref[Velocity]
			}]{},
			setupFn: func(state *initSystemState) error {
				for i := range 10 {
					if i%2 == 0 {
						_, posVel := state.PositionVelocity.Create()
						posVel.Position.Set(Position{X: i, Y: i})
						posVel.Velocity.Set(Velocity{X: i * 10, Y: i * 10})
					} else {
						_, position := state.Position.Create()
						position.Set(Position{X: i, Y: i})
					}
				}
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				search := s.(*Exact[struct {
					Position Ref[Position]
					Velocity Ref[Velocity]
				}])
				for _, comps := range search.Iter() {
					result = append(result, comps.Position.Get(), comps.Velocity.Get())
				}
				return result
			},
			validateFn: func(t *testing.T, results []any) {
				assert.Len(t, results, 10) // 5 entities * 2 components
				for i := 0; i < len(results); i += 2 {
					pos := results[i].(Position)
					vel := results[i+1].(Velocity)
					idx := i / 2 * 2 // Convert to the original index (0, 2, 4, 6, 8)
					assert.Equal(t, idx, pos.X, "Position X should match index")
					assert.Equal(t, idx, pos.Y, "Position Y should match index")
					assert.Equal(t, idx*10, vel.X, "Velocity X should match index*10")
					assert.Equal(t, idx*10, vel.Y, "Velocity Y should match index*10")
				}
			},
		},
		{
			name: "early return",
			search: &Exact[struct {
				Position Ref[Position]
				Velocity Ref[Velocity]
			}]{},
			setupFn: func(state *initSystemState) error {
				for i := range 10 {
					_, posVel := state.PositionVelocity.Create()
					posVel.Position.Set(Position{X: i, Y: i})
					posVel.Velocity.Set(Velocity{X: i * 10, Y: i * 10})
				}
				return nil
			},
			getResult: func(s systemStateField) []any {
				var result []any
				count := 0

				search := s.(*Exact[struct {
					Position Ref[Position]
					Velocity Ref[Velocity]
				}])
				iter := search.Iter()
				iter(func(_ EntityID, comps struct {
					Position Ref[Position]
					Velocity Ref[Velocity]
				}) bool {
					result = append(result, comps.Position.Get(), comps.Velocity.Get())
					count++
					return count < 3 // Stop after collecting 3 items
				})

				return result
			},
			validateFn: func(t *testing.T, results []any) {
				assert.Len(t, results, 6, "Should get 3 entities with 2 components each")
				for i := 0; i < len(results); i += 2 {
					pos := results[i].(Position)
					vel := results[i+1].(Velocity)
					idx := i / 2
					assert.Equal(t, idx, pos.X, "Position X should match index")
					assert.Equal(t, idx, pos.Y, "Position Y should match index")
					assert.Equal(t, idx*10, vel.X, "Velocity X should match index*10")
					assert.Equal(t, idx*10, vel.Y, "Velocity Y should match index*10")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := NewWorld()

			_, err := tt.search.init(w)
			require.NoError(t, err)

			RegisterSystem(w, tt.setupFn, WithHook(Init))

			w.InitSchedulers()

			err = w.InitSystems()
			require.NoError(t, err)

			results := tt.getResult(tt.search)

			tt.validateFn(t, results)
		})
	}
}

type benchmarkSystemState struct {
	Position         Exact[struct{ Ref[Position] }]
	PositionVelocity Exact[struct {
		Position Ref[Position]
		Velocity Ref[Velocity]
	}]
	PositionVelocityHealth Exact[struct {
		Position Ref[Position]
		Velocity Ref[Velocity]
		Health   Ref[Health]
	}]
}

func BenchmarkSearch_Iter(b *testing.B) {
	benchmarks := []struct {
		name    string
		setup   func(*benchmarkSystemState) error
		iterate func(*benchmarkSystemState) error
	}{
		{
			name: "single component 100",
			setup: func(state *benchmarkSystemState) error {
				for i := range 100 {
					_, position := state.Position.Create()
					position.Set(Position{X: i, Y: i})
				}
				return nil
			},
			iterate: func(state *benchmarkSystemState) error {
				for _, result := range state.Position.Iter() {
					_ = result
				}
				return nil
			},
		},
		{
			name: "two components 100",
			setup: func(state *benchmarkSystemState) error {
				for i := range 100 {
					_, posVel := state.PositionVelocity.Create()
					posVel.Position.Set(Position{X: i, Y: i})
					posVel.Velocity.Set(Velocity{X: i, Y: i})
				}
				return nil
			},
			iterate: func(state *benchmarkSystemState) error {
				for _, result := range state.PositionVelocity.Iter() {
					_ = result
				}
				return nil
			},
		},
		{
			name: "three components 100",
			setup: func(state *benchmarkSystemState) error {
				for i := range 100 {
					_, posVelHealth := state.PositionVelocityHealth.Create()
					posVelHealth.Position.Set(Position{X: i, Y: i})
					posVelHealth.Velocity.Set(Velocity{X: i, Y: i})
					posVelHealth.Health.Set(Health{Value: i})
				}
				return nil
			},
			iterate: func(state *benchmarkSystemState) error {
				for _, result := range state.PositionVelocityHealth.Iter() {
					_ = result
				}
				return nil
			},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			w := NewWorld()
			RegisterSystem(w, bm.setup, WithHook(Init))
			RegisterSystem(w, bm.iterate, WithHook(Init))

			b.ResetTimer()
			for b.Loop() { // Run only the iterate function for the benchmark.
				_ = w.initSystems[1].fn()
			}
		})
	}
}
