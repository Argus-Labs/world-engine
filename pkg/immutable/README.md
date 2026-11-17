# immutable

The collections a Cardinal component may hold: `Slice` for lists and `Map` for lookup tables.
Written for AI agents and people writing game code. The source of truth is the Go doc in
`slice.go` and `map.go`; this file is the short version.

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

### The one rule: edit in place, then `Set`

Edit a Slice the way you would a Go slice, then store the component with `Set`.

```go
inv := entity.Get[Inventory]()
inv.Items.Append(item)
inv.Items.Set(0, other)
entity.Set(inv)
```

Why this is safe: Cardinal copies every Slice a component holds when `Get` hands it out and when
`Set` takes it back. So:

- An edit without `Set` never reaches the world, a snapshot or another system.
- An edit after `Set` never reaches what was stored.

Events and commands are copied the same way: `SendTo`, `Broadcast`, `SendToShard` and
`EmitSystemEvent` copy what you send, and every `SystemEvents` and `Commands` reader gets its own
copy. Editing a payload after sending it does not change what is sent.

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
| `s = s[:0]` or `s = nil`              | `s.Reset()`                       |
| `slices.SortStableFunc(s, cmp)`       | `s.SortStableFunc(cmp)`           |
| `slices.Sort(s)`                      | `immutable.Sort(&s)`              |
| `slices.Reverse(s)`                   | `s.Reverse()`                     |
| `for i, v := range s`                 | `for i, v := range s.All()`       |
| `for _, v := range s`                 | `for v := range s.Values()`       |
| `slices.Backward(s)`                  | `s.Backward()`                    |
| `slices.IndexFunc`, `ContainsFunc`    | `s.IndexFunc(f)`, `s.ContainsFunc(f)` |
| `slices.Equal`, `Index`, `Contains`   | `immutable.Equal(a, b)`, `immutable.Index(s, v)`, `immutable.Contains(s, v)` |
| `slices.EqualFunc`                    | `a.EqualFunc(b, eq)`              |
| `[]T{a, b}`                           | `immutable.SliceOf(a, b)`         |
| `slices.Collect(seq)`                 | `immutable.Collect(seq)`          |
| `slices.Clone(s)`                     | `s.Clone()`                       |

`At` and `Set` panic out of range, like indexing a Go slice. To get a plain `[]T` for another API,
use `slices.Collect(s.Values())`; `SliceOf(xs...)` copies its input, so later changes to `xs` do not
reach the Slice.

Edits have pointer receivers, so they only work on something addressable: a variable or a struct
field reached through a variable. `entity.Get[Inventory]().Items.Append(x)` does not compile; assign
the component to a variable first.

### Things to know

**Sorting is always stable.** There is no unstable sort, so the result never depends on the sort
algorithm a Go release uses. Same world, same bytes.

**An empty Slice is always the zero value.** Deleting every element, `Reset`, `SliceOf()` and decoding
`[]` or `null` all give the zero value, so empty Slices are `reflect.DeepEqual`.

**Assignment shares, as with a Go slice.** Inside one system, `other := inv.Items` shares the array.
An edit through `other` can show up in `inv.Items`, and two copies that both `Append` can overwrite
each other's new elements. Use `Clone()` when you need a copy that shares nothing. This never affects
stored state, only your own variables.

**Elements come out by value.** `s.At(i)` returns a copy of the element. To change a field of one
element, read it, change it, and put it back:

```go
it := inv.Items.At(i)
it.Qty++
inv.Items.Set(i, it)
```

A Slice nested in an element is the exception: `inner := s.At(i)` shares `inner`'s array with the
element, as a Go slice would, so `inner.Set(...)` changes `s`. To add to a nested Slice, which may
grow its array, put the element back with `s.Set(i, inner)`.

**Every `Get` copies.** `Get` copies every Slice in the component, even when you only read it. Get a
component once per entity per system and work on that copy; don't call `Get` again inside a loop.

