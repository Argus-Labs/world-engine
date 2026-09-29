// Package sdkgen reads command structs from a World Engine backend (Go source)
// and produces typed serde. Discovery is shared; each field is classified once
// into a structured model, from which two outputs are derived:
//
//   - Go (shard side): MarshalWire/UnmarshalWire and SizeWire/AppendWire methods on the command struct,
//     emitted over protowire (see emit_wire.go, emit_wire_direct.go). Keeps the hand-written struct as
//     the type; no separate generated type.
//   - C#/proto (client side): a .proto (EmitProto) compiled by buf+protoc in
//     Docker (see buf.go).
//
// Both speak the protobuf wire format with the same field numbers, so Go and C#
// are wire-compatible. Go types with no proto representation (time.Time,
// interface{}, ...) are reported as violations with the standard alternative.
package sdkgen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// Scalar describes a scalar/string/bytes leaf type.
type Scalar struct {
	Family string // varint | bool | i32 | i64 | string | bytes
	GoType string // Go type for the read-side cast, e.g. "int32"
	Proto  string // proto type token, e.g. "int64", "string", "double", "bytes"
	// Imports are the import specs GoType needs when it names a type from another package — a NAMED
	// scalar such as cardinal.EntityID or json.Number, where the decode-side cast must target the
	// declared type rather than its underlying basic. Recorded beside the spelling that requires them,
	// so the emitter registers the import at the point it writes the cast.
	Imports []string
}

// Ownership says who generates a type's wire code. It is the single fact the rest of the generator
// routes on, replacing three overlapping booleans that answered slightly different questions under
// similar names — one of which doubled as "do not generate this file" and, read for a local package,
// silently suppressed a backend's entire output.
//
// Exactly one state holds for any type, and every downstream decision follows from it: whether a .proto
// is generated or only imported, whether conversion is a method or a free function, and whose Go the
// wire file imports.
type Ownership uint8

const (
	// OwnLocal means this run generates the type's proto and its Go. Conversion is a method on the type.
	OwnLocal Ownership = iota
	// OwnImported means another module owns the type AND already generated for it. Its .proto is emitted only
	// so this schema resolves, stamped with their go_package so protoc-gen-go emits an import of their Go
	// type instead of defining a second one. Conversion is still a method — on the type they generated.
	OwnImported
	// OwnMirrored means another module owns the type and generated nothing, so there is nothing to import and
	// this run rebuilds the message itself. Go forbids declaring methods on a foreign type, so conversion
	// is a pair of free functions in the referencing package.
	OwnMirrored
)

// Foreign reports whether another module owns the type. Distinct from Mirrored: an imported type is
// foreign but is NOT rebuilt here, and conflating those is what let a local package be treated as one
// somebody else had already generated.
func (o Ownership) Foreign() bool { return o != OwnLocal }

// Mirrored reports whether this run rebuilds the type and emits free-function converters for it.
func (o Ownership) Mirrored() bool { return o == OwnMirrored }

func (o Ownership) String() string {
	switch o {
	case OwnLocal:
		return "local"
	case OwnImported:
		return "imported"
	case OwnMirrored:
		return "mirrored"
	}
	return "unknown"
}

// TypeRef is a reference to a message type from a place that has to name it — a field, a map value, or a
// message describing how others name it. Discovery resolves it once; nothing downstream re-derives any
// part of it.
//
// It exists because these facts are only correct together. The name, the qualified Go spelling, the
// import that spelling needs, and who generates the type used to be four parallel fields per reference,
// duplicated again for map values. Every bug in this area was two of them disagreeing.
type TypeRef struct {
	// Name is the bare type name, shared by the Go struct and the proto message — one flat proto
	// namespace per package, so nothing is qualified here.
	Name string
	// GoType is how to write the type where it is referenced: bare for a type in the referencing file's
	// own package, "<alias>.<Name>" otherwise. Import is the import spec GoType needs, empty when bare.
	GoType string
	Import string
	// PkgPath is the Go import path of the declaring package; Own says who generates its wire code.
	PkgPath string
	Own     Ownership
	// GenImport is the protobuf-generated package holding this type's .pb.go. Filled by Result.ResolveGen
	// once --go-out is known, so the emitter reads it rather than deriving a path per call site — and a
	// foreign type keeps the path resolved against ITS owner instead of the generating module.
	GenImport string
}

// Set reports whether the reference points at anything. The zero value is how a scalar field records
// "no message here".
func (r TypeRef) Set() bool { return r.Name != "" }

// Key identifies the referenced type across packages, matching how Discover keys mirrored messages.
func (r TypeRef) Key() string { return r.PkgPath + "." + r.Name }

// ConvFunc names the free-function converter for a mirrored type; dir is "ToProto" or "FromProto".
//
// Named after the source package's import alias rather than its leaf. The leaf alone collides the moment
// one file mirrors two packages that share it — "component" names a package in every plugin — and the
// collision lands as a redeclaration in the emitted file, since format.Source only parses. Reusing the
// alias makes the name unique for free: checkImportAliases already refuses a run whose aliases clash.
func (r TypeRef) ConvFunc(dir string) string {
	alias := AliasFromPath(r.PkgPath)
	if alias == "" {
		return "mirror" + r.Name + dir
	}
	return "mirror" + pascalUnderscored(alias) + r.Name + dir
}

// Field is one field on a message, classified once for both Go and proto output.
type Field struct {
	Name   string
	Number int

	Kind string // scalar | message | repeated | map

	Scalar   *Scalar // scalar leaf, or repeated-scalar element
	Optional bool    // scalar via pointer (*T) → proto3 `optional`; preserves nil/presence on the wire
	Pointer  bool    // single message via pointer

	// Msg is the message this field carries — a single message, a repeated element, or a fixed-array
	// element. Val is a map value's message. Both are unset for a scalar field. They stay separate
	// references rather than one, because map[string]foreign.T points at two independent types: the value
	// is foreign while the field itself is a map.
	Msg TypeRef
	Val TypeRef

	// Timestamp marks a time.Time field, mapped to the google.protobuf.Timestamp well-known type rather
	// than a generated message. It rides the message/repeated/map plumbing (Kind + Pointer) but swaps the
	// proto type token and the wire conversion (timestamppb.New / AsTime). For a map, the VALUE is the time.
	Timestamp bool

	Key   *Scalar // map key
	ValSc *Scalar // map value scalar (nil if value is a message)
	// ArrayDims holds the dimensions of a fixed-size Go array field, outermost first
	// ([4][8]Vec2 -> {4, 8}). The array flattens to a single repeated field of its base element type
	// in row-major order: the shape is a compile-time constant, so it belongs in the schema (as the
	// shape comment) rather than being re-encoded per message as wrapper rows. Empty for every other
	// field shape.
	ArrayDims []int64

	// Slice marks a world-engine immutable.Slice[T] field. On the wire it is what a []T would be — a
	// repeated field, or bytes for Slice[byte] — so proto emission reads Kind as usual. Only the Go side
	// differs: ToProto reads through All (or Clone), FromProto rebuilds with SliceOf, and the wire file
	// imports the declaring package under SliceImport (an import spec, see importSpecFor).
	Slice       bool
	SliceImport string
}

// ArrayCount is the total number of elements a fixed array flattens to, i.e. the product of its
// dimensions. Zero when the field is not a fixed array.
func (f Field) ArrayCount() int64 {
	if len(f.ArrayDims) == 0 {
		return 0
	}
	n := int64(1)
	for _, d := range f.ArrayDims {
		n *= d
	}
	return n
}

// ArrayShape renders the Go array shape for the generated proto comment, e.g. "[4][8]".
func (f Field) ArrayShape() string {
	var b strings.Builder
	for _, d := range f.ArrayDims {
		fmt.Fprintf(&b, "[%d]", d)
	}
	return b.String()
}

// Message is a top-level wire type (command/event/component/system event) or a nested struct.
type Message struct {
	// Own says who generates this message's wire code: this run, another module that already did, or
	// another module that did not, so this run rebuilds it. Every downstream branch reads this rather
	// than reconstructing it, which is what stops proto emission, Go emission and the buf invocation from
	// disagreeing about a type.
	Own Ownership

	// Self is how a file OTHER than this message's own package names and converts it. A non-local message
	// needs it because its converters are written into somebody else's package; for a local message the
	// name is bare there and Self carries that.
	Self TypeRef

	Name string // Go struct name
	Wire string // exact wire name from a static Name(); "" for nested or correlated types
	// Correlated types have a dynamic Name() (`field + "literal"`): they own only WireSuffix, joined at
	// runtime to a prefix from WireField (e.g. RequestID). Mutually exclusive with Wire. Both "" = nested.
	WireSuffix string
	WireField  string
	Kind       string   // primary role: a top-level kind if the type plays one, else KindNested ("")
	Kinds      []string // every role the type is wired under; Targets is the union over these (generate-once)
	Targets    []Target // codegen outputs (Go engine / C# client / …); union of each role's targets; passes filter on this
	Fields     []Field
	Dir        string // filesystem dir of the package this struct is defined in
	PkgName    string // package name of that package
	PkgPath    string // import path of that package (so the codec file can import it)

	// ProtoPkg is the proto package this message lands in — one per Go package, so same-named types in
	// different Go packages never collide. e.g. "rampage.shards.gameplay.event". No version suffix: this
	// system versions per-type via Name() ("move.v2") + append-only fields, so a package-level version
	// would be a redundant axis that never increments. ProtoFile is its .proto path (drives .pb.go landing).
	ProtoPkg  string
	ProtoFile string

	// CSNamespace is the client SDK's namespace for this message. Deliberately NOT derived from
	// ProtoPkg: the proto package has to be globally agreed so two modules name the same type
	// identically, while this only has to be unique and readable inside one client. Tying them
	// together would push the module path into every C# name for no benefit to the game developer.
	CSNamespace string

	// GenImport is the Go import path of the protobuf-generated package holding this message's .pb.go,
	// resolved once here so nothing downstream re-derives it. A foreign package resolves against ITS
	// owner's module, and re-deriving from the generating one named a path nobody had written.
	//
	// For OwnImported it is the owner's, and stamping that as go_package is what makes protoc-gen-go emit
	// an import of their type rather than a second copy. Otherwise it is this run's own output path.
	GenImport string
}

// Message kinds. Commands, events, components, and system events are top-level wire types (each gets the
// generated wire methods); a struct reached only as a nested field is KindNested (no wire name — just
// ToProto/FromProto, carried inside its parent's proto file).
const (
	KindNested      = ""
	KindCommand     = "command"
	KindEvent       = "event"
	KindComponent   = "component"
	KindSystemEvent = "system_event"
)

// Target is a codegen output.
type Target int

const (
	TargetGo     Target = iota // engine wire code (proto .pb.go + MarshalWire/UnmarshalWire + SizeWire/AppendWire)
	TargetCSharp               // Unity / .NET client SDK
)

// kindTargets is the single explicit policy for which outputs each kind is generated into. Client SDK
// languages appear only on kinds a client actually sends/receives; system events are engine-internal, so
// Go only. Adding a client language (e.g. TypeScript) is a visible, per-kind edit here.
//
//nolint:gochecknoglobals // read-only policy table
var kindTargets = map[string][]Target{
	KindCommand:     {TargetGo, TargetCSharp}, // client sends commands
	KindEvent:       {TargetGo, TargetCSharp}, // client receives events
	KindComponent:   {TargetGo, TargetCSharp}, // client reads component state (queries/introspect)
	KindSystemEvent: {TargetGo},               // in-process, system→system, same tick — no client sees it
	KindNested:      {TargetGo, TargetCSharp}, // rides its parent's proto file → include in whichever runs
}

// targetsFor returns a discovered type's codegen outputs: its kind's declared targets, minus Go for
// externally-defined types (whose engine code ships with the plugin, not here). Set once at discovery;
// the buf passes filter on Message.Targets instead of switching on Kind.
func targetsFor(kind string, local bool) []Target {
	t := kindTargets[kind]
	if !local {
		t = slices.DeleteFunc(slices.Clone(t), func(x Target) bool { return x == TargetGo })
	}
	return t
}

// Issue categories. Every one stops generation; the category picks the guidance shown (see
// categoryGuidance), so it names the KIND OF FIX the field needs rather than a severity.
//
// A category is named after the Go type that caused it, not after the reason it is a problem. A field
// has exactly one shape, so it produces exactly one finding — an earlier split across "why proto cannot
// express this" and "why ECS cannot store this" reported the same field twice under two names and
// inflated every count.
//
// The field-level set splits three ways: the value is not data at all (CatUnserializable), it is data
// reached by address (the four shapes), or it is data held inline that this generator does not carry
// (CatUnsupportedType).
const (
	CatDuplicateName     = "duplicate-name"      // same proto message name in two packages
	CatDuplicateWireName = "duplicate-wire-name" // same Name() wire string across two commands
	CatUnserializable    = "unserializable"      // chan/func/unsafe/method-interface — not data
	CatUndeclared        = "undeclared"          // passed to a cardinal generic, declared through no roster

	CatSlice     = "slice"     // []T, []byte, or an array of them
	CatMap       = "map"       // map[K]V
	CatPointer   = "pointer"   // *T, or an optional scalar
	CatInterface = "interface" // any / interface{}
	// CatUnsupportedType is data, held inline, that this generator has no mapping for: uintptr, complex.
	// Named for who is declining, not for protobuf's capability — proto has no multi-dimensional array
	// type either, and fixed arrays are carried anyway by flattening them (see classifyArray). A complex
	// could ride as a {re, im} message and a uintptr's bits fit a uint64; neither is carried because an
	// address means nothing on the far side and complex has no agreed cross-language form.
	CatUnsupportedType = "unsupported-type"
)

