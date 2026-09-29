package root

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
	"github.com/rotisserie/eris"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/textinput"
)

const (
	// updateTimeout bounds the whole update: release lookup, both downloads, and the
	// swap. The archives are ~20-25MB, so this has to accommodate a slow connection;
	// stalls are caught by responseHeaderTimeout rather than by this budget.
	updateTimeout = 30 * time.Minute

	// responseHeaderTimeout bounds how long a request may stall before returning
	// headers. This is what catches a dead connection; a slow-but-live transfer is
	// allowed to run to the context deadline.
	responseHeaderTimeout = 30 * time.Second
)

// updater performs a self-update. The function fields are the seams tests replace;
// production code uses newUpdater, which wires in the real implementations.
type updater struct {
	client   *http.Client
	apiURL   string
	version  string
	platform func() (goos string, goarch string)
	writable func() error
	apply    func(binary []byte, opts selfupdate.Options) error
}

func newUpdater() updater {
	return updater{
		client: &http.Client{
			// Deliberately no Client.Timeout: it covers the body read too, which would
			// cap total transfer time and fail large downloads on slow links. The
			// caller's context bounds the operation instead.
			CheckRedirect: rejectPlaintextRedirect,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				TLSHandshakeTimeout:   responseHeaderTimeout,
				ResponseHeaderTimeout: responseHeaderTimeout,
			},
		},
		apiURL:   latestReleaseAPI,
		version:  AppVersion,
		platform: func() (string, string) { return runtime.GOOS, runtime.GOARCH },
		writable: func() error { return (&selfupdate.Options{}).CheckPermissions() },
		apply:    applyBinary,
	}
}

// run downloads and installs the latest release. It reports the release it resolved
// and whether a swap actually happened; when the running version is already current
// it returns updated=false without downloading anything.
func (u updater) run(ctx context.Context) (string, bool, error) {
	goos, goarch := u.platform()

	wantArchive, err := archiveName(goos, goarch)
	if err != nil {
		return "", false, eris.Wrap(err, "cannot update automatically; install manually from "+
			"github.com/Argus-Labs/world-cli/releases")
	}

	// Fail early with a clear message if the binary's directory is not writable,
	// rather than after downloading tens of MB.
	if err := u.writable(); err != nil {
		return "", false, eris.Wrap(err, "cannot write to the World CLI install directory; "+
			"re-run with elevated permissions or reinstall")
	}

	rel, err := fetchLatestRelease(ctx, u.client, u.apiURL)
	if err != nil {
		return "", false, err
	}

	// Dev builds have no AppVersion; always update those.
	if u.version != "" && sameVersion(rel.TagName, u.version) {
		return rel.TagName, false, nil
	}

	archiveAsset, err := rel.asset(wantArchive)
	if err != nil {
		return "", false, err
	}
	sumsAsset, err := rel.checksumsAsset()
	if err != nil {
		return "", false, err
	}

	// Resolve the expected checksum before paying for the ~20-25MB archive, so a
	// missing or unreadable manifest fails fast instead of after the full download.
	manifest, err := download(ctx, u.client, sumsAsset.URL)
	if err != nil {
		return "", false, err
	}
	sums, err := parseChecksums(manifest)
	if err != nil {
		return "", false, err
	}
	want, ok := sums[archiveAsset.Name]
	if !ok {
		return "", false, eris.Errorf("release %s publishes no checksum for %s", rel.TagName, archiveAsset.Name)
	}

	archive, err := download(ctx, u.client, archiveAsset.URL)
	if err != nil {
		return "", false, err
	}
	if err := verifyChecksum(archive, want); err != nil {
		return "", false, eris.Wrapf(err, "release archive %s failed verification", archiveAsset.Name)
	}

	binary, err := extractBinary(archive, archiveAsset.Name, binaryName(goos))
	if err != nil {
		return "", false, err
	}

	if err := u.apply(binary, selfupdate.Options{}); err != nil {
		return "", false, err
	}

	return rel.TagName, true, nil
}

// sameVersion compares a release tag ("v2.4.2") against AppVersion ("2.4.2").
func sameVersion(tag, version string) bool {
	return strings.TrimPrefix(tag, "v") == strings.TrimPrefix(version, "v")
}

// applyBinary swaps the executable named by opts (the running one, when TargetPath
// is empty) for binary.
//
// selfupdate writes ".<name>.new" beside the target, closes it, renames the target
// to ".<name>.old" (Windows forbids overwriting a locked file but allows renaming
// it), moves the new file into place, and rolls back if that final rename fails.
func applyBinary(binary []byte, opts selfupdate.Options) error {
	err := selfupdate.Apply(bytes.NewReader(binary), opts)
	if err == nil {
		return nil
	}

	target := resolveTarget(opts)
	if target != "" {
		// A failed commit leaves the staged ".<name>.new" behind. Removal is
		// best-effort, but log it so --verbose still shows why a stale file remains.
		staged := sidecar(target, ".new")
		if rmErr := os.Remove(staged); rmErr != nil && !os.IsNotExist(rmErr) {
			logger.Debugf("failed to remove staged binary %s: %v", staged, rmErr)
		}
	}

	// Wrap the original failure so errors.Is/As reach the cause; the rollback
	// failure is context in the message.
	if rollbackErr := selfupdate.RollbackError(err); rollbackErr != nil {
		return eris.Wrapf(err,
			"update failed and the previous binary could not be restored (%v). "+
				"Your previous binary is at %s — move it back to %s to recover",
			rollbackErr, sidecar(target, ".old"), target)
	}

	return eris.Wrap(err, "failed to install the new World CLI binary")
}

// resolveTarget returns the path selfupdate will replace.
func resolveTarget(opts selfupdate.Options) string {
	if opts.TargetPath != "" {
		return opts.TargetPath
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

// sidecar builds the ".<name><suffix>" path selfupdate uses beside target.
func sidecar(target, suffix string) string {
	if target == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+suffix)
}

type UpdateCmd struct{}

func (c *UpdateCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("update-command", nil)
	printer.Infoln("You are about to update the World CLI to the latest version.")
	confirmed, err := textinput.Confirm("Are you sure you want to continue?", "y")
	if err != nil {
		return eris.Wrap(err, "Failed to confirm update")
	}
	if !confirmed {
		return errorspkg.NewSilent(eris.New("update cancelled"))
	}

	printer.NewLine(1)
	printer.Infoln("Updating World CLI to the latest version...")
	printer.NewLine(1)

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	tag, updated, err := newUpdater().run(ctx)
	if err != nil {
		return err
	}

	if !updated {
		printer.Successf("World CLI is already up to date (%s).\n", tag)
		return nil
	}

	printer.Successf("World CLI updated successfully to %s.\n", tag)
	printer.NewLine(1)
	printer.Notificationln("Please restart your terminal to use the latest version of the World CLI.")
	return nil
}
