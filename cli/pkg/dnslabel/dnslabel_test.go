package dnslabel_test

import (
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/cli/pkg/dnslabel"
)

func TestSanitize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, in, want string
	}{
		{"empty", "", "unknown"},
		{"blank", "   ", "unknown"},
		{"all invalid", "!!!", "unknown"},
		{"lowercases", "Acme", "acme"},
		{"space to hyphen", "My Org", "my-org"},
		{"dots to hyphens", "a.b.c", "a-b-c"},
		{"trims edge hyphens", "--lead-trail--", "lead-trail"},
		{"collapses runs", "multi___under", "multi-under"},
		{"mixed", "UPPER_and.Mixed 99", "upper-and-mixed-99"},
		{"already a label", "already-fine", "already-fine"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dnslabel.Sanitize(c.in); got != c.want {
				t.Errorf("Sanitize(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMaxLength(t *testing.T) {
	t.Parallel()

	if dnslabel.MaxLength != 63 {
		t.Fatalf("MaxLength = %d, want 63 (DNS-1123 label max)", dnslabel.MaxLength)
	}
}

func TestIsCanonical(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		want  bool
	}{
		// Character rules (existing behaviour, unchanged by the length fix).
		{"already canonical", "my-game", true},
		{"single char", "a", true},
		{"digits and hyphens", "game-99", true},
		{"uppercase rejected", "MyGame", false},
		{"underscore rejected", "my_game", false},
		{"leading hyphen rejected", "-game", false},
		{"trailing hyphen rejected", "game-", false},
		{"dot rejected", "my.game", false},
		{"empty", "", false},

		// Length rules (the fix): 1–63 chars is allowed, 64+ is rejected even
		// when the character set is otherwise canonical.
		{"63 chars (boundary)", strings.Repeat("a", 63), true},
		{"64 chars (over boundary)", strings.Repeat("a", 64), false},
		{"100 chars", strings.Repeat("a", 100), false},
		{"63 chars with hyphens (boundary)", strings.Repeat("a", 30) + "-" + strings.Repeat("b", 32), true},
		{"64 chars with hyphens (over boundary)", strings.Repeat("a", 30) + "-" + strings.Repeat("b", 33), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := dnslabel.IsCanonical(c.value); got != c.want {
				t.Errorf("IsCanonical(%q) (len=%d) = %v, want %v", c.value, len(c.value), got, c.want)
			}
		})
	}
}

// TestSanitizeDoesNotTruncate locks the design decision that Sanitize normalizes
// characters but does not truncate to MaxLength. A caller that Sanitizes a long
// string gets a long result, which IsCanonical then rejects — so the caller can
// report the overflow rather than silently losing part of the name.
func TestSanitizeDoesNotTruncate(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 100)
	if got := dnslabel.Sanitize(long); len(got) != 100 {
		t.Fatalf("Sanitize truncated %d-char input to %d chars; it must not truncate", len(long), len(got))
	}
	// A 100-char sanitised value is appropriately non-canonical.
	if dnslabel.IsCanonical(dnslabel.Sanitize(long)) {
		t.Fatal("IsCanonical(Sanitize(100 chars)) = true, want false (should exceed MaxLength)")
	}
}
