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

// PostgresSource reads each config kind's rows from a Postgres database as JSON bytes. Every config
// table has the uniform shape (id text PRIMARY KEY, doc jsonb): one row per record, the record's
// JSON in doc. Like EmbedSource it is single-version: it ignores the requested hash and returns the
// current rows, so a snapshot restore resumes on current config (Reconcile's warn path) instead of
// erroring (the panic path).
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
	// row), otherwise a deterministic JSON array ("[]" when empty).
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
// A missing or empty table surfaces as a read error and propagates (fail loud) — config is read from
// Postgres only.
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

// readTableJSON reads table's doc column as JSON. For a singleton it returns the first row's doc as a
// JSON object; otherwise it aggregates every row's doc into one array ordered by id for a stable
// hash. table is a trusted kind identifier but is quoted via pgx.Identifier.
//
// A missing table or an empty singleton table surfaces as a query error (SQLSTATE 42P01 /
// pgx.ErrNoRows respectively) and is left to propagate — there is no fallback, so an unreadable
// table is a genuine boot failure.
func (r pgxReader) readTableJSON(ctx context.Context, table string, singleton bool) ([]byte, error) {
	ident := pgx.Identifier{table}.Sanitize()
	var query string
	if singleton {
		query = "SELECT doc::text FROM " + ident + " AS t LIMIT 1"
	} else {
		query = "SELECT coalesce(json_agg(doc ORDER BY id), '[]'::jsonb)::text FROM " + ident + " AS t"
	}
	var out string
	if err := r.pool.QueryRow(ctx, query).Scan(&out); err != nil {
		return nil, err
	}
	return []byte(out), nil
}
