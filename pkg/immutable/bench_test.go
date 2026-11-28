package immutable_test

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/vmihailenco/msgpack/v5"
)

// These benchmarks model how Cardinal uses the collections: a component is taken out of its column,
// edited, and stored back, and the stored value must not change before it is stored. Each iteration
// starts from the stored value, so every edit pays whatever a first edit costs.
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

// BenchmarkBuild builds a Slice and a Map in bulk, as a component is first filled or a rebuilding
// edit such as Insert or DeleteFunc stores its result.
func BenchmarkBuild(b *testing.B) {
	for _, n := range benchSizes {
		items := make([]benchItem, n)
		for i := range items {
			items[i] = benchItem{ID: int64(i)}
		}

		keys := rand.New(rand.NewPCG(1, 2)).Perm(n)

		b.Run(fmt.Sprintf("SliceOf/%d", n), func(b *testing.B) {
			for b.Loop() {
				sinkSlice = immutable.SliceOf(items...)
			}
		})
		b.Run(fmt.Sprintf("Collect/%d", n), func(b *testing.B) {
			for b.Loop() {
				sinkSlice = immutable.Collect(slices.Values(items))
			}
		})
		b.Run(fmt.Sprintf("MapFrom/%d", n), func(b *testing.B) {
			for b.Loop() {
				sinkMap = immutable.MapFrom(func(yield func(int, benchItem) bool) {
					for _, k := range keys {
						if !yield(k, items[k]) {
							return
						}
					}
				})
			}
		})
	}
}

// BenchmarkPop deletes the last element of a stored Slice, the cheapest delete, for an element type
// with pointers and one without.
func BenchmarkPop(b *testing.B) {
	const n = 256

	items := benchSlice(n)
	ints := immutable.Collect(func(yield func(int64) bool) {
		for i := range int64(n) {
			if !yield(i) {
				return
			}
		}
	})

	var sinkInts immutable.Slice[int64]

	b.Run("Item", func(b *testing.B) {
		for b.Loop() {
			s := items
			s.Delete(n-1, n)
			sinkSlice = s
		}
	})
	b.Run("Int", func(b *testing.B) {
		for b.Loop() {
			s := ints
			s.Delete(n-1, n)
			sinkInts = s
		}
	})

	_ = sinkInts
}

