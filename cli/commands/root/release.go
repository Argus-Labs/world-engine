package root

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/rotisserie/eris"
)

const (
	// latestReleaseAPI returns the most recent non-prerelease of the CLI. Releases are
	// published to the world-cli repo by .github/workflows/release-cli.yaml.
	latestReleaseAPI = "https://api.github.com/repos/Argus-Labs/world-cli/releases/latest"

	// checksumsSuffix matches the checksum manifest goreleaser emits alongside the
	// archives. The filename embeds the version, so it is resolved by suffix.
	checksumsSuffix = "_checksums.txt"

	// maxRedirects mirrors the cap Go applies by default, which setting CheckRedirect
	// would otherwise remove.
	maxRedirects = 10

	// goosWindows is the runtime.GOOS value for Windows.
	goosWindows = "windows"

	// maxDownloadSize caps how much we will read from a release asset, so a
	// malformed or unexpectedly huge response cannot exhaust memory. Release
	// archives are ~20-25MB compressed and the binary inside is ~70MB, so this
	// leaves generous headroom.
	maxDownloadSize = 256 << 20 // 256 MiB
)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// asset returns the asset with the given name.
func (r release) asset(name string) (releaseAsset, error) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, nil
		}
	}
	return releaseAsset{}, eris.Errorf("release %s has no asset named %s", r.TagName, name)
}

// checksumsAsset returns the checksum manifest for the release.
func (r release) checksumsAsset() (releaseAsset, error) {
	for _, a := range r.Assets {
		if strings.HasSuffix(a.Name, checksumsSuffix) {
			return a, nil
		}
	}
	return releaseAsset{}, eris.Errorf("release %s has no *%s asset", r.TagName, checksumsSuffix)
}

// archiveName builds the release archive name for a platform. It mirrors the
// name_template in apps/world-cli/.goreleaser.yaml; the two must stay in sync.
func archiveName(goos, goarch string) (string, error) {
	var osPart string
	switch goos {
	case goosWindows:
		osPart = "Windows"
	case "darwin":
		osPart = "Darwin"
	case "linux":
		osPart = "Linux"
	default:
		return "", eris.Errorf("unsupported operating system %q", goos)
	}

	var archPart string
	switch goarch {
	case "amd64":
		archPart = "x86_64"
	case "arm64":
		archPart = "arm64"
	default:
		return "", eris.Errorf("unsupported architecture %q", goarch)
	}

	ext := ".tar.gz"
	if goos == goosWindows {
		ext = ".zip"
	}

	return fmt.Sprintf("world-cli_%s_%s%s", osPart, archPart, ext), nil
}

// binaryName is the executable inside the release archive. goreleaser builds it
// as `world` (see .goreleaser.yaml `binary:`).
func binaryName(goos string) string {
	if goos == goosWindows {
		return "world.exe"
	}
	return "world"
}

// fetchLatestRelease resolves the most recent published release.
func fetchLatestRelease(ctx context.Context, client *http.Client, apiURL string) (release, error) {
	body, err := get(ctx, client, apiURL)
	if err != nil {
		return release{}, eris.Wrap(err, "failed to fetch the latest release")
	}

	var rel release
	if err := json.Unmarshal(body, &rel); err != nil {
		return release{}, eris.Wrap(err, "failed to parse the latest release")
	}
	if len(rel.Assets) == 0 {
		return release{}, eris.Errorf("release %s has no assets", rel.TagName)
	}

	return rel, nil
}

// parseChecksums reads a goreleaser checksum manifest, whose lines are
// "<sha256 hex>  <filename>", into a filename-keyed map.
func parseChecksums(manifest []byte) (map[string][]byte, error) {
	sums := make(map[string][]byte)

	for line := range strings.SplitSeq(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 { //nolint:mnd // "<sum> <name>"
			continue
		}

		sum, err := hex.DecodeString(fields[0])
		if err != nil {
			return nil, eris.Wrapf(err, "malformed checksum for %s", fields[1])
		}
		if len(sum) != sha256.Size {
			return nil, eris.Errorf("checksum for %s is not sha256", fields[1])
		}

		// goreleaser writes bare filenames, but tolerate a path prefix.
		sums[path.Base(fields[1])] = sum
	}

	if len(sums) == 0 {
		return nil, eris.New("checksum manifest contained no entries")
	}

	return sums, nil
}

// verifyChecksum reports whether archive hashes to want.
func verifyChecksum(archive, want []byte) error {
	got := sha256.Sum256(archive)
	if !bytes.Equal(got[:], want) {
		return eris.Errorf(
			"checksum mismatch: expected %s, got %s",
			hex.EncodeToString(want), hex.EncodeToString(got[:]),
		)
	}
	return nil
}

