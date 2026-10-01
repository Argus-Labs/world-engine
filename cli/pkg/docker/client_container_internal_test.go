package docker

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	"github.com/moby/moby/api/pkg/stdcopy"
)

// muxFrame builds a single Docker stdcopy frame for a non-TTY container log
// stream: an 8-byte header (STREAM_TYPE, 0, 0, 0, SIZE1..4 big-endian) followed
// by the payload bytes. This mirrors the format the Docker daemon produces for
// non-TTY containers and that stdcopy.StdCopy parses.
func muxFrame(streamType stdcopy.StdType, payload string) []byte {
	frame := make([]byte, 8+len(payload))
	frame[0] = byte(streamType)
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:], payload)
	return frame
}

func TestReadContainerLogs_DemuxesMultiplexedStream(t *testing.T) {
	t.Parallel()

	stream := bytes.Join([][]byte{
		muxFrame(stdcopy.Stdout, "stdout-line-AAA\n"),
		muxFrame(stdcopy.Stderr, "stderr-line-BBB\n"),
	}, nil)

	got, err := readContainerLogs(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("readContainerLogs returned error: %v", err)
	}

	// No binary frame headers (which contain NUL bytes) should remain.
	if strings.ContainsRune(got, 0) {
		t.Fatalf("output contains binary frame header bytes: %q", got)
	}
	if !strings.Contains(got, "stdout-line-AAA\n") || !strings.Contains(got, "stderr-line-BBB\n") {
		t.Fatalf("output missing expected log lines: %q", got)
	}
	// stdout is grouped before stderr (deterministic two-buffer concat).
	want := "stdout-line-AAA\nstderr-line-BBB\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestReadContainerLogs_GroupsStdoutBeforeStderrPreservingWithinStreamOrder(t *testing.T) {
	t.Parallel()

	// Interleaved stdout/stderr frames. The daemon does not guarantee the
	// relative ordering of stdout and stderr frames in the multiplexed stream,
	// so output groups all stdout (in frame order) before all stderr (in frame
	// order) to stay deterministic.
	stream := bytes.Join([][]byte{
		muxFrame(stdcopy.Stdout, "out-1\n"),
		muxFrame(stdcopy.Stderr, "err-1\n"),
		muxFrame(stdcopy.Stdout, "out-2\n"),
		muxFrame(stdcopy.Stderr, "err-2\n"),
	}, nil)

	got, err := readContainerLogs(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("readContainerLogs returned error: %v", err)
	}
	want := "out-1\nout-2\nerr-1\nerr-2\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestReadContainerLogs_TruncatedFrameHandledGracefully(t *testing.T) {
	t.Parallel()

	// Two complete frames followed by a third frame whose declared payload
	// size exceeds the remaining bytes, simulating io.LimitReader cutting the
	// stream mid-frame. The partial frame must be dropped, not error.
	complete := bytes.Join([][]byte{
		muxFrame(stdcopy.Stdout, "complete-1\n"),
		muxFrame(stdcopy.Stdout, "complete-2\n"),
	}, nil)
	partial := muxFrame(stdcopy.Stdout, "this-line-will-be-truncated\n")
	truncated := bytes.Join([][]byte{complete, partial[:len(partial)-10]}, nil)

	got, err := readContainerLogs(bytes.NewReader(truncated))
	if err != nil {
		t.Fatalf("truncated stream should not error, got: %v", err)
	}
	want := "complete-1\ncomplete-2\n"
	if got != want {
		t.Fatalf("output = %q, want %q (partial frame must be dropped)", got, want)
	}
}

func TestReadContainerLogs_RawReadIsGarbledVersusDemux(t *testing.T) {
	t.Parallel()

	stream := bytes.Join([][]byte{
		muxFrame(stdcopy.Stdout, "hello\n"),
		muxFrame(stdcopy.Stderr, "world\n"),
	}, nil)

	// The pre-fix code read the stream with io.ReadAll and returned it verbatim,
	// yielding binary frame headers interleaved with the text.
	raw, err := io.ReadAll(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("io.ReadAll returned error: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte{byte(stdcopy.Stdout), 0, 0, 0}) {
		t.Fatalf("raw read should begin with a stdout frame header: %q", raw)
	}

	// readContainerLogs must return clean text without any frame headers.
	got, err := readContainerLogs(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("readContainerLogs returned error: %v", err)
	}
	want := "hello\nworld\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if strings.ContainsRune(got, 0) {
		t.Fatalf("output still contains binary frame headers: %q", got)
	}
}