// Categories is every issue category the generator can report. Declared once so the guidance table and
// its coverage test cannot drift from the constants.
//
//nolint:gochecknoglobals // static list, read-only
var Categories = []string{
	CatDuplicateName, CatDuplicateWireName, CatUnserializable, CatUndeclared,
	CatSlice, CatMap, CatPointer, CatInterface, CatUnsupportedType,
}

// Guidance is the suggestion pair shown once per category group in the report: Quick is the fastest
// change that types (or unblocks) the field; Proper is the idiomatic solution.
type Guidance struct {
	Proper string
}

// categoryGuidance maps each issue category to its quick/proper suggestion.
//
//nolint:gochecknoglobals // static lookup table, read-only
var categoryGuidance = map[string]Guidance{
	CatDuplicateName: {
		Proper: "if they're the same concept, consolidate into one shared type both packages import; if genuinely different, rename",
	},
	CatDuplicateWireName: {
		Proper: "if it's one type defined twice, move it to a shared package both import; if they are genuinely different, give them distinct Name()s",
	},
	CatUndeclared: {
		Proper: "register it before StartGame with w.RegisterCommand/RegisterEvent/RegisterComponent/RegisterSystemEvent[T](). If it is already registered, this world-cli does not match your world-engine version",
	},
	CatUnserializable: {
		Proper: "replace it with something serializable — an ID or enum the other side uses to rebuild the behaviour; a component cannot hold one at all, since there is no value to copy when a snapshot is taken",
	},
	CatSlice: {
		Proper: "give it a bound the design can defend — [N]T generates a typed repeated field and [N]byte generates bytes, both stored inline, with the live length in its own field. The bound reaches the schema as a comment, not a validated constraint, so enforce it where the data enters. If the length is genuinely unbounded, hold it in immutable.Slice[T] (github.com/argus-labs/world-engine/pkg/immutable): it travels as the same repeated field, and nothing can write through it, so a column copy is safe. A location ending in [] is an immutable.Slice whose ELEMENT is the problem — the Slice is already the fix, and it cannot hold another unbounded shape, so the element is what needs the bound ([N]byte plus a length for a blob, [N]T plus a count otherwise). Anything bigger than the component belongs behind an ID the other side resolves.",
	},
	CatMap: {
		Proper: "if the key set is fixed and known, replace the map with a struct holding one field per key; otherwise a fixed array of entry structs plus a count. A proto map value cannot itself be a repeated or a map, so a collection value has to be wrapped in a named message either way — and a map key that is a float has no proto form at all.",
	},
	CatPointer: {
		Proper: "drop the star and store the value. If the pointer means absent, put presence in the data — a Has<Name> bool beside the value, or a sentinel the other side understands — rather than in an address.",
	},
	CatUnsupportedType: {
		Proper: "this generator has no wire mapping for the type. Replace it with something it does carry: a scalar, a string, or a named struct of those — a complex becomes two float64 fields, and an address has no meaning on the other side at all.",
	},
	CatInterface: {
		Proper: "replace it with the one concrete type it actually holds. If it genuinely varies, add an explicit tag field and one field per variant and select on the tag — the generator emits no oneof, so the variants have to be real fields.",
	},
}

// GuidanceFor returns the easy/proper suggestion pair for an issue category (zero value if unknown).
func GuidanceFor(category string) Guidance { return categoryGuidance[category] }

// Violation is a finding: a field, or a name collision, that stops generation. Every finding is one —
// there is no second tier, so a finding has exactly one consequence and carries nothing to qualify it.
type Violation struct {
	Category string
	Where    string
	GoType   string
	Reason   string
}

// LoadError reports type-check errors that block discovery and are NOT the expected bootstrap
// "missing wire method" errors (those are tolerated — the generator is about to create them). These are
// real problems in the backend the dev must fix before generation can read it; surfaced individually
// (not collapsed into one opaque wrap) so they're actionable.
type LoadError struct {
	Errors []string
}

func (e *LoadError) Error() string {
	return fmt.Sprintf("backend has %d type error(s) outside the generated wire layer; fix these "+
		"first:\n  - %s", len(e.Errors), strings.Join(e.Errors, "\n  - "))
}

// Result is the outcome of Discover.
type Result struct {
	Module string // Go module path of the scanned backend
	// OwnerModule maps a package's import path to the module that defines it. A foreign package's .proto
	// file is resolved against ITS owner, not the module being generated, so the owner and every consumer
	// name the same file for the same type.
	OwnerModule map[string]string
	Messages    []Message
	// ExternalMessages are top-level types (and their nested types) discovered in a dependency — e.g. a
	// plugin's commands. The dependency ships its own Go wire code, so we never regenerate Go for these;
	// but it ships no C#, so these are emitted C#-only into the consumer's namespace (see EmitProtos with
	// an empty goOutImport). Kept separate from Messages so the Go path is untouched.
	ExternalMessages []Message
	Violations       []Violation
	// Orphans are local types that carry a Name() badge but are declared nowhere — they look like wire
	// types but were never registered (World.RegisterX[T]()) nor reached as a nested field, and
	// nothing hands them to cardinal, so they're omitted from generation. Surfaced as a report warning.
	Orphans []string
}

// CommandCount returns the number of top-level commands discovered.
func (r Result) CommandCount() int {
	return r.countKind(KindCommand)
}

// EventCount returns the number of top-level events discovered.
func (r Result) EventCount() int {
	return r.countKind(KindEvent)
}

// ComponentCount returns the number of top-level components discovered.
func (r Result) ComponentCount() int {
	return r.countKind(KindComponent)
}

// SystemEventCount returns the number of top-level system events discovered.
func (r Result) SystemEventCount() int {
	return r.countKind(KindSystemEvent)
}

func (r Result) countKind(kind string) int {
	n := 0
	for _, m := range r.Messages {
		if slices.Contains(m.Kinds, kind) {
			n++
		}
	}
	return n
}

// WireFileName is the per-package wire file RenderGoWire emits.
const WireFileName = "wire.gen.go"

// wireMethods are the generated methods the engine's Serializable and Component constraints require,
// and so the ones a type-check without wire.gen.go reports as missing (see Discover).
//
//nolint:gochecknoglobals // static lookup table, read-only
var wireMethods = []string{"MarshalWire", "UnmarshalWire", "SizeWire", "AppendWire"}

// wireMethodAbsent is go/types' two phrasings for a method that does not exist yet: "missing method X"
// when a type fails an interface, "has no field or method X" when one is called on a value.
var wireMethodAbsent = regexp.MustCompile(`(?:missing method|has no field or method) (\w+)`)

// isWireMethodError reports whether a load error is about a missing generated wire method — go/types
// names one missing method per diagnostic, whichever it finds first, so every one of them must match.
// Only the absence phrasings: a wrong signature or an undefined name still surfaces as a real error.
func isWireMethodError(msg string) bool {
	m := wireMethodAbsent.FindStringSubmatch(msg)
	return m != nil && slices.Contains(wireMethods, m[1])
}

// WireBuildTag hides the generated wire layer from discovery. RenderGoWire stamps every wire.gen.go
// with `//go:build !<tag>`; Discover loads with `-tags=<tag>`, so the file is dropped before it is
// parsed. Normal `go build` passes no tags and compiles it as usual.
//
// A generator must not parse its own output: that output describes the PREVIOUS shape of the source, so
// a stale (or unparseable) wire file aborts discovery over code this same run is about to overwrite.
// Same reason google/wire tags its generated wire_gen.go `!wireinject`.
const WireBuildTag = "sdkgen"

// Discover loads packages under dir, finds the wire types registered through cardinal's World.RegisterX[T](),
// transitively collects nested message types, classifies fields, and lints.
func Discover(dir string) (Result, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps | packages.NeedModule,
		Dir:        dir,
		BuildFlags: []string{"-tags=" + WireBuildTag},
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return Result{}, fmt.Errorf("load packages under %s: %w", dir, err)
	}
	var loadErrs []string
	for _, p := range pkgs {
		for _, e := range p.Errors {
			// Wire-method tolerance. WireBuildTag drops every wire.gen.go from this load, so the wire
			// methods the engine's Serializable constraints require are absent on EVERY run, not just the
			// first — these errors are the expected steady state here, and self-resolving, since we are
			// about to generate exactly those methods. Any OTHER type error is real: it's collected and
			// reported individually (not swallowed) so the dev can see and fix the backend.
			//
			// Matching on message text is the weak point: it is now load-bearing for all generation, so a
			// reworded go/types diagnostic would break every backend at once, and a genuine error whose
			// text mentions these methods is invisible. Emitting no-op stubs under `//go:build <tag>` (as
			// google/wire does) would let this shrink back to nothing — worth doing if it ever bites.
			if isWireMethodError(e.Msg) {
				continue
			}
			// Migration backstop. WireBuildTag only hides a wire.gen.go that already carries the tag, so
			// the first run after upgrading still parses the previously-committed, untagged ones — the
			// very deadlock this is meant to end. Ignoring errors reported inside that file covers them
			// until one successful run stamps the tag. Only positioned errors: a file mangled badly
			// enough to fail parsing also yields position-less cascades (`POS="-"`), which this cannot
			// catch — the tag handles those from the second run on.
			if strings.Contains(e.Pos, WireFileName) {
				continue
			}
			loadErrs = append(loadErrs, fmt.Sprintf("%s: %s", p.PkgPath, e.Msg))
		}
	}
	if len(loadErrs) > 0 {
		slices.Sort(loadErrs)
		return Result{}, &LoadError{Errors: slices.Compact(loadErrs)}
	}

	var all []*packages.Package
	packages.Visit(pkgs, func(p *packages.Package) bool { all = append(all, p); return true }, nil)

	// Roots loaded from "./..." are the packages this run was asked to generate; dependencies are only
	// reachable transitively. We walk the full graph to resolve types and to find registrations, but
	// only emit wire code for what this run owns — a type another run generates already has its own
	// wire.gen.go, and regenerating it here would rewrite that file to import THIS run's gen package.
	local := make(map[string]bool, len(pkgs))
	for _, p := range pkgs {
		local[p.PkgPath] = true
	}

	// Which module defines each package, so a foreign one is resolved against ITS owner rather than the
	// module being generated.
	ownerModule := map[string]string{}
	mod, _ := moduleOf(dir)

	d := &discoverer{
		wireNames:   map[*types.Named]string{},
		wireCorr:    map[*types.Named]correlatedWire{},
		seenKind:    map[*types.Named]string{},
		queued:      map[*types.Named]bool{},
		kinds:       map[*types.Named][]string{},
		pkgDir:      map[*types.Package]string{},
		local:       local,
		ownerModule: ownerModule,
		genPkgCache: map[*types.Package]string{},
		module:      mod,
	}
	for _, p := range all {
		if p.Types != nil && len(p.GoFiles) > 0 {
			d.pkgDir[p.Types] = filepath.Dir(p.GoFiles[0])
		}
		if p.Module != nil && p.Module.Path != "" {
			ownerModule[p.PkgPath] = p.Module.Path
		}
	}
	collectWireNames(all, d.wireNames, d.wireCorr)
	d.discoverRosters(all, local)

	res := Result{OwnerModule: ownerModule}
	// The module path anchors each message's proto package (one per Go dir). Best-effort: if go.mod
	// can't be found, ProtoPkg is left empty and callers fall back to the single-package layout.
	res.Module = mod
	for len(d.queue) > 0 {
		q := d.queue[0]
		d.queue = d.queue[1:]
		st, ok := q.named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		res.Messages = append(res.Messages, d.buildMessage(q.named, st, q.kind, &res))
	}
	d.finalizeMessages(all, local, &res)
	d.reportIssues(&res)
	d.reportOrphans(all, &res)
	sortFindings(&res)
	return res, nil
}

// sortFindings orders findings by category then field path. Messages are built off a queue seeded
// from map iteration, so findings are appended in a different order every run — leaving the report
// unable to be diffed against a previous run, which is the only way to see what a change introduced.
func sortFindings(res *Result) {
	sort.SliceStable(res.Violations, func(i, j int) bool {
		a, b := res.Violations[i], res.Violations[j]
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Where < b.Where
	})
}

// finalizeMessages fills ProtoPkg/ProtoFile and sorts the local messages, and mirrors the dependency
// (plugin) types into res.ExternalMessages via buildExternalMessages.
func (d *discoverer) finalizeMessages(all []*packages.Package, local map[string]bool, res *Result) {
	d.buildExternalMessages(all, local, res)
	if res.Module != "" {
		for i := range res.Messages {
			res.Messages[i].ProtoPkg, res.Messages[i].ProtoFile = ProtoPackageOf(
				res.ownerOf(res.Messages[i].PkgPath), res.Messages[i].PkgPath)
			res.Messages[i].CSNamespace = CSNamespaceOf(res.Module, res.Messages[i].PkgPath)
		}
	}
	sort.Slice(res.Messages, func(i, j int) bool { return res.Messages[i].Name < res.Messages[j].Name })
}