**Map works the same way.** Edit it in place, then `Set`. See [Map](#map).

### Common mistakes

| Wrong                                          | Right                                   |
| ---------------------------------------------- | --------------------------------------- |
| `c.Items = c.Items.Append(x)` (does not compile) | `c.Items.Append(x)`                   |
| editing `c.Items` and never calling `Set`      | `entity.Set(c)` after editing           |
| `c.Items.At(i).Qty++` (does not compile)       | read, change, `c.Items.Set(i, it)`      |
| `Items []Item` in a component                  | `Items immutable.Slice[Item]`           |
| `immutable.Slice[*Item]`                       | `immutable.Slice[Item]`                 |
| `entity.Get[C]()` inside an inner loop         | `Get` once, edit the copy, `Set` once   |

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
		inv := e.Get[Inventory]() // a copy: nothing below reaches the world until Set

		// Clone: copies that share nothing, to see later whether anything changed.
		oldItems, oldRecent := inv.Items.Clone(), inv.Recent.Clone()

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
		e.Set(inv) // the world stores a copy; editing inv after this changes nothing stored

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
		sorted := ids.Clone()
		immutable.Sort(&sorted) // Sort
		newest := inv.Recent.Clone()
		newest.Reverse() // Reverse

		if immutable.Contains(inv.Recent, "golden-key") { // Contains
			slot := immutable.Index(ids, "golden-key") // Index
			w.Logger().Info().Int("slot", slot).Msg("golden key picked up")
		}
		for i, id := range inv.Recent.Backward() { // Backward: newest to oldest
			w.Logger().Debug().Int("index", i).Str("id", id).Msg("recent pickup")
		}

		w.Broadcast(InventoryChanged{IDs: ids, Sorted: sorted, Newest: newest, Total: total}) // copied on send
	}
}
```

### Coming from the old derive API

Older versions returned a new Slice from every edit, and had a builder. Translate:

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
| `b := s.Edit(); ...; s = b.Done()`   | edit `s` directly                           |
| `immutable.NewSliceBuilder[T](n)`    | `var s immutable.Slice[T]` and `Append`     |

`Chunk`, `Repeat`, `Replace`, `Compact`, `CompactFunc`, `Transform`, `Concat`, `Min`, `Max`,
`MinFunc`, `MaxFunc`, `IsSorted`, `BinarySearch` and the `Compare` functions are gone. Use a loop,
or the `slices` package on `slices.Collect(s.Values())`.

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

### The rule is the same: edit in place, then `Set`

```go
th := boss.Get[Threat]()
v, _ := th.Table.Get(player)
th.Table.Set(player, v+damage)
boss.Set(th)
```

Cardinal copies every Map on `Get`, `Set`, events and commands, exactly as it copies a Slice.

### API

| Go                                  | `immutable.Map`                                   |
| ----------------------------------- | ------------------------------------------------- |
| `len(m)`                            | `m.Len()`                                         |
| `v, ok := m[k]`                     | `v, ok := m.Get(k)`                               |
| `_, ok := m[k]`                     | `m.Has(k)`                                        |
| `m[k] = v`                          | `m.Set(k, v)`                                     |
| `delete(m, k)`                      | `m.Delete(k)`                                     |
| `maps.DeleteFunc(m, del)`           | `m.DeleteFunc(del)`                               |
| `clear(m)`                          | `m.Clear()`                                       |
| `for k, v := range m`               | `for k, v := range m.All()`, in key order         |
| `maps.Keys(m)`, `maps.Values(m)`    | `m.Keys()`, `m.Values()`, in key order            |
| `maps.Equal`, `maps.EqualFunc`      | `immutable.MapEqual(a, b)`, `a.EqualFunc(b, eq)`  |
| a map literal, `maps.Collect(seq)`  | `immutable.MapFrom(seq)`                          |
| `maps.Clone(m)`                     | `m.Clone()`                                       |

To build from a Go map, use `immutable.MapFrom(maps.All(goMap))`. The result is the same whatever
order the Go map yields.

### Things to know

**Loops run in key order.** Entries are kept sorted by key, never hashed, so `All`, `Keys`, `Values`
and the encoding always run smallest key first. Two servers running the same world loop in the same
order, and the same entries give the same bytes. "First" means smallest key; if insertion order
matters, store it in the value.

**Values come out by value**, as with a Go map of structs, where `m[k].HP++` does not compile either.
Read, change, `Set`.

**New keys cost more than updates.** Changing an existing key's value is a binary search. Adding or
removing a key shifts the entries after it, which is cheap at component sizes. To build a large Map
at once, use `MapFrom`, which sorts once.

**An empty Map is always the zero value**, however it was emptied.

**Deleting inside a loop is safe; adding is not.** As with a Go map, you can `Delete` the current key
(or any key) while ranging over `All`, `Keys` or `Values`, and the loop still visits every other entry
once. Adding a new key with `Set` inside such a loop can skip or repeat entries, so collect the keys
first: `for _, k := range slices.Collect(m.Keys())`. Changing the value of an existing key is fine.

**Assignment shares, as with Slice.** `other := th.Table` shares storage, so a value changed through
one can show in the other, though a key added or removed through one may not. Use `Clone()` for a
copy that shares nothing.

**Don't key by entity ID** unless the system that destroys those entities also deletes their keys.
Entity IDs are reused, so a stale key silently refers to the next entity in that slot. Key long-lived
data by an ID the game owns, such as a player ID.

### Common mistakes

| Wrong                                          | Right                                   |
| ---------------------------------------------- | --------------------------------------- |
| `m = m.With(k, v)` (does not compile)          | `m.Set(k, v)`                           |
| editing `c.Table` and never calling `Set`      | `entity.Set(c)` after editing           |
| relying on the order keys were added           | store the order in the value            |
| `Set` in a loop to build thousands of keys     | `immutable.MapFrom(seq)`                |
| `Set` of a new key inside `for ... range m.All()` | loop over `slices.Collect(m.Keys())` |
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
		th := e.Get[Threat]() // a copy: nothing below reaches the world until Set

		// Clone: copies that share nothing, to see later whether anything changed.
		oldTable, oldBans := th.Table.Clone(), th.Bans.Clone()

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
		e.Set(th) // the world stores a copy; editing th after this changes nothing stored

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

		w.Broadcast(TopThreat{Player: top, Table: th.Table}) // copied on send
	}
}
```

### Coming from the old derive API

| Old                                  | New                                         |
| ------------------------------------ | ------------------------------------------- |
| `m = m.With(k, v)`                   | `m.Set(k, v)`                               |
| `m = m.Without(k)`                   | `m.Delete(k)`                               |
| `m = m.Update(k, f)`                 | `v, _ := m.Get(k); m.Set(k, f(v))`          |
| `m = m.DeleteFunc(del)`              | `m.DeleteFunc(del)`                         |
| `b := m.Edit(); ...; m = b.Done()`   | the same edits on `m` directly              |
