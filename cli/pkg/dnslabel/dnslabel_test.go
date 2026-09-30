package dnslabel_test

import (
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
