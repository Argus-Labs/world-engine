package sdkgen_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/cli/internal/sdkgen"
)

// transportShim stands in for world-engine's pkg/transport registration API. Discovery matches transport by
// its exact import path, so it sits at the real path inside the nested world-engine module.
const transportShim = `package transport

type Handler func()

type Transport struct{}

func (*Transport) RegisterCommand[T any](h Handler) {}
func (*Transport) RegisterEvent[T any]()            {}
`

// decoyTransportShim has transport's package name and method names at another path. Nothing registered
// through it is a wire type.
const decoyTransportShim = `package transport

type Transport struct{}

func (*Transport) RegisterCommand[T any]() {}
`

const transportServiceTypes = `
type Join struct{ Room string }

func (Join) Name() string { return "join" }

type Joined struct{ Room string }

func (Joined) Name() string { return "joined" }

type JoinResult struct {
	RequestID string
	Room      string
}

func (r JoinResult) Name() string { return r.RequestID + "_join_result" }
`

// discoverModule writes a module holding the cardinal, transport and decoy shims plus body as package
// service, and runs Discover on it.
func discoverModule(t *testing.T, body string) sdkgen.Result {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", sdkgentestGoMod)
	write("cardinal/cardinal.go", cardinalShim)
	writeImmutableShim(write)
	write("worldengine/pkg/transport/transport.go", transportShim)
	write("decoy/transport/transport.go", decoyTransportShim)
	write("service/service.go", "package service\n\n"+body+"\n")

	res, err := sdkgen.Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return res
}

// TestDiscover_TransportRegistration pins discovery for a service that serves through pkg/transport with
// no cardinal: its commands, events and correlated results get the same kinds, targets and wire names as
// when cardinal declares them, and a look-alike package elsewhere declares nothing.
func TestDiscover_TransportRegistration(t *testing.T) {
	t.Parallel()

	viaTransport := discoverModule(t, `import (
	"github.com/argus-labs/world-engine/pkg/transport"
	decoy "sdkgentest/decoy/transport"
)
`+transportServiceTypes+`
type Decoy struct{ X int32 }

func (Decoy) Name() string { return "decoy" }

func Setup(tr *transport.Transport, d *decoy.Transport) {
	tr.RegisterCommand[Join](nil)
	tr.RegisterEvent[Joined]()
	tr.RegisterEvent[JoinResult]()
	d.RegisterCommand[Decoy]()
}`)
	viaCardinal := discoverModule(t, `import "sdkgentest/cardinal"
`+transportServiceTypes+`
func Setup(w *cardinal.World) {
	w.RegisterCommand[Join]()
	w.RegisterEvent[Joined]()
	w.RegisterEvent[JoinResult]()
}`)

	summary := func(res sdkgen.Result) map[string]sdkgen.Message {
		out := map[string]sdkgen.Message{}
		for _, m := range res.Messages {
			out[m.Name] = m
		}
		return out
	}
	got, want := summary(viaTransport), summary(viaCardinal)
	if names := slices.Sorted(maps.Keys(got)); !slices.Equal(names, []string{"Join", "JoinResult", "Joined"}) {
		t.Fatalf("discovered %v, want Join, JoinResult and Joined", names)
	}
	for name, w := range want {
		g := got[name]
		if g.Kind != w.Kind || !slices.Equal(g.Targets, w.Targets) || g.Wire != w.Wire ||
			g.WireSuffix != w.WireSuffix || g.WireField != w.WireField {
			t.Errorf("%s through transport = kind %q targets %v wire %q suffix %q field %q; "+
				"through cardinal = kind %q targets %v wire %q suffix %q field %q", name,
				g.Kind, g.Targets, g.Wire, g.WireSuffix, g.WireField,
				w.Kind, w.Targets, w.Wire, w.WireSuffix, w.WireField)
		}
	}
	if got["JoinResult"].WireSuffix != "_join_result" {
		t.Errorf("JoinResult not discovered as a correlated result: %+v", got["JoinResult"])
	}
	if want := []string{`Decoy (Name()="decoy")`}; !slices.Equal(viaTransport.Orphans, want) {
		t.Errorf("orphans\n  got  %q\n  want %q", viaTransport.Orphans, want)
	}
}
