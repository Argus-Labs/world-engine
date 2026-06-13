package system

import "context"

// Definition is the contract a config kind must satisfy to be loaded by the data plugin.
//
// A Definition is whatever Go type the plugin unmarshals JSON into and stores in its in-memory
// catalog. The Name() value is the catalog key (game systems read via data.Get[T](plugin), which
// looks the value up under T's Name()). JSONFile() is the source-relative path the plugin hands
// to Source.Fetch at boot.
//
//	type AbilityData struct{ ID string; Cooldown float64 }
//	type Abilities struct{ Items []AbilityData }
//
//	func (Abilities) Name() string     { return "abilities" }
//	func (Abilities) JSONFile() string { return "abilities.json" }
//
// Implement Name() and JSONFile() on value receivers — the plugin instantiates a zero T to read
// them before any pointer is taken.
type Definition interface {
	// Name uniquely identifies this kind within the plugin's catalog (and, by convention,
	// matches Cardinal's component-name conventions: letters, digits, underscores).
	Name() string

	// JSONFile is the source-relative path Source.Fetch is called with at world init.
	JSONFile() string
}

// Singleton is an optional marker a Definition may implement to declare that its config is a single
// object rather than a collection of records. A singleton kind's source returns one JSON object
// (e.g. {"maxPlayers":8,"roundSeconds":90}) and its backing table holds at most one row, whereas a
// non-singleton kind's source returns a JSON array of records.
//
// SingleObject is a pure marker — it is never called. Implementing it (on a value receiver, like
// Name/JSONFile) is the entire signal.
type Singleton interface {
	SingleObject()
}

// Resolver is an optional interface a Definition may implement to perform a second-stage load
// after the primary JSON has been unmarshaled — typically to fetch additional designer-bundled
// files the JSON references (e.g. Tiled .tmj tilemaps that map_levels.json points at).
//
// The Source handed to Resolve is ALWAYS the local embedded filesystem, never the operator. The
// reasoning: Resolver-fetched files are heavy build-time assets that don't fit a "designer edits
// a row in a web UI" workflow — they change when someone commits a new .tmj, which is a deploy
// event, not a database write. They are also NOT version-pinned in the snapshot manifest; an
// in-flight game resuming after a redeploy will see the current embedded files.
//
// Resolve is called after Unmarshal and before the value is written to the catalog, so the
// receiver already has every field the JSON populated. Mutate the receiver in place. Returning
// an error panics — same failure mode as a fetch or unmarshal error.
//
// Implement on a pointer receiver so mutations stick.
type Resolver interface {
	Resolve(ctx context.Context, source Source) error
}

// Validator is an optional interface a Definition may implement to enforce post-load structural
// invariants — duplicate IDs, required fields, canonical enum values, cross-field rules, etc.
// The plugin calls Validate after Unmarshal (and after Resolve, if implemented), so the value
// seen by Validate is exactly what would be written to the catalog.
//
// Returning an error panics. This is the right home for checks that the old per-kind loader
// systems used to enforce: fail loud at boot rather than letting a malformed JSON file produce a
// quietly-broken runtime.
type Validator interface {
	Validate() error
}
