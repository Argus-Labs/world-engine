package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeReader is a configReader stub for exercising the Fetch contract without a database.
type fakeReader struct {
	rows  map[string][]byte
	err   error
	calls []string
}

func (f *fakeReader) readTableJSON(_ context.Context, table string) ([]byte, error) {
	f.calls = append(f.calls, table)
	if f.err != nil {
		return nil, f.err
	}
	return f.rows[table], nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestTableFromFile(t *testing.T) {
	cases := map[string]string{
		"testdata/abilities.json": "abilities",
		"abilities.json":          "abilities",
		"data/mobs.json":          "mobs",
		"loot_table.json":         "loot_table",
	}
	for in, want := range cases {
		require.Equalf(t, want, tableFromFile(in), "tableFromFile(%q)", in)
	}
}

func TestPostgresSourceFetchReturnsRowsAndHash(t *testing.T) {
	body := []byte(`[{"id":"fireball","cooldown":3}]`)
	src := &PostgresSource{
		reader:   &fakeReader{rows: map[string][]byte{"abilities": body}},
		tableFor: tableFromFile,
	}

	got, hash, err := src.Fetch(context.Background(), "testdata/abilities.json", "")

	require.NoError(t, err)
	require.Equal(t, body, got)
	require.Equal(t, sha256Hex(body), hash, "hash must be sha256 of the returned bytes, matching EmbedSource")
}

// Single-version: an unservable hash returns current rows (no error), so restore takes the warn
// path, not the panic path.
func TestPostgresSourceFetchIgnoresRequestedHash(t *testing.T) {
	body := []byte(`[{"id":"a"}]`)
	src := &PostgresSource{
		reader:   &fakeReader{rows: map[string][]byte{"abilities": body}},
		tableFor: tableFromFile,
	}

	got, hash, err := src.Fetch(context.Background(), "abilities.json", "some-old-snapshot-hash")

	require.NoError(t, err, "an unservable historical hash must NOT error (single-version semantics)")
	require.Equal(t, body, got)
	require.Equal(t, sha256Hex(body), hash)
	require.NotEqual(t, "some-old-snapshot-hash", hash)
}

func TestPostgresSourceFetchResolvesTableFromFile(t *testing.T) {
	fr := &fakeReader{rows: map[string][]byte{"mobs": []byte("[]")}}
	src := &PostgresSource{reader: fr, tableFor: tableFromFile}

	_, _, err := src.Fetch(context.Background(), "config/mobs.json", "")

	require.NoError(t, err)
	require.Equal(t, []string{"mobs"}, fr.calls, "Fetch must query the table derived from the file name")
}

func TestPostgresSourceFetchWrapsReaderError(t *testing.T) {
	sentinel := errors.New("connection refused")
	src := &PostgresSource{
		reader:   &fakeReader{err: sentinel},
		tableFor: tableFromFile,
	}

	_, _, err := src.Fetch(context.Background(), "abilities.json", "")

	require.Error(t, err)
	require.ErrorIs(t, err, sentinel, "reader errors must propagate (a boot-time load failure), not be swallowed")
}
