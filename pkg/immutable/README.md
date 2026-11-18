# immutable

The collections a Cardinal component may hold: `Slice` for lists and `Map` for lookup tables.
Written for AI agents and people writing game code. The source of truth is the Go doc in
`slice.go` and `map.go`; this file is the short version. How they are stored and why is in
[DESIGN.md](DESIGN.md).

## Slice

### When to use it

A component may not hold a plain `[]T` (`RegisterComponent` panics). Use:

- `[N]T` plus a count field, when the list has a small, known upper bound.
- `immutable.Slice[T]`, when it has no bound you can defend. `cardinal.Slice[T]` is the same type.

```go
type Inventory struct {
	Items immutable.Slice[Item] `json:"items"`
}
```

Elements must be values: numbers, strings, bools, structs of those, fixed arrays, `time.Time`,
another `immutable.Slice`, or an `immutable.Map`. No pointers, interfaces, Go slices or Go maps.

### The one rule: edit, then `Set`

Edit a Slice the way you would a Go slice, then store the component with `Set`.

```go
inv := entity.Get[Inventory]()
inv.Items.Append(item)
inv.Items.Set(0, other)
entity.Set(inv)
```

Why this is safe: a Slice never writes to storage once it holds it. An edit copies the small piece
it changes and points at the copy, so every other holder of the old storage still sees the old
contents. So:

- An edit without `Set` never reaches the world, a snapshot or another system.
- An edit after `Set` never reaches what was stored.
- `Get`, `Set` and plain assignment copy nothing.

Events and commands are safe the same way: editing a payload after `SendTo`, `Broadcast`,
`SendToShard` or `EmitSystemEvent` does not change what is sent, and no `SystemEvents` or `Commands`
reader can change what another reader sees.

### API

| Go                                    | `immutable.Slice`                 |
| ------------------------------------- | --------------------------------- |
| `len(s)`                              | `s.Len()`                         |
| `s[i]`                                | `s.At(i)`                         |
| `s[i] = v`                            | `s.Set(i, v)`                     |
| `s = append(s, a, b)`                 | `s.Append(a, b)`                  |
| `s = slices.Insert(s, i, v)`          | `s.Insert(i, v)`                  |
| `s = slices.Delete(s, i, j)`          | `s.Delete(i, j)`                  |
| `s = slices.DeleteFunc(s, del)`       | `s.DeleteFunc(del)`               |
| `for i := range s { s[i] = f(i, s[i]) }` | `s.UpdateFunc(f)`, one rebuild  |
| `s = s[:0]` or `s = nil`              | `s.Reset()`                       |
| `slices.SortStableFunc(s, cmp)`       | `s.SortStableFunc(cmp)`           |
| `slices.Sort(s)`                      | `immutable.Sort(&s)`              |
| `slices.Reverse(s)`                   | `s.Reverse()`                     |
| `for i, v := range s`                 | `for i, v := range s.All()`       |
| `for _, v := range s`                 | `for v := range s.Values()`       |
| `for i, v := range s[lo:hi]`          | `for i, v := range s.Range(lo, hi)`; `i` is the index in `s` |
| `slices.Backward(s)`                  | `s.Backward()`                    |
| `slices.IndexFunc`, `ContainsFunc`    | `s.IndexFunc(f)`, `s.ContainsFunc(f)` |
| `slices.Equal`, `Index`, `Contains`   | `immutable.Equal(a, b)`, `immutable.Index(s, v)`, `immutable.Contains(s, v)` |
| `slices.EqualFunc`                    | `a.EqualFunc(b, eq)`              |
| `[]T{a, b}`                           | `immutable.SliceOf(a, b)`         |
| `slices.Collect(seq)`                 | `immutable.Collect(seq)`          |
| `slices.Clone(s)`                     | `c := s`, a free copy             |

`At` and `Set` panic out of range, like indexing a Go slice. To get a plain `[]T` for another API,
use `slices.Collect(s.Values())`; `SliceOf(xs...)` copies its input, so later changes to `xs` do not
reach the Slice.

