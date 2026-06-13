package system

import (
	"context"
	"errors"
)

// HybridSource is the live-config Source used when a config database is configured. It reads each
// kind from Postgres and, for any kind whose table does not exist yet, falls back per-kind to the
// build-time embedded JSON. This lets the database hold only the subset of kinds that have been
// migrated while the rest keep serving their embedded defaults.
//
// Only ErrTableNotFound triggers the fallback. Every other Postgres error (connectivity, an empty
// singleton table, a malformed query) fails loud — a misconfigured or unreachable config database
// must not silently degrade to embedded data.
type HybridSource struct {
	pg    *PostgresSource
	embed EmbedSource
}

var (
	_ Source             = (*HybridSource)(nil)
	_ SingletonRegistrar = (*HybridSource)(nil)
	_ KindRegistrar      = (*HybridSource)(nil)
)

// Fetch reads file from Postgres, falling back to the embedded copy when the kind's table does not
// exist. Any non-ErrTableNotFound error propagates (fail loud).
func (h *HybridSource) Fetch(ctx context.Context, file, hash string) ([]byte, string, error) {
	raw, gotHash, err := h.pg.Fetch(ctx, file, hash)
	if err != nil {
		if errors.Is(err, ErrTableNotFound) {
			return h.embed.Fetch(ctx, file, hash)
		}
		return nil, "", err
	}
	return raw, gotHash, nil
}

// RegisterSingleton forwards to the Postgres source so its table is read as a single object. The
// embedded fallback is file-based and needs no per-kind shape hint.
func (h *HybridSource) RegisterSingleton(file string) { h.pg.RegisterSingleton(file) }

// RegisterKind forwards the file→table mapping to the Postgres source. The embedded fallback keys
// off the file path directly, so it needs no mapping.
func (h *HybridSource) RegisterKind(file, table string) { h.pg.RegisterKind(file, table) }
