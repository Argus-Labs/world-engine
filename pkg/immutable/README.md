# immutable

The collections a Cardinal component may hold: `Slice` for lists and `Map` for lookup tables.
Written for AI agents and people writing game code. The source of truth is the Go doc in
`slice.go` and `map.go`; this file is the short version. Map is documented in `map.go` for now.

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

**Map is different.** `immutable.Map` is never edited in place. Its derivations return a new Map
(`m = m.With(k, v)`). Don't mix up the two styles.

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
