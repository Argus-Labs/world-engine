package sdkgen

import (
	"slices"
	"strings"
	"testing"
)

// newWireGenForTest builds a wireGen whose owner import has already been registered the way
// RenderGoWire does, so the call sites under test see the accumulator in its real post-newWireGen state.
func newWireGenForTest() *wireGen { return newWireGen("example.com/mygame/gen/pkg") }

// TestRequalifyGoType pins the helper hand relies on to align a mirror's Go type with the registered
// import: it rewrites discovery's hand-alias prefix to the qualifier the wire file actually imports, and
// leaves everything else (same-package, no-prefix, and already-matching) untouched.
func TestRequalifyGoType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		goType string
		from   string
		to     string
		want   string
	}{
		{
			name:   "rewrite alias to bare",
			goType: "known_timestamppb.Timestamp",
			from:   "known_timestamppb", to: "timestamppb",
			want: "timestamppb.Timestamp",
		},
		{
			name:   "no-op when already matching",
			goType: "known_timestamppb.Timestamp",
			from:   "known_timestamppb", to: "known_timestamppb",
			want: "known_timestamppb.Timestamp",
		},
		{
			name:   "no-op when no source prefix",
			goType: "MoveCommand", from: "", to: "timestamppb",
			want: "MoveCommand",
		},
		{
			name:   "no-op when prefix absent",
			goType: "dep_stamp.Stamp", from: "known_timestamppb", to: "timestamppb",
			want: "dep_stamp.Stamp",
		},
		{
			name:   "preserve generic suffix",
			goType: "pkg.Foo[bar.Baz]", from: "pkg", to: "alispkg",
			want: "alispkg.Foo[bar.Baz]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := requalifyGoType(tc.goType, tc.from, tc.to); got != tc.want {
				t.Errorf("requalifyGoType(%q, %q, %q) = %q; want %q", tc.goType, tc.from, tc.to, got, tc.want)
			}
		})
	}
}

// TestSpecFirstRegistrationWins pins that spec never displaces an entry need or an earlier spec already
// claimed. Upgrading a bare (need) entry to a hand alias would orphan the bare [path.Base] reference
// need's caller wrote — the original bug report's recommended fix did exactly that and just moved the
// refusal from known_timestamppb to timestamppb. First registration wins, and the body adapts via
// qualify/hand.
func TestSpecFirstRegistrationWins(t *testing.T) {
	t.Parallel()

	t.Run("need then spec keeps bare", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		g.need(timestampPkg)
		g.spec(`known_timestamppb "google.golang.org/protobuf/types/known/timestamppb"`)
		if got := g.imports[timestampPkg]; got != "" {
			t.Errorf("need then spec: alias must stay bare; got %q — upgrading orphans the bare body ref", got)
		}
	})

	t.Run("spec then need keeps alias", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		g.spec(`known_timestamppb "google.golang.org/protobuf/types/known/timestamppb"`)
		g.need(timestampPkg)
		if got := g.imports[timestampPkg]; got != "known_timestamppb" {
			t.Errorf("spec then need: alias should win %q, got %q", "known_timestamppb", got)
		}
	})

	t.Run("first spec alias wins over a later spec", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		g.spec(`first "google.golang.org/protobuf/types/known/timestamppb"`)
		g.spec(`second "google.golang.org/protobuf/types/known/timestamppb"`)
		if got := g.imports[timestampPkg]; got != "first" {
			t.Errorf("first spec alias should win, got %q", got)
		}
	})
}

