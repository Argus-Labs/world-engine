package system

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data/component"
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
)

// State owns the data plugin's per-instance load state: registered kinds, the in-memory catalog,
// and the per-file manifest hashes currently reflected in that catalog.
//
// The plugin facade (data.Plugin) holds a pointer to a State and forwards every operation here.
// Splitting it out keeps plugin.go a thin facade (matching the lobby / physics2d convention) and
// keeps all the load-and-reconcile logic colocated with the components it operates on.
type State struct {
	loaders  map[string]kindLoader // jsonFile → loader
	catalog  map[string]Definition // Name() → loaded value
	manifest map[string]string     // jsonFile → hash currently reflected in catalog
}

// NewState builds an empty State. Call AddKind for each registered kind before LoadAll.
func NewState() *State {
	return &State{
		loaders:  map[string]kindLoader{},
		catalog:  map[string]Definition{},
		manifest: map[string]string{},
	}
}

// AssembleFunc materializes raw bytes into a fully-loaded Definition: json.Unmarshal → Resolve (if
// implemented) → Validate (if implemented). Used both at initial load and at reconcile re-fetches
// so a snapshot-pinned version goes through the same hooks as a fresh load.
//
// resolverSource is the Source the Resolver hook (if any) uses to fetch additional files. By
// convention this is always the local embed (never the operator), because Resolver-fetched files
// are heavy, designer-bundled, build-time assets that don't fit a "designer edits a row in a web
// UI" workflow.
type AssembleFunc func(ctx context.Context, resolverSource Source, raw []byte) (Definition, error)

// kindLoader is one entry in State.loaders, keyed by jsonFile: enough to assemble bytes into a
// loaded Definition and key the result by Name().
type kindLoader struct {
	name     string
	assemble AssembleFunc
}

// MakeAssemble returns the standard assemble function for kind T: json.Unmarshal into a fresh T,
// run Resolve on a pointer (so mutations stick), run Validate on the value. Errors from any step
// propagate to the caller (LoadAll and Reconcile both panic on them).
func MakeAssemble[T Definition]() AssembleFunc {
	return func(ctx context.Context, resolverSource Source, raw []byte) (Definition, error) {
		var def T
		if err := json.Unmarshal(raw, &def); err != nil {
			return nil, err
		}
		if r, ok := any(&def).(Resolver); ok {
			if err := r.Resolve(ctx, resolverSource); err != nil {
				return nil, err
			}
		}
		if v, ok := any(&def).(Validator); ok {
			if err := v.Validate(); err != nil {
				return nil, err
			}
		}
		return def, nil
	}
}

// AddKind enrolls a kind into the catalog. Panics on duplicate Name or JSONFile — both are wiring
// bugs that would otherwise silently collide.
func (s *State) AddKind(name, jsonFile string, assemble AssembleFunc) {
	if existing, ok := s.loaders[jsonFile]; ok {
		panic(eris.Errorf("data: JSON file %q already registered (by kind %q)", jsonFile, existing.name))
	}
	for _, l := range s.loaders {
		if l.name == name {
			panic(eris.Errorf("data: kind name %q already registered", name))
		}
	}
	s.loaders[jsonFile] = kindLoader{name: name, assemble: assemble}
}

// Get looks up a kind in the catalog by name. Returns (value, true) on hit; (nil, false) on miss.
// The generic data.Get[T] wraps this and panics on miss with a typed message.
func (s *State) Get(name string) (Definition, bool) {
	v, ok := s.catalog[name]
	return v, ok
}

// MustGet returns the loaded value for the kind keyed by name, panicking with a clear message if
// it was never registered. Used as the implementation of data.Get[T].
func (s *State) MustGet(name string) Definition {
	v, ok := s.catalog[name]
	if !ok {
		panic(fmt.Sprintf("data: kind %q not registered with this plugin", name))
	}
	return v
}

// LoadAll fetches every registered kind's JSON via primary, assembles it (handing resolverSource
// to any Resolver hook), and stores the result in the catalog. Records the per-file manifest
// hashes the primary source reported. Panics on any error — startup-time loading failures should
// be loud and immediate.
//
// primary is the data source for each kind's JSONFile() (embed in dev, operator in prod).
// resolverSource is what Resolver hooks fetch additional files through — always the local embed,
// never the operator (see AssembleFunc doc).
//
// Iterates jsonFile in sorted order so boot-time log output is reproducible across runs.
func (s *State) LoadAll(ctx context.Context, primary, resolverSource Source) {
	for _, file := range slices.Sorted(maps.Keys(s.loaders)) {
		l := s.loaders[file]
		raw, gotHash, err := primary.Fetch(ctx, file, "")
		if err != nil {
			panic(eris.Wrapf(err, "data: fetching %q", file))
		}
		def, err := l.assemble(ctx, resolverSource, raw)
		if err != nil {
			panic(eris.Wrapf(err, "data: loading %q", file))
		}
		s.catalog[l.name] = def
		s.manifest[file] = gotHash
	}
}