// buildExternalMessages runs the C#-only 2nd pass for the dependency (plugin) top-level types recorded in
// pass 1: it re-discovers them treating the dependency packages as "local" (so their types are generatable),
// then fills their ProtoPkg and sorts. Go is never emitted for these — the plugin ships its own; only
// res.ExternalMessages (→ EmitProtos with an empty goOutImport) consumes them.
func (d *discoverer) buildExternalMessages(all []*packages.Package, local map[string]bool, res *Result) {
	if len(d.externalTop) == 0 {
		return
	}
	extLocal := make(map[string]bool)
	for _, p := range all {
		if p.Types != nil && !local[p.PkgPath] {
			extLocal[p.PkgPath] = true
		}
	}
	d2 := &discoverer{
		wireNames:   d.wireNames,
		seenKind:    map[*types.Named]string{},
		queued:      map[*types.Named]bool{},
		kinds:       map[*types.Named][]string{},
		pkgDir:      d.pkgDir,
		local:       extLocal,
		ownerModule: d.ownerModule,
		module:      d.module,
		genPkgCache: d.genPkgCache,
	}
	for _, q := range d.externalTop {
		d2.enqueue(q.named, q.kind)
	}
	for len(d2.queue) > 0 {
		q := d2.queue[0]
		d2.queue = d2.queue[1:]
		st, ok := q.named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		m := d2.buildMessage(q.named, st, q.kind, res)
		// Ownership comes from the ORIGINAL discoverer, not d2. d2 marks the dependency's own packages
		// local so it will build messages for them at all, which makes buildMessage skip the OwnImported
		// branch and leave GenImport empty. ResolveGen fills an empty one with this run's --go-out, and it
		// fills byKey from ExternalMessages last — so for a type reached BOTH as a local field and as an
		// external top-level type, the empty copy would overwrite the owner path the field copy carries,
		// and the wire file would import a package nothing writes.
		if d.ownershipOf(q.named) == OwnImported {
			m.GenImport = d.ownerGenPackage(q.named.Obj().Pkg())
			m.Self.GenImport = m.GenImport
		}
		res.ExternalMessages = append(res.ExternalMessages, m)
	}
	if res.Module != "" {
		for i := range res.ExternalMessages {
			// ownerOf, matching finalizeMessages. Resolving against res.Module here gave the same package
			// two different .proto paths when it was reached both as a local field and as an external
			// top-level type — one proto package in two files, which protoc rejects as a redefinition.
			res.ExternalMessages[i].ProtoPkg, res.ExternalMessages[i].ProtoFile = ProtoPackageOf(
				res.ownerOf(res.ExternalMessages[i].PkgPath), res.ExternalMessages[i].PkgPath)
			res.ExternalMessages[i].CSNamespace = CSNamespaceOf(res.Module, res.ExternalMessages[i].PkgPath)
		}
	}
	sort.Slice(
		res.ExternalMessages,
		func(i, j int) bool { return res.ExternalMessages[i].Name < res.ExternalMessages[j].Name },
	)
}

// reportIssues runs the post-discovery integrity checks that can only be decided once every message is
// known: duplicate proto/wire names. (A type wired under several roles is no longer a clash — it is
// generated once with the union of its roles' targets; see enqueue/unionTargets.)
func (d *discoverer) reportIssues(res *Result) {
	checkDuplicates(res)
}

// reportOrphans flags local types that carry a Name() badge (so they look like wire types) but no roster
// declared and no field reached — they'd be silently omitted from generation. The rosters are what
// discover a type; this cross-checks them against the badge to catch a forgotten registration.
//
// An orphan the source still hands to a cardinal generic (w.Commands[T](), Entity.Get[T]()) is a blocking
// finding instead: the engine uses it as a wire type, so generating without its wire code breaks the
// build. That means the type was never registered, or this generator predates the API the backend
// declares it with. The rosters match API names, so a renamed API lands here instead of silently deleting
// the type's generated code.
func (d *discoverer) reportOrphans(all []*packages.Package, res *Result) {
	used := d.cardinalUses(all)
	for n, wire := range d.wireNames {
		if _, discovered := d.seenKind[n]; discovered {
			continue // declared (top-level) or reached as a nested field
		}
		pkg := n.Obj().Pkg()
		if pkg == nil || !d.local[pkg.Path()] {
			continue // only this run's own types — a dependency's undeclared type isn't its concern
		}
		if generics := used[n]; len(generics) > 0 {
			name := pkg.Name() + "." + n.Obj().Name()
			res.Violations = append(res.Violations, Violation{
				Category: CatUndeclared,
				Where:    name,
				GoType:   strings.Join(generics, ", "),
				Reason: fmt.Sprintf("used through %s but declared through no API this generator recognizes, "+
					"so its wire code would not be generated", strings.Join(generics, ", ")),
			})
			continue
		}
		res.Orphans = append(res.Orphans, fmt.Sprintf("%s (Name()=%q)", n.Obj().Name(), wire))
	}
	slices.Sort(res.Orphans) // map iteration is random; keep the report stable
}

// cardinalUses maps each named type the local source instantiates a cardinal generic with to those
// generics' qualified names, sorted and deduplicated.
func (d *discoverer) cardinalUses(all []*packages.Package) map[*types.Named][]string {
	uses := map[*types.Named][]string{}
	for _, p := range all {
		if p.TypesInfo == nil || !d.local[p.PkgPath] {
			continue
		}
		for id, inst := range p.TypesInfo.Instances {
			o := cardinalGeneric(p.TypesInfo, id, inst)
			if o == nil {
				continue
			}
			for t := range inst.TypeArgs.Types() {
				if n, ok := types.Unalias(t).(*types.Named); ok {
					uses[n] = append(uses[n], o.Pkg().Name()+"."+o.Name())
				}
			}
		}
	}
	for n, generics := range uses {
		slices.Sort(generics)
		uses[n] = slices.Compact(generics)
	}
	return uses
}

// checkDuplicates surfaces the two collisions the generator can't otherwise express, each of which would
// otherwise blow up later as a cryptic error. They key on different things, so neither subsumes the other:
//   - two messages with the same proto full name (ProtoPkg + Name) — protoc reports "symbol already
//     defined". Keying on the proto package (one per Go dir) means same-named types in *different* Go
//     packages no longer collide; only a genuine same-package/same-name clash (or a sanitizer collapse)
//     trips it. Falls back to a bare-name check when ProtoPkg is empty (single-package fallback).
//   - same Name() wire string across two command structs — cardinal routes commands by name (one catalog
//     slot per name), so two different command types sharing a Name() collide on that slot at runtime: the
//     second silently rides the first's queue. Events dispatch by type, so a shared event name is a routing
//     concern, not a silent collision.
func checkDuplicates(res *Result) {
	byName := map[string][]string{}
	byWire := map[string][]string{}
	for _, m := range res.Messages {
		byName[m.ProtoPkg+"."+m.Name] = append(byName[m.ProtoPkg+"."+m.Name], m.PkgPath)
		if m.Wire != "" && slices.Contains(m.Kinds, KindCommand) {
			byWire[m.Wire] = append(byWire[m.Wire], m.Name)
		}
	}
	// Two passes, not one:
	//   - pass 1 tallies every message into byName/byWire, counting all of them — a duplicate can't be
	//     confirmed until the end, since the match could be the last message.
	//   - pass 2 must run after pass 1 because it reads those now-complete tallies (len >= 2 is only valid
	//     once counting is done). It walks res.Messages (sorted by name in Discover), not the maps, for a
	//     stable violation order (map iteration is random); nil-ing the bucket reports each group once.
	for _, m := range res.Messages {
		nameKey := m.ProtoPkg + "." + m.Name // must match the store key above, or the check never fires
		if pkgs := byName[nameKey]; len(pkgs) >= 2 {
			byName[nameKey] = nil // emit one violation per duplicated name, not per occurrence
			res.Violations = append(res.Violations, Violation{
				Category: CatDuplicateName,
				Where:    m.Name,
				GoType:   m.Name,
				Reason: fmt.Sprintf("defined in %d packages (%s) with the same name — proto message names are a "+
					"flat namespace, so this collides", len(pkgs), strings.Join(pkgs, ", ")),
			})
		}
		if m.Wire == "" {
			continue
		}
		if names := byWire[m.Wire]; len(names) >= 2 {
			byWire[m.Wire] = nil // emit one violation per duplicated wire name
			res.Violations = append(res.Violations, Violation{
				Category: CatDuplicateWireName,
				Where:    m.Wire,
				GoType:   strings.Join(names, ", "),
				Reason: fmt.Sprintf("wire name %q is returned by %d command types (%s) — cardinal routes commands "+
					"by name (one catalog slot per name), so these collide on that slot at runtime", m.Wire, len(names), strings.Join(names, ", ")),
			})
		}
	}
}

type queued struct {
	named *types.Named
	kind  string // KindCommand, KindEvent, or KindNested
}

// correlatedWire is the parsed form of a dynamic Name() like `return r.RequestID + "_get_map_result"`:
// a runtime correlation prefix (Field) joined to a static Suffix. The type owns only the suffix; the
// prefix is a per-request value, so these can't be exact-matched like a static Name() — a consumer
// resolves an incoming event name by matching its trailing Suffix.
type correlatedWire struct {
	Suffix string // the static literal, incl. its leading separator, e.g. "_get_map_result"
	Field  string // the receiver field supplying the runtime prefix, e.g. "RequestID" or "LobbyID"
}

type discoverer struct {
	wireNames map[*types.Named]string
	wireCorr  map[*types.Named]correlatedWire // types whose Name() is `field + "literal"` (dynamic/correlated)
	seenKind  map[*types.Named]string         // first role each type was enqueued under (presence = seen)
	// queued tracks membership of d.queue. Separate from seenKind because a type's ROLES and the
	// work queue answer different questions: a foreign type wired top-level is recorded as seen but
	// deliberately not queued, and a later field sighting still has to queue it.
	queued map[*types.Named]bool
	kinds  map[*types.Named][]string // every role each type is wired under (deduped); Targets unions over these
	pkgDir map[*types.Package]string
	// ownerModule maps an import path to the module defining it, so a foreign package can be resolved
	// against ITS owner rather than the module being generated. Same data as Result.OwnerModule.
	ownerModule map[string]string
	// genPkgCache memoizes ownerGenPackage. It is called once per named struct field via isMirrored, and
	// every call would otherwise re-read the same handful of wire.gen.go files from disk.
	genPkgCache map[*types.Package]string
	module      string          // the module being generated, used when the loader reported no owner
	local       map[string]bool // package paths in the local module (the only ones we emit Go for)
	queue       []queued
	externalTop []queued       // dependency (plugin) top-level types — mirrored for a C#-only pass
	ownerPkg    *types.Package // package of the message currently being built (for same/cross-package scope)
}

// discoverRosters enqueues every wire type a module registers on cardinal's World: commands
// (RegisterCommand, plus send-only ones via SendToShard), events (RegisterEvent), components
// (RegisterComponent), and system events (RegisterSystemEvent). A type reached under several rosters is
// enqueued once and unions its targets (see enqueue). Requires collectWireNames to have run first.
func (d *discoverer) discoverRosters(all []*packages.Package, local map[string]bool) {
	// A command must be discovered by the module that DEFINES it (only that module generates+ships its wire
	// code). A module reveals its commands two ways: RegisterCommand[T] (it receives T) and SendToShard(T)
	// (it sends T). Scanning only the first misses send-only commands (e.g. the lobby plugin sends
	// NotifySessionStart but never receives it), so we scan both. enqueue's local filter still scopes
	// emission to this module, so sending a dependency's command here doesn't regenerate it.
	cmdTypes := discoverGenericInstances(all, "RegisterCommand")
	for cmd := range discoverSentCommandTypes(all, local, d.wireNames) {
		cmdTypes[cmd] = true
	}
	for cmd := range cmdTypes {
		d.enqueue(cmd, KindCommand)
	}
	for ev := range discoverGenericInstances(all, "RegisterEvent") {
		d.enqueue(ev, KindEvent)
	}
	// Components are serialized for snapshots as proto, so they get the same wire methods (MarshalWire +
	// UnmarshalWire).
	for c := range discoverGenericInstances(all, "RegisterComponent") {
		d.enqueue(c, KindComponent)
	}
	// System events are consumed in-process, but implement the same wire interface as events so they can be
	// handled uniformly (e.g. streamed as typed values to a telemetry/debug service).
	for se := range discoverGenericInstances(all, "RegisterSystemEvent") {
		d.enqueue(se, KindSystemEvent)
	}
}

func (d *discoverer) enqueue(n *types.Named, kind string) {
	if n == nil {
		return
	}
	if _, ok := d.seenKind[n]; ok {
		// Already seen. Record this extra role so its targets union in: a type can be wired under several
		// roles (e.g. RegisterEvent[T] and RegisterSystemEvent[T]) and is generated once, carrying the union
		// of every role's targets. Not a clash — every wire kind emits the same methods; kind only tunes the
		// output target. A KindNested sighting adds no top-level role.
		d.addKind(n, kind)
		// Seen is not the same as queued. A foreign type wired under a top-level role is handed to the
		// C#-only pass and put on no build queue; reaching it again as a FIELD makes it data this schema
		// carries, so it has to be built here too. Returning outright queued it for neither, and the file
		// still called its converter — an undefined symbol, plus a proto import of a file nothing wrote.
		if kind == KindNested && !d.queued[n] {
			d.queued[n] = true
			d.queue = append(d.queue, queued{named: n, kind: kind})
		}
		return
	}
	if _, ok := n.Underlying().(*types.Struct); !ok {
		return
	}
	d.seenKind[n] = kind
	d.addKind(n, kind)
	pkg := n.Obj().Pkg()
	if pkg == nil {
		return
	}
	if !d.local[pkg.Path()] {
		switch kind {
		case KindNested:
			// Reached as a field, so it is data this schema carries. Rebuild it here — the walk continues
			// into its own fields to whatever depth they go, which terminates because Go forbids import
			// cycles. Go emission is filtered later by Message.Targets, so no file is written into the
			// owner's directory.
			d.queued[n] = true
			d.queue = append(d.queue, queued{named: n, kind: kind})
		default:
			// A top-level command/event/component someone else owns. Its Go ships with them, but nobody
			// ships C#, so it is recorded for the C#-only second pass (see Discover), which pulls in its
			// nested types too.
			d.externalTop = append(d.externalTop, queued{named: n, kind: kind})
		}
		return
	}
	d.queued[n] = true
	d.queue = append(d.queue, queued{named: n, kind: kind})
}

