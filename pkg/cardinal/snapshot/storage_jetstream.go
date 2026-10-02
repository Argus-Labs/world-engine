package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/caarlos0/env/v11"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

const defaultObjectName = "snapshot"

// objectStoreStreamNameTmpl is the JetStream stream-name template the SDK uses to back an
// ObjectStore bucket (see jetstream.objNameTmpl = "OBJ_%s"). It is reproduced here because the
// SDK keeps the template unexported; the bind path needs the stream name to read the existing
// bucket's MaxBytes from the server.
const objectStoreStreamNameTmpl = "OBJ_%s"

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

	clientOpts := []micro.ClientOption{micro.WithLogger(opts.Logger)}
	if opts.NATSConfig != nil {
		clientOpts = append(clientOpts, micro.WithNATSConfig(*opts.NATSConfig))
	}
	client, err := micro.NewClient(clientOpts...)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create micro client")
	}

	js, err := jetstream.New(client.Conn)
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
		if errors.Is(err, jetstream.ErrBucketExists) {
			// Bucket already exists, get the existing one.
			//
			// errors.Is (not eris.Is) is required here: the SDK wraps the bucket-exists
			// sentinel with errors.Join(fmt.Errorf("%w: %s", ErrBucketExists, bucket), err),
			// and eris.Is cannot traverse a multi-Unwrap joined error (eris.Unwrap only
			// handles Unwrap() error). Without errors.Is, a restart with a changed
			// CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES would fall through to the error return
			// and fail boot instead of binding to the surviving bucket.
			os, err = js.ObjectStore(ctx, bucketName)
			if err != nil {
				return nil, eris.Wrapf(err, "failed to get existing ObjectStore (bucket=%s)", bucketName)
			}
			// js.ObjectStore is a read-only bind: it never transmits ObjectStoreConfig to the
			// server, so a changed CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES is silently dropped on
			// every restart after the first. Surface the drift with a boot-time warning instead.
			//
			// CreateOrUpdateObjectStore is intentionally NOT used here: it routes through
			// prepareObjectStoreConfig and re-sends the entire StreamConfig (replicas, discard
			// policy, compression, etc.), which would overwrite operator-tuned settings on the
			// existing bucket. MaxBytes is the only field Cardinal manages, so only its drift is
			// reported; the operator remediates out-of-band (e.g. `nats stream update`).
			warnMaxBytesMismatch(ctx, js, bucketName, osConfig.MaxBytes, opts.Logger)
		} else {
			return nil, eris.Wrapf(err, "failed to create ObjectStore (bucket=%s, maxBytes=%d)",
				osConfig.Bucket, osConfig.MaxBytes)
		}
	}

	return &JetStreamStorage{os: os}, nil
}

// Store writes the snapshot bytes to the ObjectStore, overwriting the previous snapshot.
func (j *JetStreamStorage) Store(ctx context.Context, _ uint64, data []byte) error {
	if _, err := j.os.PutBytes(ctx, defaultObjectName, data); err != nil {
		return eris.Wrap(err, "failed to store snapshot in ObjectStore")
	}
	return nil
}

// Load reads the stored snapshot bytes.
func (j *JetStreamStorage) Load(ctx context.Context) ([]byte, error) {
	object, err := j.os.Get(ctx, defaultObjectName)
	if err != nil {
		if eris.Is(err, jetstream.ErrObjectNotFound) {
			return nil, eris.Wrap(ErrSnapshotNotFound, "no snapshot exists")
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
	return data, nil
}

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

type JetStreamStorageOptions struct {
	Address    *micro.ServiceAddress
	Logger     zerolog.Logger
	NATSConfig *micro.NATSConfig // Optional NATS config override (nil = use env/defaults)

	// Maximum bytes for snapshot storage (ObjectStore). Required by some NATS providers like Synadia Cloud.
	SnapshotStorageMaxBytes uint64 `env:"CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES" envDefault:"0"`
}

func (opt *JetStreamStorageOptions) Validate() error {
	if opt.Address == nil {
		return eris.New("service address cannot be nil")
	}
	// SnapshotStorageMaxBytes can be 0 which means unlimited storage. No need to validate here.
	return nil
}

// normalizeObjectStoreMaxBytes maps a MaxBytes value to its effective form for comparison.
// The SDK's prepareObjectStoreConfig maps a zero MaxBytes to -1 (unlimited) on create, so a
// configured 0 and a server-side -1 both mean "unlimited" and must compare equal. The same
// mapping is applied to the server-side value for robustness, since some servers report 0
// instead of -1 for an unlimited bucket.
func normalizeObjectStoreMaxBytes(b int64) int64 {
	if b == 0 {
		return -1
	}
	return b
}

// warnMaxBytesMismatch fetches the existing object store bucket's stream MaxBytes and logs a
// boot-time warning if it differs from the configured value. It never modifies server state.
//
// A failure to inspect the stream degrades to a diagnostic warning rather than a boot error:
// a transient read failure must not prevent Cardinal from starting, and the existing bind via
// js.ObjectStore has already succeeded at this point.
func warnMaxBytesMismatch(
	ctx context.Context,
	js jetstream.JetStream,
	bucketName string,
	configuredMaxBytes int64,
	logger zerolog.Logger,
) {
	streamName := fmt.Sprintf(objectStoreStreamNameTmpl, bucketName)
	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		logger.Warn().Err(err).
			Str("bucket", bucketName).
			Int64("configured_max_bytes", configuredMaxBytes).
			Msg("failed to inspect existing ObjectStore stream; skipping CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES drift check")
		return
	}
	info, err := stream.Info(ctx)
	if err != nil {
		logger.Warn().Err(err).
			Str("bucket", bucketName).
			Int64("configured_max_bytes", configuredMaxBytes).
			Msg("failed to read existing ObjectStore stream info; skipping CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES drift check")
		return
	}
	serverMaxBytes := info.Config.MaxBytes
	if normalizeObjectStoreMaxBytes(configuredMaxBytes) == normalizeObjectStoreMaxBytes(serverMaxBytes) {
		return
	}
	logger.Warn().
		Str("bucket", bucketName).
		Str("stream", streamName).
		Int64("configured_max_bytes", configuredMaxBytes).
		Int64("actual_max_bytes", serverMaxBytes).
		Msg("CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES was not applied to the existing ObjectStore bucket " +
			"(the bucket already exists and is bound read-only on restart); to apply the new cap, " +
			"update the bucket out-of-band, e.g. 'nats stream update " + streamName + " --max-bytes=<value>'")
}
