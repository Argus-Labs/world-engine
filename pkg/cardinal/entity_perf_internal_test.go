package cardinal

import "testing"

type perfArchetype struct {
	Position WithComponent[Position3D]
	Velocity WithComponent[Velocity3D]
}

type perfEntityState struct {
	BaseSystemState
	Entities Contains[perfArchetype]
	Optional Contains[struct{ Inventory WithComponent[Inventory] }]
}

// newPerfEntityState excludes world and archetype initialization from steady-state measurements.
func newPerfEntityState(t testing.TB) *perfEntityState {
	t.Helper()
	w := newBenchWorld()
	state := &perfEntityState{}
	mustInitSystemFields(t, w, state)
	return state
}

func BenchmarkEntityOperations(b *testing.B) {
	b.Run("AddRemove", func(b *testing.B) {
		state := newPerfEntityState(b)
		_, entity := state.Entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			entity.Remove[Velocity3D]()
			entity.Set(Velocity3D{X: 1})
		}
		if !entity.Has[Velocity3D]() {
			b.Fatal("component was not restored")
		}
	})
	b.Run("FilterLimitSingle", func(b *testing.B) {
		state := newPerfEntityState(b)
		id, entity := state.Entities.Create()
		entity.Set(Position3D{X: 1})
		b.ReportAllocs()
		for b.Loop() {
			foundID, found, err := state.Entities.Iter().Filter(func(_ EntityID, e Entity) bool {
				return e.Get[Position3D]().X == 1
			}).Limit(1).Single()
			if err != nil || foundID != id || found.Get[Position3D]().X != 1 {
				b.Fatal("query returned an unexpected result")
			}
		}
	})
	b.Run("Get", func(b *testing.B) {
		state := newPerfEntityState(b)
		_, entity := state.Entities.Create()
		entity.Set(Position3D{X: 1})
		var value Position3D
		b.ReportAllocs()
		for b.Loop() {
			value = entity.Get[Position3D]()
		}
		if value.X != 1 {
			b.Fatal("unexpected component value")
		}
	})
	b.Run("Set", func(b *testing.B) {
		state := newPerfEntityState(b)
		_, entity := state.Entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			entity.Set(Position3D{X: 1})
		}
		if entity.Get[Position3D]().X != 1 {
			b.Fatal("component was not written")
		}
	})
	b.Run("HasPresent", func(b *testing.B) {
		state := newPerfEntityState(b)
		_, entity := state.Entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			if !entity.Has[Position3D]() {
				b.Fatal("missing component")
			}
		}
	})
	b.Run("HasAbsent", func(b *testing.B) {
		state := newPerfEntityState(b)
		_, entity := state.Entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			if entity.Has[Inventory]() {
				b.Fatal("unexpected component")
			}
		}
	})
	b.Run("HasUnregistered", func(b *testing.B) {
		state := newPerfEntityState(b)
		_, entity := state.Entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			if entity.Has[NetworkSync]() {
				b.Fatal("unexpected component")
			}
		}
	})
	b.Run("HasMissingEntity", func(b *testing.B) {
		state := newPerfEntityState(b)
		entity := state.Entity(999)
		b.ReportAllocs()
		for b.Loop() {
			if entity.Has[Position3D]() {
				b.Fatal("unexpected entity")
			}
		}
	})
	b.Run("LookupGet", func(b *testing.B) {
		state := newPerfEntityState(b)
		id, entity := state.Entities.Create()
		entity.Set(Position3D{X: 1})
		b.ReportAllocs()
		for b.Loop() {
			found, err := state.Entities.GetByID(id)
			if err != nil || found.Get[Position3D]().X != 1 {
				b.Fatal("entity lookup failed")
			}
		}
	})
	b.Run("CreateDestroy", func(b *testing.B) {
		state := newPerfEntityState(b)
		_, warmup := state.Entities.Create()
		warmup.Destroy()
		b.ReportAllocs()
		for b.Loop() {
			_, entity := state.Entities.Create()
			if !entity.Destroy() {
				b.Fatal("entity was not destroyed")
			}
		}
	})
	b.Run("GenericCreateDestroy", func(b *testing.B) {
		state := newPerfEntityState(b)
		_, warmup := state.Entities.Create()
		warmup.Destroy()
		b.ReportAllocs()
		for b.Loop() {
			entity := state.Create[perfArchetype]()
			if !entity.Destroy() {
				b.Fatal("entity was not destroyed")
			}
		}
	})
}

