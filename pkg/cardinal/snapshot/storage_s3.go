package snapshot

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/micro"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/caarlos0/env/v11"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultS3ObjectKey = "snapshot"

// S3Storage implements Storage using AWS S3 (or S3-compatible services like MinIO, R2).
//
// Required environment variables:
//
//	CARDINAL_SNAPSHOT_STORAGE_TYPE=S3   # Selects S3 as the snapshot backend
//	CARDINAL_S3_BUCKET=<bucket-name>   # S3 bucket name (must be pre-provisioned)
//	AWS_ACCESS_KEY_ID=<access-key>     # AWS access key ID
//	AWS_SECRET_ACCESS_KEY=<secret-key> # AWS secret access key
//	AWS_REGION=<region>                # AWS region (e.g. us-east-1)
//
// Optional environment variables:
//
//	CARDINAL_S3_ENDPOINT=<url>         # Custom endpoint for S3-compatible services
//	AWS_SESSION_TOKEN=<token>          # Session token for temporary credentials (STS/IRSA)
//
// Snapshots are stored at the key: {org}/{project}/{serviceId}/snapshot
// A single shared bucket can serve all orgs/projects; key prefixes prevent collisions.
//
// The bucket must already exist. The IAM principal needs s3:PutObject and s3:GetObject permissions.
// Enable S3 versioning on the bucket for automatic backup retention of previous snapshots.
type S3Storage struct {
	client *s3.Client
	bucket string
	key    string
	logger zerolog.Logger
}

var _ Storage = (*S3Storage)(nil)

// NewS3Storage creates a new S3-based snapshot storage.
// It loads AWS credentials from the default credential chain (env vars, IRSA, instance roles).
func NewS3Storage(opts S3StorageOptions) (*S3Storage, error) {
	if err := env.Parse(&opts); err != nil {
		return nil, eris.Wrap(err, "failed to parse env")
	}

	if err := opts.Validate(); err != nil {
		return nil, eris.Wrap(err, "invalid options passed")
	}

	// Build the S3 key. Region scoping is handled at the bucket level (one bucket per region),
	// so the key only needs org/project/serviceId to be unique within a region.
	objectKey := fmt.Sprintf("%s/%s/%s/%s",
		opts.Address.GetOrganization(),
		opts.Address.GetProject(),
		opts.Address.GetServiceId(),
		defaultS3ObjectKey,
	)

	// Load AWS config from environment (reads AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION).
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, eris.Wrap(err, "failed to load AWS config")
	}

	// Build S3 client. When Endpoint is set, use it for S3-compatible services (Garage, R2, etc.).
	// When Endpoint is empty, the SDK uses the default AWS S3 endpoint resolved from AWS_REGION
	// (e.g., https://s3.us-east-1.amazonaws.com).
	var s3Opts []func(*s3.Options)
	if opts.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(opts.Endpoint)
			o.UsePathStyle = true // Required for most S3-compatible services
		})
	}

	client := s3.NewFromConfig(cfg, s3Opts...)

	return &S3Storage{
		client: client,
		bucket: opts.Bucket,
		key:    objectKey,
		logger: opts.Logger,
	}, nil
}

func (s *S3Storage) Store(ctx context.Context, snapshot *Snapshot) error {
	var worldState cardinalv1.WorldState
	if err := proto.Unmarshal(snapshot.Data, &worldState); err != nil {
		return eris.Wrap(err, "failed to unmarshal world state")
	}
	snapshotPb := &cardinalv1.Snapshot{
		TickHeight:        snapshot.TickHeight,
		Timestamp:         timestamppb.New(snapshot.Timestamp),
		WorldState:        &worldState,
		Version:           snapshot.Version,
		DiskState:         snapshot.DiskState,
		DiskStateChecksum: snapshot.DiskStateChecksum,
	}
	data, err := proto.Marshal(snapshotPb)
	if err != nil {
		return eris.Wrap(err, "failed to marshal snapshot")
	}

	// Overwrite the existing snapshot if any.
	if _, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/x-protobuf"),
	}); err != nil {
		return eris.Wrap(err, "failed to store snapshot in S3")
	}

	return nil
}

func (s *S3Storage) Load(ctx context.Context) (*Snapshot, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key),
	})
	if err != nil {
		// Check for "not found" errors (NoSuchKey).
		var noSuchKey *types.NoSuchKey
		if eris.As(err, &noSuchKey) {
			return nil, eris.Wrap(ErrSnapshotNotFound, "no snapshot exists")
		}
		// Fallback for S3-compatible services that may return a generic smithy error.
		var apiErr smithy.APIError
		if eris.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchKey" {
			return nil, eris.Wrap(ErrSnapshotNotFound, "no snapshot exists")
		}
		return nil, eris.Wrap(err, "failed to get snapshot from S3")
	}
	defer func() {
		_ = result.Body.Close()
	}()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, eris.Wrap(err, "failed to read from S3 object")
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
		TickHeight:        snapshotPb.GetTickHeight(),
		Timestamp:         snapshotPb.GetTimestamp().AsTime(),
		Data:              worldStateBytes,
		DiskState:         snapshotPb.GetDiskState(),
		DiskStateChecksum: snapshotPb.GetDiskStateChecksum(),
		Version:           snapshotPb.GetVersion(),
	}, nil
}

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

// S3StorageOptions configures the S3 snapshot storage.
// Bucket and Endpoint are loaded from environment variables via env tags.
// AWS credentials (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION) are loaded
// automatically by the AWS SDK default credential chain.
type S3StorageOptions struct {
	Address *micro.ServiceAddress
	Logger  zerolog.Logger

	// S3 bucket name for snapshot storage. Required.
	// The bucket must be pre-provisioned; the application does not create it.
	Bucket string `env:"CARDINAL_S3_BUCKET"`

	// Custom endpoint URL for S3-compatible services. Optional.
	// Set this to use MinIO (e.g. "http://localhost:9000"), Cloudflare R2, DigitalOcean Spaces, etc.
	// When set, path-style addressing is enabled automatically.
	Endpoint string `env:"CARDINAL_S3_ENDPOINT"`
}

func (opt *S3StorageOptions) Validate() error {
	if opt.Address == nil {
		return eris.New("service address cannot be nil")
	}
	if opt.Bucket == "" {
		return eris.New("CARDINAL_S3_BUCKET environment variable is required")
	}
	return nil
}
