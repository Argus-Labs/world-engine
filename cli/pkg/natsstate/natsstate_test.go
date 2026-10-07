package natsstate_test

import (
	"testing"

	"github.com/argus-labs/world-engine/cli/pkg/natsstate"
)

func TestSnapshotBucket(t *testing.T) {
	if got := natsstate.SnapshotBucket("argus", "rampage", "gameplay-2"); got != "argus_rampage_gameplay-2_snapshot" {
		t.Fatalf("bucket = %q", got)
	}
}
