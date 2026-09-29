package version

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorldEngineFrom(t *testing.T) {
	t.Parallel()

	engine := func(v string) *debug.Module { return &debug.Module{Path: worldEngineModule, Version: v} }
	tests := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"world command at a release", &debug.BuildInfo{Main: *engine("v0.18.0")}, "v0.18.0"},
		{
			"imported by another program",
			&debug.BuildInfo{
				Main: debug.Module{Path: "github.com/argus-labs/monorepo", Version: "(devel)"},
				Deps: []*debug.Module{{Path: "golang.org/x/mod", Version: "v0.37.0"}, engine("v0.17.1")},
			},
			"v0.17.1",
		},
		{"built from a checkout", &debug.BuildInfo{Main: *engine("(devel)")}, "main"},
		{"built at a pseudo-version", &debug.BuildInfo{Main: *engine("v0.17.1-0.20260928000000-abcdef123456")}, "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, worldEngineFrom(tt.info))
		})
	}
}
