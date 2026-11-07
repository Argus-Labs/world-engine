// Package natsstate wipes shard snapshot buckets in JetStream. Buckets are named
// "<project>_<instance>_snapshot" (pkg/cardinal/snapshot/storage_jetstream.go); callers
// stop the shard before wiping.
package natsstate

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rotisserie/eris"
)

const snapshotBucketSuffix = "_snapshot"

// SnapshotBucket is the object store a shard instance snapshots into.
func SnapshotBucket(project, instanceID string) string {
	return fmt.Sprintf("%s_%s%s", project, instanceID, snapshotBucketSuffix)
}

// PurgeShard deletes one instance's snapshot bucket; a missing bucket is fine.
func PurgeShard(ctx context.Context, natsURL, project, instanceID string) error {
	if project == "" || instanceID == "" {
		return eris.New("PurgeShard: project and instanceID are required")
	}
	js, done, err := connect(natsURL)
	if err != nil {
		return err
	}
	defer done()
	bucket := SnapshotBucket(project, instanceID)
	if err := js.DeleteObjectStore(ctx, bucket); err != nil && !errors.Is(err, jetstream.ErrBucketNotFound) {
		return eris.Wrapf(err, "delete snapshot bucket %s", bucket)
	}
	return nil
}

func connect(natsURL string) (jetstream.JetStream, func(), error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, eris.Wrapf(err, "connect to NATS at %s", natsURL)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, eris.Wrap(err, "create jetstream context")
	}
	return js, nc.Close, nil
}
