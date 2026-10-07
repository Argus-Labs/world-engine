package version

import (
	"runtime/debug"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// Nats is the pinned NATS server version; cli/pkg/k8s/charts/nats/values.yaml must agree.
const Nats = "2.12.2"

const worldEngineModule = "github.com/argus-labs/world-engine"

// devWorldEngine is the World Engine ref for builds without a release: templates are cloned from the
// main branch, and go mod tidy resolves a main requirement to its canonical version.
const devWorldEngine = "main"

// WorldEngine returns the World Engine release this binary was built from. The CLI ships in the
// world-engine module, so this is the CLI's own version and the release it scaffolds and generates
// code for. Builds from a checkout or a pseudo-version return "main".
func WorldEngine() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return devWorldEngine
	}
	return worldEngineFrom(info)
}

// worldEngineFrom finds the world-engine module in info. It is the main module when running the world
// command, and a dependency when another program (e.g. cardinal-editor) imports the CLI packages.
func worldEngineFrom(info *debug.BuildInfo) string {
	for _, mod := range append([]*debug.Module{&info.Main}, info.Deps...) {
		if mod.Path == worldEngineModule && semver.IsValid(mod.Version) && !module.IsPseudoVersion(mod.Version) {
			return mod.Version
		}
	}
	return devWorldEngine
}