// addKind records a role for a type (deduped). Targets is the union over every recorded role.
func (d *discoverer) addKind(n *types.Named, kind string) {
	if slices.Contains(d.kinds[n], kind) {
		return
	}
	d.kinds[n] = append(d.kinds[n], kind)
}

// primaryKind returns a top-level kind if the type plays one (all top-level kinds emit the same wire
// methods, so any works), else KindNested. It decides whether MarshalWire is emitted.
func primaryKind(kinds []string) string {
	for _, k := range kinds {
		if k != KindNested {
			return k
		}
	}
	return KindNested
}

// unionTargets is the set union of every role's targets — a type wired under several roles is generated
// once, reaching every output any role needs (e.g. event {Go,C#} ∪ system-event {Go} = {Go,C#}).
func unionTargets(kinds []string, local bool) []Target {
	seen := map[Target]bool{}
	var out []Target
	for _, k := range kinds {
		for _, t := range targetsFor(k, local) {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

func (d *discoverer) buildMessage(nt *types.Named, st *types.Struct, _ string, res *Result) Message {
	kinds := d.kinds[nt]
	m := Message{Name: nt.Obj().Name(), Kind: primaryKind(kinds), Kinds: kinds}
	d.ownerPkg = nt.Obj().Pkg()
	if pkg := nt.Obj().Pkg(); pkg != nil {
		m.PkgName = pkg.Name()
		m.PkgPath = pkg.Path()
		m.Dir = d.pkgDir[pkg]
	}
	// One ownership decision for the message, from the same function every field reference uses. Note it
	// answers "foreign?" from d.local before anything else, so a message is never mistaken for local
	// merely because it is the package currently being built.
	m.Own = d.ownershipOf(nt)
	// Every discovered message has an output, so anything with a package gets its roles' targets.
	m.Targets = unionTargets(kinds, m.PkgPath != "")
	if m.Own == OwnImported {
		// The owner's gen package is recorded in their wire.gen.go and is the only place their --go-out
		// choice exists, so it is read here rather than derived. Local and mirrored messages land in this
		// run's own output instead, which is not known until --go-out arrives (see Result.ResolveGen).
		m.GenImport = d.ownerGenPackage(nt.Obj().Pkg())
	}
	m.Self = TypeRef{Name: m.Name, GoType: m.Name, PkgPath: m.PkgPath, Own: m.Own}
	if m.Own.Foreign() {
		// A foreign type still needs Go, emitted into the REFERENCING package rather than the owner's
		// directory — so Dir stays empty and nothing is written next to somebody else's source, and the
		// type has to be named qualified wherever it appears.
		m.Dir = ""
		if pkg := nt.Obj().Pkg(); pkg != nil {
			m.Self.GoType = pkgAlias(pkg) + "." + m.Name
			m.Self.Import = importSpecFor(pkg)
		}
	}
	if m.Kind != KindNested {
		m.Wire = d.wireNames[nt]
		if cw, ok := d.wireCorr[nt]; ok {
			m.WireSuffix = cw.Suffix
			m.WireField = cw.Field
		}
	}
	b := &msgBuilder{d: d, m: &m, res: res}
	b.addFields(st)
	return m
}

// msgBuilder accumulates a message's fields as it walks a struct (and its embedded structs), tracking
// field numbering and name collisions across the flattened set.
type msgBuilder struct {
	d   *discoverer
	m   *Message
	res *Result
	num int
}

// report records a finding against a field. Whether it stops generation is decided by the severity
// table rather than here, so a category can be raised without touching any of the places that detect
// it.
func (b *msgBuilder) report(category, field, gotype, reason string) {
	b.res.Violations = append(
		b.res.Violations,
		Violation{Category: category, Where: b.m.Name + "." + field, GoType: gotype, Reason: reason},
	)
}

// addFields walks a struct's exported fields. Embedded named structs remain nested sub-messages,
// matching the Go structure used by ToProto and FromProto.
func (b *msgBuilder) addFields(s *types.Struct) {
	for f := range s.Fields() {
		if !f.Exported() {
			continue // unexported: no cross-package access
		}
		// Embedded fields aren't special-cased: an embedded named struct flows through classify like any
		// other field (f.Name() is the type name), so it nests as a sub-message — ToProto/FromProto follow
		// the Go structure. No flattening.
		b.num++
		fld, err := b.d.classify(f.Type())
		if err != nil {
			// Refused. The classifier's own message says what is wrong; the category picks which fix to
			// offer, and it comes from the type rather than from the error text.
			where, gotype := findingLocation(f)
			b.report(refusedCategory(f.Type()), where, gotype, err.Error())
			continue
		}
		fld.Name = f.Name()
		fld.Number = b.num
		// One finding per field, named after the shape that caused it — reporting the encoding problem and
		// the storage problem as separate findings listed the same field twice under two names, for one
		// defect with one fix.
		//
		// This field CLASSIFIED: it maps to a real proto type. The finding is only that its shape is
		// reached by address, so an ECS column cannot hold it.
		if cat := shapeCategory(f.Type()); cat != "" {
			where, gotype := findingLocation(f)
			b.report(cat, where, gotype, shapeReason(cat))
		}
		b.m.Fields = append(b.m.Fields, fld)
	}
}

// findingLocation names what a finding is actually about: the field, or — when the field is an
// immutable.Slice — its element.
//
// A finding on a Slice field is never about the Slice, which is the fix rather than the problem. It is
// the element that is reached by address. The report prints the location and the Go type but never the
// reason, so this is the only place that distinction can reach a developer: without it they are told
// to hold an unbounded list in an immutable.Slice while already holding it in one.
// Returns the location to report and the Go type to print, in that order.
func findingLocation(f *types.Var) (string, string) {
	if elem, _, ok := engineSliceElem(f.Type()); ok {
		return f.Name() + "[]", elem.String()
	}
	return f.Name(), f.Type().String()
}

// nestedScope is where a struct type referenced by a command field is defined, relative to the command.
// It is the ONE decision that determines how a nested type is handled — read scopeOf to see the cases,
// and resolveNested for the rule each one implies. (Commands owned by another module are handled
// earlier, in enqueue: they're skipped so the owning module's committed wire code is used as-is.)
type nestedScope int

const (
	// scopeSamePackage: same package as the command. Converter named unqualified (bare type names).
	scopeSamePackage nestedScope = iota
	// scopeCrossPackage: another package in the SAME module. Still generated; its converter is reached
	// via exported methods (no qualification needed), and the wire file imports its package so decode-
	// side var declarations can name the type.
	scopeCrossPackage
	// scopeExternal: another MODULE. We can't generate it into this proto, so it's a violation — the
	// command must use a local type (flatten the fields, or mirror it as a local struct).
	scopeExternal
)

// scopeOf identifies which of the three nested-type cases a referenced type falls into. This is the
// single switch the rest of the generator routes through — change the locality rules here, not inline.
func (d *discoverer) scopeOf(n *types.Named) nestedScope {
	pkg := n.Obj().Pkg()
	switch {
	case pkg == nil || !d.local[pkg.Path()]:
		return scopeExternal
	case d.ownerPkg != nil && pkg.Path() == d.ownerPkg.Path():
		return scopeSamePackage
	default:
		return scopeCrossPackage
	}
}

// importSpecFor renders the import line for a package, adding an explicit alias only when the import
// path's last element doesn't already name the package (e.g. dir other_world, package otherworld).
func importSpecFor(p *types.Package) string {
	// Always alias with pkgAlias (parent_name) so it matches goRef's type qualifier and can't collide
	// with another referenced package that shares the bare package name.
	return pkgAlias(p) + " " + strconv.Quote(p.Path())
}

// goRef returns how a nested type is written in the owning command's Go source — bare for a same-package
// type, "<pkg>.<Name>" for a cross-package one — and the import spec the wire file needs for the latter
// (empty for same-package). The proto message name is always the bare name (one flat proto namespace).
func (d *discoverer) goRef(n *types.Named) (string, string) {
	name := n.Obj().Name()
	pkg := n.Obj().Pkg()
	if sc := d.scopeOf(n); sc != scopeCrossPackage && sc != scopeExternal {
		return name, ""
	}
	return pkgAlias(pkg) + "." + name, importSpecFor(pkg)
}

// pkgAlias is a disambiguated import alias for a cross-package Go type: parent dir + package name. Using
// the bare package name collides when two referenced packages share it (e.g. gameplay/component and
// gameplay/internal/walls/components are both package "component"); the parent segment separates them.
func pkgAlias(p *types.Package) string { return AliasFromPath(p.Path()) }

// AliasFromPath is pkgAlias's rule on the import path alone, for callers that hold a path rather than a
// package. Two trailing segments, because a single leaf collides constantly — "component" names a package
// in every plugin.
func AliasFromPath(pkgPath string) string {
	parts := strings.Split(pkgPath, "/")
	if len(parts) >= 2 {
		return sanitizeToIdent(parts[len(parts)-2]) + "_" + sanitizeToIdent(parts[len(parts)-1])
	}
	return sanitizeToIdent(pkgPath)
}

// qualifiedGoType renders a type as Go source for the owning command's package — unqualified for a
// same-package type, "<pkg>.<Name>" for any other package — and returns the import specs for the
// packages it names. Used for the decode-side cast of a *named* scalar (e.g. json.Number, EntityID),
// where the cast must target the declared field type, not its underlying basic. Unnamed basics (int,
// string, ...) have no package, so the qualifier is never called and no import is added.
func (d *discoverer) qualifiedGoType(t types.Type) (string, []string) {
	var imports []string
	seen := map[string]bool{}
	q := func(p *types.Package) string {
		// Unqualified ONLY for a package this run emits into. ownerPkg alone is wrong for a mirrored
		// message: it is then the FOREIGN package, while the converter is written into the local one, so
		// a named type declared beside the struct ("type Health int32") came out bare and undefined.
		// goRef routes through scopeOf for the same reason; this is the same rule.
		if p == nil || (d.ownerPkg != nil && p.Path() == d.ownerPkg.Path() && d.local[p.Path()]) {
			return "" // same package as the file being written (or builtin): unqualified
		}
		spec := importSpecFor(p)
		if !seen[spec] {
			seen[spec] = true
			imports = append(imports, spec)
		}
		return pkgAlias(p) // match importSpecFor's alias so the qualifier and the import agree
	}
	return types.TypeString(t, q), imports
}

// resolveNested enqueues a referenced struct type and returns how to name it in the owner's Go source
// (see goRef). importSpec is non-empty only for a type in another package, which the caller names in a
// decode-side var declaration.
func (d *discoverer) resolveNested(n *types.Named) (string, string) {
	d.enqueue(n, KindNested)
	return d.goRef(n)
}

// errNoExportedFields refuses a named struct that would generate an empty message: it has fields, but
// none a proto encoder can read. Checked wherever a struct becomes a message — directly, as a slice
// element, a map value, or behind a pointer — because the consequence does not depend on the spelling.
// Encoding writes an empty message and decoding hands back a zero value, neither reporting anything, so
// the field is lost in silence. An empty struct{} carries no data to lose and is allowed.
func errNoExportedFields(n *types.Named) error {
	st, isStruct := n.Underlying().(*types.Struct)
	if !isStruct || st.NumFields() == 0 || hasExportedField(st) {
		return nil
	}
	return fmt.Errorf("type %s has no exported fields — there is nothing to put on the "+
		"wire, so it holds runtime state rather than data", n.Obj().Name())
}

// hasExportedField reports whether any of s's fields is readable from another package.
func hasExportedField(s *types.Struct) bool {
	for f := range s.Fields() {
		if f.Exported() {
			return true
		}
	}
	return false
}

// refusedCategory buckets a field the classifier refused, choosing which fix the report offers. Asked in
// order: not data at all, then the shape, then data this generator has no mapping for.
func refusedCategory(t types.Type) string {
	if notData(t) {
		return CatUnserializable
	}
	if cat := shapeCategory(t); cat != "" {
		return cat
	}
	return CatUnsupportedType
}

// notData reports whether t holds, anywhere inside it, something that is not data: a chan or func (a
// handle to live machinery), an unsafe.Pointer, a method interface, or a struct whose fields are all
// unexported. No encoding of any format can carry these, which is what separates them from a type this
// generator merely declines to map.
//
// It walks the type because the reason is often below the surface: []chan int is refused for the chan,
// not for the slice, and reporting it as a slice would offer "give it a bound" as the fix.
//
// The question is only whether the value is data, never whether some encoder could carry it. Those come
// apart: encoding/json rejects a float map key by type, so deciding by encodability files map[float64]V
// as unserializable when it is a map and wants a map's fix.
func notData(t types.Type) bool {
	seen := map[types.Type]bool{}
	var walk func(types.Type) bool
	walk = func(t types.Type) bool {
		if seen[t] { // guard against cyclic types (e.g. struct A{ *A })
			return false
		}
		seen[t] = true
		if elem, _, ok := engineSliceElem(t); ok {
			return walk(elem) // the Slice itself is carried; only what it holds can be refused
		}
		switch u := t.Underlying().(type) {
		case *types.Chan, *types.Signature:
			return true
		case *types.Basic:
			return u.Kind() == types.UnsafePointer
		case *types.Interface:
			return !u.Empty() // a method interface is machinery; any/interface{} is a shape
		case *types.Pointer:
			return walk(u.Elem())
		case *types.Slice:
			return walk(u.Elem())
		case *types.Array:
			return walk(u.Elem())
		case *types.Map:
			return walk(u.Key()) || walk(u.Elem())
		case *types.Struct:
			// Every field unexported means nothing crosses the wire: the message would encode empty and
			// decode to a zero value, dropping the field in silence. Reachable for any library type
			// reached as a field (sync.Mutex and most internals look like this).
			if u.NumFields() > 0 && !hasExportedField(u) {
				return true
			}
			for f := range u.Fields() {
				// Only EXPORTED fields are checked: nothing else crosses the wire, so an unexported chan
				// is not a reason to refuse the struct that holds it.
				if !f.Exported() {
					continue
				}
				if walk(f.Type()) {
					return true
				}
			}
		}
		return false
	}
	return walk(t)
}

// shapeCategory buckets a type by the shape that makes it a finding, for the report. "" if it has none.
func shapeCategory(t types.Type) string {
	if elem, _, ok := engineSliceElem(t); ok {
		// A Slice is the fix for a slice, exactly as a fixed array is — unless its element is itself a
		// reference, in which case the element's shape is what needs changing.
		return shapeCategory(elem)
	}
	switch u := t.Underlying().(type) {
	case *types.Slice:
		return CatSlice
	case *types.Map:
		return CatMap
	case *types.Pointer:
		return CatPointer
	case *types.Interface:
		return CatInterface
	case *types.Array:
		// A fixed array is the fix, not a finding — unless its element is itself a reference, in which
		// case the element's shape is what needs changing.
		return shapeCategory(u.Elem())
	}
	return ""
}

// shapeReason states why a shape is refused, in one form for every category AND every wire kind: the
// bytes of the field are not the whole value.
//
// Both consequences follow from that one fact and both are named, because this is the entire
// justification a developer gets for a refused field. Naming only the component one reads as irrelevant
// on a command, where nothing is stored.
func shapeReason(cat string) string {
	held := "a " + cat + " is reached by address"
	if cat == CatInterface {
		held = "an interface holds a reference"
	}
	return held + ", so the bytes of the field are not the whole value: a component column copy would " +
		"share memory with the live world, and on any wire type the schema states no bound for it"
}

// classify maps a Go field type into the structured Field, enqueuing nested messages, or returns an
// error if the type has no proto representation.

func (d *discoverer) classify(t types.Type) (Field, error) {
	if elem, pkg, ok := engineSliceElem(t); ok {
		return d.classifyEngineSlice(elem, pkg)
	}
	switch u := t.Underlying().(type) {
	case *types.Slice:
		return d.classifySlice(u)
	case *types.Map:
		return d.classifyMap(u)
	case *types.Array:
		return d.classifyArray(u)
	default:
		return d.classifySingle(t)
	}
}

// classifyArray maps a fixed-size Go array onto proto.
//
// Every consecutive array dimension is peeled off and the counts multiplied, so [4][8]Vec2 becomes a
// single `repeated Vec2` carrying 32 elements in row-major order rather than four wrapper messages of
// eight. The shape is known at compile time on both sides, so nothing about it needs to travel with
// the data — the decoder rebuilds the indices arithmetically from constants.
//
// [N]byte is the one shape that is not repeated: it maps to proto `bytes`, matching how []byte is
// handled, which is what makes fixed-width identifiers and hashes ([16]byte, [32]byte) land on a
// sensible type instead of a run of integers.
//
// Peeling stops at the first non-array element. If that element is itself a collection (a slice, map
// or pointer) the shape cannot be flattened — inner lengths are data, not schema — so it is refused
// like any other unrepresentable type, and reported under the ELEMENT's shape rather than as an array.
func (d *discoverer) classifyArray(a *types.Array) (Field, error) {
	dims := []int64{a.Len()}
	elem := a.Elem()
	for {
		inner, ok := elem.Underlying().(*types.Array)
		if !ok {
			break
		}
		dims = append(dims, inner.Len())
		elem = inner.Elem()
	}

	// A zero-length NON-LEADING dimension has no row-major layout. The decode emitter rebuilds each
	// index as a quotient/remainder of the flat position by the stride for its dimension, and the stride
	// is the product of the dimensions to its right — which becomes 0 when any inner dim is 0 — so the
	// generated code indexes `c.V[i/0][i%0]`, a compile-time division by zero. A zero LEADING dimension
	// ([0][8]int32) carries no data but no stride or modulus becomes 0, so its (unreachable) loop body
	// still compiles; only a non-leading zero is unrepresentable. Refused here, like every other
	// unrepresentable array shape, so it is reported rather than emitted as broken code (see addFields).
	for j, n := range dims {
		if n <= 0 && j > 0 {
			return Field{}, fmt.Errorf(
				"array dimension %d is zero — a zero-length non-leading dimension has no row-major layout", j)
		}
	}

	// A byte array is a fixed-width blob rather than a run of integers, at any depth: [4][16]byte is
	// 64 contiguous bytes exactly as [16]byte is 16. Checked before delegating below, because the
	// slice rules map []byte to a bytes scalar and that would not survive the repeated-only check.
	if isByte(elem) {
		return Field{Kind: kindScalar, Scalar: bytesScalar(), ArrayDims: dims}, nil
	}

	// Reuse the slice element rules by classifying a synthetic []elem: an array and a slice agree on
	// what a valid element is, and only differ in whether the length is schema or data.
	f, err := d.classifySlice(types.NewSlice(elem))
	if err != nil {
		return Field{}, err
	}
	if f.Kind != kindRepeated {
		// A non-repeated answer means the element was itself a collection, which cannot be flattened.
		return Field{}, fmt.Errorf("unsupported array element %s", elem)
	}
	f.ArrayDims = dims
	return f, nil
}

// classifyEngineSlice maps world-engine's immutable.Slice[T] onto the wire exactly as []T would go: the
// same repeated field, or bytes for a byte element. Unlike the raw slice it is not a finding, because
// nothing can write through a Slice — the engine hands out a view whose backing array is unreachable, so
// a column copy sharing memory with the live world is harmless (compare shapeReason). What differs is the
// Go side, which reads through All and rebuilds with SliceOf (see Field.Slice).
//
// The ELEMENT is held to every rule a field is. Classifying a synthetic []T reuses the slice rules, and
// shapeCategory looks through the Slice, so a pointer inside it is reported under the pointer's fix.
func (d *discoverer) classifyEngineSlice(elem types.Type, pkg *types.Package) (Field, error) {
	f, err := d.classifySlice(types.NewSlice(elem))
	if err != nil {
		return Field{}, err
	}
	f.Slice = true
	f.SliceImport = importSpecFor(pkg)
	return f, nil
}

func (d *discoverer) classifySingle(t types.Type) (Field, error) {
	if p, ok := t.Underlying().(*types.Pointer); ok {
		return d.classifyPointer(t, p)
	}
	if b, ok := t.Underlying().(*types.Basic); ok {
		s, err := scalarOf(b)
		if err != nil {
			return Field{}, err
		}
		gt, imps := d.qualifiedGoType(t) // cast to the declared (possibly named) type, not the underlying
		s.GoType, s.Imports = gt, imps
		return Field{Kind: kindScalar, Scalar: s}, nil
	}
	if _, ok := t.Underlying().(*types.Struct); ok {
		n, ok := named(t)
		if !ok {
			return Field{}, errors.New("anonymous struct is not supported — use a named type")
		}
		if isTime(n) {
			return Field{Kind: kindMessage, Timestamp: true}, nil
		}
		if err := errNoExportedFields(n); err != nil {
			return Field{}, err
		}
		return Field{Kind: kindMessage, Msg: d.refTo(n)}, nil
	}
	if _, ok := t.Underlying().(*types.Interface); ok {
		return Field{}, errAny
	}
	return Field{}, fmt.Errorf("%s is not representable in protobuf", t)
}

// classifyPointer handles a pointer field. A *scalar becomes a proto3 `optional` scalar (nil/presence
// preserved on the wire); a *struct is a nested message pointer. *time.Time becomes a nil-able
// google.protobuf.Timestamp.
func (d *discoverer) classifyPointer(t types.Type, p *types.Pointer) (Field, error) {
	// *scalar → optional scalar. protoc-gen-go models `optional double` as *float64, so this round-trips
	// the Go pointer's nil-ness with no reflection and no dynamic typing.
	if b, ok := p.Elem().Underlying().(*types.Basic); ok {
		s, err := scalarOf(b)
		if err != nil {
			return Field{}, err
		}
		gt, imps := d.qualifiedGoType(p.Elem()) // cast to the declared (possibly named) elem type
		s.GoType, s.Imports = gt, imps
		return Field{Kind: kindScalar, Scalar: s, Optional: true}, nil
	}
	// Checked before the struct rules, which would otherwise blame the Slice for having no exported
	// field. The finding is still the pointer's; this only makes the reason name the real fix.
	if _, _, ok := engineSliceElem(p.Elem()); ok {
		return Field{}, errors.New(
			"a pointer to a Slice gains nothing — nothing can write through a Slice, so hold it by value",
		)
	}
	n, ok := named(p.Elem())
	if !ok {
		return Field{}, fmt.Errorf("unsupported pointer type %s", t)
	}
	if _, isStruct := n.Underlying().(*types.Struct); !isStruct {
		return Field{}, fmt.Errorf("unsupported pointer type %s", t)
	}
	if isTime(n) {
		return Field{Kind: kindMessage, Timestamp: true, Pointer: true}, nil
	}
	if err := errNoExportedFields(n); err != nil {
		return Field{}, err
	}
	return Field{Kind: kindMessage, Msg: d.refTo(n), Pointer: true}, nil
}

func (d *discoverer) classifySlice(s *types.Slice) (Field, error) {
	// Reached for Slice[Slice[T]], [N]Slice[T] and []Slice[T] alike: all three are a repeated inside a
	// repeated, which protobuf cannot express any more than it can [][]T.
	if _, _, ok := engineSliceElem(s.Elem()); ok {
		return Field{}, errors.New(
			"a Slice inside another repeated field is not representable in protobuf — wrap the inner Slice in a named struct",
		)
	}
	// Only an UNNAMED byte element is a proto bytes blob. A named-byte slice ([]MyByte) must fall through
	// to the repeated-scalar path below — Go rejects a []MyByte→[]byte slice cast, so it needs element-wise
	// conversion (repeated uint32), not the bytesScalar shortcut.
	if b, ok := s.Elem().(*types.Basic); ok && b.Kind() == types.Byte {
		return Field{Kind: kindScalar, Scalar: bytesScalar()}, nil // []byte -> bytes
	}
	if inner, ok := s.Elem().Underlying().(*types.Slice); ok {
		if isByte(inner.Elem()) {
			return Field{Kind: kindRepeated, Scalar: bytesScalar()}, nil // [][]byte -> repeated bytes
		}
		return Field{}, errors.New(
			"[][]T (nested slice) is not representable in protobuf — wrap the inner slice in a message",
		)
	}
	if b, ok := s.Elem().Underlying().(*types.Basic); ok {
		sc, err := scalarOf(b)
		if err != nil {
			return Field{}, err
		}
		gt, imps := d.qualifiedGoType(s.Elem())
		sc.GoType, sc.Imports = gt, imps
		return Field{Kind: kindRepeated, Scalar: sc}, nil
	}
	if n, ok := named(s.Elem()); ok {
		if isTime(n) {
			return Field{Kind: kindRepeated, Timestamp: true}, nil // []time.Time -> repeated Timestamp
		}
		if _, isStruct := n.Underlying().(*types.Struct); isStruct {
			if err := errNoExportedFields(n); err != nil {
				return Field{}, err
			}
			return Field{Kind: kindRepeated, Msg: d.refTo(n)}, nil
		}
	}
	return Field{}, fmt.Errorf("unsupported slice element %s", s.Elem())
}

func (d *discoverer) classifyMap(m *types.Map) (Field, error) {
	kb, ok := m.Key().Underlying().(*types.Basic)
	if !ok || isFloat(kb) {
		return Field{}, fmt.Errorf(
			"map key %s is not allowed in protobuf — use a repeated message of {key, value}",
			m.Key(),
		)
	}
	key, err := scalarOf(kb)
	if err != nil {
		return Field{}, fmt.Errorf("map key: %w", err)
	}
	kgt, kimps := d.qualifiedGoType(m.Key())
	key.GoType, key.Imports = kgt, kimps
	f := Field{Kind: kindMap, Key: key}
	// Same rule as a []T value below: a Slice is a repeated field, and a map value cannot be one.
	if _, _, ok := engineSliceElem(m.Elem()); ok {
		return Field{}, fmt.Errorf(
			"map value %s (repeated) is not allowed in protobuf — a Slice travels as a repeated field; wrap it in a named struct (map[K]Wrapper)",
			m.Elem(),
		)
	}
	if vs, ok := m.Elem().Underlying().(*types.Slice); ok && !isByte(vs.Elem()) {
		return Field{}, fmt.Errorf(
			"map value %s (repeated) is not allowed in protobuf — use map[K]Wrapper with a repeated field",
			m.Elem(),
		)
	}
	if _, ok := m.Elem().Underlying().(*types.Map); ok {
		return Field{}, fmt.Errorf(
			"map value %s (map) is not allowed in protobuf — wrap the inner map in a message",
			m.Elem(),
		)
	}
	if vb, ok := m.Elem().Underlying().(*types.Basic); ok {
		vs, verr := scalarOf(vb)
		if verr != nil {
			return Field{}, fmt.Errorf("map value: %w", verr)
		}
		vgt, vimps := d.qualifiedGoType(m.Elem())
		vs.GoType, vs.Imports = vgt, vimps
		f.ValSc = vs
		return f, nil
	}
	if isByte(m.Elem()) {
		f.ValSc = bytesScalar()
		return f, nil
	}
	if n, ok := named(m.Elem()); ok {
		if isTime(n) {
			f.Timestamp = true // map[K]time.Time -> map<K, Timestamp>
			return f, nil
		}
		if _, isStruct := n.Underlying().(*types.Struct); isStruct {
			if err := errNoExportedFields(n); err != nil {
				return Field{}, err
			}
			f.Val = d.refTo(n)
			return f, nil
		}
	}
	return Field{}, fmt.Errorf("unsupported map value %s", m.Elem())
}

// Field.Kind discriminator values.
const (
	kindScalar   = "scalar"
	kindMessage  = "message"
	kindRepeated = "repeated"
	kindMap      = "map"
)

// protoBytes is the proto scalar type token for a byte blob, used for []byte fields.
const protoBytes = "bytes"

// wktTimestampProto is the import path of the google.protobuf.Timestamp well-known type (bundled with
// protoc/buf, so no image change), used to carry time.Time fields.
const wktTimestampProto = "google/protobuf/timestamp.proto"

var (
	errAny = errors.New(
		"any/interface{} is intentionally disallowed to avoid runtime reflection — " +
			"consider refactoring how this command works to use a concrete type (or a oneof for a known set of variants)")
)

func scalarOf(b *types.Basic) (*Scalar, error) {
	switch b.Kind() {
	case types.Bool:
		return &Scalar{Family: "bool", GoType: "bool", Proto: "bool"}, nil
	case types.String:
		return &Scalar{Family: "string", GoType: "string", Proto: "string"}, nil
	case types.Int:
		return &Scalar{Family: "varint", GoType: "int", Proto: "int64"}, nil
	case types.Int64:
		return &Scalar{Family: "varint", GoType: "int64", Proto: "int64"}, nil
	case types.Int8:
		return &Scalar{Family: "varint", GoType: "int8", Proto: "int32"}, nil
	case types.Int16:
		return &Scalar{Family: "varint", GoType: "int16", Proto: "int32"}, nil
	case types.Int32:
		return &Scalar{Family: "varint", GoType: "int32", Proto: "int32"}, nil
	case types.Uint:
		return &Scalar{Family: "varint", GoType: "uint", Proto: "uint64"}, nil
	case types.Uint64:
		return &Scalar{Family: "varint", GoType: "uint64", Proto: "uint64"}, nil
	case types.Uint8:
		return &Scalar{Family: "varint", GoType: "uint8", Proto: "uint32"}, nil
	case types.Uint16:
		return &Scalar{Family: "varint", GoType: "uint16", Proto: "uint32"}, nil
	case types.Uint32:
		return &Scalar{Family: "varint", GoType: "uint32", Proto: "uint32"}, nil
	case types.Float32:
		return &Scalar{Family: "i32", GoType: "float32", Proto: "float"}, nil
	case types.Float64:
		return &Scalar{Family: "i64", GoType: "float64", Proto: "double"}, nil
	case types.Invalid, types.Uintptr, types.Complex64, types.Complex128,
		types.UnsafePointer, types.UntypedBool, types.UntypedInt, types.UntypedRune,
		types.UntypedFloat, types.UntypedComplex, types.UntypedString, types.UntypedNil:
		// not representable in protobuf — handled by the return below
	}
	return nil, fmt.Errorf("%s is not representable in protobuf — use a fixed-width numeric, string, or bytes", b)
}

func bytesScalar() *Scalar { return &Scalar{Family: protoBytes, GoType: "[]byte", Proto: protoBytes} }

func isByte(t types.Type) bool {
	// Only an UNNAMED byte is a proto bytes blob. A named byte (type X byte) must not take the
	// bytes path: Go rejects a []X -> []byte slice cast, so the bytes-blob wire emitter would write
	// uncompilable conversions across the defined-type boundary. Mirrors classifySlice's direct
	// *types.Basic check for the top-level []byte case (added when named-byte slice handling was
	// moved to the repeated-scalar path); do not peer through Underlying() here.
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.Byte
}

func isFloat(b *types.Basic) bool {
	return b.Kind() == types.Float32 || b.Kind() == types.Float64
}

func isTime(n *types.Named) bool {
	return n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "time" && n.Obj().Name() == "Time"
}

// immutablePkgPath is where world-engine keeps its value-only collections.
const immutablePkgPath = "github.com/argus-labs/world-engine/pkg/immutable"

// engineSliceElem reports whether t is world-engine's immutable.Slice[T], returning T and the package
// that declares Slice. An alias of one counts; a raw []T never does.
//
// The Slice is a struct with one unexported field, so every generic rule would refuse it: nothing in it
// crosses the wire by itself. It is recognised by name and origin instead.
//
// The package is matched by EXACT import path, never by package name or method set. This function is
// the only thing standing between a field and the value-only rule — immutable.Slice's own doc says
// "the wire generator refuses such an element" — so a package merely named immutable would otherwise
// buy the exemption for a type that aliases freely. A method set cannot establish it either:
// Clone() []T { return s.items } has the right signature and hands the backing array straight out.
// Only identity says the type is the one whose guarantees we checked.
func engineSliceElem(t types.Type) (types.Type, *types.Package, bool) {
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.TypeArgs().Len() != 1 {
		return nil, nil, false
	}
	o := n.Origin().Obj()
	if o.Name() != "Slice" || o.Pkg() == nil || o.Pkg().Path() != immutablePkgPath {
		return nil, nil, false
	}
	return n.TypeArgs().At(0), o.Pkg(), true
}

func named(t types.Type) (*types.Named, bool) {
	n, ok := t.(*types.Named)
	return n, ok
}

// pkgPathOf returns the Go import path of a named type's defining package (empty if none).
func pkgPathOf(n *types.Named) string {
	if p := n.Obj().Pkg(); p != nil {
		return p.Path()
	}
	return ""
}

// discoverGenericInstances returns every T the source instantiates cardinal.<generic>[T] with, for any of
// generics. It reads the type-checker's record of generic instantiations (types.Info.Instances), which
// holds a generic method call like w.RegisterCommand[T]() wherever it appears. Registration already implies
// the role (RegisterEvent[T] ⇒ T is an event, …), so there is nothing to over-discover.
func discoverGenericInstances(pkgs []*packages.Package, generics ...string) map[*types.Named]bool {
	out := map[*types.Named]bool{}
	for _, p := range pkgs {
		if p.TypesInfo == nil {
			continue
		}
		for id, inst := range p.TypesInfo.Instances {
			if inst.TypeArgs.Len() != 1 {
				continue
			}
			if o := cardinalGeneric(p.TypesInfo, id, inst); o == nil || !slices.Contains(generics, o.Name()) {
				continue
			}
			if t, ok := inst.TypeArgs.At(0).(*types.Named); ok {
				out[t] = true
			}
		}
	}
	return out
}

// cardinalGeneric returns the cardinal generic an instantiation is of, or nil if it is not cardinal's.
// A generic type is recorded with a *Named type; a generic function or method (w.RegisterCommand[T](),
// Go 1.27) with a *Signature, whose declaring object only the identifier's use records.
//
// go/types records an explicit instantiation even when T fails the constraint, which is the steady state
// here: discovery hides wire.gen.go, so no T has its wire methods yet. An INFERRED call that fails its
// constraint (w.Broadcast(ev)) is not recorded at all, so only explicit instantiations can declare a type.
func cardinalGeneric(info *types.Info, id *ast.Ident, inst types.Instance) types.Object {
	var o types.Object
	switch t := inst.Type.(type) {
	case *types.Named:
		o = t.Origin().Obj()
	case *types.Signature:
		f, ok := info.Uses[id].(*types.Func)
		if !ok {
			return nil
		}
		o = f
	default:
		return nil
	}
	if o.Pkg() == nil || o.Pkg().Name() != "cardinal" {
		return nil
	}
	return o
}

// discoverSentCommandTypes finds commands dispatched via w.SendToShard(to, Cmd{...}) in local packages.
// RegisterCommand discovery only sees commands a module receives; a command a module only sends (e.g. the
// lobby plugin's NotifySessionStart) would otherwise be invisible to its own module and never get a
// codec. The command is SendToShard's last argument; we resolve its concrete type (unwrapping pointers
// and aliases — a re-exported command is a types.Alias, not a Named). enqueue's local filter still scopes
// emission to this module. An argument forwarded as the command interface has no concrete type to
// resolve and is skipped. Gating on wireNames keeps unrelated SendToShard-named methods out.
func discoverSentCommandTypes(
	pkgs []*packages.Package, local map[string]bool, wireNames map[*types.Named]string,
) map[*types.Named]bool {
	cmds := map[*types.Named]bool{}
	for _, p := range pkgs {
		if p.TypesInfo == nil || !local[p.PkgPath] {
			continue
		}
		for _, f := range p.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				// A command sent only via SendToShard must still be discovered here, or the module never
				// generates its wire code and the receiving shard has nothing to decode the send with.
				//
				// The last-arg position is an assumption coupled to SendToShard's signature. It fails safe:
				// the wireNames[nm] != "" filter below only accepts a type that has a Name() method (a real
				// command), so if the signature ever grew a trailing arg, this would discover nothing rather
				// than the wrong type.
				if !ok || sel.Sel.Name != "SendToShard" || len(call.Args) == 0 {
					return true
				}
				t := p.TypesInfo.TypeOf(call.Args[len(call.Args)-1])
				if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
					t = ptr.Elem()
				}
				if nm, ok := types.Unalias(t).(*types.Named); ok {
					if _, isStruct := nm.Underlying().(*types.Struct); isStruct && wireNames[nm] != "" {
						cmds[nm] = true
					}
				}
				return true
			})
		}
	}
	return cmds
}

