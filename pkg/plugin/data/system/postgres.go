package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rotisserie/eris"
)

// PostgresSource reads each config kind's rows from a Postgres database as JSON-array bytes.
// Like EmbedSource it is single-version: it ignores the requested hash and returns the current
// rows, so a snapshot restore resumes on current config (Reconcile's warn path) instead of
// erroring (the panic path).
type PostgresSource struct {
	reader   configReader
	tableFor func(file string) string
}

// configReader is the DB seam, extracted so Fetch is unit-testable without a live database.
type configReader interface {
	// readTableJSON returns all rows of table as a deterministic JSON array ("[]" when empty).
	readTableJSON(ctx context.Context, table string) ([]byte, error)
}

// NewPostgresSource opens a pgx pool against dsn (a read-only config-database DSN). The pool
// connects lazily: a malformed dsn fails here, connectivity fails on the first Fetch.
func NewPostgresSource(ctx context.Context, dsn string) (*PostgresSource, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, eris.Wrap(err, "data: opening config postgres pool")
	}
	return &PostgresSource{reader: pgxReader{pool: pool}, tableFor: tableFromFile}, nil
}

// Fetch returns the current rows of file's table as JSON plus their sha256 hex. hash is ignored.
func (p *PostgresSource) Fetch(ctx context.Context, file, _ string) ([]byte, string, error) {
	table := p.tableFor(file)
	raw, err := p.reader.readTableJSON(ctx, table)
	if err != nil {
		return nil, "", eris.Wrapf(err, "data: postgres source reading table %q for %q", table, file)
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

// tableFromFile maps a kind's JSONFile() ("testdata/abilities.json") to its table ("abilities").
func tableFromFile(file string) string {
	base := path.Base(file)
	return strings.TrimSuffix(base, path.Ext(base))
}

type pgxReader struct {
	pool *pgxpool.Pool
}

// readTableJSON aggregates table's rows into one JSON array, ordered by JSON text for a stable
// hash. table is a trusted kind identifier but is quoted via pgx.Identifier.
func (r pgxReader) readTableJSON(ctx context.Context, table string) ([]byte, error) {
	query := "SELECT coalesce(json_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text), '[]'::json)::text " +
		"FROM " + pgx.Identifier{table}.Sanitize() + " AS t"
	var out string
	if err := r.pool.QueryRow(ctx, query).Scan(&out); err != nil {
		return nil, err
	}
	return []byte(out), nil
}
