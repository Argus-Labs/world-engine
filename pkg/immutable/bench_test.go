package immutable_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
)

// These benchmarks model how Cardinal uses the collections: a component is copied out of its
// column, edited, and stored back, and the stored value must not change before it is stored.
// Each iteration starts from the stored value, so every edit pays whatever a first edit costs.
// The scenario names are stable so results can be compared across commits with benchstat.

type benchItem struct {
	ID, Qty, Slot int64
	Name          string
}

var benchSizes = []int{16, 256, 4096}

var (
	sinkSlice immutable.Slice[benchItem]
	sinkMap   immutable.Map[int, benchItem]
	sinkInt   int64
)

func benchSlice(n int) immutable.Slice[benchItem] {
	items := make([]benchItem, n)
	for i := range items {
		items[i] = benchItem{ID: int64(i)}
	}
	return immutable.SliceOf(items...)
}

func benchMap(n int) (immutable.Map[int, benchItem], []int) {
	keys := rand.New(rand.NewPCG(1, 2)).Perm(n)
	m := immutable.MapFrom(func(yield func(int, benchItem) bool) {
		for _, k := range keys {
			if !yield(k, benchItem{ID: int64(k)}) {
				return
			}
		}
	})
	return m, keys
}

// BenchmarkSlice copies the Slice out on every iteration and back in at the end, as Cardinal's Get
// and Set do, so each scenario includes both copies. ReadAll pays the Get copy too: Get copies
// whether or not the system edits.
func BenchmarkSlice(b *testing.B) {
	for _, n := range benchSizes {
		stored := benchSlice(n)
		b.Run(fmt.Sprintf("AppendOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored.Clone()
				s.Append(benchItem{})
				sinkSlice = s.Clone()
			}
		})
		b.Run(fmt.Sprintf("AppendLoop/%d", n), func(b *testing.B) {
			for b.Loop() {
				var s immutable.Slice[benchItem]
				for i := range n {
					s.Append(benchItem{ID: int64(i)})
				}
				sinkSlice = s.Clone()
			}
		})
		b.Run(fmt.Sprintf("SetOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored.Clone()
				s.Set(n/2, benchItem{Qty: 1})
				sinkSlice = s.Clone()
			}
		})
		b.Run(fmt.Sprintf("SetAll/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored.Clone()
				for i := range n {
					s.Set(i, benchItem{Qty: 1})
				}
				sinkSlice = s.Clone()
			}
		})
		b.Run(fmt.Sprintf("InsertFront/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored.Clone()
				s.Insert(0, benchItem{})
				sinkSlice = s.Clone()
			}
		})
		b.Run(fmt.Sprintf("FilterHalf/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored.Clone()
				s.DeleteFunc(func(it benchItem) bool { return it.ID%2 != 0 })
				sinkSlice = s.Clone()
			}
		})
		b.Run(fmt.Sprintf("TrimFront/%d", n), func(b *testing.B) {
			// A capped log: one entry in, the oldest out.
			for b.Loop() {
				s := stored.Clone()
				s.Append(benchItem{})
				s.Delete(0, 1)
				sinkSlice = s.Clone()
			}
		})
		b.Run(fmt.Sprintf("ReadAll/%d", n), func(b *testing.B) {
			for b.Loop() {
				var sum int64
				for _, it := range stored.Clone().All() {
					sum += it.ID
				}
				sinkInt = sum
			}
		})
	}
}

// BenchmarkMap pays the same copies as BenchmarkSlice. GetAll reads the stored Map without the Get
// copy, so it measures lookups alone.
func BenchmarkMap(b *testing.B) {
	for _, n := range benchSizes {
		stored, keys := benchMap(n)
		b.Run(fmt.Sprintf("SetExistingOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored.Clone()
				m.Set(keys[0], benchItem{Qty: 1})
				sinkMap = m.Clone()
			}
		})
		b.Run(fmt.Sprintf("InsertOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored.Clone()
				m.Set(n+1, benchItem{})
				sinkMap = m.Clone()
			}
		})
		b.Run(fmt.Sprintf("InsertLoop/%d", n), func(b *testing.B) {
			for b.Loop() {
				var m immutable.Map[int, benchItem]
				for _, k := range keys {
					m.Set(k, benchItem{})
				}
				sinkMap = m.Clone()
			}
		})
		b.Run(fmt.Sprintf("UpdateAll/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored.Clone()
				for _, k := range keys {
					v, _ := m.Get(k)
					v.Qty++
					m.Set(k, v)
				}
				sinkMap = m.Clone()
			}
		})
		b.Run(fmt.Sprintf("DeleteHalf/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored.Clone()
				m.DeleteFunc(func(k int, _ benchItem) bool { return k%2 == 0 })
				sinkMap = m.Clone()
			}
		})
		b.Run(fmt.Sprintf("GetAll/%d", n), func(b *testing.B) {
			for b.Loop() {
				var sum int64
				for _, k := range keys {
					v, _ := stored.Get(k)
					sum += v.ID
				}
				sinkInt = sum
			}
		})
	}
}

// benchComponent is a component with one of each collection, as a system sees it.
type benchComponent struct {
	Log   immutable.Slice[benchItem]
	Stats immutable.Map[int, int64]
}

// BenchmarkTick models one tick over many entities: each system run copies the component out,
// appends two log entries and drops the two oldest, bumps two stats, and stores it back. The copies
// are the ones Cardinal makes on Get and Set, built with CloneFunc as Cardinal builds them.
func BenchmarkTick(b *testing.B) {
	const entities = 1000
	col := make([]benchComponent, entities)
	for i := range col {
		stats, _ := benchMapInt(16)
		col[i] = benchComponent{Log: benchSlice(16), Stats: stats}
	}
	clone := immutable.CloneFunc[benchComponent]()
	b.Run("InPlace", func(b *testing.B) {
		for b.Loop() {
			for e := range col {
				c := col[e]
				clone(&c)
				c.Log.Append(benchItem{}, benchItem{})
				c.Log.Delete(0, 2)
				v, _ := c.Stats.Get(1)
				c.Stats.Set(1, v+1)
				v, _ = c.Stats.Get(2)
				c.Stats.Set(2, v+1)
				clone(&c)
				col[e] = c
			}
		}
	})
}

func benchMapInt(n int) (immutable.Map[int, int64], []int) {
	keys := rand.New(rand.NewPCG(3, 4)).Perm(n)
	m := immutable.MapFrom(func(yield func(int, int64) bool) {
		for _, k := range keys {
			if !yield(k, int64(k)) {
				return
			}
		}
	})
	return m, keys
}