// collectWireNames classifies each type's Name() method by the two shapes the backend uses:
//   - static  `return "get_map"`                     -> exact wire name, into `out`
//   - dynamic `return r.LobbyID + "_get_map_result"` -> correlated (prefix+suffix), into `corr`
//
// A type appears in exactly one map (its Name() is one shape). Unrecognized shapes are skipped.
func collectWireNames(pkgs []*packages.Package, out map[*types.Named]string, corr map[*types.Named]correlatedWire) {
	for _, p := range pkgs {
		if p.Types == nil {
			continue
		}
		scope := p.Types.Scope()
		for _, f := range p.Syntax {
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Recv == nil || fd.Name.Name != "Name" || len(fd.Recv.List) == 0 {
					continue
				}
				exact, cw := parseNameReturn(fd.Body)
				if exact == "" && cw.Suffix == "" {
					continue
				}
				rn := recvBaseName(fd.Recv.List[0].Type)
				if rn == "" {
					continue
				}
				tn, ok := scope.Lookup(rn).(*types.TypeName)
				if !ok {
					continue
				}
				nm, ok := tn.Type().(*types.Named)
				if !ok {
					continue
				}
				if exact != "" {
					out[nm] = exact
				} else {
					corr[nm] = cw
				}
			}
		}
	}
}

