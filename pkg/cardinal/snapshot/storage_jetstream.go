package snapshot

import (
	"context"
	"fmt"
	"io"
	"math"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/micro"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/caarlos0/env/v11"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultObjectName = "snapshot"

// JetStreamStorage implements SnapshotStorage using NATS JetStream ObjectStore.
type JetStreamStorage struct {
	os jetstream.ObjectStore
}

var _ Storage = (*JetStreamStorage)(nil)

// NewJetStreamStorage creates a new JetStream ObjectStore-based snapshot storage.
// It creates its own NATS client using the default configuration from environment variables.
func NewJetStreamStorage(opts JetStreamStorageOptions) (*JetStreamStorage, error) {
	if err := opts.Validate(); err != nil {
		return nil, eris.Wrap(err, "invalid options passed")
	}

	// Just parse the env here for now.
	// TODO: remove storage max bytes option or make it explicit.
	if err := env.Parse(&opts); err != nil {
		return nil, eris.Wrap(err, "failed to parse env")
	}

	js, err := jetstream.New(opts.Client.Conn)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create JetStream client")
	}

	ctx := context.Background()

	// Same format as streams because it regular service address format isn't accepted.
	bucketName := fmt.Sprintf("%s_%s_%s_snapshot",
		opts.Address.GetOrganization(), opts.Address.GetProject(), opts.Address.GetServiceId())

	if opts.SnapshotStorageMaxBytes > math.MaxInt64 {
		return nil, eris.New("snapshot storage max bytes exceeds maximum int64 value")
	}

	osConfig := jetstream.ObjectStoreConfig{
		Bucket:   bucketName,
		MaxBytes: int64(opts.SnapshotStorageMaxBytes), // Required by some NATS providers like Synadia Cloud
	}
	os, err := js.CreateObjectStore(ctx, osConfig)
	if err != nil {
		if eris.Is(err, jetstream.ErrBucketExists) {
			// Bucket already exists, get the existing one.
			os, err = js.ObjectStore(ctx, bucketName)
			if err != nil {
				return nil, eris.Wrapf(err, "failed to get existing ObjectStore (bucket=%s)", bucketName)
			}
		} else {
			return nil, eris.Wrapf(err, "failed to create ObjectStore (bucket=%s, maxBytes=%d)",
				osConfig.Bucket, osConfig.MaxBytes)
		}
	}

	return &JetStreamStorage{os: os}, nil
}

func (j *JetStreamStorage) Store(snapshot *Snapshot) error {
	var worldState cardinalv1.WorldState
	if err := proto.Unmarshal(snapshot.Data, &worldState); err != nil {
		return eris.Wrap(err, "failed to unmarshal world state")
	}
	snapshotPb := &cardinalv1.Snapshot{
		TickHeight: snapshot.TickHeight,
		Timestamp:  timestamppb.New(snapshot.Timestamp),
		WorldState: &worldState,
	}
	data, err := proto.Marshal(snapshotPb)
	if err != nil {
		return eris.Wrap(err, "failed to marshal snapshot")
	}

	// Overwrite the existing snapshot if any.
	if _, err = j.os.PutBytes(context.Background(), defaultObjectName, data); err != nil {
		return eris.Wrap(err, "failed to store snapshot in ObjectStore")
	}

	return nil
}

func (j *JetStreamStorage) Load() (*Snapshot, error) {
	object, err := j.os.Get(context.Background(), defaultObjectName)
	if err != nil {
		if eris.Is(err, jetstream.ErrObjectNotFound) {
			return nil, eris.New("no snapshot exists")
		}
		return nil, eris.Wrap(err, "failed to get snapshot from ObjectStore")
	}
	defer func() {
		_ = object.Close()
	}()

	data, err := io.ReadAll(object)
	if err != nil {
		return nil, eris.Wrap(err, "failed to read from object")
	}

	snapshotPb := cardinalv1.Snapshot{}
	if err = proto.Unmarshal(data, &snapshotPb); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal snapshot")
	}
	if err = protovalidate.Validate(&snapshotPb); err != nil {
		return nil, eris.Wrap(err, "failed to validate snapshot")
	}

	worldStateBytes, err := proto.Marshal(snapshotPb.GetWorldState())
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal world state")
	}

	return &Snapshot{
		TickHeight: snapshotPb.GetTickHeight(),
		Timestamp:  snapshotPb.GetTimestamp().AsTime(),
		Data:       worldStateBytes,
	}, nil
}

func (j *JetStreamStorage) Exists() bool {
	_, err := j.os.GetInfo(context.Background(), defaultObjectName)
	return err == nil
}

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

type JetStreamStorageOptions struct {
	Client  *micro.Client
	Address *micro.ServiceAddress

	// Maximum bytes for snapshot storage (ObjectStore). Required by some NATS providers like Synadia Cloud.
	SnapshotStorageMaxBytes uint64 `env:"CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES" envDefault:"0"`
}

func (opt *JetStreamStorageOptions) Validate() error {
	if opt.Client == nil {
		return eris.New("NATS client cannot be nil")
	}
	if opt.Address == nil {
		return eris.New("service address cannot be nil")
	}
	// SnapshotStorageMaxBytes can be 0 which means unlimited storage. No need to validate here.
	return nil
}