Edits have pointer receivers, so they only work on something addressable: a variable or a struct
field reached through a variable. `entity.Get[Inventory]().Items.Append(x)` does not compile; assign
the component to a variable first.

### Things to know

**Sorting is always stable.** There is no unstable sort, so the result never depends on the sort
algorithm a Go release uses. Same world, same bytes.

**An empty Slice is always the zero value.** Deleting every element, `Reset`, `SliceOf[T]()` and decoding
`[]` or `null` all give the zero value, so empty Slices are `reflect.DeepEqual`.

**Assignment is a real copy, and free.** After `other := inv.Items`, an edit to either never shows in
the other. Unlike a Go slice, two copies that both `Append` never overwrite each other.

**Elements come out by value.** `s.At(i)` returns a copy of the element. To change a field of one
element, read it, change it, and put it back:

```go
it := inv.Items.At(i)
it.Qty++
inv.Items.Set(i, it)
```

A Slice nested in an element is no exception: `inner := s.At(i)` is a copy, so edit `inner` and put
it back with `s.Set(i, inner)`.

**Reading is free.** `Get` and reading a Slice allocate nothing. An edit copies about 32 elements and
a few small tree nodes, however long the Slice is; see [Costs](#costs).

**Loop with `All` or `Values`, not `At(i)`.** `At` walks down from the root on every call, while the
iterators walk each chunk once: about twice as fast at 4,096 elements.

**Build in one call.** `SliceOf(items...)` or `Collect(seq)` builds the whole Slice at once. `Append` in
a loop copies the tail on every call.

**Change every element with `UpdateFunc`, not `Set` in a loop.** `UpdateFunc` rebuilds once; each `Set`
copies a chunk and its path. At 4,096 elements that is about 40 µs against 1.4 ms.

**Read part of a Slice with `Range(lo, hi)`.** It goes straight to `lo`, where a loop of `At` calls
walks down from the root for every element.

**Remove one element by moving the last one into its place, when order does not matter:**

```go
last := s.Len() - 1
s.Set(i, s.At(last))
s.Delete(last, last+1)
```

That costs O(log₃₂ n), where `Delete(i, i+1)` in the middle rebuilds the Slice. To remove many while
looping, use `DeleteFunc`; a hand-written loop doing this must run from the last index down.

**Map works the same way.** Edit it, then `Set`. See [Map](#map).

### Common mistakes

| Wrong                                          | Right                                   |
| ---------------------------------------------- | --------------------------------------- |
| `c.Items = c.Items.Append(x)` (does not compile) | `c.Items.Append(x)`                   |
| editing `c.Items` and never calling `Set`      | `entity.Set(c)` after editing           |
| `c.Items.At(i).Qty++` (does not compile)       | read, change, `c.Items.Set(i, it)`      |
| `Items []Item` in a component                  | `Items immutable.Slice[Item]`           |
| `immutable.Slice[*Item]`                       | `immutable.Slice[Item]`                 |
| `entity.Get[C]()` inside an inner loop         | `Get` once, edit, `Set` once            |
| a queue of thousands: `Append`, then `Delete(0, 1)` | a head index; see [Costs](#costs)  |

### Full example

One system that uses every Slice function. A comment marks each one.

```go
import (
	"cmp"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

type Item struct {
	ID  string
	Qty int
}

type Inventory struct {
	Items  immutable.Slice[Item]   `json:"items"`
	Recent immutable.Slice[string] `json:"recent"` // IDs picked up, newest last
}

func (Inventory) Name() string { return "inventory" }

type Pickup struct{ Item Item }

func (Pickup) Name() string { return "pickup" }

type InventoryChanged struct {
	IDs    immutable.Slice[string] `json:"ids"`    // biggest stack first
	Sorted immutable.Slice[string] `json:"sorted"` // A to Z
	Newest immutable.Slice[string] `json:"newest"` // newest pickup first
	Total  int                     `json:"total"`
}

func (InventoryChanged) Name() string { return "inventory-changed" }

// NewInventory starts every player with a map.
func NewInventory() Inventory {
	return Inventory{Items: immutable.SliceOf(Item{ID: "map", Qty: 1})} // SliceOf
}

type InventorySystem struct{}

func (s *InventorySystem) Run(w *cardinal.World) {
	var pickups []Item
	for cmd := range w.Commands[Pickup]() {
		pickups = append(pickups, cmd.Payload.Item)
	}

	for e := range w.Contains[struct{ Inv Inventory }]().Iter() {
		inv := e.Get[Inventory]() // nothing below reaches the world until Set

		// Plain assignment: free copies that the edits below never touch.
		oldItems, oldRecent := inv.Items, inv.Recent

		for _, p := range pickups {
			// Stack onto an existing item, or add a new one.
			if i := inv.Items.IndexFunc(func(it Item) bool { return it.ID == p.ID }); i >= 0 { // IndexFunc
				it := inv.Items.At(i) // At: a copy of the element
				it.Qty += p.Qty
				inv.Items.Set(i, it) // Set: put it back
			} else {
				inv.Items.Append(p) // Append
			}
			inv.Recent.Append(p.ID)
		}

		// Everyone keeps a map, in the first slot.
		if !inv.Items.ContainsFunc(func(it Item) bool { return it.ID == "map" }) { // ContainsFunc
			inv.Items.Insert(0, Item{ID: "map", Qty: 1}) // Insert
		}

		// Drop empty stacks, then put the biggest first. Stable: equal stacks keep their order.
		inv.Items.DeleteFunc(func(it Item) bool { return it.Qty <= 0 })                    // DeleteFunc
		inv.Items.SortStableFunc(func(a, b Item) int { return cmp.Compare(b.Qty, a.Qty) }) // SortStableFunc

		// Keep the 5 newest pickups; clear the list on a quiet tick.
		if len(pickups) == 0 {
			inv.Recent.Reset() // Reset
		} else if n := inv.Recent.Len(); n > 5 { // Len
			inv.Recent.Delete(0, n-5) // Delete: removes [0, n-5)
		}

		sameItems := inv.Items.EqualFunc(oldItems, func(a, b Item) bool { return a == b }) // EqualFunc
		sameRecent := immutable.Equal(inv.Recent, oldRecent)                               // Equal
		if sameItems && sameRecent {
			continue // nothing changed: no Set, no event
		}
		e.Set(inv) // editing inv after this changes nothing stored

		total := 0
		for it := range inv.Items.Values() { // Values
			total += it.Qty
		}
		ids := immutable.Collect(func(yield func(string) bool) { // Collect: from any iterator
			for _, it := range inv.Items.All() { // All: index and element
				if !yield(it.ID) {
					return
				}
			}
		})
		sorted := ids // a free copy: sorting it leaves ids as it is
		immutable.Sort(&sorted) // Sort
		newest := inv.Recent
		newest.Reverse() // Reverse

		if immutable.Contains(inv.Recent, "golden-key") { // Contains
			slot := immutable.Index(ids, "golden-key") // Index
			w.Logger().Info().Int("slot", slot).Msg("golden key picked up")
		}
		for i, id := range inv.Recent.Backward() { // Backward: newest to oldest
			w.Logger().Debug().Int("index", i).Str("id", id).Msg("recent pickup")
		}

		w.Broadcast(InventoryChanged{IDs: ids, Sorted: sorted, Newest: newest, Total: total}) // later edits never change what was sent
	}
}
```

### Coming from the old derive API

Older versions returned a new Slice from every edit. Translate:

| Old                                  | New                                         |
| ------------------------------------ | ------------------------------------------- |
| `s = s.Append(x)`                    | `s.Append(x)`                               |
| `s = s.With(i, v)`                   | `s.Set(i, v)`                               |
| `s = s.Without(i)`                   | `s.Delete(i, i+1)`                          |
| `s = s.Filter(keep)`                 | `s.DeleteFunc(func(v T) bool { return !keep(v) })` |
| `s = s.Insert(i, x)`                 | `s.Insert(i, x)`                            |
| `s = s.Delete(i, j)`                 | `s.Delete(i, j)`                            |
| `s = s.Sub(i, j)`                    | `s.Delete(j, s.Len())` then `s.Delete(0, i)` |
| `s = s.Reversed()`                   | `s.Reverse()`                               |
| `s = s.SortedFunc(cmp)`              | `s.SortStableFunc(cmp)`                     |
| `s = immutable.Sorted(s)`            | `immutable.Sort(&s)`                        |

`Chunk`, `Repeat`, `Replace`, `Compact`, `CompactFunc`, the `immutable.Map` function, `Concat`, `Min`, `Max`,
`MinFunc`, `MaxFunc`, `IsSorted`, `BinarySearch` and the `Compare` functions are gone. Use a loop,
or the `slices` package on `slices.Collect(s.Values())`. `immutable.Map` is now the Map type, so a
leftover `immutable.Map(s, f)` call fails with a "cannot use generic type immutable.Map … without
instantiation" error rather than "undefined".

## Map

### When to use it

A component may not hold a plain `map[K]V` (`RegisterComponent` panics). Use:

- a struct with one field per key, when the set of keys is fixed.
- `immutable.Map[K, V]`, when it is not. `cardinal.Map[K, V]` is the same type.

```go
type Threat struct {
	Table immutable.Map[string, int64] `json:"table"` // damage per player ID
}
```

Keys are integers, strings, or named types of them (`type PlayerID uint64`). Anything else does not
compile. Values follow the same rule as Slice elements.

### The rule is the same: edit, then `Set`

```go
th := boss.Get[Threat]()
v, _ := th.Table.Get(player)
th.Table.Set(player, v+damage)
boss.Set(th)
```

As with Slice, nothing is written once a Map holds it, so `Get`, `Set`, assignment, events and
commands copy nothing and stay safe.

### API

| Go                                  | `immutable.Map`                                   |
| ----------------------------------- | ------------------------------------------------- |
| `len(m)`                            | `m.Len()`                                         |
| `v, ok := m[k]`                     | `v, ok := m.Get(k)`                               |
| `_, ok := m[k]`                     | `m.Has(k)`                                        |
| `m[k] = v`                          | `m.Set(k, v)`                                     |
| `delete(m, k)`                      | `m.Delete(k)`                                     |
| `maps.DeleteFunc(m, del)`           | `m.DeleteFunc(del)`                               |
| `for k, v := range m { m[k] = f(k, v) }` | `m.UpdateFunc(f)`, one rebuild               |
| `clear(m)`                          | `m.Clear()`                                       |
| `for k, v := range m`               | `for k, v := range m.All()`, in key order         |
| `maps.Keys(m)`, `maps.Values(m)`    | `m.Keys()`, `m.Values()`, in key order            |
| `maps.Equal`, `maps.EqualFunc`      | `immutable.MapEqual(a, b)`, `a.EqualFunc(b, eq)`  |
| a map literal, `maps.Collect(seq)`  | `immutable.MapFrom(seq)`                          |
| `maps.Clone(m)`                     | `c := m`, a free copy                             |
| no equivalent: the largest key      | `k, ok := m.MaxKey()`                             |
| no equivalent: keys in [lo, hi)     | `for k, v := range m.Range(lo, hi)`, in key order |

The smallest key is the first one `Keys` yields. `MaxKey` and `Range` exist because a scan to the
end or to `lo` would cost O(n), where they go straight there in O(log n).

To build from a Go map, use `immutable.MapFrom(maps.All(goMap))`. The result is the same whatever
order the Go map yields.

### Things to know

**Loops run in key order.** Entries are kept sorted by key, so `All`, `Keys`, `Values` and the
encoding always run smallest key first. Keys are hashed only to decide where storage chunks end,
never to order them. Two servers running the same world loop in the same
order, and the same entries give the same bytes. "First" means smallest key; if insertion order
matters, store it in the value.

**Values come out by value**, as with a Go map of structs, where `m[k].HP++` does not compile either.
Read, change, `Set`.

**Every edit costs about the same.** Changing, adding or removing a key copies one chunk of about 32
entries and a short path, O(log n); see [Costs](#costs). To build a large Map at once, use `MapFrom`,
which sorts once.

**An empty Map is always the zero value**, however it was emptied.

**Editing inside a loop is safe.** A loop over `All`, `Keys` or `Values` reads the Map as it was when
the loop started, so `Set` and `Delete` inside it never skip or repeat entries. The loop does not see
its own edits.

**Assignment is a real copy, and free**, as with Slice.

**Don't key by entity ID** unless the system that destroys those entities also deletes their keys.
Entity IDs are reused, so a stale key silently refers to the next entity in that slot. Key long-lived
data by an ID the game owns, such as a player ID.

### Common mistakes

| Wrong                                          | Right                                   |
| ---------------------------------------------- | --------------------------------------- |
| editing `c.Table` and never calling `Set`      | `entity.Set(c)` after editing           |
| relying on the order keys were added           | store the order in the value            |
| `Set` in a loop to build thousands of keys     | `immutable.MapFrom(seq)`                |
| `immutable.Map[EntityID, V]` for long-lived data | key by an ID the game owns            |

### Full example

One system that uses every Map function. A comment marks each one.

```go
import (
	"maps"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

type Threat struct {
	Table immutable.Map[string, int64]  `json:"table"` // damage per player ID
	Bans  immutable.Map[string, uint64] `json:"bans"`  // player ID to the tick the ban ends
}

func (Threat) Name() string { return "threat" }

type Hit struct {
	Attacker string
	Damage   int64
}

func (Hit) Name() string { return "hit" }

type TopThreat struct {
	Player string                       `json:"player"`
	Table  immutable.Map[string, int64] `json:"table"`
}

func (TopThreat) Name() string { return "top-threat" }

// NewThreat starts every boss ignoring the test accounts for the first 1000 ticks.
func NewThreat() Threat {
	return Threat{Bans: immutable.MapFrom(maps.All(map[string]uint64{"tester-1": 1000, "tester-2": 1000}))} // MapFrom
}

type ThreatSystem struct{}

func (s *ThreatSystem) Run(w *cardinal.World) {
	now := w.TickHeight()
	var hits []Hit
	for cmd := range w.Commands[Hit]() {
		hits = append(hits, cmd.Payload)
	}

	for e := range w.Contains[struct{ Threat Threat }]().Iter() {
		th := e.Get[Threat]() // nothing below reaches the world until Set

		// Plain assignment: free copies that the edits below never touch.
		oldTable, oldBans := th.Table, th.Bans

		for _, h := range hits {
			if end, banned := th.Bans.Get(h.Attacker); banned && end > now { // Get
				continue
			}
			if !th.Table.Has(h.Attacker) { // Has
				w.Logger().Info().Str("player", h.Attacker).Msg("new attacker")
			}
			v, _ := th.Table.Get(h.Attacker)     // Get: the zero value when absent
			th.Table.Set(h.Attacker, v+h.Damage) // Set
		}

		th.Table.Delete("")                                                 // Delete: does nothing if absent
		th.Table.DeleteFunc(func(_ string, v int64) bool { return v <= 0 }) // DeleteFunc: healing cancelled it out
		th.Bans.DeleteFunc(func(_ string, end uint64) bool { return end <= now })
		if th.Table.Len() > 100 { // Len
			th.Table.Clear() // Clear: reset a boss the whole server has hit
		}

		sameTable := immutable.MapEqual(th.Table, oldTable)                              // MapEqual
		sameBans := th.Bans.EqualFunc(oldBans, func(a, b uint64) bool { return a == b }) // EqualFunc
		if sameTable && sameBans {
			continue // nothing changed: no Set, no event
		}
		e.Set(th) // editing th after this changes nothing stored

		// All runs in key order, so on a tie the smallest player ID wins, on every server.
		top, best := "", int64(0)
		for player, v := range th.Table.All() { // All
			if v > best {
				top, best = player, v
			}
		}
		var total int64
		for v := range th.Table.Values() { // Values
			total += v
		}
		for player := range th.Table.Keys() { // Keys
			w.Logger().Debug().Str("player", player).Msg("on threat table")
		}
		w.Logger().Info().Str("top", top).Int64("total", total).Msg("threat")

		w.Broadcast(TopThreat{Player: top, Table: th.Table}) // later edits never change what was sent
	}
}
```

## Sugar functions you write yourself

Slice and Map follow Go's `slices` and `maps`, but only add a function when it beats what you can
write with the existing ones; see [DESIGN.md](DESIGN.md#which-functions-exist). These have been asked
for, and cost the same written by hand:

| Go | Write instead |
| --- | --- |
| `slices.Min`, `Max`, `IsSorted`, `Compare` | a loop over `s.Values()` |
| `slices.BinarySearch(s, v)` | `sort.Search(s.Len(), func(i int) bool { return s.At(i) >= v })` |
| `slices.Compact(s)` | `DeleteFunc` with a closure that remembers the previous element |
| `slices.Replace(s, i, j, v...)` | `s.Delete(i, j)`, then `s.Insert(i, v...)` |
| `slices.Concat`, `AppendSeq` | `s.Append(slices.Collect(seq)...)` |
| `slices.Sorted(seq)` | `s := immutable.Collect(seq)`, then `immutable.Sort(&s)` |
| a plain `[]T` | `slices.AppendSeq(dst, s.Values())` |
| `maps.Insert`, `maps.Copy` | a loop of `m.Set` |
| the smallest key | the first key `m.Keys()` yields |

## Costs

n is the number of elements or entries. "How often" says when the worst case actually happens.

| Operation | Typical | Worst case | How often |
| --- | --- | --- | --- |
| Copy, `Get`, `Set` on the component | free | free | never more |
| Slice `At`, `Set`, `Append`, delete from the end | O(log₃₂ n): 1 to 4 tree levels up to 1M | the same | fixed by length |
| Slice `Insert` or `Delete` in the middle, delete from the front of a Slice over 32 elements | O(n) | O(n) | every call |
| Slice `Sort`, `SortStableFunc` | O(n log n) | O(n log n) | every call |
| Slice `Range(lo, hi)`, reading k elements | O(log₃₂ n + k) | the same | fixed by length |
| Map `Get`, `Has`, `MaxKey` | O(log n) | about 1.7 × log₂ n comparisons | not seen in practice |
| Map `Range(lo, hi)`, reading k entries | O(log n + k) | the same as `Get`, plus k | not seen in practice |
| Map `Set`, `Delete` | O(log n) | O(n) | needs keys built against the hash seed; see [DESIGN.md](DESIGN.md#why-the-hash-is-seeded) |
| `DeleteFunc`, `UpdateFunc` | O(n) | O(n) | every call; it is one pass |
| `MapFrom` | O(n log n) | O(n log n) | every call; it sorts once |
| Map decoding | O(n) | O(n log n) | only for input not in key order; this package always encodes in key order |

**Slice reads cost O(log₃₂ n), not O(1):** up to 4 tree levels below a million elements. It is the
price of cheap edits; [DESIGN.md](DESIGN.md#slice-a-32-way-trie-over-the-index) explains it and why
nodes have 32 children.

**Long queues need a head index.** Dropping the front of a Slice and appending is fine at game sizes
(about 2 µs at 256 elements), but removing the front rebuilds the Slice, so it costs O(n) per pop:
27 µs at 4,096. Keep a head index next to the Slice and advance it instead, and drop the consumed front
in one `Delete(0, head)` once it passes half the Slice. A Map keyed by a sequence number also works,
and removes its smallest key in O(log n).

**Edits allocate.** Reads and copies allocate nothing, but an edit allocates once per node it
copies: once for a small value, a few times for a large one. In Go no
design with safe copies can avoid that: writing in place is only safe when nothing else holds the
storage, and Go cannot tell whether `d := a` happened. A small, bounded list that must not allocate
belongs in a fixed-size array field, such as `[16]Item` plus a count.

**The Map hash is seeded at random when the program starts**, so nobody can pick keys that slow a
Map down. Nothing changes for callers; [DESIGN.md](DESIGN.md#why-the-hash-is-seeded) explains why.