func recvBaseName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return recvBaseName(t.X)
	case *ast.IndexExpr:
		return recvBaseName(t.X)
	case *ast.IndexListExpr:
		return recvBaseName(t.X)
	}
	return ""
}

// parseNameReturn reads a Name() body and classifies its single string return:
//   - `return "lit"`          -> exact = "lit"
//   - `return recv.F + "lit"` -> correlated{Suffix: "lit", Field: "F"}  (either operand order)
//
// Anything else yields both empty (unrecognized — skipped by the caller). Only the first return is used.
func parseNameReturn(body *ast.BlockStmt) (string, correlatedWire) {
	if body == nil {
		return "", correlatedWire{}
	}
	var exact string
	var corr correlatedWire
	var done bool
	ast.Inspect(body, func(n ast.Node) bool {
		if done {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return true
		}
		done = true
		switch e := ret.Results[0].(type) {
		case *ast.BasicLit:
			if s, ok := stringLit(e); ok {
				exact = s
			}
		case *ast.BinaryExpr:
			if e.Op != token.ADD {
				return false
			}
			// One operand is the static literal, the other is the runtime correlation field
			// (a selector like `recv.RequestID`). Accept either order.
			lit, litOK := binaryStringLit(e)
			fld, fldOK := binarySelectorField(e)
			if litOK && fldOK {
				corr = correlatedWire{Suffix: lit, Field: fld}
			}
		}
		return false
	})
	return exact, corr
}

