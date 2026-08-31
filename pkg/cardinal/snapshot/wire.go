package snapshot

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"google.golang.org/protobuf/encoding/protowire"
)

// Envelope encoding: the Snapshot message written by hand, so the tick can produce the complete
// snapshot bytes in one pass into one buffer, with no proto message graph in between.
//
// The envelope is tiny and fixed — tick_height (1), timestamp (2), world_state (3), version (4) —
// and the world_state length prefix needs the body size, which the ECS size pass supplies. Fields
// are written in field-number order, matching what proto.Marshal produces, so hand-encoded and
// library-encoded envelopes are byte-identical. The one decode boundary (Decode) uses the ordinary
// generated message; only the write side is hand-rolled.

// EnvelopeSize returns the exact total size of the encoded Snapshot: envelope fields plus the
// world-state body the ECS reported. Use it to allocate the buffer once, exactly.
func EnvelopeSize(tick uint64, timestamp time.Time, bodySize int) int {
	n := 0
	if tick != 0 {
		n += protowire.SizeTag(1) + protowire.SizeVarint(tick)
	}
	if ts := timestampWireSize(timestamp); ts > 0 {
		n += protowire.SizeTag(2) + protowire.SizeBytes(ts)
	}
	n += protowire.SizeTag(3) + protowire.SizeBytes(bodySize)
	n += protowire.SizeTag(4) + protowire.SizeVarint(uint64(CurrentVersion))
	return n
}

// AppendEnvelopeHeader writes every envelope field before the world-state body: tick_height,
// timestamp, and the world_state tag with its length prefix. The caller appends exactly bodySize
// bytes of body next, then AppendEnvelopeFooter.
func AppendEnvelopeHeader(buf []byte, tick uint64, timestamp time.Time, bodySize int) []byte {
	if tick != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, tick)
	}
	if ts := timestampWireSize(timestamp); ts > 0 {
		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		buf = protowire.AppendVarint(buf, uint64(ts))
		buf = appendTimestampWire(buf, timestamp)
	}
	buf = protowire.AppendTag(buf, 3, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(bodySize)) //nolint:gosec // sizes are non-negative
	return buf
}

// AppendEnvelopeFooter writes the fields that follow the world-state body — just the version.
func AppendEnvelopeFooter(buf []byte) []byte {
	buf = protowire.AppendTag(buf, 4, protowire.VarintType)
	buf = protowire.AppendVarint(buf, uint64(CurrentVersion))
	return buf
}

// timestampWireSize is the encoded size of a google.protobuf.Timestamp message body:
// {int64 seconds = 1; int32 nanos = 2}, zero-valued fields skipped.
func timestampWireSize(t time.Time) int {
	n := 0
	if s := t.Unix(); s != 0 {
		n += protowire.SizeTag(1) + protowire.SizeVarint(uint64(s)) //nolint:gosec // sign-extends, matching proto varint
	}
	if ns := int64(t.Nanosecond()); ns != 0 {
		n += protowire.SizeTag(2) + protowire.SizeVarint(uint64(ns)) //nolint:gosec // nanos are non-negative
	}
	return n
}

func appendTimestampWire(buf []byte, t time.Time) []byte {
	if s := t.Unix(); s != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(s)) //nolint:gosec // sign-extends, matching proto varint
	}
	if ns := int64(t.Nanosecond()); ns != 0 {
		buf = protowire.AppendTag(buf, 2, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(ns)) //nolint:gosec // nanos are non-negative
	}
	return buf
}

// Encode assembles a complete snapshot: envelope around the body produced by appendBody, in one
// exactly-sized buffer. appendBody must append exactly bodySize bytes — the assert holds the whole
// scheme together, converting a size/bytes divergence into a crash instead of a corrupt snapshot.
//
// The returned buffer is freshly allocated and handed to the caller: ownership transfers to
// storage with no reuse, so nothing downstream can race a reused buffer. One allocation per
// snapshot, exact-size — against the old path's per-component graph, the rounding error.
func Encode(tick uint64, timestamp time.Time, bodySize int, appendBody func([]byte) []byte) []byte {
	total := EnvelopeSize(tick, timestamp, bodySize)
	buf := make([]byte, 0, total)
	buf = AppendEnvelopeHeader(buf, tick, timestamp, bodySize)

	bodyStart := len(buf)
	buf = appendBody(buf)
	assert.That(len(buf)-bodyStart == bodySize, "snapshot body wrote %d bytes, size pass computed %d",
		len(buf)-bodyStart, bodySize)

	buf = AppendEnvelopeFooter(buf)
	assert.That(len(buf) == total, "snapshot envelope wrote %d bytes, computed %d", len(buf), total)
	return buf
}
