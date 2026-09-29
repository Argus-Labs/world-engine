package root

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minio/selfupdate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		goos, goarch string
		want         string
		wantErr      bool
	}{
		{goos: "windows", goarch: "amd64", want: "world-cli_Windows_x86_64.zip"},
		{goos: "windows", goarch: "arm64", want: "world-cli_Windows_arm64.zip"},
		{goos: "darwin", goarch: "arm64", want: "world-cli_Darwin_arm64.tar.gz"},
		{goos: "darwin", goarch: "amd64", want: "world-cli_Darwin_x86_64.tar.gz"},
		{goos: "linux", goarch: "amd64", want: "world-cli_Linux_x86_64.tar.gz"},
		{goos: "linux", goarch: "arm64", want: "world-cli_Linux_arm64.tar.gz"},
		{goos: "plan9", goarch: "amd64", wantErr: true},
		{goos: "linux", goarch: "386", wantErr: true},
		{goos: "windows", goarch: "386", wantErr: true},
		{goos: "linux", goarch: "mips", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.goos+"/"+tc.goarch, func(t *testing.T) {
			t.Parallel()

			got, err := archiveName(tc.goos, tc.goarch)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSameVersion(t *testing.T) {
	t.Parallel()

	assert.True(t, sameVersion("v2.4.2", "2.4.2"), "goreleaser sets AppVersion without the v prefix")
	assert.True(t, sameVersion("v2.4.2", "v2.4.2"))
	assert.False(t, sameVersion("v2.4.3", "2.4.2"))
	assert.False(t, sameVersion("v2.10.0", "2.9.0"))
}

func TestParseChecksums(t *testing.T) {
	t.Parallel()

	sum := sha256.Sum256([]byte("payload"))
	hexSum := hex.EncodeToString(sum[:])

	t.Run("parses goreleaser format", func(t *testing.T) {
		t.Parallel()

		manifest := fmt.Sprintf("%s  world-cli_Linux_x86_64.tar.gz\n%s  world-cli_Windows_x86_64.zip\n", hexSum, hexSum)
		got, err := parseChecksums([]byte(manifest))
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, sum[:], got["world-cli_Linux_x86_64.tar.gz"])
	})

	t.Run("rejects an empty manifest", func(t *testing.T) {
		t.Parallel()

		_, err := parseChecksums([]byte("\n\n"))
		require.Error(t, err)
	})

	t.Run("rejects a non-hex checksum", func(t *testing.T) {
		t.Parallel()

		_, err := parseChecksums([]byte("zzzz  world-cli_Linux_x86_64.tar.gz\n"))
		require.Error(t, err)
	})

	t.Run("rejects a non-sha256 checksum", func(t *testing.T) {
		t.Parallel()

		_, err := parseChecksums([]byte("abcd  world-cli_Linux_x86_64.tar.gz\n"))
		require.Error(t, err)
	})
}

func TestVerifyChecksum(t *testing.T) {
	t.Parallel()

	payload := []byte("payload")
	sum := sha256.Sum256(payload)

	require.NoError(t, verifyChecksum(payload, sum[:]))

	err := verifyChecksum([]byte("corrupted"), sum[:])
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestReadAtMost(t *testing.T) {
	t.Parallel()

	t.Run("errors instead of truncating an oversized payload", func(t *testing.T) {
		t.Parallel()

		_, err := readAtMost(endlessReader{}, "release archive")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds the")
	})

	t.Run("rejects an empty payload", func(t *testing.T) {
		t.Parallel()

		_, err := readAtMost(bytes.NewReader(nil), "release archive")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is empty")
	})
}

func TestExtractBinary(t *testing.T) {
	t.Parallel()

	want := []byte("\x7fELF fake binary")

	t.Run("from zip", func(t *testing.T) {
		t.Parallel()

		got, err := extractBinary(makeZip(t, "world.exe", want), "world-cli_Windows_x86_64.zip", "world.exe")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("from tar.gz", func(t *testing.T) {
		t.Parallel()

		got, err := extractBinary(makeTarGz(t, "world", want), "world-cli_Linux_x86_64.tar.gz", "world")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("missing entry is an error", func(t *testing.T) {
		t.Parallel()

		_, err := extractBinary(makeTarGz(t, "README.md", want), "world-cli_Linux_x86_64.tar.gz", "world")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not contain world")
	})

	t.Run("corrupt archive is an error", func(t *testing.T) {
		t.Parallel()

		_, err := extractBinary([]byte("not an archive"), "world-cli_Linux_x86_64.tar.gz", "world")
		require.Error(t, err)
	})
}

// TestApplyBinary exercises the real binary swap against a temp file. This is the only
// genuinely dangerous code in the update path, so it runs selfupdate.Apply for real
// rather than stubbing it; TargetPath keeps it away from the test binary.
func TestApplyBinary(t *testing.T) {
	t.Parallel()

	t.Run("replaces the target and leaves no staged file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		target := filepath.Join(dir, "world")
		require.NoError(t, os.WriteFile(target, []byte("old binary"), 0o755))

		require.NoError(t, applyBinary([]byte("new binary"), selfupdate.Options{TargetPath: target}))

		got, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "new binary", string(got))
		assert.NoFileExists(t, sidecar(target, ".new"), "staged file must not survive a successful swap")
	})

	t.Run("preserves the executable bit", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		target := filepath.Join(dir, "world")
		require.NoError(t, os.WriteFile(target, []byte("old binary"), 0o755))

		require.NoError(t, applyBinary([]byte("new binary"), selfupdate.Options{TargetPath: target}))

		info, err := os.Stat(target)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode().Perm()&0o100, "replacement must stay executable")
	})

	t.Run("errors and cleans up when the target directory is missing", func(t *testing.T) {
		t.Parallel()

		target := filepath.Join(t.TempDir(), "no-such-dir", "world")

		err := applyBinary([]byte("new binary"), selfupdate.Options{TargetPath: target})
		require.Error(t, err)
		assert.NoFileExists(t, sidecar(target, ".new"))
	})
}

func TestSidecar(t *testing.T) {
	t.Parallel()

	// Must match selfupdate's ".<name>.new" / ".<name>.old" convention exactly, or
	// cleanup silently does nothing and the recovery message names a bogus path.
	assert.Equal(t, filepath.Join("/opt/bin", ".world.new"), sidecar("/opt/bin/world", ".new"))
	assert.Equal(t, filepath.Join("/opt/bin", ".world.exe.old"), sidecar("/opt/bin/world.exe", ".old"))
	assert.Empty(t, sidecar("", ".new"))
}

// TestUpdaterRun exercises release resolution, download, verification, and extraction
// against a fake release server. The swap is stubbed here (TestApplyBinary covers the
// real one) so these run in parallel without touching the filesystem.
func TestUpdaterRun(t *testing.T) {
	t.Parallel()

	newBinary := []byte("\x7fELF world v9.9.9")

	t.Run("installs the verified binary", func(t *testing.T) {
		t.Parallel()

		archive := makeTarGz(t, "world", newBinary)
		srv := fakeReleaseServer(t, "v9.9.9", "world-cli_Linux_x86_64.tar.gz", archive, checksumOf(archive))

		var applied []byte
		u := testUpdater(srv, "linux", "amd64", "2.0.0", func(b []byte) error {
			applied = b
			return nil
		})

		tag, updated, err := u.run(t.Context())
		require.NoError(t, err)
		assert.True(t, updated)
		assert.Equal(t, "v9.9.9", tag)
		assert.Equal(t, newBinary, applied, "the extracted binary should reach the swap step")
	})

	t.Run("walks the Windows zip path end to end", func(t *testing.T) {
		t.Parallel()

		archive := makeZip(t, "world.exe", newBinary)
		srv := fakeReleaseServer(t, "v9.9.9", "world-cli_Windows_x86_64.zip", archive, checksumOf(archive))

		var applied []byte
		u := testUpdater(srv, "windows", "amd64", "2.0.0", func(b []byte) error {
			applied = b
			return nil
		})

		_, updated, err := u.run(t.Context())
		require.NoError(t, err)
		assert.True(t, updated)
		assert.Equal(t, newBinary, applied)
	})

	t.Run("skips the download when already current", func(t *testing.T) {
		t.Parallel()

		archive := makeTarGz(t, "world", newBinary)
		srv := fakeReleaseServer(t, "v9.9.9", "world-cli_Linux_x86_64.tar.gz", archive, checksumOf(archive))

		applied := false
		u := testUpdater(srv, "linux", "amd64", "9.9.9", func([]byte) error {
			applied = true
			return nil
		})

		tag, updated, err := u.run(t.Context())
		require.NoError(t, err)
		assert.False(t, updated, "an up-to-date CLI must not swap its binary")
		assert.Equal(t, "v9.9.9", tag)
		assert.False(t, applied)
	})

	t.Run("still updates a dev build with no AppVersion", func(t *testing.T) {
		t.Parallel()

		archive := makeTarGz(t, "world", newBinary)
		srv := fakeReleaseServer(t, "v9.9.9", "world-cli_Linux_x86_64.tar.gz", archive, checksumOf(archive))

		u := testUpdater(srv, "linux", "amd64", "", func([]byte) error { return nil })

		_, updated, err := u.run(t.Context())
		require.NoError(t, err)
		assert.True(t, updated)
	})

	t.Run("refuses a corrupted archive", func(t *testing.T) {
		t.Parallel()

		archive := makeTarGz(t, "world", newBinary)
		wrong := sha256.Sum256([]byte("a different archive"))
		srv := fakeReleaseServer(t, "v9.9.9", "world-cli_Linux_x86_64.tar.gz", archive, wrong[:])

		applied := false
		u := testUpdater(srv, "linux", "amd64", "2.0.0", func([]byte) error {
			applied = true
			return nil
		})

		_, _, err := u.run(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "checksum mismatch")
		assert.False(t, applied, "a corrupted archive must never reach the swap step")
	})

	t.Run("does not download the archive when the manifest is unusable", func(t *testing.T) {
		t.Parallel()

		// The manifest is tiny and the archive is ~20-25MB, so the manifest must be
		// resolved first: a bad manifest should cost nothing.
		const archiveFile = "world-cli_Linux_x86_64.tar.gz"
		var archiveHits atomic.Int32

		mux := http.NewServeMux()
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		base := "https://" + srv.Listener.Addr().String()

		mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"tag_name":"v9.9.9","assets":[`+
				`{"name":%q,"browser_download_url":%q},`+
				`{"name":"world-cli_v9.9.9_checksums.txt","browser_download_url":%q}]}`,
				archiveFile, base+"/dl/archive", base+"/dl/sums")
		})
		mux.HandleFunc("/dl/archive", func(w http.ResponseWriter, _ *http.Request) {
			archiveHits.Add(1)
			_, _ = w.Write(makeTarGz(t, "world", newBinary))
		})
		mux.HandleFunc("/dl/sums", func(w http.ResponseWriter, _ *http.Request) {
			// Valid manifest, but it lists a file that is not the archive we need.
			fmt.Fprintf(w, "%s  some-other-file.tar.gz\n",
				hex.EncodeToString(checksumOf([]byte("x"))))
		})

		u := testUpdater(srv, "linux", "amd64", "2.0.0", func([]byte) error { return nil })

		_, _, err := u.run(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "publishes no checksum")
		assert.Zero(t, archiveHits.Load(), "archive must not be downloaded before the checksum resolves")
	})

	t.Run("errors when the platform archive is absent", func(t *testing.T) {
		t.Parallel()

		archive := makeTarGz(t, "world", newBinary)
		srv := fakeReleaseServer(t, "v9.9.9", "world-cli_Linux_x86_64.tar.gz", archive, checksumOf(archive))

		u := testUpdater(srv, "windows", "amd64", "2.0.0", func([]byte) error { return nil })

		_, _, err := u.run(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "world-cli_Windows_x86_64.zip")
	})

	t.Run("errors on an unsupported platform", func(t *testing.T) {
		t.Parallel()

		archive := makeTarGz(t, "world", newBinary)
		srv := fakeReleaseServer(t, "v9.9.9", "world-cli_Linux_x86_64.tar.gz", archive, checksumOf(archive))

		u := testUpdater(srv, "plan9", "amd64", "2.0.0", func([]byte) error { return nil })

		_, _, err := u.run(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported operating system")
	})

	t.Run("surfaces an unwritable install directory before downloading", func(t *testing.T) {
		t.Parallel()

		archive := makeTarGz(t, "world", newBinary)
		srv := fakeReleaseServer(t, "v9.9.9", "world-cli_Linux_x86_64.tar.gz", archive, checksumOf(archive))

		u := testUpdater(srv, "linux", "amd64", "2.0.0", func([]byte) error { return nil })
		u.writable = func() error { return os.ErrPermission }

		_, _, err := u.run(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot write to the World CLI install directory")
	})
}

func TestGetRejectsNonHTTPS(t *testing.T) {
	t.Parallel()

	// Asset URLs come from the API response, so a tampered response must not be able
	// to downgrade the transport to plaintext.
	_, err := get(t.Context(), http.DefaultClient, "http://example.com/world.tar.gz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-HTTPS")
}

func TestRejectPlaintextRedirect(t *testing.T) {
	t.Parallel()

	req := func(raw string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	t.Run("allows an https hop", func(t *testing.T) {
		t.Parallel()

		// GitHub redirects release assets to githubusercontent.com; that must keep working.
		require.NoError(t, rejectPlaintextRedirect(
			req("https://release-assets.githubusercontent.com/x"), nil))
	})

	t.Run("refuses a downgrade to plaintext", func(t *testing.T) {
		t.Parallel()

		err := rejectPlaintextRedirect(req("http://attacker.example/world.tar.gz"), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-HTTPS")
	})

	t.Run("preserves Go's redirect depth cap", func(t *testing.T) {
		t.Parallel()

		// Setting CheckRedirect replaces Go's default limit, so it must be re-applied.
		err := rejectPlaintextRedirect(
			req("https://example.com/x"), make([]*http.Request, maxRedirects))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "redirects")
	})
}

func TestRateLimitError(t *testing.T) {
	t.Parallel()

	t.Run("reports when the limit resets", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
		resp.Header.Set("X-Ratelimit-Remaining", "0")
		resp.Header.Set("X-Ratelimit-Reset", strconv.FormatInt(time.Now().Add(12*time.Minute).Unix(), 10))

		err := rateLimitError(resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rate limit reached")
	})

	t.Run("ignores a 403 that is not a rate limit", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
		resp.Header.Set("X-Ratelimit-Remaining", "42")

		require.NoError(t, rateLimitError(resp))
	})

	t.Run("ignores unrelated statuses", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, rateLimitError(&http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}}))
	})
}

// //////////////////////////////////////////////////////////////////////////
// HELPERS
// //////////////////////////////////////////////////////////////////////////

// testUpdater builds an updater pointed at a fake release server, with the swap
// stubbed. The client rewrites https:// to http:// at dial time so the production
// HTTPS guard in get() stays exercised against a plain httptest server.
func testUpdater(srv *httptest.Server, goos, goarch, version string, apply func([]byte) error) updater {
	return updater{
		client:   &http.Client{Transport: httpsToHTTP{}},
		apiURL:   "https://" + srv.Listener.Addr().String() + "/releases/latest",
		version:  version,
		platform: func() (string, string) { return goos, goarch },
		writable: func() error { return nil },
		apply:    func(b []byte, _ selfupdate.Options) error { return apply(b) },
	}
}

// httpsToHTTP downgrades https:// to http:// so tests can hand get() HTTPS URLs that
// resolve to a plain httptest server.
type httpsToHTTP struct{}

func (httpsToHTTP) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	return http.DefaultTransport.RoundTrip(clone)
}

// fakeReleaseServer serves a GitHub-shaped release with one archive and a checksum
// manifest, advertising its asset URLs as https://.
func fakeReleaseServer(t *testing.T, tag, archiveFilename string, archive, sum []byte) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	base := "https://" + srv.Listener.Addr().String()
	manifest := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum), archiveFilename)
	sumsFilename := "world-cli_" + tag + "_checksums.txt"

	const body = `{"tag_name":%q,"assets":[` +
		`{"name":%q,"browser_download_url":%q},` +
		`{"name":%q,"browser_download_url":%q}]}`

	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, body,
			tag,
			archiveFilename, base+"/dl/"+archiveFilename,
			sumsFilename, base+"/dl/"+sumsFilename,
		)
	})
	mux.HandleFunc("/dl/"+archiveFilename, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/dl/"+sumsFilename, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(manifest))
	})

	return srv
}

// endlessReader yields bytes forever, to drive readAtMost past its cap.
type endlessReader struct{}

func (endlessReader) Read(p []byte) (int, error) { return len(p), nil }

func checksumOf(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	return buf.Bytes()
}
