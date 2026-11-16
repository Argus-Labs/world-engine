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

func BenchmarkSlice(b *testing.B) {
	for _, n := range benchSizes {
		stored := benchSlice(n)
		b.Run(fmt.Sprintf("AppendOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s = s.Append(benchItem{})
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("AppendLoop/%d", n), func(b *testing.B) {
			for b.Loop() {
				var s immutable.Slice[benchItem]
				for i := range n {
					s = s.Append(benchItem{ID: int64(i)})
				}
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("AppendLoopBuilder/%d", n), func(b *testing.B) {
			for b.Loop() {
				bl := immutable.NewSliceBuilder[benchItem](0)
				for i := range n {
					bl.Append(benchItem{ID: int64(i)})
				}
				sinkSlice = bl.Done()
			}
		})
		b.Run(fmt.Sprintf("SetOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s = s.With(n/2, benchItem{Qty: 1})
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("SetAll/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				for i := range n {
					s = s.With(i, benchItem{Qty: 1})
				}
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("SetAllBuilder/%d", n), func(b *testing.B) {
			for b.Loop() {
				bl := stored.Edit()
				for i := range n {
					bl.Set(i, benchItem{Qty: 1})
				}
				sinkSlice = bl.Done()
			}
		})
		b.Run(fmt.Sprintf("InsertFront/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s = s.Insert(0, benchItem{})
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("FilterHalf/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s = s.Filter(func(it benchItem) bool { return it.ID%2 == 0 })
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("TrimFront/%d", n), func(b *testing.B) {
			// A capped log: one entry in, the oldest out.
			for b.Loop() {
				s := stored
				s = s.Append(benchItem{})
				s = s.Delete(0, 1)
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("ReadAll/%d", n), func(b *testing.B) {
			for b.Loop() {
				var sum int64
				for _, it := range stored.All() {
					sum += it.ID
				}
				sinkInt = sum
			}
		})
	}
}

func BenchmarkMap(b *testing.B) {
	for _, n := range benchSizes {
		stored, keys := benchMap(n)
		b.Run(fmt.Sprintf("SetExistingOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored
				m = m.With(keys[0], benchItem{Qty: 1})
				sinkMap = m
			}
		})
		b.Run(fmt.Sprintf("InsertOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored
				m = m.With(n+1, benchItem{})
				sinkMap = m
			}
		})
		b.Run(fmt.Sprintf("InsertLoop/%d", n), func(b *testing.B) {
			for b.Loop() {
				var m immutable.Map[int, benchItem]
				for _, k := range keys {
					m = m.With(k, benchItem{})
				}
				sinkMap = m
			}
		})
		b.Run(fmt.Sprintf("InsertLoopBuilder/%d", n), func(b *testing.B) {
			for b.Loop() {
				bl := immutable.Map[int, benchItem]{}.Edit()
				for _, k := range keys {
					bl.Set(k, benchItem{})
				}
				sinkMap = bl.Done()
			}
		})
		b.Run(fmt.Sprintf("UpdateAll/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored
				for _, k := range keys {
					m = m.Update(k, func(v benchItem) benchItem { v.Qty++; return v })
				}
				sinkMap = m
			}
		})
		b.Run(fmt.Sprintf("UpdateAllBuilder/%d", n), func(b *testing.B) {
			for b.Loop() {
				bl := stored.Edit()
				for _, k := range keys {
					v, _ := bl.Get(k)
					v.Qty++
					bl.Set(k, v)
				}
				sinkMap = bl.Done()
			}
		})
		b.Run(fmt.Sprintf("DeleteHalf/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored
				m = m.DeleteFunc(func(k int, _ benchItem) bool { return k%2 == 0 })
				sinkMap = m
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
// appends two log entries and drops the two oldest, bumps two stats, and stores it back.
func BenchmarkTick(b *testing.B) {
	const entities = 1000
	newColumn := func() []benchComponent {
		col := make([]benchComponent, entities)
		for i := range col {
			stats, _ := benchMapInt(16)
			col[i] = benchComponent{Log: benchSlice(16), Stats: stats}
		}
		return col
	}
	b.Run("Derive", func(b *testing.B) {
		col := newColumn()
		for b.Loop() {
			for e := range col {
				c := col[e]
				c.Log = c.Log.Append(benchItem{}, benchItem{})
				c.Log = c.Log.Delete(0, 2)
				c.Stats = c.Stats.Update(1, func(v int64) int64 { return v + 1 })
				c.Stats = c.Stats.Update(2, func(v int64) int64 { return v + 1 })
				col[e] = c
			}
		}
	})
	b.Run("Builder", func(b *testing.B) {
		col := newColumn()
		for b.Loop() {
			for e := range col {
				c := col[e]
				lb := c.Log.Edit()
				lb.Append(benchItem{}, benchItem{})
				lb.Delete(0, 2)
				c.Log = lb.Done()
				mb := c.Stats.Edit()
				v, _ := mb.Get(1)
				mb.Set(1, v+1)
				v, _ = mb.Get(2)
				mb.Set(2, v+1)
				c.Stats = mb.Done()
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
