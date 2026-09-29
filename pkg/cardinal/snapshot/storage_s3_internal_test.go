package snapshot

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestS3StorageStoresBytesVerbatim: Store is pure byte transport — the object S3 receives is
// exactly the slice handed in, and a second Store writes the new bytes.
func TestS3StorageStoresBytesVerbatim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store, fake := newFakeS3Storage(t)
	first := []byte{0x01, 0x02, 0x03}
	require.NoError(t, store.Store(ctx, 1, first))
	require.Len(t, fake.puts, 1, "Store must write exactly one object")
	assert.Equal(t, first, fake.puts[0], "the object S3 received must be the bytes handed to Store")

	second := []byte{0x09, 0x08}
	require.NoError(t, store.Store(ctx, 2, second))
	require.Len(t, fake.puts, 2)
	assert.Equal(t, second, fake.puts[1])
}

// TestS3StorageLoadsBytesVerbatim: Load returns exactly what the bucket holds.
func TestS3StorageLoadsBytesVerbatim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store, fake := newFakeS3Storage(t)
	fake.get = []byte{0xAA, 0xBB, 0xCC}

	data, err := store.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, fake.get, data)
}
