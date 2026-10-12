package docker

import (
	"bytes"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

// buildKitTraceID marks /build stream events whose aux is BuildKit's progress:
// a base64 protobuf moby.buildkit.v1.StatusResponse.
const buildKitTraceID = "moby.buildkit.trace"

// maxOutputLines bounds how much build output a build error quotes: enough
// for a Go compiler error block.
const maxOutputLines = 20

// Field numbers from BuildKit's api/services/control/control.proto.
const (
	statusLogs protowire.Number = 3 // StatusResponse.logs
	logMsg     protowire.Number = 4 // VertexLog.msg
)

// buildOutput collects what the build's steps print, from the trace. In
// cardinal.Dockerfile only go build prints (module downloads, then any
// errors), so the tail ends with the failure's own output, e.g. the compiler's.
type buildOutput struct {
	buf bytes.Buffer
}

// add reads one trace payload. Best effort: the trace only adds detail, so an
// unreadable payload is skipped rather than failing the build.
func (o *buildOutput) add(payload []byte) {
	eachBytesField(payload, func(num protowire.Number, log []byte) {
		if num != statusLogs {
			return
		}
		eachBytesField(log, func(n protowire.Number, msg []byte) {
			if n == logMsg {
				o.buf.Write(msg)
			}
		})
	})
}

// tail returns the last output lines, each after a newline, ready to append to
// the build error; "" if the build printed nothing.
func (o *buildOutput) tail() string {
	out := strings.TrimRight(o.buf.String(), "\n")
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	return "\n" + strings.Join(lines[max(0, len(lines)-maxOutputLines):], "\n")
}

// eachBytesField calls fn with each length-delimited field (string, bytes or
// sub-message) of a protobuf message, skipping other wire types. It stops at
// the first malformed field.
func eachBytesField(msg []byte, fn func(num protowire.Number, v []byte)) {
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			return
		}
		msg = msg[n:]
		if typ == protowire.BytesType {
			v, m := protowire.ConsumeBytes(msg)
			if m < 0 {
				return
			}
			fn(num, v)
			msg = msg[m:]
			continue
		}
		m := protowire.ConsumeFieldValue(num, typ, msg)
		if m < 0 {
			return
		}
		msg = msg[m:]
	}
}
