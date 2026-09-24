package snapshot

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: these tests mutate the shared CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES and NATS_URL env vars
// (the latter is consumed by micro.NewClient's env.ParseAs), so they must NOT run in parallel with
// each other. NewJetStreamStorage parses those env vars on every call, which is exactly the boot
// path under test.

// -------------------------------------------------------------------------------------------------
// Pure unit tests (no NATS, may run in parallel)
// -------------------------------------------------------------------------------------------------

// TestNormalizeObjectStoreMaxBytes pins the comparison rule: a configured 0 and a server-side -1
// both mean "unlimited" and must compare equal; every other value passes through unchanged so a
// real cap is never silently treated as a match.
func TestNormalizeObjectStoreMaxBytes(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want int64 }{
		{0, -1},  // configured unset / unlimited -> server's -1 form
		{-1, -1}, // server-side unlimited already in -1 form
		{1, 1},   // a real cap is unchanged
		{1 << 30, 1 << 30},
		{1<<62 + 7, 1<<62 + 7},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, normalizeObjectStoreMaxBytes(c.in), "normalize(%d)", c.in)
	}
}

// -------------------------------------------------------------------------------------------------
// Integration tests against the embedded nats-server (serial)
// -------------------------------------------------------------------------------------------------

// captureLogger builds a zerolog.Logger that writes JSON to a buffer for assertion.
func captureLogger(buf *bytes.Buffer) zerolog.Logger {
	return zerolog.New(buf)
}

// makeJetStreamStorage is the test seam around NewJetStreamStorage. It sets
// CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES (the only JetStreamStorageOptions env var) to maxBytes and
// points NewJetStreamStorage's internal NATS client at the embedded test server via NATS_URL. The
// same address must be reused across calls to exercise the create-then-bind (restart) path.
func makeJetStreamStorage(
	t *testing.T,
	addr *micro.ServiceAddress,
	maxBytes uint64,
	logBuf *bytes.Buffer,
) *JetStreamStorage {
	t.Helper()
	t.Setenv("CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES", strconv.FormatUint(maxBytes, 10))
	t.Setenv("NATS_URL", TestNATS.ClientURL())
	t.Setenv("NATS_NAME", "snapshot-test")
	s, err := NewJetStreamStorage(JetStreamStorageOptions{
		Address: addr,
		Logger:  captureLogger(logBuf),
	})
	require.NoError(t, err)
	return s
}

