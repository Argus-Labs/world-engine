package immutable

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"unsafe"
	"weak"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPayload returns a value in its own allocation and a weak pointer to it.
func newPayload(i int) (string, weak.Pointer[byte]) {
	payload := strings.Repeat(string(rune('a'+i%26)), 4<<10)
	return payload, weak.Make(unsafe.StringData(payload))
}

// reachable collects garbage and counts the values that are still alive.
//
//go:noinline
func reachable(refs []weak.Pointer[byte]) int {
	runtime.GC()
	runtime.GC()

	n := 0

	for _, ref := range refs {
		if ref.Value() != nil {
			n++
		}
	}

	return n
}

// mapFromDuplicates builds a Map from distinct unique keys, each with a short value, followed by
// repeats values under the key "~", which sorts after them. It returns weak pointers to the values
// yielded under "~"; the last one wins. It doesn't inline, so no value lingers in the caller's frame.
//
//go:noinline
func mapFromDuplicates(distinct, repeats int) (Map[string, string], []weak.Pointer[byte]) {
	refs := make([]weak.Pointer[byte], 0, repeats)
	m := MapFrom(func(yield func(string, string) bool) {
		for i := range distinct {
			if !yield(fmt.Sprintf("k%03d", i), "short") {
				return
			}
		}

		for i := range repeats {
			payload, ref := newPayload(i)
			refs = append(refs, ref)

			if !yield("~", payload) {
				return
			}
		}
	})

	return m, refs
}

// MapFrom keeps only the last value under a repeated key, so the Map it returns must not keep the
// values it dropped alive.
func TestMapFrom_ReleasesDroppedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		distinct, repeats int
	}{
		// Almost every entry is dropped.
		{name: "one key", distinct: 0, repeats: 32},
		// Few are dropped, and they sort last, where compacting in place leaves them behind. MapFrom's
		// array then holds at most twice the entries it keeps, so shrink keeps it rather than copy,
		// and only clearing the dropped entries frees them.
		{name: "few repeats", distinct: 300, repeats: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.distinct > 0 {
				// MapFrom appends every pair to one array, so this array's capacity is the one it gets.
				var entries []entry[string, string]
				for range tt.distinct + tt.repeats {
					entries = append(entries, entry[string, string]{})
				}

				require.LessOrEqual(t, cap(entries), 2*(tt.distinct+1), "shrink would copy, so clear goes untested")
			}

			m, refs := mapFromDuplicates(tt.distinct, tt.repeats)
			require.Equal(t, tt.distinct+1, m.Len())

			got, ok := m.Get("~")
			require.True(t, ok)
			require.Same(t, refs[len(refs)-1].Value(), unsafe.StringData(got), "the last value wins")

			dropped, kept := refs[:len(refs)-1], refs[len(refs)-1:]
			assert.Zero(t, reachable(dropped), "dropped values stay reachable from the Map")
			assert.Equal(t, 1, reachable(kept), "the kept value is lost")
			runtime.KeepAlive(m)
		})
	}
}