// BenchmarkSlice starts every iteration from the stored Slice, as a system starts from the value Get
// returns, so each scenario pays whatever a first edit costs.
func BenchmarkSlice(b *testing.B) {
	for _, n := range benchSizes {
		stored := benchSlice(n)
		b.Run(fmt.Sprintf("AppendOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s.Append(benchItem{})
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("AppendLoop/%d", n), func(b *testing.B) {
			for b.Loop() {
				var s immutable.Slice[benchItem]
				for i := range n {
					s.Append(benchItem{ID: int64(i)})
				}

				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("SetOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s.Set(n/2, benchItem{Qty: 1})
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("SetAll/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				for i := range n {
					s.Set(i, benchItem{Qty: 1})
				}

				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("InsertFront/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s.Insert(0, benchItem{})
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("FilterHalf/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s.DeleteFunc(func(it benchItem) bool { return it.ID%2 != 0 })
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("TrimFront/%d", n), func(b *testing.B) {
			// A capped log: one entry in, the oldest out.
			for b.Loop() {
				s := stored
				s.Append(benchItem{})
				s.Delete(0, 1)
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("UpdateFunc/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s.UpdateFunc(func(_ int, it benchItem) benchItem {
					it.Qty++
					return it
				})
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("DeleteMiddle/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s.Delete(n/2, n/2+1)
				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("ReadLast64At/%d", n), func(b *testing.B) {
			lo := max(0, n-64)

			for b.Loop() {
				var sum int64
				for i := lo; i < n; i++ {
					sum += stored.At(i).ID
				}

				sinkInt = sum
			}
		})
		b.Run(fmt.Sprintf("ReadLast64Range/%d", n), func(b *testing.B) {
			for b.Loop() {
				var sum int64
				for _, it := range stored.Range(max(0, n-64), n) {
					sum += it.ID
				}

				sinkInt = sum
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

// BenchmarkMap starts every iteration from the stored Map, as BenchmarkSlice does.
func BenchmarkMap(b *testing.B) {
	for _, n := range benchSizes {
		stored, keys := benchMap(n)
		b.Run(fmt.Sprintf("SetExistingOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored
				m.Set(keys[0], benchItem{Qty: 1})
				sinkMap = m
			}
		})
		b.Run(fmt.Sprintf("InsertOne/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored
				m.Set(n+1, benchItem{})
				sinkMap = m
			}
		})
		b.Run(fmt.Sprintf("InsertLoop/%d", n), func(b *testing.B) {
			for b.Loop() {
				var m immutable.Map[int, benchItem]
				for _, k := range keys {
					m.Set(k, benchItem{})
				}

				sinkMap = m
			}
		})
		b.Run(fmt.Sprintf("UpdateAll/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored
				for _, k := range keys {
					v, _ := m.Get(k)
					v.Qty++
					m.Set(k, v)
				}

				sinkMap = m
			}
		})
		b.Run(fmt.Sprintf("UpdateFunc/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored
				m.UpdateFunc(func(_ int, v benchItem) benchItem {
					v.Qty++
					return v
				})
				sinkMap = m
			}
		})
		b.Run(fmt.Sprintf("DeleteHalf/%d", n), func(b *testing.B) {
			for b.Loop() {
				m := stored
				m.DeleteFunc(func(k int, _ benchItem) bool { return k%2 == 0 })
				sinkMap = m
			}
		})
		b.Run(fmt.Sprintf("MaxKey/%d", n), func(b *testing.B) {
			for b.Loop() {
				k, _ := stored.MaxKey()
				sinkInt = int64(k)
			}
		})
		b.Run(fmt.Sprintf("ReadLast64Range/%d", n), func(b *testing.B) {
			for b.Loop() {
				var sum int64
				for _, v := range stored.Range(n-64, n) {
					sum += v.ID
				}

				sinkInt = sum
			}
		})
		b.Run(fmt.Sprintf("ReadLast64Scan/%d", n), func(b *testing.B) {
			for b.Loop() {
				var sum int64

				for k, v := range stored.All() {
					if k >= n-64 {
						sum += v.ID
					}
				}

				sinkInt = sum
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

// BenchmarkTick models one tick over many entities: each system run takes the component out,
// appends two log entries and drops the two oldest, bumps two stats, and stores it back. Get and Set
// copy only the component struct; the collections share storage that no edit writes to.
func BenchmarkTick(b *testing.B) {
	const entities = 1000

	col := make([]benchComponent, entities)
	for i := range col {
		stats, _ := benchMapInt(16)
		col[i] = benchComponent{Log: benchSlice(16), Stats: stats}
	}

	b.Run("InPlace", func(b *testing.B) {
		for b.Loop() {
			for e := range col {
				c := col[e]
				c.Log.Append(benchItem{}, benchItem{})
				c.Log.Delete(0, 2)
				v, _ := c.Stats.Get(1)
				c.Stats.Set(1, v+1)
				v, _ = c.Stats.Get(2)
				c.Stats.Set(2, v+1)
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

// BenchmarkDecode decodes a Slice and a Map from MessagePack, as a command carrying one arrives or a
// snapshot restores.
func BenchmarkDecode(b *testing.B) {
	for _, n := range benchSizes {
		sliceBytes, err := msgpack.Marshal(benchSlice(n))
		if err != nil {
			b.Fatal(err)
		}

		stored, _ := benchMap(n)

		mapBytes, err := msgpack.Marshal(stored)
		if err != nil {
			b.Fatal(err)
		}

		b.Run(fmt.Sprintf("Slice/%d", n), func(b *testing.B) {
			for b.Loop() {
				var s immutable.Slice[benchItem]
				if err := msgpack.Unmarshal(sliceBytes, &s); err != nil {
					b.Fatal(err)
				}

				sinkSlice = s
			}
		})
		b.Run(fmt.Sprintf("Map/%d", n), func(b *testing.B) {
			for b.Loop() {
				var m immutable.Map[int, benchItem]
				if err := msgpack.Unmarshal(mapBytes, &m); err != nil {
					b.Fatal(err)
				}

				sinkMap = m
			}
		})
	}
}

// BenchmarkEqual compares a value with a copy, as a system checking whether a component changed
// since the last tick does.
func BenchmarkEqual(b *testing.B) {
	const n = 4096

	sameItem := func(a, b benchItem) bool { return a == b }
	stored := benchSlice(n)
	edited := stored
	edited.Set(n-10, benchItem{ID: -1})
	rebuilt := benchSlice(n)
	storedMap, _ := benchMap(n)
	editedMap := storedMap
	editedMap.Set(n-1, benchItem{ID: -1})
	rebuiltMap, _ := benchMap(n)

	for _, c := range []struct {
		name string
		a, b immutable.Slice[benchItem]
	}{{"Slice/Copy", stored, stored}, {"Slice/OneChange", stored, edited}, {"Slice/Rebuilt", stored, rebuilt}} {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				if c.a.EqualFunc(c.b, sameItem) {
					sinkInt++
				}
			}
		})
	}

	for _, c := range []struct {
		name string
		a, b immutable.Map[int, benchItem]
	}{
		{"Map/Copy", storedMap, storedMap},
		{"Map/OneChange", storedMap, editedMap},
		{"Map/Rebuilt", storedMap, rebuiltMap},
	} {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				if c.a.EqualFunc(c.b, sameItem) {
					sinkInt++
				}
			}
		})
	}
}

// BenchmarkSort sorts a shuffled Slice by one field, with many ties, and builds a Map from shuffled
// pairs, which sorts them.
func BenchmarkSort(b *testing.B) {
	for _, n := range benchSizes {
		r := rand.New(rand.NewPCG(3, 4))

		items := make([]benchItem, n)
		for i := range items {
			items[i] = benchItem{ID: int64(i), Qty: int64(r.IntN(max(1, n/8)))}
		}

		stored := immutable.SliceOf(items...)
		byQty := func(a, b benchItem) int { return cmp.Compare(a.Qty, b.Qty) }
		b.Run(fmt.Sprintf("Slice/%d", n), func(b *testing.B) {
			for b.Loop() {
				s := stored
				s.SortStableFunc(byQty)
				sinkSlice = s
			}
		})
		keys := r.Perm(n)
		b.Run(fmt.Sprintf("MapFrom/%d", n), func(b *testing.B) {
			for b.Loop() {
				sinkMap = immutable.MapFrom(func(yield func(int, benchItem) bool) {
					for _, k := range keys {
						if !yield(k, benchItem{}) {
							return
						}
					}
				})
			}
		})
	}
}

// BenchmarkEncode encodes a Slice and a Map to MessagePack, as an event carrying one or a snapshot
// does.
func BenchmarkEncode(b *testing.B) {
	for _, n := range benchSizes {
		s := benchSlice(n)
		m, _ := benchMap(n)
		b.Run(fmt.Sprintf("Slice/%d", n), func(b *testing.B) {
			for b.Loop() {
				data, err := msgpack.Marshal(s)
				if err != nil {
					b.Fatal(err)
				}

				sinkInt += int64(len(data))
			}
		})
		b.Run(fmt.Sprintf("Map/%d", n), func(b *testing.B) {
			for b.Loop() {
				data, err := msgpack.Marshal(m)
				if err != nil {
					b.Fatal(err)
				}

				sinkInt += int64(len(data))
			}
		})
	}
}
