package cardinal

import "testing"

type perfArchetype struct {
	Position Position3D
	Velocity Velocity3D
}

// newPerfEntityState resolves the search once so world setup and archetype resolution stay out of
// steady-state measurements.
func newPerfEntityState(t testing.TB) (*World, Search) {
	t.Helper()
	w := newBenchWorld()
	return w, w.Contains[perfArchetype]()
}

func BenchmarkEntityOperations(b *testing.B) {
	b.Run("AddRemove", func(b *testing.B) {
		_, entities := newPerfEntityState(b)
		entity := entities.Create()
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
		_, entities := newPerfEntityState(b)
		entity := entities.Create()
		id := entity.ID()
		entity.Set(Position3D{X: 1})
		b.ReportAllocs()
		for b.Loop() {
			found, err := entities.Iter().Filter(func(e Entity) bool {
				return e.Get[Position3D]().X == 1
			}).Limit(1).Single()
			if err != nil || found.ID() != id || found.Get[Position3D]().X != 1 {
				b.Fatal("query returned an unexpected result")
			}
		}
	})
	b.Run("Get", func(b *testing.B) {
		_, entities := newPerfEntityState(b)
		entity := entities.Create()
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
		_, entities := newPerfEntityState(b)
		entity := entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			entity.Set(Position3D{X: 1})
		}
		if entity.Get[Position3D]().X != 1 {
			b.Fatal("component was not written")
		}
	})
	b.Run("HasPresent", func(b *testing.B) {
		_, entities := newPerfEntityState(b)
		entity := entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			if !entity.Has[Position3D]() {
				b.Fatal("missing component")
			}
		}
	})
	b.Run("HasAbsent", func(b *testing.B) {
		_, entities := newPerfEntityState(b)
		entity := entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			if entity.Has[Inventory]() {
				b.Fatal("unexpected component")
			}
		}
	})
	b.Run("HasUnregistered", func(b *testing.B) {
		_, entities := newPerfEntityState(b)
		entity := entities.Create()
		b.ReportAllocs()
		for b.Loop() {
			if entity.Has[NetworkSync]() {
				b.Fatal("unexpected component")
			}
		}
	})
	b.Run("HasMissingEntity", func(b *testing.B) {
		w, _ := newPerfEntityState(b)
		entity := w.Entity(999)
		b.ReportAllocs()
		for b.Loop() {
			if entity.Has[Position3D]() {
				b.Fatal("unexpected entity")
			}
		}
	})
	b.Run("LookupGet", func(b *testing.B) {
		_, entities := newPerfEntityState(b)
		entity := entities.Create()
		id := entity.ID()
		entity.Set(Position3D{X: 1})
		b.ReportAllocs()
		for b.Loop() {
			found, err := entities.GetByID(id)
			if err != nil || found.Get[Position3D]().X != 1 {
				b.Fatal("entity lookup failed")
			}
		}
	})
	b.Run("CreateDestroy", func(b *testing.B) {
		_, entities := newPerfEntityState(b)
		warmup := entities.Create()
		warmup.Destroy()
		b.ReportAllocs()
		for b.Loop() {
			entity := entities.Create()
			if !entity.Destroy() {
				b.Fatal("entity was not destroyed")
			}
		}
	})
	b.Run("GenericCreateDestroy", func(b *testing.B) {
		w, entities := newPerfEntityState(b)
		warmup := entities.Create()
		warmup.Destroy()
		b.ReportAllocs()
		for b.Loop() {
			entity := w.Create[perfArchetype]()
			if !entity.Destroy() {
				b.Fatal("entity was not destroyed")
			}
		}
	})
}

func benchmarkEntityIteration[T any](b *testing.B) {
	entities := newBenchWorld().Contains[T]()
	for range 100 {
		entities.Create()
	}
	b.ReportAllocs()
	for b.Loop() {
		var sum EntityID
		for id := range entities.Iter() {
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

// BenchmarkEntityRegistration measures world setup plus the reflection that resolves and
// caches an archetype on its first Contains call.
func BenchmarkEntityRegistration(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		newPerfEntityState(b)
	}
}

// Keep successful handle operations and expected absence checks off the heap.
// Storage growth and component payload allocations are intentionally outside this contract.
func TestEntity_SteadyStateAllocations(t *testing.T) {
	w, entities := newPerfEntityState(t)
	entity := entities.Create()
	id := entity.ID()
	entity.Set(Position3D{X: 42})
	missing := w.Entity(999)
	cases := []struct {
		name string
		run  func() bool
	}{
		{"Bind", func() bool { return w.Entity(id).Alive() }},
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
			found, err := entities.GetByID(id)
			return err == nil && found.Get[Position3D]().X == 42
		}},
		{"Iterate", func() bool {
			count := 0
			for found := range entities.Iter() {
				if found.Get[Position3D]().X != 42 {
					return false
				}
				count++
			}
			return count == 1
		}},
		{"GenericCreateDestroy", func() bool {
			created := w.Create[perfArchetype]()
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
	_, entities := newPerfEntityState(t)
	id := entities.Create().ID()
	ok := true
	allocs := testing.AllocsPerRun(100, func() {
		found, err := entities.Iter().Filter(func(e Entity) bool {
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