func stringLit(lit *ast.BasicLit) (string, bool) {
	if lit == nil || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// binaryStringLit returns the string literal from whichever side of `X + Y` is a string literal.
func binaryStringLit(e *ast.BinaryExpr) (string, bool) {
	if lit, ok := e.Y.(*ast.BasicLit); ok {
		if s, ok := stringLit(lit); ok {
			return s, true
		}
	}
	if lit, ok := e.X.(*ast.BasicLit); ok {
		if s, ok := stringLit(lit); ok {
			return s, true
		}
	}
	return "", false
}

// binarySelectorField returns the field name from whichever side of `X + Y` is a `recv.Field` selector.
func binarySelectorField(e *ast.BinaryExpr) (string, bool) {
	if sel, ok := e.X.(*ast.SelectorExpr); ok {
		return sel.Sel.Name, true
	}
	if sel, ok := e.Y.(*ast.SelectorExpr); ok {
		return sel.Sel.Name, true
	}
	return "", false
}

// CSNamespaceOf derives the client SDK namespace for a Go package: <Product>.<Reldir PascalCased>,
// where Product is the generating module's leaf. This is the old proto-package scheme, kept for the
// client alone. It stays module-relative and short because a game developer reads these names —
// "Rampage.Shards.Gameplay.Event", not the module path spelled out. A package outside the module keeps
// its full import path, which is what the client already sees for plugin types today.
func CSNamespaceOf(module, pkgPath string) string {
	product := sanitizeToIdent(path.Base(module))
	rel := ""
	switch {
	case pkgPath == module:
	case module != "" && strings.HasPrefix(pkgPath, module+"/"):
		rel = strings.TrimPrefix(pkgPath, module+"/")
	default:
		rel = pkgPath
	}
	segs := []string{product}
	for s := range strings.SplitSeq(rel, "/") {
		if s != "" {
			segs = append(segs, sanitizeToIdent(s))
		}
	}
	return pascalDotted(strings.Join(segs, "."))
}

// ownershipOf is the single place a type's ownership is decided. Everything downstream — whether a
// .proto is generated or only imported, whether conversion is a method or a free function, whose Go the
// wire file imports — reads the result rather than re-deriving its own version of the question.
func (d *discoverer) ownershipOf(n *types.Named) Ownership {
	if d.scopeOf(n) != scopeExternal {
		return OwnLocal
	}
	if d.ownerGenPackage(n.Obj().Pkg()) != "" {
		return OwnImported
	}
	return OwnMirrored
}

// refTo resolves a reference to a named struct type: its name, how to write it here, the import that
// spelling needs, and who generates it. Every field and map value goes through this, so two references
// to one type can no longer be resolved by two slightly different routes.
func (d *discoverer) refTo(n *types.Named) TypeRef {
	goType, imp := d.resolveNested(n)
	return TypeRef{
		Name:    n.Obj().Name(),
		GoType:  goType,
		Import:  imp,
		PkgPath: pkgPathOf(n),
		Own:     d.ownershipOf(n),
	}
}

// ownerGenPackage returns the Go import path of the gen package the OWNER of pkg already generated for
// it, or "" if the owner generated nothing.
//
// This is what decides import versus copy. When the owner has generated, stamping their gen path as
// go_package makes protoc-gen-go emit an import of their package rather than a second Go type for the
// same message — one definition, assignable to the owner's own ToProto result. When they have not, there
// is nothing to point at and the consumer must generate its own.
//
// The path is read out of the owner's wire.gen.go, which imports its own gen package by definition. That
// is the only place it is recorded: it is a choice the owner made via --go-out, not something derivable
// from their source. The file is on disk but absent from the type graph, since WireBuildTag hides the
// generated layer from discovery.
func (d *discoverer) ownerGenPackage(pkg *types.Package) string {
	if pkg == nil {
		return ""
	}
	if hit, ok := d.genPkgCache[pkg]; ok {
		return hit
	}
	found := d.findOwnerGenPackage(pkg)
	d.genPkgCache[pkg] = found
	return found
}

// findOwnerGenPackage is ownerGenPackage without the memo.
func (d *discoverer) findOwnerGenPackage(pkg *types.Package) string {
	owner := d.module
	if m, ok := d.ownerModule[pkg.Path()]; ok && m != "" {
		owner = m
	}
	dir := d.pkgDir[pkg]
	if dir == "" {
		return ""
	}
	src, err := os.ReadFile(filepath.Join(dir, WireFileName))
	if err != nil {
		return ""
	}
	// protoc-gen-go lands a package's messages under a mirror of its source dir, so the import we want
	// ENDS with this package's own module-relative path. Matching on "somewhere under the same module"
	// instead returned whichever gen import gofmt happened to sort first — and a wire file has more than
	// one whenever a message references another package.
	_, protoFile := ProtoPackageOf(owner, pkg.Path())
	suffix := "/" + path.Dir(protoFile)
	if suffix == "/." {
		suffix = ""
	}
	var marked, all []string
	for line := range strings.SplitSeq(string(src), "\n") {
		alias, imp, ok := importLine(line)
		if !ok {
			continue
		}
		if suffix != "" && !strings.HasSuffix(imp, suffix) {
			continue
		}
		// Everything before the suffix is the owner's gen root; it has to sit under their module, on a
		// path boundary — a bare prefix test lets module "x/game" swallow "x/gamelib".
		root := strings.TrimSuffix(imp, suffix)
		if root == owner || !strings.HasPrefix(root, owner+"/") {
			continue
		}
		all = append(all, imp)
		// genAlias stamps every gen import with the alias its own leaf produces, so reconstructing that
		// alias tells a package's generated counterpart from the hand-written packages beside it. Matched
		// whole rather than by its "pb" prefix: the suffix filter above has already pinned the last two
		// path segments, so a hand-written import sitting under a directory named "pb..." would carry a
		// prefix-matching alias of its own. A preference and not a filter: an owner whose wire file
		// predates the scheme still has to be read as "generated", or this run would mirror a type they
		// already ship.
		if alias == genAlias(path.Base(imp)) {
			marked = append(marked, imp)
		}
	}
	return pickGenImport(marked, all, suffix == "")
}

// pickGenImport chooses the owner's gen import from the candidates a wire file offered, preferring the
// ones the current alias scheme marks as generated.
//
// atRoot says the package sits at its module root, where its module-relative path is empty and so filters
// nothing — leaving every gen import in the file a candidate and the first one read winning arbitrarily.
// The right one there is the root the others hang off (<genRoot> against <genRoot>/<dir>), which is the
// shortest. With a suffix to match on there is only ever one real answer, so the first is it.
func pickGenImport(marked, all []string, atRoot bool) string {
	candidates := marked
	if len(candidates) == 0 {
		candidates = all
	}
	if len(candidates) == 0 {
		return ""
	}
	if !atRoot {
		return candidates[0]
	}
	best := candidates[0]
	for _, imp := range candidates[1:] {
		if len(imp) < len(best) {
			best = imp
		}
	}
	return best
}

// importLine splits one line of Go source into an import's alias (empty when it has none) and its path.
// Handles both forms an import can take: a line inside a block, and a single-line `import alias "path"`,
// whose leading keyword is not part of the alias. Reports false for any line that is not an import.
func importLine(line string) (string, string, bool) {
	head, rest, found := strings.Cut(line, `"`)
	if !found {
		return "", "", false
	}
	imp, _, found := strings.Cut(rest, `"`)
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(head), "import")), imp, true
}

// MergeMessages unions message lists, keeping the first copy of any message that appears more than once.
//
// Keyed by proto full name (package + message) rather than Go identity, because that is what a duplicate
// means to protoc: two `message Vec2` in one `dep.component` is "symbol already defined" no matter which
// list each came from. A dependency's type genuinely reaches a run twice — as a local field and as a
// wired top-level type — and emitting each list separately produced two .proto files claiming one path,
// where only the last survived and every message the loser held vanished with nothing reported.
//
// Earlier groups win, so callers pass the authoritative list first.
func MergeMessages(groups ...[]Message) []Message {
	var out []Message
	seen := map[string]bool{}
	for _, group := range groups {
		for _, m := range group {
			key := m.ProtoPkg + "." + m.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, m)
		}
	}
	return out
}

// ResolveGen stamps every message, and every reference to one, with the gen package holding its
// generated Go. Call it once after Discover, when --go-out is finally known.
//
// This is the carry-rather-than-derive half of the fix. Emission used to work these paths out again at
// each call site, from the GENERATING module — which is wrong for a foreign package, since that resolves
// against its own owner — and the result was wire files importing paths nobody had written. Resolving
// here means the emitter reads a field and there is nothing left to disagree with.
// The value receiver matches Result's other methods. It still mutates: every write goes through the
// message slices, whose backing arrays the caller shares.
func (r Result) ResolveGen(goOutImport string) {
	byKey := make(map[string]string, len(r.Messages)+len(r.ExternalMessages))
	fill := func(m *Message) {
		// An imported message already carries its owner's path; anything else lands in this run's output,
		// mirroring its .proto directory (paths=source_relative), the module root included.
		if m.GenImport == "" && goOutImport != "" && m.ProtoFile != "" {
			if dir := path.Dir(m.ProtoFile); dir != "." && dir != "" {
				m.GenImport = goOutImport + "/" + dir
			} else {
				m.GenImport = goOutImport
			}
		}
		m.Self.GenImport = m.GenImport
		byKey[m.PkgPath+"."+m.Name] = m.GenImport
	}
	for i := range r.Messages {
		fill(&r.Messages[i])
	}
	for i := range r.ExternalMessages {
		fill(&r.ExternalMessages[i])
	}

	// Every reference resolves to the same path as the message it points at, by construction.
	link := func(ref *TypeRef) {
		if ref.Set() {
			ref.GenImport = byKey[ref.Key()]
		}
	}
	for _, group := range [][]Message{r.Messages, r.ExternalMessages} {
		for i := range group {
			for j := range group[i].Fields {
				link(&group[i].Fields[j].Msg)
				link(&group[i].Fields[j].Val)
			}
		}
	}
}

// ownerOf returns the module that defines pkgPath, falling back to the scanned module when the loader
// reported none (a package with no module info can only be one of ours).
func (r Result) ownerOf(pkgPath string) string {
	if m, ok := r.OwnerModule[pkgPath]; ok && m != "" {
		return m
	}
	return r.Module
}

// ProtoPackageOf derives the proto package and .proto file path for a Go package.
//
// The PACKAGE is the full Go import path, dotted — the same value whoever generates it. That matters
// because a type reached from another module has to end up with one identity, not one per consumer: if
// each backend derived its own name for world-engine's Vec2, every backend would define a different
// message for the same Go type, and nothing downstream could tell they were the same thing. The import
// path is already globally unique (it contains the module path), so it gives agreement and collision-
// freedom from the same string. Note this is NOT the C# namespace — see Message.CSNamespace.
//
// The FILE is module-relative, so it stays short and a package's generated code lands where its source
// is. A foreign package's file is resolved against ITS owner's module, not the generating one, so the
// owner and every consumer name the same file for the same type.
//
// No version suffix — this system versions per-type via Name(), so a package version would never
// increment. Examples, for module github.com/argus-labs/rampage:
//
//	.../rampage/shards/gameplay/event  ->  pkg "github_com.argus_labs.rampage.shards.gameplay.event"
//	                                       file "shards/gameplay/event/event.proto"
//	.../world-engine/pkg/plugin/lobby/system (a dependency, owner module world-engine)
//	                                    ->  pkg "github_com.argus_labs.world_engine.pkg.plugin.lobby.system"
//	                                       file "pkg/plugin/lobby/system/system.proto"
func ProtoPackageOf(module, pkgPath string) (string, string) {
	// Only a real path boundary counts: TrimPrefix alone would make module "github.com/x/game" swallow
	// the unrelated "github.com/x/gamelib/component" and merge it into this module's tree.
	rel := ""
	switch {
	case pkgPath == module:
	case module != "" && strings.HasPrefix(pkgPath, module+"/"):
		rel = strings.TrimPrefix(pkgPath, module+"/")
	default:
		rel = pkgPath // not under this module; the caller resolves it against its own owner
	}

	// The proto PACKAGE name keeps every segment — including "internal" — since proto/C# have no such
	// rule and it preserves the source path for traceability. The FILE path (which drives where the .pb.go
	// lands, and thus its Go import path) ESCAPES "internal" to "internal_": Go's internal-import rule
	// polices a literal "internal" path element, so the escaped form keeps the gen package importable from
	// outside its subtree — while, crucially, staying a 1:1 function of the source dir. (Stripping the
	// segment instead would collapse ".../internal/x" and ".../x" onto the same file → silent merge.)
	var segs []string
	for s := range strings.SplitSeq(pkgPath, "/") {
		if s != "" {
			segs = append(segs, sanitizeToIdent(s))
		}
	}
	pkg := strings.Join(segs, ".")

	var dirs []string
	for s := range strings.SplitSeq(rel, "/") {
		if s == "" {
			continue
		}
		if s == "internal" {
			s = "internal_" // path: escape it — importable, and still unique per source dir
		}
		dirs = append(dirs, s)
	}

	base := sanitizeToIdent(path.Base(module)) // module root: name the file after the product
	if len(dirs) > 0 {
		base = dirs[len(dirs)-1] // else after the leaf dir
	}
	file := base + ".proto"
	if len(dirs) > 0 {
		file = strings.Join(dirs, "/") + "/" + base + ".proto"
	}
	return pkg, file
}

// findModule walks up from dir to the nearest enclosing go.mod, returning both the module path and the
// directory that holds that go.mod (the module root). moduleOf and DeriveGoPackage share this walk.
func findModule(dir string) (string, string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	for d := abs; ; {
		if data, rerr := os.ReadFile(filepath.Join(d, "go.mod")); rerr == nil {
			m := modfile.ModulePath(data)
			if m == "" {
				return "", "", fmt.Errorf("no module path in %s/go.mod", d)
			}
			return m, d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", "", fmt.Errorf("%q is not inside a Go module (no go.mod found in it or any parent)", dir)
		}
		d = parent
	}
}

// moduleOf returns the Go module path of the module containing dir (walks up to the first go.mod).
func moduleOf(dir string) (string, error) {
	mod, _, err := findModule(dir)
	return mod, err
}

// protoRef is a referenced package's resolved proto identity, indexed by Go import path.
// protoRef is a referenced package's resolved proto identity, indexed by Go import path.
type protoRef struct{ pkg, file string }

// ProtoFileOut is one generated .proto file: its module-relative path and content.
type ProtoFileOut struct {
	Path    string
	Content string

	// ImportOnly marks a file that exists so the schema resolves, but must not be generated from: its
	// go_package names another module's package, which already holds the generated type. Generating it
	// would write a second copy of that type into this module and defeat the point of the import.
	ImportOnly bool
}