// extractBinary pulls the world executable out of a release archive.
func extractBinary(archive []byte, archiveFilename, wantName string) ([]byte, error) {
	if strings.HasSuffix(archiveFilename, ".zip") {
		return extractFromZip(archive, wantName)
	}
	return extractFromTarGz(archive, wantName)
}

func extractFromZip(archive []byte, wantName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, eris.Wrap(err, "failed to open release archive")
	}

	for _, f := range zr.File {
		if path.Base(f.Name) != wantName {
			continue
		}
		return readZipEntry(f, wantName)
	}

	return nil, eris.Errorf("release archive does not contain %s", wantName)
}

// readZipEntry reads a single zip entry. Kept separate from the loop in
// extractFromZip so the handle is closed by function scope rather than by a defer
// registered inside a loop body.
func readZipEntry(f *zip.File, wantName string) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, eris.Wrapf(err, "failed to open %s in release archive", wantName)
	}
	defer rc.Close()

	return readAtMost(rc, wantName)
}

func extractFromTarGz(archive []byte, wantName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, eris.Wrap(err, "failed to open release archive")
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if eris.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, eris.Wrap(err, "failed to read release archive")
		}
		if hdr.Typeflag != tar.TypeReg || path.Base(hdr.Name) != wantName {
			continue
		}

		return readAtMost(tr, wantName)
	}

	return nil, eris.Errorf("release archive does not contain %s", wantName)
}

func readAtMost(r io.Reader, what string) ([]byte, error) {
	// Read one byte past the cap so an oversized payload is an error rather than a
	// silent truncation: io.LimitReader reports a clean EOF at its limit, which
	// would otherwise surface as a bogus checksum mismatch, or worse, as a
	// truncated binary installed over the running one.
	b, err := io.ReadAll(io.LimitReader(r, maxDownloadSize+1))
	if err != nil {
		return nil, eris.Wrapf(err, "failed to read %s", what)
	}
	if len(b) > maxDownloadSize {
		return nil, eris.Errorf("%s exceeds the %d byte limit", what, maxDownloadSize)
	}
	if len(b) == 0 {
		return nil, eris.Errorf("%s is empty", what)
	}
	return b, nil
}

// download fetches a release asset.
func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	body, err := get(ctx, client, url)
	if err != nil {
		return nil, eris.Wrapf(err, "failed to download %s", path.Base(url))
	}
	return body, nil
}

func get(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	// Asset URLs come from the release API response rather than from this binary, so
	// require HTTPS: a tampered response must not be able to downgrade a download to
	// plaintext. GitHub redirects release assets to a githubusercontent.com host, so
	// the same check is applied to every hop by rejectPlaintextRedirect; validating
	// only this first URL would leave the hop that carries the payload unchecked.
	// Host is left unconstrained because that redirect target is not contractual.
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, eris.Wrapf(err, "malformed release URL %q", rawURL)
	}
	if parsed.Scheme != "https" {
		return nil, eris.Errorf("refusing to fetch release asset over non-HTTPS URL %q", rawURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create request")
	}
	req.Header.Set("User-Agent", "world-cli/"+AppVersion)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, eris.Wrap(err, "request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if err := rateLimitError(resp); err != nil {
			return nil, err
		}
		return nil, eris.Errorf("unexpected status %s", resp.Status)
	}

	return readAtMost(resp.Body, "response body")
}

// rateLimitError converts an exhausted GitHub rate limit into an actionable error.
// Unauthenticated requests are limited to 60/hour per IP, which is easy to hit from
// a shared NAT or a CI runner. Returns nil when the response is not a rate limit.
func rateLimitError(resp *http.Response) error {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	if resp.Header.Get("X-Ratelimit-Remaining") != "0" {
		return nil
	}

	reset, err := strconv.ParseInt(resp.Header.Get("X-Ratelimit-Reset"), 10, 64)
	if err != nil {
		return eris.New("GitHub API rate limit reached; try again later")
	}

	return eris.Errorf("GitHub API rate limit reached; try again in %s",
		time.Until(time.Unix(reset, 0)).Round(time.Second))
}

// rejectPlaintextRedirect refuses any redirect that would leave HTTPS, and preserves
// Go's default cap on redirect depth (setting CheckRedirect replaces that default).
func rejectPlaintextRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return eris.Errorf("refusing to follow redirect to non-HTTPS URL %q", req.URL.String())
	}
	if len(via) >= maxRedirects {
		return eris.Errorf("stopped after %d redirects", maxRedirects)
	}
	return nil
}
