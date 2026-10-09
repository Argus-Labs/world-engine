package docker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

// The fixture is a real /build stream (Docker 29.1.3) of a two-stage build
// whose go build fails. Before, the error stopped at "exit code: 1".
func TestDrainBuildResponseQuotesFailedStep(t *testing.T) {
	t.Parallel()

	f, err := os.Open("testdata/buildkit_compile_error.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	err = drainBuildResponse(context.Background(), f, "demo-game-shard")
	want := `build error for demo-game-shard: process "/bin/sh -c CGO_ENABLED=0 go build -o /out/shard ./shards/game" ` +
		`did not complete successfully: exit code: 1
# demo/shards/game/system
shards/game/system/run.go:4:2: undefined: undefinedThing
shards/game/system/run.go:5:6: declared and not used: unused`
	if err == nil || err.Error() != want {
		t.Fatalf("got:\n%v\nwant:\n%s", err, want)
	}
}

func TestBuildOutputTail(t *testing.T) {
	t.Parallel()

	var o buildOutput
	if got := o.tail(); got != "" {
		t.Fatalf("no output: want \"\", got %q", got)
	}

	var out strings.Builder
	for i := range maxOutputLines + 5 {
		fmt.Fprintf(&out, "line %d\n", i)
	}
	log := protowire.AppendString(protowire.AppendTag(nil, logMsg, protowire.BytesType), out.String())
	o.add(protowire.AppendBytes(protowire.AppendTag(nil, statusLogs, protowire.BytesType), log))

	var want strings.Builder
	for i := 5; i < maxOutputLines+5; i++ {
		fmt.Fprintf(&want, "\nline %d", i)
	}
	if got := o.tail(); got != want.String() {
		t.Fatalf("want lines 5-24, got %q", got)
	}
}
