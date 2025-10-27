package micro

import (
	"context"
	"fmt"
	"io"
	"math"

	"github.com/caarlos0/env/v11"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// JetStreamSnapshotStorage implements SnapshotStorage using NATS JetStream ObjectStore.
type JetStreamSnapshotStorage struct {
	os         jetstream.ObjectStore
	objectName string
}

var _ SnapshotStorage = (*JetStreamSnapshotStorage)(nil)

// NewJetStreamSnapshotStorage creates a new JetStream ObjectStore-based snapshot storage.
// It creates its own NATS client using the default configuration from environment variables.
func NewJetStreamSnapshotStorage(opts JetStreamSnapshotStorageOptions) (*JetStreamSnapshotStorage, error) {
	js, err := jetstream.New(opts.client.Conn)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create JetStream client")
	}

	ctx := context.Background()

	// Same format as streams because it regular service address format isn't accepted.
	bucketName := fmt.Sprintf("%s_%s_%s_snapshot",
		opts.address.GetOrganization(), opts.address.GetProject(), opts.address.GetServiceId())

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

	return &JetStreamSnapshotStorage{os: os, objectName: opts.ObjectName}, nil
}

func (j *JetStreamSnapshotStorage) Store(snapshot *Snapshot) error {
	data, err := proto.Marshal(snapshot)
	if err != nil {
		return eris.Wrap(err, "failed to marshal snapshot")
	}

	// Overwrite the existing snapshot if any.
	if _, err = j.os.PutBytes(context.Background(), j.objectName, data); err != nil {
		return eris.Wrap(err, "failed to store snapshot in ObjectStore")
	}

	return nil
}

func (j *JetStreamSnapshotStorage) Load() (*Snapshot, error) {
	object, err := j.os.Get(context.Background(), j.objectName)
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

	var snapshot Snapshot
	err = proto.Unmarshal(data, &snapshot)
	if err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal snapshot")
	}

	return &snapshot, nil
}

func (j *JetStreamSnapshotStorage) Exists() bool {
	_, err := j.os.GetInfo(context.Background(), j.objectName)
	return err == nil
}

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

type JetStreamSnapshotStorageOptions struct {
	ObjectName string `env:"SHARD_SNAPSHOT_JETSTREAM_OBJECT_NAME" envDefault:"snapshot"`
	// Maximum bytes for snapshot storage (ObjectStore). // Required by some NATS providers like Synadia Cloud.
	SnapshotStorageMaxBytes uint64 `env:"SHARD_SNAPSHOT_STORAGE_MAX_BYTES" envDefault:"0"`

	client  *Client
	address *ServiceAddress
}

var _ SnapshotStorageOptions = (*JetStreamSnapshotStorageOptions)(nil)

func newJetstreamSnapshotStorageOptions() (JetStreamSnapshotStorageOptions, error) {
	// Set default values.
	opts := JetStreamSnapshotStorageOptions{
		ObjectName:              "snapshot",
		SnapshotStorageMaxBytes: 0,
		// Guaranteed to be not nil from a validated shard options.
		client:  nil,
		address: nil,
	}

	if err := env.Parse(&opts); err != nil {
		return opts, eris.Wrap(err, "failed to parse env")
	}

	return opts, nil
}

func (opt *JetStreamSnapshotStorageOptions) apply(shardOpts ShardOptions) {
	userOpts, ok := shardOpts.SnapshotStorageOptions.(*JetStreamSnapshotStorageOptions)
	// Only apply user-provided options if it is the correct type. Otherwise stick to the defaults.
	if ok {
		if userOpts.ObjectName != "" {
			opt.ObjectName = userOpts.ObjectName
		}
	}

	// shardOpts is already validated, just apply.
	opt.client = shardOpts.Client
	opt.address = shardOpts.Address
}

// validate validates the options. We only need to validate the public fields as the private ones
// come from a validated ShardOptions.
func (opt *JetStreamSnapshotStorageOptions) validate() error {
	if opt.ObjectName == "" {
		return eris.New("object name cannot be empty")
	}
	if opt.client == nil {
		return eris.New("NATS client cannot be nil")
	}
	if opt.address == nil {
		return eris.New("service address cannot be nil")
	}
	// SnapshotStorageMaxBytes can be 0 which means unlimited storage. No need to validate here.
	return nil
}
