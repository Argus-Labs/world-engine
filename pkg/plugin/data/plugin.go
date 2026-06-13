// Package data is a Cardinal plugin that loads JSON configuration into a process-local catalog at
// world init.
//
// Each kind is a user-defined type implementing Definition (Name() + JSONFile()). The plugin
// fetches each file via a Source, unmarshals the bytes into the kind, and stores the result in an
// in-memory catalog keyed by Name(). Game systems read the data via:
//
//	mobs := data.Get[component.Mobs](plugin)
//
// Loaded data does not live on Cardinal components — only the catalog does. The single
// snapshot-resident artifact is a ConfigManifest component recording, per file, the content hash
// of what's currently loaded. On a snapshot restore the reconcile system reads the snapshot's
// manifest and re-fetches each changed file at the snapshot's hash (from a versioned operator
// source) so an in-flight game resumes on the config it was running. Where the source can't pin
// history (the embed: a rebuilt binary served new bytes for an old hash) the system logs a warning
// and keeps the current config; where it should be able to but can't (operator says "I don't have
// that version") the system panics — fail loud, ops problem.
//
// Usage:
//
//	dataPlugin := data.NewPlugin(data.Config{
//	    EmbeddedFS: myEmbeddedFS,
//	})
//	data.Register[component.Abilities](dataPlugin)
//	data.Register[component.Mobs](dataPlugin)
//	cardinal.RegisterPlugin(world, dataPlugin)
//
//	// Anywhere downstream:
//	abilities := data.Get[component.Abilities](dataPlugin)
//
// All Register[T] calls must happen before cardinal.RegisterPlugin so the plugin can load every
// kind into the catalog before the tick loop begins. Get[T] is valid immediately after
// cardinal.RegisterPlugin returns.
//
// The plugin picks the underlying Source based on the runtime environment (embedded files for
// local dev; operator/forge integration is a future addition that flips automatically when
// OPERATOR_ADDR is set). Shards do not select a source themselves — they hand the plugin their
// embedded data and the plugin handles the rest.
package data

import (
	"context"
	"embed"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data/component"
	"github.com/argus-labs/world-engine/pkg/plugin/data/system"
)

// Re-export user-facing types so callers only need to import the plugin root.
type (
	// Definition is the contract a config kind must satisfy to be loaded by the plugin.
	Definition = system.Definition

	// Resolver is an optional Definition interface for loading additional files from the same
	// Source after the primary JSON has been unmarshaled.
	Resolver = system.Resolver

	// Singleton is an optional Definition marker declaring the kind's config is a single object
	// rather than a collection — its source returns one JSON object instead of an array.
	Singleton = system.Singleton

	// Validator is an optional Definition interface for enforcing post-load invariants. A
	// non-nil error panics.
	Validator = system.Validator

	// Source supplies the raw bytes for a named file along with its content hash (embed, local
	// disk, operator, etc).
	Source = system.Source

	// EmbedSource serves files baked into the binary via go:embed.
	EmbedSource = system.EmbedSource

	// ConfigManifest is the snapshot-resident component recording, per file, the content hash of
	// the config the world is running. Internal to the plugin under normal use; re-exported for
	// tests and any tooling that needs to inspect it.
	ConfigManifest = component.ConfigManifest
)

// Config holds plugin options.
type Config struct {
	// EmbeddedFS is the build-time embedded filesystem used as the data source in local dev and as
	// a fallback once operator integration ships. Required unless Source is set.
	EmbeddedFS embed.FS

	// Source overrides automatic source selection. Tests and custom integrations set this;
	// production callers leave it nil and let the plugin pick based on the environment.
	Source Source
}

// Plugin implements cardinal.Plugin. The actual catalog, manifest, and reconcile logic live on
// the embedded *system.State; this struct is the thin facade users see (matches the lobby /
// physics2d convention).
type Plugin struct {
	config Config
	state  *system.State
}

var _ cardinal.Plugin = (*Plugin)(nil)