func benchmarkEntityIteration[T any](b *testing.B) {
	w := newBenchWorld()
	state := &struct{ Entities Contains[T] }{}
	mustInitSystemFields(b, w, state)
	for range 100 {
		state.Entities.Create()
	}
	b.ReportAllocs()
	for b.Loop() {
		var sum EntityID
		for id := range state.Entities.Iter() {
			sum += id
		}
		if sum != 4950 {
			b.Fatal("iteration missed an entity")
		}
	}
}

func BenchmarkEntityIteration(b *testing.B) {
	b.Run("1Component", benchmarkEntityIteration[struct{ Position WithComponent[Position3D] }])
	b.Run("5Components", benchmarkEntityIteration[struct {
		Position  WithComponent[Position3D]
		Velocity  WithComponent[Velocity3D]
		Health    WithComponent[Health2]
		Transform WithComponent[Transform]
		Inventory WithComponent[Inventory]
	}])
	b.Run("10Components", benchmarkEntityIteration[struct {
		Position    WithComponent[Position3D]
		Velocity    WithComponent[Velocity3D]
		Health      WithComponent[Health2]
		Transform   WithComponent[Transform]
		Inventory   WithComponent[Inventory]
		PlayerStats WithComponent[PlayerStats]
		AIBehavior  WithComponent[AIBehavior]
		Renderer    WithComponent[Renderer]
		Physics     WithComponent[Physics]
		NetworkSync WithComponent[NetworkSync]
	}])
}

func BenchmarkEntityRegistration(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		newPerfEntityState(b)
	}
}

// Keep successful handle operations and expected absence checks off the heap.
// Storage growth and component payload allocations are intentionally outside this contract.
func TestEntity_SteadyStateAllocations(t *testing.T) {
	state := newPerfEntityState(t)
	id, entity := state.Entities.Create()
	entity.Set(Position3D{X: 42})
	missing := state.Entity(999)
	cases := []struct {
		name string
		run  func() bool
	}{
		{"Bind", func() bool { return state.Entity(id).Alive() }},
		{"Get", func() bool { return entity.Get[Position3D]().X == 42 }},
		{"Set", func() bool {
			entity.Set(Position3D{X: 42})
			return entity.Get[Position3D]().X == 42
		}},
		{"HasPresent", func() bool { return entity.Has[Position3D]() }},
		{"HasAbsent", func() bool { return !entity.Has[Inventory]() }},
		{"HasUnregistered", func() bool { return !entity.Has[NetworkSync]() }},
		{"HasMissingEntity", func() bool { return !missing.Has[Position3D]() }},
		{"Lookup", func() bool {
			found, err := state.Entities.GetByID(id)
			return err == nil && found.Get[Position3D]().X == 42
		}},
		{"Iterate", func() bool {
			count := 0
			for _, found := range state.Entities.Iter() {
				if found.Get[Position3D]().X != 42 {
					return false
				}
				count++
			}
			return count == 1
		}},
		{"GenericCreateDestroy", func() bool {
			created := state.Create[perfArchetype]()
			return created.Alive() && created.Destroy()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok := true
			allocs := testing.AllocsPerRun(100, func() { ok = tc.run() && ok })
			if !ok {
				t.Fatal("operation returned an unexpected result")
			}
			if allocs != 0 {
				t.Fatalf("expected zero steady-state allocations, got %g per operation", allocs)
			}
		})
	}
}

// Function-valued query composition can retain callbacks. Bound its cost rather
// than hiding those allocations behind the zero-allocation direct-operation tests.
func TestEntity_QueryCompositionAllocations(t *testing.T) {
	state := newPerfEntityState(t)
	id, _ := state.Entities.Create()
	ok := true
	allocs := testing.AllocsPerRun(100, func() {
		_, found, err := state.Entities.Iter().Filter(func(_ EntityID, e Entity) bool {
			return e.Has[Position3D]()
		}).Limit(1).Single()
		ok = ok && err == nil && found.ID() == id
	})
	if !ok {
		t.Fatal("query returned an unexpected result")
	}
	if allocs > 4 {
		t.Fatalf("query composition exceeded its four-allocation budget: %g", allocs)
	}
}
