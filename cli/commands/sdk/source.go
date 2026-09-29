package sdk

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/printer"
)

// resolveSource returns a local directory to scan. A remote git URL is
// shallow-cloned into a temp dir (cleanup removes it); a local path is used in
// place (cleanup is a no-op).
func resolveSource(ctx context.Context, src, ref string) (string, func(), error) {
	noop := func() {}
	if !isRemoteSource(src) {
		return src, noop, nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "", noop, eris.New("git is required to clone a remote source")
	}
	tmp, err := os.MkdirTemp("", "sdkgen-src-")
	if err != nil {
		return "", noop, eris.Wrap(err, "create temp dir for clone")
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }

	printer.Infof("Cloning %s ...", src)
	printer.NewLine(1)
	if ferr := fetchSource(ctx, tmp, normalizeRepoURL(src), ref); ferr != nil {
		cleanup()
		return "", noop, ferr
	}
	return tmp, cleanup, nil
}

// fetchSource shallow-fetches url@ref into dir. ref may be a branch, tag, or full
// commit SHA; empty means the default branch. A plain `clone --branch` rejects
// commit hashes, so for an explicit ref we init + fetch the single commit instead.
func fetchSource(ctx context.Context, dir, url, ref string) error {
	if ref == "" {
		return runGit(ctx, "clone", "--depth", "1", url, dir)
	}
	for _, args := range [][]string{
		{"init", "-q", dir},
		{"-C", dir, "remote", "add", "origin", url},
		{"-C", dir, "fetch", "-q", "--depth", "1", "origin", ref},
		{"-C", dir, "checkout", "-q", "FETCH_HEAD"},
	} {
		if err := runGit(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

func runGit(ctx context.Context, args ...string) error {
	if out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
		return eris.Wrapf(err, "git %s failed: %s", args[0], strings.TrimSpace(string(out)))
	}
	return nil
}

// isRemoteSource reports whether src should be cloned rather than read in place.
func isRemoteSource(src string) bool {
	return strings.HasPrefix(src, "http://") ||
		strings.HasPrefix(src, "https://") ||
		strings.HasPrefix(src, "git@") ||
		strings.HasPrefix(src, "ssh://") ||
		strings.HasPrefix(src, "github.com/")
}

// normalizeRepoURL turns the bare "github.com/owner/repo" form into an https URL.
func normalizeRepoURL(src string) string {
	if strings.HasPrefix(src, "github.com/") {
		return "https://" + src
	}
	return src
}
