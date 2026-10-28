package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	crand "crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

// TestBufferedBuildContextDoesNotBufferGzipInMemory guards the OOM that
// killed `world build` / `world reload` on asset-heavy projects: the gzipped
// context must NOT accumulate in an unbounded, doubling [bytes.Buffer]. The
// regression reserved ~3x the compressed context in process memory; a 400 MB
// incompressible working tree was measured at ~1.2 GB reserved.
//
// We pack a single cryptographically-random file (gzip cannot compress it,
// matching real game-asset data) and assert the bytes allocated during the
// pack stay within 1.5x the input. The old in-memory sink grew the
// [bytes.Buffer] holding the gzip output through repeated doublings, whose
// cumulative allocations ran ~2.3x the input; the temp-file sink writes to
// disk and reads the result back in a single pre-sized allocation. TotalAlloc
// is monotonic and GC-independent, so the assertion is deterministic. This test
// does not run in parallel: it measures process-wide allocations.
func TestBufferedBuildContextDoesNotBufferGzipInMemory(t *testing.T) {
	const blobSize = 16 << 20 // 16 MiB of incompressible project content

	root := t.TempDir()
	blob := make([]byte, blobSize)
	if _, err := crand.Read(blob); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets.bin"), blob, 0o600); err != nil {
		t.Fatalf("write assets.bin: %v", err)
	}

	// Everything above is allocated before the snapshot, so it is excluded
	// from the TotalAlloc delta below. The pack reads assets.bin from disk,
	// not from the in-memory blob, so holding the slice does not perturb the
	// measurement.
	df := []byte("FROM scratch\n")

	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	buildContext, err := bufferedBuildContext(context.Background(), root, nil, df, testDockerfileName)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	// gzip cannot compress random data, so the gzipped context carries
	// essentially the whole blob; otherwise this test isn't exercising the
	// OOM path it guards.
	if lowerBound := blobSize * 9 / 10; len(buildContext) < lowerBound {
		t.Fatalf(
			"gzipped context = %d bytes, want >= %d; the incompressible blob was "+
				"compressed, so the memory bound isn't being exercised",
			len(buildContext), lowerBound,
		)
	}

	alloc := int64(after.TotalAlloc - before.TotalAlloc)
	// Temp-file sink: ~1x blob (single read-back) plus a few hundred KiB of
	// tar/gzip overhead. Doubling [bytes.Buffer] sink: ~2.3x blob. 1.5x
	// separates the two with margin on both sides.
	if alloc > int64(blobSize)*3/2 {
		t.Fatalf(
			"allocated %d bytes during pack, want <= %d (1.5x blob): the gzipped "+
				"context was buffered in a doubling in-memory buffer",
			alloc, int64(blobSize)*3/2,
		)
	}
}

// TestBufferedBuildContextLeavesNoTempFile asserts the temp file that backs the
// gzipped context is removed on both the success and the cancellation paths,
// so `world build` does not leak disk proportional to project size.
func TestBufferedBuildContextLeavesNoTempFile(t *testing.T) {
	// Isolate the temp dir so we can assert the build-context file is removed
	// without racing other tests or stale /tmp entries. t.Setenv requires the
	// test (and its subtests) not to run in parallel.
	isolated := t.TempDir()
	t.Setenv("TMPDIR", isolated)

	root := writeTestContext(t)
	df := []byte("FROM scratch\n")

	t.Run("success", func(t *testing.T) {
		if _, err := bufferedBuildContext(context.Background(), root, nil, df, testDockerfileName); err != nil {
			t.Fatal(err)
		}
		requireNoBuildContextTemp(t, isolated)
	})

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := bufferedBuildContext(ctx, root, nil, df, testDockerfileName)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		requireNoBuildContextTemp(t, isolated)
	})
}

func requireNoBuildContextTemp(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read isolated temp dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "cardinal-build-context-") {
			t.Errorf("temp build-context file not removed: %s", e.Name())
		}
	}
}
