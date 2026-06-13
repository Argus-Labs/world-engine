package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rotisserie/eris"
)

// ErrTableNotFound reports that a kind's config table does not exist in the database (Postgres
// SQLSTATE 42P01). It is the signal HybridSource uses to fall back to embedded JSON for that one
// kind; every other read error fails loud.
var ErrTableNotFound = eris.New("data: config table not found")

// PostgresSource reads each config kind's rows from a Postgres database as JSON bytes. Like
// EmbedSource it is single-version: it ignores the requested hash and returns the current rows, so
// a snapshot restore resumes on current config (Reconcile's warn path) instead of erroring (the
// panic path).
//
// A kind may be registered as a singleton (RegisterSingleton) — its table holds at most one row and
// Fetch returns one JSON object instead of an array — and may be given an explicit table name
// (RegisterKind). Unregistered kinds fall back to a table name derived from the JSON file.
type PostgresSource struct {
	reader     configReader
	singletons map[string]bool   // jsonFile → read as a single object instead of an array
	tables     map[string]string // jsonFile → table name (overrides tableFromFile)
}

var (
	_ Source             = (*PostgresSource)(nil)
	_ SingletonRegistrar = (*PostgresSource)(nil)
	_ KindRegistrar      = (*PostgresSource)(nil)
)

// configReader is the DB seam, extracted so Fetch is unit-testable without a live database.
type configReader interface {
	// readTableJSON returns table's rows as JSON: a single object when singleton is true (the first
	// row), otherwise a deterministic JSON array ("[]" when empty). A missing table surfaces as
	// ErrTableNotFound.
	readTableJSON(ctx context.Context, table string, singleton bool) ([]byte, error)
}

// SingletonRegistrar is implemented by sources that can be told a kind's config is a single object
// rather than a collection of records.
type SingletonRegistrar interface {
	RegisterSingleton(file string)
}

// KindRegistrar is implemented by sources that accept an explicit table name for a kind's JSON file.
type KindRegistrar interface {
	RegisterKind(file, table string)
}

// NewPostgresSource opens a pgx pool against dsn (a read-only config-database DSN). The pool
// connects lazily: a malformed dsn fails here, connectivity fails on the first Fetch.
func NewPostgresSource(ctx context.Context, dsn string) (*PostgresSource, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, eris.Wrap(err, "data: opening config postgres pool")
	}
	return &PostgresSource{
		reader:     pgxReader{pool: pool},
		singletons: map[string]bool{},
		tables:     map[string]string{},
	}, nil
}

// RegisterSingleton marks file's kind as a single-object config: Fetch returns one JSON object and
// the backing table is expected to hold at most one row.
func (p *PostgresSource) RegisterSingleton(file string) {
	if p.singletons == nil {
		p.singletons = map[string]bool{}
	}
	p.singletons[file] = true
}

// RegisterKind records the table name to read file's kind from, overriding the default derived from
// the file name.
func (p *PostgresSource) RegisterKind(file, table string) {
	if p.tables == nil {
		p.tables = map[string]string{}
	}
	p.tables[file] = table
}

// tableFor returns the table registered for file, falling back to the name derived from the file.
func (p *PostgresSource) tableFor(file string) string {
	if t, ok := p.tables[file]; ok {
		return t
	}
	return tableFromFile(file)
}

// Fetch returns the current contents of file's table as JSON plus their sha256 hex. hash is ignored.
// A missing table surfaces as ErrTableNotFound (eris-wrapped) so HybridSource can fall back to the
// embedded copy.
func (p *PostgresSource) Fetch(ctx context.Context, file, _ string) ([]byte, string, error) {
	table := p.tableFor(file)
	raw, err := p.reader.readTableJSON(ctx, table, p.singletons[file])
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

// readTableJSON reads table as JSON. For a singleton it returns the first row as a JSON object;
// otherwise it aggregates every row into one array ordered by JSON text for a stable hash. table is
// a trusted kind identifier but is quoted via pgx.Identifier.
//
// A missing table (SQLSTATE 42P01) is translated to ErrTableNotFound so callers can distinguish
// "this kind isn't in the database yet" from a real failure. An existing-but-empty singleton table
// surfaces as pgx.ErrNoRows and is left untouched — that is a genuine error (fail loud), not a
// missing table.
func (r pgxReader) readTableJSON(ctx context.Context, table string, singleton bool) ([]byte, error) {
	ident := pgx.Identifier{table}.Sanitize()
	var query string
	if singleton {
		query = "SELECT to_jsonb(t)::text FROM " + ident + " AS t LIMIT 1"
	} else {
		query = "SELECT coalesce(json_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text), '[]'::json)::text " +
			"FROM " + ident + " AS t"
	}
	var out string
	if err := r.pool.QueryRow(ctx, query).Scan(&out); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UndefinedTable {
			return nil, ErrTableNotFound
		}
		return nil, err
	}
	return []byte(out), nil
}
