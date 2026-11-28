package docker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// The daemon always has an image at the tag from an earlier build, so a build
// that never finished must not pass just because the tag resolves.
func TestBuildSingleCardinalWithDockerRequiresCompletedStream(t *testing.T) {
	t.Parallel()

	const progress = `{"stream":"Step 1/2 : compiling\n"}` + "\n"

	tests := []struct {
		name    string
		body    string
		cutOff  bool // declare more bytes than are sent, as when the daemon connection drops
		wantErr string
	}{
		{
			name: "buildkit stream reports the image",
			body: progress + `{"id":"moby.image.id","aux":{"ID":"sha256:new"}}` + "\n",
		},
		{
			name: "classic stream reports the image",
			body: progress + `{"aux":{"ID":"sha256:new"}}` + "\n",
		},
		{
			name:    "connection drops mid-stream",
			body:    progress,
			cutOff:  true,
			wantErr: "read build output",
		},
		{
			name:    "stream ends without an image",
			body:    progress,
			wantErr: "ended before the daemon reported a built image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/build"):
					_, _ = io.Copy(io.Discard, r.Body)

					size := len(tt.body)
					if tt.cutOff {
						size += 100
					}

					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Content-Length", strconv.Itoa(size))
					_, _ = io.WriteString(w, tt.body)
				case strings.HasSuffix(r.URL.Path, "/images/demo-game-shard:latest/json"):
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"Id":"sha256:old","RepoTags":["demo-game-shard:latest"]}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			cli, err := client.New(
				client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")),
				client.WithAPIVersion("1.47"),
			)
			require.NoError(t, err)

			defer cli.Close()

			c := &Client{
				client: cli,
				cfg:    &service.Config{WorldToml: worldtoml.Config{Project: "demo"}},
				logger: slog.New(slog.DiscardHandler),
			}
			sb := &shardBuild{image: "demo-game-shard:latest", shardPath: "shards/game", serviceName: "demo-game-shard"}

			err = c.buildSingleCardinalWithDocker(context.Background(), sb,
				ocispec.Platform{OS: "linux", Architecture: "amd64"}, []byte("context"))

			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestDrainBuildResponseReturnsReadError(t *testing.T) {
	t.Parallel()

	errRead := errors.New("daemon connection reset")
	body := io.MultiReader(strings.NewReader(`{"stream":"Step 1/2 : compiling\n"}`+"\n"), iotest.ErrReader(errRead))

	err := drainBuildResponse(context.Background(), body, "demo-game-shard")
	assert.ErrorIs(t, err, errRead)
}