// TestQualifyAgreesWithRegisteredAlias pins the body-side half of the fix: qualify returns the alias
// whoever claimed the path first registered, and only falls back to the bare leaf when nothing has
// claimed it yet. Both field orders (time.Time before the timestamppb mirror, and after) must produce a
// qualifier the import block agrees with.
func TestQualifyAgreesWithRegisteredAlias(t *testing.T) {
	t.Parallel()

	t.Run("unclaimed registers bare leaf", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		if got := g.qualify(timestampPkg); got != "timestamppb" {
			t.Fatalf("qualify on an unclaimed path = %q; want bare leaf %q", got, "timestamppb")
		}
		if got := g.imports[timestampPkg]; got != "" {
			t.Errorf("qualify should register a bare entry, got alias %q", got)
		}
	})

	t.Run("bare claim keeps bare leaf", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		g.need(timestampPkg) // a time.Time field processed first, as the bug scenario's When field is
		if got := g.qualify(timestampPkg); got != "timestamppb" {
			t.Fatalf("qualify after need = %q; want bare leaf %q", got, "timestamppb")
		}
		if got := g.imports[timestampPkg]; got != "" {
			t.Errorf("bare entry should be preserved, got alias %q", got)
		}
	})

	t.Run("alias claim wins", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		g.spec(`known_timestamppb "google.golang.org/protobuf/types/known/timestamppb"`) // the mirror field first
		if got := g.qualify(timestampPkg); got != "known_timestamppb" {
			t.Fatalf("qualify after spec = %q; want the hand alias %q", got, "known_timestamppb")
		}
	})

	t.Run("empty path is a no-op", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		if got := g.qualify(""); got != "" {
			t.Errorf("qualify(\"\") = %q; want \"\" for a same-package type", got)
		}
		if _, ok := g.imports[""]; ok {
			t.Errorf("qualify(\"\") should not register an empty-path import")
		}
	})
}

// TestHandRequalifiesToRegisteredAlias pins the mirror-side half of the fix: hand returns the type
// spelled the way the import block will actually name the package, so a mirror whose package need
// already claimed bare (time.Time + raw timestamppb.Timestamp in one file) is written timestamppb.X
// rather than known_timestamppb.X.
func TestHandRequalifiesToRegisteredAlias(t *testing.T) {
	t.Parallel()

	tsRef := TypeRef{
		Name:    "Timestamp",
		GoType:  "known_timestamppb.Timestamp",
		Import:  `known_timestamppb "google.golang.org/protobuf/types/known/timestamppb"`,
		PkgPath: "google.golang.org/protobuf/types/known/timestamppb",
	}

	t.Run("bare claim rewrites mirror to bare", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		g.need(timestampPkg) // time.Time field processed first
		if got := g.hand(tsRef); got != "timestamppb.Timestamp" {
			t.Fatalf("hand after need = %q; want bare %q", got, "timestamppb.Timestamp")
		}
		if got := g.imports[timestampPkg]; got != "" {
			t.Errorf("import should stay bare, got alias %q", got)
		}
	})

	t.Run("alias claim keeps the hand alias", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		// mirror field processed first: spec claims the hand alias, no bare need precedes it.
		if got := g.hand(tsRef); got != "known_timestamppb.Timestamp" {
			t.Fatalf("hand with no prior need = %q; want the hand alias %q", got, "known_timestamppb.Timestamp")
		}
		if got := g.imports[timestampPkg]; got != "known_timestamppb" {
			t.Errorf("import should be the hand alias, got %q", got)
		}
	})
}

// TestImportSpecsEmitsOneEntryPerPath pins that the timestamppb path yields exactly one import line —
// bare when need won, aliased when spec won — so the body and the header never disagree over one package.
func TestImportSpecsEmitsOneEntryPerPath(t *testing.T) {
	t.Parallel()

	t.Run("bare entry emits an unaliased spec", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		g.need(timestampPkg)
		g.spec(`known_timestamppb "google.golang.org/protobuf/types/known/timestamppb"`)
		specs := g.importSpecs()
		if got := slices.Index(specs, `"google.golang.org/protobuf/types/known/timestamppb"`); got < 0 {
			t.Errorf("bare import spec missing; got %v", specs)
		}
		for _, s := range specs {
			if strings.Contains(s, "known_timestamppb ") {
				t.Errorf("aliased spec should NOT be emitted when bare won; got %q", s)
			}
		}
	})

	t.Run("alias entry emits one aliased spec", func(t *testing.T) {
		t.Parallel()
		g := newWireGenForTest()
		g.spec(`known_timestamppb "google.golang.org/protobuf/types/known/timestamppb"`)
		g.need(timestampPkg)
		specs := g.importSpecs()
		count := 0
		for _, s := range specs {
			if strings.Contains(s, "google.golang.org/protobuf/types/known/timestamppb") {
				if !strings.Contains(s, `known_timestamppb "`) {
					t.Errorf("the timestamppb path should be imported under the hand alias, got %q", s)
				}
				count++
			}
		}
		if count != 1 {
			t.Errorf("want exactly one import spec for the timestamppb path, got %d (%v)", count, specs)
		}
	})
}
