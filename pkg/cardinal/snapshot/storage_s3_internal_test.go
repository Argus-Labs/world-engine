package snapshot

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// fakeS3 records PUT request bodies and supplies GET response bodies. It does not use a network.
type fakeS3 struct {
	puts [][]byte // PUT request bodies, in request order
	get  []byte   // GET response body
}

func (f *fakeS3) Do(req *http.Request) (*http.Response, error) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       http.NoBody,
		Request:    req,
	}
	switch req.Method {
	case http.MethodPut:
		var buf bytes.Buffer
		if req.Body != nil {
			if _, err := io.Copy(&buf, req.Body); err != nil {
				return nil, err
			}
		}
		f.puts = append(f.puts, buf.Bytes())
	case http.MethodGet:
		resp.Body = io.NopCloser(bytes.NewReader(f.get))
		resp.ContentLength = int64(len(f.get))
	}
	return resp, nil
}

// newFakeS3Storage creates an S3Storage that uses fakeS3 as its HTTP transport.
// It does not call NewS3Storage because that function reads credentials and endpoints from the environment.
func newFakeS3Storage(t *testing.T) (*S3Storage, *fakeS3) {
	t.Helper()

	fake := &fakeS3{}
	client := s3.New(s3.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String("https://s3.local"),
		UsePathStyle: true,
		HTTPClient:   fake,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test", Source: "test"}, nil
		}),
		RetryMaxAttempts: 1,
	})
	return &S3Storage{client: client, bucket: "bucket", key: "org/project/0/snapshot"}, fake
}

// TestS3StorageStoreOwnership verifies the ownership rules for S3Storage.Store.
// Store must write the snapshot before it returns. Store must not change or retain the snapshot.
//
// Cardinal does not change a snapshot after it calls Store. This test changes the snapshot only to
// detect a retained reference.
func TestS3StorageStoreOwnership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	prng := testutils.NewRand(t)

	store, fake := newFakeS3Storage(t)
	snap := randomSnapshot(prng)
	snap.WorldState.Archetypes = append(snap.WorldState.GetArchetypes(), &cardinalv1.Archetype{
		Columns: []*cardinalv1.Column{{
			ComponentName: "ownership-probe",
			Components:    [][]byte{{byte(prng.Uint32())}},
		}},
	})

	before, err := marshalSnapshot(snap)
	require.NoError(t, err)

	require.NoError(t, store.Store(ctx, snap))
	require.Len(t, fake.puts, 1, "Store must write exactly one object")
	written := fake.puts[0]
	assert.Equal(t, before, written, "the object S3 received must be the envelope handed to Store")

	// Store must not change the caller's snapshot.
	unchanged, err := marshalSnapshot(snap)
	require.NoError(t, err)
	assert.Equal(t, before, unchanged, "Store mutated the caller's snapshot")

	// A later change to the snapshot must not change the stored bytes.
	snap.GetWorldState().NextId ^= 1
	probe := snap.GetWorldState().GetArchetypes()[len(snap.GetWorldState().GetArchetypes())-1]
	probe.GetColumns()[0].Components[0][0] ^= 1
	assert.Equal(t, before, fake.puts[0], "S3Storage kept a reference to the caller's graph")

	// A second Store must write the changed snapshot.
	require.NoError(t, store.Store(ctx, snap))
	require.Len(t, fake.puts, 2)
	assert.NotEqual(t, written, fake.puts[1], "Store must write the graph as it is at call time")
}

// TestS3StorageLoadOwnership verifies that each Load returns an independent snapshot.
func TestS3StorageLoadOwnership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	prng := testutils.NewRand(t)

	store, fake := newFakeS3Storage(t)
	expected := randomSnapshot(prng)
	stored, err := marshalSnapshot(expected)
	require.NoError(t, err)
	fake.get = stored

	first, err := store.Load(ctx)
	require.NoError(t, err)
	assert.True(t, proto.Equal(expected, first), "Load did not return what was stored")

	// A change to one result must not change the next result.
	first.TickHeight = 999
	first.WorldState = &cardinalv1.WorldState{}

	second, err := store.Load(ctx)
	require.NoError(t, err)
	assert.NotSame(t, first, second, "Load must not hand out a shared message")
	assert.True(t, proto.Equal(expected, second), "a previous caller's mutation reached the next Load")
}
