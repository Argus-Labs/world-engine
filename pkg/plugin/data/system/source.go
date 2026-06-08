package system

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"os"

	"github.com/rotisserie/eris"
)

// Source supplies the raw bytes for a named file along with the content hash the source attaches
// to those bytes. Implementations are how the plugin stays dev/prod portable: tests and local dev
// use EmbedSource (build-time JSON), while on dev/prod PostgresSource reads config rows live from
// a Postgres database.
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

// PickSource returns the Source the plugin should use given the current environment.
//
// If CONFIG_DB_DSN is set (a read-only config-database DSN) the plugin reads config rows live
// from Postgres via PostgresSource; otherwise it serves the shard's build-time embedded JSON via
// EmbedSource. Source selection happens here so every shard's main.go stays unchanged. A
// set-but-unusable CONFIG_DB_DSN is a fatal misconfiguration — fail loud.
func PickSource(fs embed.FS) Source {
	if dsn := os.Getenv("CONFIG_DB_DSN"); dsn != "" {
		src, err := NewPostgresSource(context.Background(), dsn)
		if err != nil {
			panic(eris.Wrap(err, "data: CONFIG_DB_DSN is set but the postgres config source failed to initialise"))
		}
		return src
	}
	return EmbedSource{FS: fs}
}