// EmitProtos renders one .proto file per Go package (grouped by Message.ProtoFile). Each file declares
// its own proto package (Message.ProtoPkg) so same-named types in different Go packages never collide;
// a field whose message type lives in another package is rendered fully qualified and its file imported.
// module anchors proto-package derivation; goOutImport is the import path of the --go-out module, used
// for each file's go_package option (empty omits it, e.g. a cs-only run).
func EmitProtos(module, goOutImport string, msgs []Message) ([]ProtoFileOut, error) {
	byFile := map[string][]Message{}
	var order []string
	for _, m := range msgs {
		if _, seen := byFile[m.ProtoFile]; !seen {
			order = append(order, m.ProtoFile)
		}
		byFile[m.ProtoFile] = append(byFile[m.ProtoFile], m)
	}
	sort.Strings(order)

	// One identity per referenced package, taken from the message that owns it. Re-deriving here with the
	// generating module produced a different answer for a foreign package than the one its own file was
	// written under, so the emitted import named a path nothing had written.
	ref := map[string]protoRef{}
	for _, m := range msgs {
		if m.PkgPath != "" {
			ref[m.PkgPath] = protoRef{pkg: m.ProtoPkg, file: m.ProtoFile}
		}
	}
	if err := checkFileCollision(msgs); err != nil {
		return nil, err
	}

	out := make([]ProtoFileOut, 0, len(order))
	for _, file := range order {
		content, err := emitOneProto(module, goOutImport, file, byFile[file], ref)
		if err != nil {
			return nil, err
		}
		out = append(out, ProtoFileOut{
			Path: file, Content: content, ImportOnly: byFile[file][0].Own == OwnImported,
		})
	}
	return out, nil
}

// checkFileCollision refuses two Go packages that resolve to one .proto path.
//
// The proto PACKAGE is the full import path and is unique; the FILE is module-relative and is not — a
// local "game/component" and a dependency's "dep/component" both give "component/component.proto".
// EmitProtos groups by file, so they would be concatenated into one, taking the package, go_package and
// csharp_namespace from whichever message sorted first: half the types silently stamped with another
// module's identity, and protoc failing later with an undefined reference. Loud here beats mysterious
// there. checkDuplicates cannot see this — it keys on proto package plus name, which differ.
func checkFileCollision(msgs []Message) error {
	owner := map[string]string{} // proto file -> the one import path allowed to write it
	for _, m := range msgs {
		if m.ProtoFile == "" || m.PkgPath == "" {
			continue
		}
		if prev, ok := owner[m.ProtoFile]; ok && prev != m.PkgPath {
			return fmt.Errorf("packages %s and %s both resolve to %s — they share a module-relative "+
				"path, so their messages would be merged into one file under one package; rename one "+
				"directory, or generate them in separate runs", prev, m.PkgPath, m.ProtoFile)
		}
		owner[m.ProtoFile] = m.PkgPath
	}
	return nil
}

// emitOneProto renders a single .proto file for one package's messages, collecting the imports its
// cross-package field types require.
func emitOneProto(module, goOutImport, file string, msgs []Message, ref map[string]protoRef) (string, error) {
	ownerPkg := msgs[0].ProtoPkg
	imports := map[string]bool{}

	var body strings.Builder
	var werr error
	bp := func(format string, args ...any) {
		if werr == nil {
			_, werr = fmt.Fprintf(&body, format, args...)
		}
	}
	for _, m := range msgs {
		if m.Wire != "" {
			bp("// wire name: %q\n", m.Wire)
		}
		bp("message %s {\n", m.Name)
		for _, f := range m.Fields {
			typeTok := protoFieldType(module, ownerPkg, f, imports, ref)
			if len(f.ArrayDims) > 0 {
				// A flat repeated field does not carry the Go shape, so record it for readers of the
				// schema and for clients in the languages this also targets. A comment rather than a
				// protovalidate option: the codegen toolchain resolves no BSR dependencies (see the
				// embedded Dockerfile), and the bound is enforced by the generated decoder regardless.
				order := ""
				if len(f.ArrayDims) > 1 {
					order = " (row-major)"
				}
				bp("  // Go: %s%s%s, %d elements\n", f.ArrayShape(), arrayElemDoc(f), order, f.ArrayCount())
			}
			if label := protoLabel(f); label != "" {
				bp("  %s %s %s = %d;\n", label, typeTok, f.Name, f.Number)
			} else {
				bp("  %s %s = %d;\n", typeTok, f.Name, f.Number)
			}
		}
		body.WriteString("}\n\n")
	}
	if werr != nil {
		return "", werr
	}

	var h strings.Builder
	h.WriteString("// Code generated by world sdk generate. DO NOT EDIT.\n")
	h.WriteString("syntax = \"proto3\";\n\n")
	fmt.Fprintf(&h, "package %s;\n\n", ownerPkg)

	dir := path.Dir(file)
	switch {
	case msgs[0].Own == OwnImported:
		// Their package already exists, so point at it: protoc-gen-go emits an import of their type
		// rather than generating a second one here, and the two stay assignable.
		imp := msgs[0].GenImport
		fmt.Fprintf(&h, "option go_package = %q;\n", imp+";"+goPkgName(path.Base(imp)))
	case goOutImport != "":
		imp := goOutImport
		if dir != "." {
			imp += "/" + dir
		}
		fmt.Fprintf(&h, "option go_package = %q;\n", imp+";"+goPkgName(path.Base(dir)))
	}
	fmt.Fprintf(&h, "option csharp_namespace = %q;\n\n", msgs[0].CSNamespace)

	var imps []string
	for f := range imports {
		if f != file { // a same-file (same-package) reference needs no import
			imps = append(imps, f)
		}
	}
	sort.Strings(imps)
	for _, im := range imps {
		fmt.Fprintf(&h, "import %q;\n", im)
	}
	if len(imps) > 0 {
		h.WriteString("\n")
	}
	return h.String() + body.String(), nil
}

// protoFieldType renders a field's proto type token, qualifying a message type that lives in another
// proto package as "<pkg>.<Name>" and recording its file in imports. Same-package refs stay bare.
func protoFieldType(
	module, ownerPkg string, f Field, imports map[string]bool, ref map[string]protoRef,
) string {
	qual := func(bare, pkgPath string) string {
		if pkgPath == "" {
			return bare
		}
		rp, rf := ProtoPackageOf(module, pkgPath)
		if r, ok := ref[pkgPath]; ok {
			rp, rf = r.pkg, r.file // the referenced message's own identity wins over re-derivation
		}
		if rp == "" || rp == ownerPkg {
			return bare
		}
		imports[rf] = true
		return rp + "." + bare
	}
	// timestamp records the well-known-type import and yields its qualified name.
	timestamp := func() string {
		imports[wktTimestampProto] = true
		return "google.protobuf.Timestamp"
	}
	switch f.Kind {
	case kindScalar:
		return f.Scalar.Proto
	case kindMessage:
		if f.Timestamp {
			return timestamp()
		}
		return qual(f.Msg.Name, f.Msg.PkgPath)
	case kindRepeated:
		if f.Scalar != nil {
			return f.Scalar.Proto
		}
		if f.Timestamp {
			return timestamp()
		}
		return qual(f.Msg.Name, f.Msg.PkgPath)
	case kindMap:
		var val string
		switch {
		case f.ValSc != nil:
			val = f.ValSc.Proto
		case f.Timestamp:
			val = timestamp()
		default:
			val = qual(f.Val.Name, f.Val.PkgPath)
		}
		return fmt.Sprintf("map<%s, %s>", f.Key.Proto, val)
	}
	return ""
}

// arrayElemDoc names the Go element type in the shape comment, best-effort: the exact name is only
// needed to make the comment readable, not to generate anything.
func arrayElemDoc(f Field) string {
	switch {
	case f.Kind == kindScalar:
		return "byte"
	case f.Msg.Set():
		return f.Msg.Name
	case f.Timestamp:
		return timeGoType
	case f.Scalar != nil && f.Scalar.GoType != "":
		return f.Scalar.GoType
	}
	return "?"
}

// protoLabel is the proto field label: "repeated" for a slice, "optional" for a pointer scalar, else "".
func protoLabel(f Field) string {
	if f.Kind == kindRepeated {
		return "repeated"
	}
	if f.Kind == kindScalar && f.Optional {
		return "optional"
	}
	return ""
}

// EmitWireNamesCS renders C# partial classes that make each top-level wire type implement IWireNamed,
// exposing its backend Name() (Message.Wire). protoc output carries only the descriptor name, so this is
// the C# mirror of the third Serializable member (IMessage already gives Marshal/Unmarshal). One file,
// grouped by C# namespace; returns "" when there is nothing to emit.
func EmitWireNamesCS(res Result) string {
	// A type is exact (static Name() -> WireName) or correlated (dynamic Name() -> WireSuffix + field);
	// never both. Emit the matching interface partial for each, grouped by namespace.
	type entry struct{ class, wire, suffix, field string }
	byNS := map[string][]entry{}
	collect := func(msgs []Message) {
		for _, m := range msgs {
			if !slices.Contains(m.Targets, TargetCSharp) {
				continue
			}
			switch {
			case m.Wire != "":
				byNS[m.CSNamespace] = append(
					byNS[m.CSNamespace],
					entry{class: m.Name, wire: m.Wire},
				)
			case m.WireSuffix != "":
				byNS[m.CSNamespace] = append(
					byNS[m.CSNamespace],
					entry{class: m.Name, suffix: m.WireSuffix, field: m.WireField},
				)
			}
		}
	}
	// Merged, not collected in sequence: a dependency type reached BOTH as a field of a local type and as
	// a top-level external appears in each list, and emitting it twice writes two partial declarations of
	// one class that both define WireName — which C# rejects as a duplicate member. MergeMessages keys on
	// proto package plus name, the same identity the class has, so a same-named type in another namespace
	// still gets its own partial. EmitProtos is merged for the same reason (see emitProtos).
	collect(MergeMessages(res.Messages, res.ExternalMessages))
	if len(byNS) == 0 {
		return ""
	}

	namespaces := make([]string, 0, len(byNS))
	for ns := range byNS {
		namespaces = append(namespaces, ns)
	}
	slices.Sort(namespaces)

	var b strings.Builder
	b.WriteString("// Code generated by world sdk generate. DO NOT EDIT.\n")
	b.WriteString("// Wire identity partials, the C# mirror of Serializable.Name():\n")
	b.WriteString("//   IWireNamed      — types with a static Name() (exact wire name).\n")
	b.WriteString("//   IWireCorrelated — types with a dynamic Name() (`field + \"suffix\"`); the wire name is a\n")
	b.WriteString("//                     runtime prefix + WireSuffix, so consumers resolve by trailing suffix.\n\n")
	for _, ns := range namespaces {
		es := byNS[ns]
		slices.SortFunc(es, func(a, c entry) int { return strings.Compare(a.class, c.class) })
		fmt.Fprintf(&b, "namespace %s\n{\n", ns)
		for _, e := range es {
			if e.wire != "" {
				fmt.Fprintf(&b, "    public sealed partial class %s : global::WorldEngine.SDK.IWireNamed\n", e.class)
				fmt.Fprintf(&b, "    {\n        public string WireName => %q;\n    }\n\n", e.wire)
				continue
			}
			fmt.Fprintf(&b, "    public sealed partial class %s : global::WorldEngine.SDK.IWireCorrelated\n", e.class)
			fmt.Fprintf(
				&b,
				"    {\n        public string WireSuffix => %q;\n        public string CorrelationField => %q;\n    }\n\n",
				e.suffix,
				e.field,
			)
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}

// pascalDotted PascalCases each dot-segment of a proto package for a C# namespace:
// "rampage.shards.gameplay.event" -> "Rampage.Shards.Gameplay.Event".
func pascalDotted(pkg string) string {
	segs := strings.Split(pkg, ".")
	for i, s := range segs {
		if s != "" {
			segs[i] = strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return strings.Join(segs, ".")
}

// sanitizeToIdent maps every rune that cannot appear in an identifier to '_', producing a valid
// Go/proto identifier body. An existing '_' passes through unchanged: protobuf's style guide requires
// every underscore to be followed by a letter, so escaping one as "__" produces a name the spec
// disallows — and it leaked into the C# namespace, which is client-facing and had shipped already.
//
// That leaves the mapping non-injective: "abc-x", "abc.x" and "abc_x" all become "abc_x", so two
// packages can collapse onto one proto identity. Which is fine, because it is not the thing keeping
// them apart — the duplicate proto-name check does that, loudly and at the two directories that caused
// it (see TestDuplicateName). Escaping here was a second mechanism for a job already done.
func sanitizeToIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out != "" && out[0] >= '0' && out[0] <= '9' {
		// A proto package segment and a C# namespace segment are identifiers, which may not start with a
		// digit — but a Go import path element may ("game/2d/component"). Prefix rather than drop, so the
		// result stays a function of the input.
		out = "_" + out
	}
	return out
}

// GoImportPath returns the import-path half of a DeriveGoPackage result
// ("<import-path>;<pkg>" -> "<import-path>").
func GoImportPath(goPackage string) string {
	if i := strings.LastIndex(goPackage, ";"); i >= 0 {
		return goPackage[:i]
	}
	return goPackage
}

// GoPackageName returns the package-name half of a DeriveGoPackage result
// ("<import-path>;<pkg>" -> "<pkg>").
func GoPackageName(goPackage string) string {
	if i := strings.LastIndex(goPackage, ";"); i >= 0 {
		return goPackage[i+1:]
	}
	return goPackage
}

// DeriveGoPackage computes the protoc-gen-go `go_package` value
// ("<import-path>;<pkg>") for generated code placed in dir, by reading the
// nearest enclosing go.mod.
func DeriveGoPackage(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	mod, modDir, err := findModule(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(modDir, abs)
	if err != nil {
		return "", err
	}
	imp := mod
	if rel != "." {
		imp = mod + "/" + filepath.ToSlash(rel)
	}
	return imp + ";" + goPkgName(path.Base(imp)), nil
}

func goPkgName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "gen"
	}
	return strings.ToLower(b.String())
}
