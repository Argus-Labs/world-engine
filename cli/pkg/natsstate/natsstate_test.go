package natsstate_test

import (
	"testing"

	"github.com/argus-labs/world-engine/cli/pkg/natsstate"
)

func TestSnapshotBucket(t *testing.T) {
	// Must match pkg/cardinal/snapshot/storage_jetstream.go, which names the bucket from
	// the shard's project.serviceId address; the organization is not part of it.
	if got := natsstate.SnapshotBucket("rampage", "gameplay-2"); got != "rampage_gameplay-2_snapshot" {
		t.Fatalf("bucket = %q", got)
	}
}