// NewPlugin builds a data plugin instance. Call Register[T] for each kind before passing the
// plugin to cardinal.RegisterPlugin.
//
// If config.Source is nil (the production path), the plugin picks a Source from the current
// environment via system.PickSource using config.EmbeddedFS as the embedded fallback.
func NewPlugin(config Config) *Plugin {
	if config.Source == nil {
		config.Source = system.PickSource(config.EmbeddedFS)
	}
	return &Plugin{config: config, state: system.NewState()}
}

// Source returns the configured Source so games can write custom loaders for kinds that need
// per-kind conditional logic (e.g. environment-gated content) and still go through the same
// dev/prod data path.
func (p *Plugin) Source() Source {
	return p.config.Source
}

// Register adds T to the plugin's load list. Each registered kind is fetched, unmarshaled, and
// (if applicable) run through Resolve and Validate at cardinal.RegisterPlugin time.
//
// Panics if another kind has already claimed the same Name() or JSONFile() — a wiring bug that
// would otherwise silently collide.
func Register[T Definition](p *Plugin) {
	var zero T
	p.state.AddKind(zero.Name(), zero.JSONFile(), system.MakeAssemble[T]())

	// Tell the source how to read this kind, when the source supports it. EmbedSource and the test
	// fakes don't (they key off the file path), so these are no-ops for them; a Postgres-backed
	// source records the file→table mapping and, for a Singleton kind, that the table is read as a
	// single object instead of an array.
	if r, ok := p.config.Source.(system.KindRegistrar); ok {
		r.RegisterKind(zero.JSONFile(), zero.Name())
	}
	if _, isSingleton := any(zero).(system.Singleton); isSingleton {
		if r, ok := p.config.Source.(system.SingletonRegistrar); ok {
			r.RegisterSingleton(zero.JSONFile())
		}
	}
}

// registered is the process-global plugin instance set by Plugin.Register(world). It lets
// data.Get[T]() resolve config without callers threading a *Plugin handle through every system.
// A shard is a single process running one Cardinal world, so one global is the right shape;
// tests that need multi-plugin isolation can use Plugin.GetT (instance-scoped, below).
//
//nolint:gochecknoglobals // Process-global plugin handle for data.Get[T]() consumer ergonomics.
var registered *Plugin

// Get returns the loaded value for kind T from the registered plugin. Call this from any game
// system after cardinal.RegisterPlugin(world, dataPlugin) has run:
//
//	mobs := data.Get[component.Mobs]()
//
// Panics if no plugin has been registered yet, or if T was never registered with that plugin —
// both are wiring bugs and should be caught loudly the first time any system reads.
func Get[T Definition]() T {
	if registered == nil {
		panic("data: no plugin registered — call cardinal.RegisterPlugin(world, dataPlugin) first")
	}
	var zero T
	v, ok := registered.state.MustGet(zero.Name()).(T)
	if !ok {
		// MakeAssemble[T] stores the concrete T in the catalog keyed by T's Name(), so this
		// branch is unreachable in practice — but the checked assertion satisfies errcheck and
		// fails loud if a future change ever breaks the invariant.
		panic("data: catalog entry for " + zero.Name() + " is not of type T (catalog key collided across kinds)")
	}
	return v
}

// Register implements cardinal.Plugin. Called synchronously by cardinal.RegisterPlugin, before
// StartGame. Loads every registered kind into the catalog, stashes the plugin as the
// process-global for data.Get[T](), and registers the per-tick reconcile system that keeps the
// catalog matched to whatever ConfigManifest the snapshot restored.
func (p *Plugin) Register(world *cardinal.World) {
	// Resolver hooks always go through the local embed regardless of how the primary source is
	// configured (operator, fake, etc.). Resolver-fetched files are heavy designer-bundled assets
	// — tilemaps, prefab manifests — that ship with the binary and aren't operator-editable.
	resolverSource := system.EmbedSource{FS: p.config.EmbeddedFS}
	p.state.LoadAll(context.Background(), p.config.Source, resolverSource)
	registered = p
	cardinal.RegisterSystem(world, func(rs *system.ReconcileState) {
		p.state.Reconcile(rs, p.config.Source, resolverSource)
	}, cardinal.WithHook(cardinal.PreUpdate))
}
