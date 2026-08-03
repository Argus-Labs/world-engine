package snapshot

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// fakeS3 answers PutObject and GetObject without a network, recording every body the SDK actually
// wrote. It is deliberately dumb: the point is to see the bytes S3Storage produced, not to emulate
// S3.
type fakeS3 struct {
	puts [][]byte // one entry per PutObject, in order
	get  []byte   // body GetObject returns
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

// newFakeS3Storage builds the real S3Storage against the fake transport. The struct is assembled
// directly because NewS3Storage resolves credentials and endpoints from the environment; every
// field that Store and Load touch is the production one.
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

// TestS3StorageStoreOwnership is the production-backend half of Storage.Store's ownership rule.
// Cardinal hands Store the live world-state graph and keeps owning it, so the bytes S3 receives
// must be fixed before Store returns, and the graph must come back untouched.
//
// The edits below are a PROBE for a retained reference, not a reproduction of caller behaviour: a
// snapshot graph is frozen once built, so nothing in cardinal ever writes into one it handed over.
// Mutating it here is just how a test tells "serialized at call time" from "kept the pointer".
func TestS3StorageStoreOwnership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store, fake := newFakeS3Storage(t)
	snap := goldenSnapshot()

	before, err := marshalSnapshot(snap)
	require.NoError(t, err)

	require.NoError(t, store.Store(ctx, snap))
	require.Len(t, fake.puts, 1, "Store must write exactly one object")
	written := fake.puts[0]
	assert.Equal(t, before, written, "the object S3 received must be the envelope handed to Store")

	// Store must not have modified the caller's graph.
	unchanged, err := marshalSnapshot(snap)
	require.NoError(t, err)
	assert.Equal(t, before, unchanged, "Store mutated the caller's snapshot")

	// Edit the graph the caller still owns. What was already written must not follow along, which
	// is only true because Store finished reading before it returned.
	snap.GetWorldState().NextId += 1000
	snap.GetWorldState().GetArchetypes()[0].GetColumns()[0].Components[0] = []byte{0xde, 0xad}
	assert.Equal(t, before, fake.puts[0], "S3Storage kept a reference to the caller's graph")

	// And a second Store writes the mutated graph, so the first write was not a stale cache.
	require.NoError(t, store.Store(ctx, snap))
	require.Len(t, fake.puts, 2)
	assert.NotEqual(t, written, fake.puts[1], "Store must write the graph as it is at call time")
}

// TestS3StorageLoadOwnership is the read half: cardinal publishes the returned message to the debug
// reader and feeds it to FromProto, so every Load must hand back a message nothing else holds.
func TestS3StorageLoadOwnership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store, fake := newFakeS3Storage(t)
	stored, err := marshalSnapshot(goldenSnapshot())
	require.NoError(t, err)
	fake.get = stored

	first, err := store.Load(ctx)
	require.NoError(t, err)
	assert.True(t, proto.Equal(goldenSnapshot(), first), "Load did not return what was stored")

	// A caller owns what it gets, so corrupting it must not reach the next caller.
	first.TickHeight = 999
	first.WorldState = &cardinalv1.WorldState{}

	second, err := store.Load(ctx)
	require.NoError(t, err)
	assert.NotSame(t, first, second, "Load must not hand out a shared message")
	assert.True(t, proto.Equal(goldenSnapshot(), second), "a previous caller's mutation reached the next Load")
}