// inspectJetStream opens an independent JetStream context for reading server-side stream config
// in assertions. It is separate from the storage under test so its logger never pollutes logBuf.
func inspectJetStream(t *testing.T) jetstream.JetStream {
	t.Helper()
	c, err := micro.NewClient(
		micro.WithNATSConfig(micro.NATSConfig{Name: "snapshot-inspect", URL: TestNATS.ClientURL()}),
		micro.WithLogger(zerolog.Nop()),
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	js, err := jetstream.New(c.Conn)
	require.NoError(t, err)
	return js
}

// bucketNameFor mirrors the bucket-name derivation in NewJetStreamStorage so tests can look up the
// backing stream / assert on the bucket the storage actually used.
func bucketNameFor(addr *micro.ServiceAddress) string {
	return addr.GetOrganization() + "_" + addr.GetProject() + "_" + addr.GetServiceId() + "_snapshot"
}

// warnLevel is the JSON marker a zerolog Warn-level line carries.
const warnLevel = `"level":"warn"`

// driftMarker is the substring unique to the MaxBytes drift warning; its absence proves the bind
// path did not emit the warning.
const driftMarker = "was not applied to the existing ObjectStore bucket"

// containsWarn asserts logBuf holds a Warn-level line containing every fragment.
func containsWarn(t *testing.T, logBuf *bytes.Buffer, fragments ...string) {
	t.Helper()
	out := logBuf.String()
	require.Containsf(t, out, warnLevel, "expected a warn-level log line; got:\n%s", out)
	for _, f := range fragments {
		assert.Containsf(t, out, f, "warn log missing fragment %q; got:\n%s", f, out)
	}
}

// assertNoDriftWarn asserts logBuf holds no MaxBytes drift warning.
func assertNoDriftWarn(t *testing.T, logBuf *bytes.Buffer) {
	t.Helper()
	assert.NotContainsf(t, logBuf.String(), driftMarker, "no drift warning expected; got:\n%s", logBuf.String())
}

// TestJetStreamStorageCreatesBucketWithConfiguredMaxBytes verifies the first-launch create path
// honors CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES: the backing stream's MaxBytes equals the configured
// value, and no drift warning is emitted (there is nothing to drift from).
func TestJetStreamStorageCreatesBucketWithConfiguredMaxBytes(t *testing.T) {
	addr := randServiceAddress(t)
	bucket := bucketNameFor(addr)
	const capBytes = int64(1 << 20) // 1 MiB

	var logBuf bytes.Buffer
	_ = makeJetStreamStorage(t, addr, uint64(capBytes), &logBuf)

	js := inspectJetStream(t)
	assert.Equal(t, capBytes, requireObjectStoreMaxBytes(t, js, bucket), "first launch must set the configured cap")
	assertNoDriftWarn(t, &logBuf)
}

// TestJetStreamStorageWarnsOnMaxBytesMismatchOnRestart is the core regression for the bug: on a
// restart where the env var changed, the bind path must NOT silently drop the new value. It must
// emit a boot-time warning naming the bucket, the configured value, and the actual server value,
// and it must NOT mutate server-side state (Approach B: read-then-warn, never update).
func TestJetStreamStorageWarnsOnMaxBytesMismatchOnRestart(t *testing.T) {
	addr := randServiceAddress(t)
	bucket := bucketNameFor(addr)
	const firstCap = int64(1 << 20)  // 1 MiB applied on first creation
	const secondCap = int64(4) << 20 // 4 MiB configured after restart; must NOT be applied

	_ = makeJetStreamStorage(t, addr, uint64(firstCap), &bytes.Buffer{})

	// Sanity: first launch established the original cap server-side.
	js := inspectJetStream(t)
	require.Equal(t, firstCap, requireObjectStoreMaxBytes(t, js, bucket))

	var restartLog bytes.Buffer
	store := makeJetStreamStorage(t, addr, uint64(secondCap), &restartLog)

	// The warning must name configured (secondCap) and actual (firstCap), and the bucket.
	containsWarn(t, &restartLog,
		driftMarker,
		`"configured_max_bytes":`+strconv.FormatInt(secondCap, 10),
		`"actual_max_bytes":`+strconv.FormatInt(firstCap, 10),
		`"bucket":"`+bucket+`"`,
		"CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES",
		"nats stream update", // remediation hint
	)

	// Approach B must not change server state: the server-side cap is still the original.
	assert.Equal(t, firstCap, requireObjectStoreMaxBytes(t, js, bucket),
		"bind path must not mutate server-side MaxBytes")

	// The bound store remains usable end-to-end.
	ctx := context.Background()
	payload := []byte("restart-snapshot")
	require.NoError(t, store.Store(ctx, 1, payload))
	loaded, err := store.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, payload, loaded)
}

// TestJetStreamStorageNoWarnWhenMaxBytesMatchesOnRestart verifies that restarting with the same
// configured cap emits no drift warning (the bind path's no-op-on-match property).
func TestJetStreamStorageNoWarnWhenMaxBytesMatchesOnRestart(t *testing.T) {
	addr := randServiceAddress(t)
	bucket := bucketNameFor(addr)
	const capBytes int64 = 2 << 20

	_ = makeJetStreamStorage(t, addr, uint64(capBytes), &bytes.Buffer{})

	var restartLog bytes.Buffer
	_ = makeJetStreamStorage(t, addr, uint64(capBytes), &restartLog)

	assertNoDriftWarn(t, &restartLog)
	js := inspectJetStream(t)
	assert.Equal(t, capBytes, requireObjectStoreMaxBytes(t, js, bucket))
}

// TestJetStreamStorageNoWarnWhenBothUnlimitedOnRestart verifies the normalization rule: a default
// (unset, 0) env var and a server-side -1 (unlimited) compare equal, so a restart over an
// unlimited bucket emits no spurious warning. Without normalization the bind path would warn on
// every restart of a default-config deployment.
func TestJetStreamStorageNoWarnWhenBothUnlimitedOnRestart(t *testing.T) {
	addr := randServiceAddress(t)
	bucket := bucketNameFor(addr)

	// First launch with the default (0). prepareObjectStoreConfig maps 0 -> -1 on the server.
	_ = makeJetStreamStorage(t, addr, 0, &bytes.Buffer{})

	var restartLog bytes.Buffer
	_ = makeJetStreamStorage(t, addr, 0, &restartLog)

	assertNoDriftWarn(t, &restartLog)
	js := inspectJetStream(t)
	assert.Equal(t, int64(-1), requireObjectStoreMaxBytes(t, js, bucket), "default maps to unlimited (-1) server-side")
}

