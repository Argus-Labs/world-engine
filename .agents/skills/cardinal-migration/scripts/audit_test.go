// Run: cd .agents/skills/cardinal-migration/scripts && go test audit.go audit_test.go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const goMod = "module example.com/game\n\ngo 1.24\n"

const oldSystem = `package game

import "github.com/argus-labs/world-engine/pkg/cardinal"

type SysState struct{ cardinal.BaseSystemState }

func Sys(state *SysState) error { return nil }

func register(world *cardinal.World) { cardinal.RegisterSystem(world, Sys) }
`

func TestOldAPIReported(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{"go.mod": goMod, "game/old.go": oldSystem})

	out, code := runAudit(root)

	assert.Equal(t, `-    1  cardinal.RegisterSystem(world, fn)
        game/old.go:9
-    1  state struct embeds cardinal.BaseSystemState
        game/old.go:5`, section(t, out, "Old API and hazards"))
	assert.Equal(t, 1, code)
}

func TestIgnoredDirectoriesSkipped(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":                  goMod,
		"testdata/old.go":         oldSystem,
		"testdata/broken.go":      "package (",
		"_scratch/old.go":         oldSystem,
		"game/nested/_old/old.go": oldSystem,
	})

	out, code := runAudit(root)

	assert.Equal(t, "- none", section(t, out, "Old API and hazards"))
	assert.Equal(t, 0, code)
}

func TestPointerArgumentsReported(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": goMod,
		"main.go": `package main

import (
	"math/big"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type SpawnSystem struct{}

func (s *SpawnSystem) Run(w *cardinal.World) {
	c := Health{HP: 10}
	e := w.Create[Player]()
	e.Set(&c)
	ev := Died{}
	w.Broadcast(&ev)
	for p := range w.Contains[Player]().Iter() {
		p.Set(&c)
	}
	w.Entity(id).Set(&c)
	e.Set(c)
	var x, z big.Int
	z.Set(&x)
	n := new(big.Int)
	n.Set(&x)
}
`,
	})

	out, code := runAudit(root)

	// Set is flagged on lines 14, 18 and 20; examples show one per file.
	assert.Equal(t, `-    3  pointer passed to Set (pass the value)
        main.go:14
-    1  pointer passed to Broadcast (pass the value)
        main.go:16`, section(t, out, "Old API and hazards"))
	assert.Equal(t, 1, code)
}

func TestPluginVariablesResolvedToConstructor(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": goMod,
		"plugins.go": `package main

import "github.com/argus-labs/world-engine/pkg/plugin/physics2d"

var phys = physics2d.NewPlugin()
`,
		"main.go": `package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/lobby"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

func register(w *cardinal.World) {
	w.RegisterComponent[Health]()
	dataPlugin := data.NewPlugin()
	w.RegisterPlugin(dataPlugin)
	w.RegisterPlugin(phys)
	var r runner
	r.lobby = lobby.NewPlugin(lobby.Config{})
	w.RegisterPlugin(r.lobby)
	w.RegisterPlugin(noopPlugin{})
}

func spawn(e cardinal.Entity) {
	e.Set(Health{})
	e.Set(physics2d.Transform{})
	e.Set(data.Manifest{})
	e.Set(lobby.Player{})
}
`,
	})

	// An earlier version credited plugins in map order, so repeat the run.
	for range 20 {
		out, code := runAudit(root)
		require.Equal(t, "- none", section(t, out, "Old API and hazards"))
		require.Equal(t, "- none", section(t, out, "Used but never registered"))
		require.Equal(t, 0, code)
	}
}

func TestPluginVariableCreditsOnlyItsPackage(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": goMod,
		"main.go": `package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

func register(w *cardinal.World) {
	w.RegisterComponent[Health]()
	dataPlugin := data.NewPlugin()
	w.RegisterPlugin(dataPlugin)
}

func spawn(e cardinal.Entity) {
	e.Set(Health{})
	e.Set(physics2d.Transform{})
	e.Set(data.Manifest{})
}
`,
	})

	out, code := runAudit(root)

	assert.Equal(t, "- component github.com/argus-labs/world-engine/pkg/plugin/physics2d.Transform (first use main.go:17)",
		section(t, out, "Used but never registered"))
	assert.Equal(t, 1, code)
}

func TestUnresolvedPluginVariableReported(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": goMod,
		"main.go": `package main

import "github.com/argus-labs/world-engine/pkg/cardinal"

func register(w *cardinal.World, plugin cardinal.Plugin) {
	w.RegisterComponent[Health]()
	w.RegisterPlugin(plugin)
}
`,
	})

	out, code := runAudit(root)

	assert.Equal(t, `-    1  w.RegisterPlugin(x) with x not traced to a plugin constructor: confirm which plugin
        main.go:7`, section(t, out, "Old API and hazards"))
	assert.Equal(t, 1, code)
}

func TestShardAuditReadsSharedDeclarations(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": goMod,
		"component/health.go": `package component

type Health struct{ HP int }

func (Health) Name() string { return "health" }
`,
		"shared/player.go": `package shared

import "example.com/game/component"

type Player struct{ Health component.Health }
`,
		"shards/arena/main.go": `package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"example.com/game/component"
	"example.com/game/shared"
)

func register(w *cardinal.World) {
	w.RegisterCommand[Attack]()
	w.Create[shared.Player]()
	w.Contains[component.Health]()
}
`,
	})

	out, code := runAudit(filepath.Join(root, "shards", "arena"))

	assert.Equal(t, `-    1  component passed where an archetype belongs: wrap it in struct{ C }
        main.go:13`, section(t, out, "Old API and hazards"))
	assert.Equal(t, "- component example.com/game/component.Health (first use main.go:12)",
		section(t, out, "Used but never registered"))
	assert.Equal(t, 1, code)
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, src := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(src), 0o600))
	}
	return root
}

func runAudit(root string) (string, int) {
	var out bytes.Buffer
	code := run(root, &out)
	return out.String(), code
}

// section returns the lines under the "## title" heading of the report.
func section(t *testing.T, out, title string) string {
	t.Helper()
	_, rest, ok := strings.Cut(out, "## "+title+"\n")
	require.True(t, ok, "no section %q in:\n%s", title, out)
	body, _, _ := strings.Cut(rest, "\n\n")
	return body
}
