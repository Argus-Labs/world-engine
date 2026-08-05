package system

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
)

// Source supplies the raw bytes for a named file along with the content hash the source attaches
// to those bytes. Implementations keep the plugin dev/prod portable: tests and local dev use
// EmbedSource (build-time JSON), while PostgresSource reads config rows live from a database.
//
// The hash argument selects which version to return:
//   - ""        — the version the source currently serves.
//   - non-empty — a specific historical content hash (used on crash-restart to resume the
//     snapshot's exact config). A versioned source that no longer holds the requested hash
//     returns an error.
//
// A single-version source (EmbedSource) ignores the requested hash and returns its current
// bytes; the caller compares the returned gotHash to the requested hash to detect that case.
type Source interface {
	Fetch(ctx context.Context, file, hash string) (bytes []byte, gotHash string, err error)
}

// EmbedSource serves files baked into the binary via go:embed. It is single-version by design —
// there is only one snapshot of the embedded files for the lifetime of a binary.
type EmbedSource struct {
	FS embed.FS
}

// Fetch reads file from the embedded filesystem and returns the bytes plus their sha256 hex digest.
// The hash argument is ignored — EmbedSource only has one version. Callers compare the returned
// hash to what they asked for to know whether they got the version they wanted.
func (e EmbedSource) Fetch(_ context.Context, file, _ string) ([]byte, string, error) {
	raw, err := e.FS.ReadFile(file)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

// PickSource returns the Source the plugin should use: the embedded JSON via EmbedSource.
func PickSource(fs embed.FS) Source {
	return EmbedSource{FS: fs}
}