// TestJetStreamStorageWarnsWhenConfiguredUnlimitedButServerCapped verifies the asymmetry: raising
// the configured value to "unlimited" (0) over an existing capped bucket still drifts (the operator
// likely intended to remove a cap that is still enforced server-side), so a warning is required.
func TestJetStreamStorageWarnsWhenConfiguredUnlimitedButServerCapped(t *testing.T) {
	addr := randServiceAddress(t)
	bucket := bucketNameFor(addr)
	const firstCap int64 = 1 << 20

	_ = makeJetStreamStorage(t, addr, uint64(firstCap), &bytes.Buffer{})

	var restartLog bytes.Buffer
	_ = makeJetStreamStorage(t, addr, 0, &restartLog) // now configured unlimited

	containsWarn(t, &restartLog,
		`"configured_max_bytes":0`,
		`"actual_max_bytes":`+strconv.FormatInt(firstCap, 10),
		`"bucket":"`+bucket+`"`,
	)
	js := inspectJetStream(t)
	assert.Equal(t, firstCap, requireObjectStoreMaxBytes(t, js, bucket), "server cap unchanged")
}

// TestJetStreamStorageWarnsWhenConfiguredCappedButServerUnlimited verifies the other asymmetry:
// setting a cap on a previously-unlimited bucket drifts and must warn.
func TestJetStreamStorageWarnsWhenConfiguredCappedButServerUnlimited(t *testing.T) {
	addr := randServiceAddress(t)
	bucket := bucketNameFor(addr)
	const secondCap int64 = 1 << 20

	_ = makeJetStreamStorage(t, addr, 0, &bytes.Buffer{}) // first: unlimited (server -1)

	var restartLog bytes.Buffer
	_ = makeJetStreamStorage(t, addr, uint64(secondCap), &restartLog) // now capped

	containsWarn(t, &restartLog,
		`"configured_max_bytes":`+strconv.FormatInt(secondCap, 10),
		`"actual_max_bytes":-1`,
		`"bucket":"`+bucket+`"`,
	)
	js := inspectJetStream(t)
	assert.Equal(t, int64(-1), requireObjectStoreMaxBytes(t, js, bucket), "server cap unchanged (still unlimited)")
}

// TestJetStreamStorageStoreAndLoadRoundTrip verifies the storage mechanics unaffected by the fix:
// a fresh bucket stores and reloads the exact bytes, and Load before any Store reports not-found.
func TestJetStreamStorageStoreAndLoadRoundTrip(t *testing.T) {
	addr := randServiceAddress(t)
	store := makeJetStreamStorage(t, addr, 0, &bytes.Buffer{})

	ctx := context.Background()

	_, err := store.Load(ctx)
	require.ErrorIs(t, err, ErrSnapshotNotFound, "empty bucket must surface ErrSnapshotNotFound")

	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	require.NoError(t, store.Store(ctx, 7, payload))
	loaded, err := store.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, payload, loaded, "Load must return the exact bytes handed to Store")

	// A second Store overwrites the first.
	second := []byte{0x01, 0x02}
	require.NoError(t, store.Store(ctx, 8, second))
	loaded, err = store.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, second, loaded)
}

// TestJetStreamStorageStoreFailsPastServerCap documents that the embedded nats-server (v2.12.6,
// the version pinned in go.mod) enforces the stream MaxBytes as a hard limit on object puts. This
// is the "enforcing server" runtime failure mode the bug report predicts: once a snapshot grows
// past the still-original server-side cap, Store fails loudly per-tick. The fix is the boot-time
// warning that flags the drift before it can bite; this test locks in the server's enforcement so
// the warning's premise is known to hold on the pinned server.
func TestJetStreamStorageStoreFailsPastServerCap(t *testing.T) {
	addr := randServiceAddress(t)
	// Create a bucket with a one-byte stream cap.
	store := makeJetStreamStorage(t, addr, 1, &bytes.Buffer{})

	ctx := context.Background()
	// Putting substantially more than the cap must be rejected by the server.
	err := store.Store(ctx, 1, make([]byte, 1<<14)) // 16 KiB into a 1-byte cap
	require.Error(t, err, "enforcing server must reject a snapshot larger than the stream cap")
	lowered := strings.ToLower(err.Error())
	assert.True(t,
		strings.Contains(lowered, "max bytes") || strings.Contains(lowered, "resource") ||
			strings.Contains(lowered, "exceed") || strings.Contains(lowered, "maximum"),
		"error should indicate the cap was exceeded; got: %v", err)
}
