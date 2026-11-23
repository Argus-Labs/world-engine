package ecs

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
)

// benchEntities is the live entity count for the steady-state benchmarks.
const benchEntities = 1024

// newBenchEntities returns a world holding benchEntities entities that each have ComponentA.
func newBenchEntities(b *testing.B) (*World, []EntityID) {
	b.Helper()

	w := NewWorld()
	if _, err := w.RegisterComponent[testutils.ComponentA](); err != nil {
		b.Fatal(err)
	}

	ids := make([]EntityID, benchEntities)
	for i := range ids {
		ids[i] = w.Create()
		if err := w.Set(ids[i], testutils.ComponentA{X: float64(i)}); err != nil {
			b.Fatal(err)
		}
	}

	return w, ids
}

func BenchmarkEntity_Create(b *testing.B) {
	w := NewWorld()

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		if i == 1<<20 { // Keep the world from growing without bound.
			b.StopTimer()

			w = NewWorld()
			i = 0

			b.StartTimer()
		}

		w.Create()
	}
}

// BenchmarkEntity_DestroyCreate destroys an entity and creates one in its place, the free-list path.
func BenchmarkEntity_DestroyCreate(b *testing.B) {
	w, ids := newBenchEntities(b)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		slot := i % len(ids)
		if !w.Destroy(ids[slot]) {
			b.Fatal("destroy failed")
		}

		ids[slot] = w.Create()
	}
}

func BenchmarkEntity_Alive(b *testing.B) {
	w, ids := newBenchEntities(b)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		if !w.Alive(ids[i%len(ids)]) {
			b.Fatal("entity not alive")
		}
	}
}

func BenchmarkEntity_Get(b *testing.B) {
	w, ids := newBenchEntities(b)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		if _, err := w.Get[testutils.ComponentA](ids[i%len(ids)]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEntity_Set(b *testing.B) {
	w, ids := newBenchEntities(b)

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		if err := w.Set(ids[i%len(ids)], testutils.ComponentA{X: 1}); err != nil {
			b.Fatal(err)
		}
	}
}
