package cardinal

import "testing"

type perfArchetype struct {
	Position Position3D
	Velocity Velocity3D
}

type perfEntityState struct {
	BaseSystemState
	Entities Search
}

// newPerfEntityState excludes world and archetype resolution from steady-state measurements.
func newPerfEntityState(t testing.TB) *perfEntityState {
	t.Helper()
	state := &perfEntityState{BaseSystemState: BaseSystemState{world: newBenchWorld()}}
	state.Entities = state.Contains[perfArchetype]()
	return state
}

func BenchmarkEntityOperations(b *testing.B) {
	b.Run("AddRemove", func(b *testing.B) {
		state := newPerfEntityState(b)
		entity := state.Entities.Create()
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
		entity := state.Entities.Create()
		id := entity.ID()
		entity.Set(Position3D{X: 1})
		b.ReportAllocs()
		for b.Loop() {
			found, err := state.Entities.Iter().Filter(func(e Entity) bool {
				return e.Get[Position3D]().X == 1
			}).Limit(1).Single()
			if err != nil || found.ID() != id || found.Get[Position3D]().X != 1 {
				b.Fatal("query returned an unexpected result")
			}
		}
	})
	b.Run("Get", func(b *testing.B) {
		state := newPerfEntityState(b)
		entity := state.Entities.Create()
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
		entity := state.Entities.Create()
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
		entity := state.Entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			if !entity.Has[Position3D]() {
				b.Fatal("missing component")
			}
		}
	})
	b.Run("HasAbsent", func(b *testing.B) {
		state := newPerfEntityState(b)
		entity := state.Entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			if entity.Has[Inventory]() {
				b.Fatal("unexpected component")
			}
		}
	})
	b.Run("HasUnregistered", func(b *testing.B) {
		state := newPerfEntityState(b)
		entity := state.Entities.Create()
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
		entity := state.Entities.Create()
		id := entity.ID()
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
		warmup := state.Entities.Create()
		warmup.Destroy()
		b.ReportAllocs()
		for b.Loop() {
			entity := state.Entities.Create()
			if !entity.Destroy() {
				b.Fatal("entity was not destroyed")
			}
		}
	})
	b.Run("GenericCreateDestroy", func(b *testing.B) {
		state := newPerfEntityState(b)
		warmup := state.Entities.Create()
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
	state := newBenchState(newBenchWorld())
	for range 100 {
		state.Create[T]()
	}
	b.ReportAllocs()
	for b.Loop() {
		var sum EntityID
		for id := range state.Contains[T]().Iter() {
			sum += id.ID()
		}
		if sum != 4950 {
			b.Fatal("iteration missed an entity")
		}
	}
}

func BenchmarkEntityIteration(b *testing.B) {
	b.Run("1Component", benchmarkEntityIteration[arch1])
	b.Run("5Components", benchmarkEntityIteration[arch5])
	b.Run("10Components", benchmarkEntityIteration[arch10])
}

// BenchmarkEntityQueryBuild measures building a query once its archetype is cached: the
// per-call cost a system pays for state.Contains[T]().
func BenchmarkEntityQueryBuild(b *testing.B) {
	state := newPerfEntityState(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = state.Contains[perfArchetype]()
	}
}

// Keep successful handle operations and expected absence checks off the heap.
// Storage growth and component payload allocations are intentionally outside this contract.
func TestEntity_SteadyStateAllocations(t *testing.T) {
	state := newPerfEntityState(t)
	entity := state.Entities.Create()
	id := entity.ID()
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
			for found := range state.Entities.Iter() {
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
	id := state.Entities.Create().ID()
	ok := true
	allocs := testing.AllocsPerRun(100, func() {
		found, err := state.Entities.Iter().Filter(func(e Entity) bool {
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
