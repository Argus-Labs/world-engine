package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

const testDockerfileName = "__test.Dockerfile"

func writeTestContext(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                "module example\n",
		"shards/game/main.go":   "package main\n",
		"Dockerfile":            "FROM user-dockerfile-kept-in-context\n",
		"node_modules/x/a.js":   "ignored\n",
		"shards/game/README.md": "kept\n",
	}
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// readContext gunzips and lists the archive: entry names in order and the Dockerfile's body.
func readContext(t *testing.T, r io.Reader) ([]string, string) {
	t.Helper()
	gz, err := gzip.NewReader(r)
	if err != nil {
		t.Fatalf("context is not gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	var names []string
	var dockerfile string
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("context is not a valid tar: %v", err)
		}
		names = append(names, hdr.Name)
		if hdr.Name == testDockerfileName {
			b, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			dockerfile = string(b)
		}
	}
	return names, dockerfile
}

func TestBufferedBuildContextContents(t *testing.T) {
	root := writeTestContext(t)
	ignores := []string{"node_modules"}
	df := []byte("FROM scratch\n")
	buildContext, err := bufferedBuildContext(context.Background(), root, ignores, df, testDockerfileName)
	if err != nil {
		t.Fatal(err)
	}

	names, dockerfile := readContext(t, bytes.NewReader(buildContext))
	if len(names) == 0 || names[0] != testDockerfileName {
		t.Fatalf("embedded Dockerfile must be the first entry, got %v", names)
	}
	if dockerfile != "FROM scratch\n" {
		t.Errorf("embedded Dockerfile = %q", dockerfile)
	}
	// Exact names: "Dockerfile" is also a substring of the embedded entry's name.
	for _, want := range []string{"go.mod", "shards/game/main.go", "shards/game/README.md", "Dockerfile"} {
		if !slices.Contains(names, want) {
			t.Errorf("missing %s in %v", want, names)
		}
	}
	for _, name := range names {
		if strings.HasPrefix(name, "node_modules") {
			t.Errorf("ignored path leaked into context: %s", name)
		}
	}
}

// Namespace CI runners' Docker rejects chunked /build uploads ("invalid tar header"), so the
// context must reach the daemon with a Content-Length.
func TestBufferedBuildContextSendsContentLength(t *testing.T) {
	root := writeTestContext(t)
	df := []byte("FROM scratch\n")
	buildContext, err := bufferedBuildContext(context.Background(), root, nil, df, testDockerfileName)
	if err != nil {
		t.Fatal(err)
	}
	size := int64(len(buildContext))

	type seen struct {
		contentLength    int64
		transferEncoding []string
		body             []byte
	}
	got := make(chan seen, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/build") {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		got <- seen{r.ContentLength, r.TransferEncoding, body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{}\n")
	}))
	defer srv.Close()

	cli, err := client.New(
		client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")),
		client.WithAPIVersion("1.47"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	opts := client.ImageBuildOptions{Dockerfile: testDockerfileName}
	// The same call buildSingleCardinalWithDocker makes.
	resp, err := cli.ImageBuild(context.Background(), bytes.NewReader(buildContext), opts)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	s := <-got
	if len(s.transferEncoding) > 0 {
		t.Fatalf("build context sent with Transfer-Encoding %v; want a Content-Length upload", s.transferEncoding)
	}
	if s.contentLength != size {
		t.Fatalf("Content-Length = %d, want %d", s.contentLength, size)
	}
	names, _ := readContext(t, strings.NewReader(string(s.body)))
	if len(names) == 0 || names[0] != testDockerfileName {
		t.Fatalf("daemon received an unreadable context: %v", names)
	}
}

func TestBufferedBuildContextStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	df := []byte("FROM scratch\n")
	_, err := bufferedBuildContext(ctx, writeTestContext(t), nil, df, testDockerfileName)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