// ReconcileState is the system state for the data plugin's per-tick reconcile pass.
//
// The embedded Exact search holds the ConfigManifest singleton; declaring this field is also what
// registers ConfigManifest with Cardinal via system-field reflection.
type ReconcileState struct {
	cardinal.BaseSystemState
	Manifest cardinal.Exact[struct {
		Item cardinal.Ref[component.ConfigManifest]
	}]
}

// Reconcile is the data plugin's per-tick reconcile pass. Runs every PreUpdate (cardinal's
// earliest tick hook), fast-paths the steady state, and on a post-restore mismatch attempts to
// re-fetch each changed file at the snapshot's hash so the world keeps running on the config it
// was running before the restart.
//
// Lifecycle coverage with this one system:
//   - Fresh boot: ErrSingleNoResult → create ConfigManifest from s.manifest.
//   - Restart, same config: manifests match → no-op (one search + one map compare).
//   - Restart, restored snapshot whose config differs: re-fetch each changed file at the
//     snapshot's hash. Source can deliver → swap catalog atomically. Source errors (versioned
//     source lost a version) → panic. Source returns wrong-hash content (single-version source,
//     i.e. embed → rebuilt binary) → log a warning, keep current config, rewrite the component to
//     s.manifest so the next snapshot is self-consistent and the warning doesn't re-fire.
//   - cardinal.Reset()+Init() debug reload: wipes entities but Plugin.Register is not re-run, so
//     the catalog persists. Next tick's reconcile sees ErrSingleNoResult and re-creates the
//     ConfigManifest from s.manifest.
//   - Restore from a pre-feature snapshot (no ConfigManifest): ErrSingleNoResult → create.
//
// All-or-nothing: a versioned source delivers every needed historical file or it panics; a
// single-version source bails to the warn path on the first hash that doesn't match. No
// half-old/half-new catalog state.
//
// primary is the data source for each kind's JSONFile() re-fetch at the snapshot's hash.
// resolverSource is what Resolver hooks fetch additional files through (always local embed).
func (s *State) Reconcile(rs *ReconcileState, primary, resolverSource Source) {
	_, ent, err := rs.Manifest.Iter().Single()
	switch {
	case errors.Is(err, cardinal.ErrSingleNoResult):
		_, ent = rs.Manifest.Create()
		ent.Item.Set(component.ConfigManifest{Files: maps.Clone(s.manifest)})
		return
	case errors.Is(err, cardinal.ErrSingleMultipleResult):
		panic(eris.New("data: more than one config-manifest singleton"))
	case err != nil:
		panic(eris.Wrap(err, "data: querying config-manifest singleton"))
	}

	snap := ent.Item.Get().Files
	if maps.Equal(snap, s.manifest) {
		return
	}

	ctx := context.Background()
	temp := map[string]Definition{}
	for file, snapHash := range snap {
		if cur, ok := s.manifest[file]; ok && cur == snapHash {
			continue
		}
		loader, owned := s.loaders[file]
		if !owned {
			continue
		}
		raw, gotHash, fetchErr := primary.Fetch(ctx, file, snapHash)
		if fetchErr != nil {
			panic(eris.Wrapf(fetchErr, "data: source cannot serve %q at hash %s required by snapshot", file, snapHash))
		}
		if gotHash != snapHash {
			rs.Logger().Warn().
				Interface("snapshot", snap).
				Interface("current", s.manifest).
				Msg("data: config changed since snapshot; resuming on current config")
			ent.Item.Set(component.ConfigManifest{Files: maps.Clone(s.manifest)})
			return
		}
		def, assembleErr := loader.assemble(ctx, resolverSource, raw)
		if assembleErr != nil {
			panic(eris.Wrapf(assembleErr, "data: loading %q at hash %s", file, snapHash))
		}
		temp[loader.name] = def
	}

	maps.Copy(s.catalog, temp)
	for file, snapHash := range snap {
		if _, owned := s.loaders[file]; owned {
			s.manifest[file] = snapHash
		}
	}
	ent.Item.Set(component.ConfigManifest{Files: maps.Clone(s.manifest)})
}
