package sdk

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/rotisserie/eris"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/sdkgen"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/spinner"
	"github.com/argus-labs/world-engine/cli/pkg/modfile"
	"github.com/argus-labs/world-engine/cli/pkg/version"
)

type GenerateCmd struct {
	Source string `arg:"" help:"Backend dir, or a remote git URL/owner/repo, to scan for commands, events, components and system events"`

	GoOut string `help:"Directory to write the generated protobuf Go into — must be inside your backend's Go module (e.g. <backend>/gen). The wire.gen.go converters are written next to your source, not here. Local source only (a remote clone is read-only)."`
	CsOut string `help:"Directory to write generated C# (client SDK) into"`
	// NoEmit stops the run after the report. Discovery and classification are untouched — only emission
	// is dropped — so the findings are exactly the ones a real run would act on. It writes nothing, which
	// is also why it cannot stub a missing --go-out package: a backend whose wire.gen.go imports a gen/
	// tree that does not exist yet has to generate once before it can be reported on.
	NoEmit bool `help:"Report what would be generated, then stop. Runs discovery and prints the findings but emits nothing — no Docker, no files written. Exits non-zero if anything blocks."`
	// ProtoOut is a debugging aid, not a build input, and is slated for deprecation. Nothing in
	// generation or in CI reads the exported schema — buf's module is the hand-written proto/ tree, so
	// these files are not under a breaking check — and the backends that once persisted it
	// (rampage-backend, the world-engine plugins) keep no copy. It stays so the wire contract can be
	// read by hand; retiring it means deleting the flag, ownProtos, writeProtos, and the --go-out
	// requirement below.
	ProtoOut      string `help:"DEBUGGING ONLY, slated for deprecation. Directory to persist the generated .proto schema into, for READING the wire contract. Nothing in generation reads it — the schema is compiled in a temp dir and the copy is an export. It holds only this backend's own files: a dependency's schema is left out while the import statement naming it stays, so the export does not compile standalone and cannot feed buf breaking. Requires --go-out."`
	GoVersion     string `help:"override protoc-gen-go version (default: repo-pinned)"`
	CSharpVersion string `help:"override C#/protoc version (default: repo-pinned)"`
	Ref           string `help:"git branch/tag/commit to clone when source is a remote URL"`
}

// Run discovers commands, emits .proto, runs buf generate in Docker, and copies the generated
// code to the target dirs.
//
// Field rules, applied the same way to commands, events, components and system events. Every finding
// stops generation; the category names the fix the field needs, one finding per field:
//
//   - unserializable — chan, func, unsafe pointer, a method interface, a struct with no exported
//     fields: runtime state rather than data, so no encoding could carry it.
//   - slice, map, pointer, interface — each holds a reference, so the bytes of the field are not the
//     whole value and it cannot live in an ECS column.
//   - unsupported-type — data held inline that this generator has no mapping for (uintptr, complex).
//
// A type another module owns — a plugin's, or a library's — is rebuilt into this schema from its Go
// source, so it needs nothing from its author and reports under the same rules as anything local.
//
// Clean: scalars, string, nested structs, fixed arrays ([N]T; [N]byte becomes proto bytes), time.Time
// (becomes Timestamp).
//
// Versioning: put the version in Name() ("move.v2"); append fields only within a version; a breaking
// change is a new vN type.
//
// Two runtime gotchas the lint can't catch: (1) send commands by value, not by
// pointer — the command is asserted to its value type on decode, so &cmd fails at send with
// "expected X, got *X"; (2) narrow integer fields widen on the wire (int8/16→int32,
// uint8→uint32, int/uint→64-bit), so a peer (e.g. the C# client) can send a value
// outside the Go type's range and it silently truncates on decode — don't rely on a
// field's width for validation.
// validate checks the flag combination before any work: at least one output unless --no-emit (which
// produces none); no Go output for a read-only remote source; and --proto-out only alongside --go-out
// (the schema's go_package is derived from it).
func (c *GenerateCmd) validate(remote bool) error {
	switch {
	case c.GoOut == "" && c.CsOut == "" && !c.NoEmit:
		return eris.New("nothing to generate: pass --go-out and/or --cs-out, " +
			"or --no-emit to stop after the report")
	case remote && c.GoOut != "":
		// A remote clone is read-only and deleted after generation, and the Go wire code must live in the
		// backend's own packages (emitted in-place). So a remote source can only produce the client SDK.
		return eris.New("--go-out is not valid for a remote source — a remote clone is read-only and " +
			"the Go wire code must be generated from a local checkout of the backend; pass --cs-out only")
	case c.ProtoOut != "" && c.GoOut == "":
		// The stored schema carries the go_package option derived from --go-out, so it needs that context.
		return eris.New("--proto-out requires --go-out (the stored schema's go_package comes from it)")
	}
	return nil
}

// emitProtos renders the .proto for each requested output: Go over every local wire type, C# over the
// client-facing subset plus mirrored dependency (plugin) types (which ship their own Go, so are C#-only).
// generateAll clears ImportOnly, which is a Go-pass concept and wrong for C#.
//
// ImportOnly means "the owner already generated this type, so generating it again would produce a second
// Go type for one message". That reasoning is specific to Go: the owner ships Go and nothing ships C#,
// which is the whole reason the external pass exists. Carrying the flag into the C# run made buf skip
// those files, so the client got messages referencing classes that were never emitted.
func generateAll(protos []sdkgen.ProtoFileOut) []sdkgen.ProtoFileOut {
	out := make([]sdkgen.ProtoFileOut, len(protos))
	for i, p := range protos {
		p.ImportOnly = false
		out[i] = p
	}
	return out
}

func (c *GenerateCmd) emitProtos(
	res sdkgen.Result, goOutImport string,
) ([]sdkgen.ProtoFileOut, []sdkgen.ProtoFileOut, error) {
	var goProtos, csProtos []sdkgen.ProtoFileOut
	if c.GoOut != "" {
		p, err := sdkgen.EmitProtos(res.Module, goOutImport, withTarget(res.Messages, sdkgen.TargetGo))
		if err != nil {
			return nil, nil, eris.Wrap(err, "emit Go proto")
		}
		goProtos = p
	}
	if c.CsOut != "" {
		// One call over the union, not two concatenated. A dependency's package can appear in BOTH lists —
		// reached as a local field and wired as an external top-level type — and each call resolves the
		// same package to the same .proto path. Emitting separately produced two files claiming one path,
		// which the work dir stores in a map: the second silently replaced the first and every message
		// only the loser held vanished from the client SDK. Merging first lets EmitProtos group them into
		// the one file they belong in, and its own collision check see them together.
		p, err := sdkgen.EmitProtos(
			res.Module, goOutImport,
			sdkgen.MergeMessages(withTarget(res.Messages, sdkgen.TargetCSharp), res.ExternalMessages),
		)
		if err != nil {
			return nil, nil, eris.Wrap(err, "emit C# proto")
		}
		csProtos = generateAll(p)
	}
	return goProtos, csProtos, nil
}

// resolveOutputPackage creates and stubs the --go-out package, and answers its proto identity. It runs
// before discovery because a backend's wire.gen.go imports its own gen/ tree: on a first run there is
// nothing for the type-checker to resolve until that stub exists. --no-emit writes nothing, so it skips
// this entirely and therefore needs a backend that already type-checks.
func (c *GenerateCmd) resolveOutputPackage(dir string) (string, error) {
	if c.NoEmit {
		return "", nil
	}
	return c.resolveGoPackage(dir)
}

// reportFindings prints what discovery found, then decides whether the run continues. The report says
// what is in the backend and the outcome says what this run did about it; they print as separate blocks
// because they answer different questions — the findings are worth reading whether or not the run went
// on to generate, and every reason for stopping then reads the same way. done means Run returns here.
func (c *GenerateCmd) reportFindings(res sdkgen.Result) (bool, error) {
	printReport(res.Violations, res.Orphans)
	printer.Infof("Discovered %d commands, %d events, %d components, %d system events.",
		res.CommandCount(), res.EventCount(), res.ComponentCount(), res.SystemEventCount())
	printer.NewLine(1)

	switch {
	case len(res.Violations) > 0:
		// The returned error is this run's outcome line. Printing one here as well would state the same
		// verdict twice, once below the report and once as the error.
		return true, eris.Errorf("Not generated — %d blocking finding(s). Fix them and re-run.",
			len(res.Violations))
	case c.NoEmit:
		printer.Successf("Not generated — --no-emit. Nothing blocks; a real run would generate.")
		printer.NewLine(1)
		return true, nil
	}
	return false, nil
}

func (c *GenerateCmd) Run(ctx context.Context) error {
	remote := isRemoteSource(c.Source)
	if err := c.validate(remote); err != nil {
		return err
	}

	// A remote git URL is shallow-cloned to a temp dir; a local path is used in
	// place. cleanup removes the clone (no-op for a local path).
	dir, cleanup, err := resolveSource(ctx, c.Source, c.Ref)
	if err != nil {
		return err
	}
	defer cleanup()

	printEngineCheck(dir)

	goPkg, err := c.resolveOutputPackage(dir)
	if err != nil {
		return err
	}

	res, err := sdkgen.Discover(dir)
	if err != nil {
		if le, ok := errors.AsType[*sdkgen.LoadError](err); ok {
			printer.NewLine(1)
			printer.Errorln("DISCOVERY FAILED — the backend has type errors outside the generated wire " +
				"layer (which this command rewrites for you). Fix these, then re-run:")
			for _, e := range le.Errors {
				printer.Infoln("    · " + e)
			}
			printer.NewLine(1)
			return eris.New("sdk generate: backend doesn't type-check — nothing generated")
		}
		return eris.Wrap(err, "discover commands")
	}
	if done, rerr := c.reportFindings(res); done {
		return rerr
	}

	// Two independent passes, each gated by its output flag and filtered by message targets:
	//   Go — every locally-defined wire type (commands, events, components, system events).
	//   C# — the client-facing subset (commands/events/components) + mirrored plugin types.
	// System events carry only the Go target, so they never enter the C# set — never shipped to a client.
	// goOutImport anchors each Go file's go_package; res.Module anchors proto package names (one per dir).
	goOutImport := sdkgen.GoImportPath(goPkg)
	// Resolve every generated-package path once, here, where --go-out is finally known. Everything
	// downstream reads those paths off the messages instead of working them out again per call site,
	// which is what used to let proto emission and Go emission name different packages for one type.
	res.ResolveGen(goOutImport)

	goProtos, csProtos, err := c.emitProtos(res, goOutImport)
	if err != nil {
		return err
	}

	printer.Infof("Emitting %d proto file(s).", max(len(goProtos), len(csProtos)))
	printer.NewLine(1)

	// EnsureImage/RunBufGo/RunBufCSharp shell out to Docker and capture their output, so nothing prints while they run —
	// and the first run builds the toolchain image, which takes minutes. Drive an elapsed-time spinner so
	// it's visibly alive instead of looking hung; the captured Docker output still surfaces on error.
	var goWork, csWork string
	if err := spinner.Run(ctx,
		"Building codegen toolchain + generating (first run builds the image, may take a few minutes)",
		func(ctx context.Context) error {
			tag, ierr := sdkgen.EnsureImage(ctx, sdkgen.GenOptions{
				GoVersion:     c.GoVersion,
				CSharpVersion: c.CSharpVersion,
			})
			if ierr != nil {
				return eris.Wrap(ierr, "ensure buf image")
			}
			if len(goProtos) > 0 {
				goWork, ierr = sdkgen.RunBufGo(ctx, tag, goProtos)
				if ierr != nil {
					return eris.Wrap(ierr, "buf generate (Go)")
				}
			}
			if len(csProtos) > 0 {
				csWork, ierr = sdkgen.RunBufCSharp(ctx, tag, csProtos)
				if ierr != nil {
					return eris.Wrap(ierr, "buf generate (C#)")
				}
			}
			return nil
		}, spinner.Options{Elapsed: true}); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(goWork) }()
	defer func() { _ = os.RemoveAll(csWork) }()

	if err := c.writeOutputs(goWork, csWork, goPkg, res); err != nil {
		return err
	}
	if c.ProtoOut != "" {
		// Only this module's own schema. An import-only file is another module's, stamped with their
		// go_package — persisting it drops a foreign contract into this repo's proto tree, where it reads
		// as something this module owns and goes stale the moment they regenerate.
		if err := writeProtos(c.ProtoOut, ownProtos(goProtos)); err != nil {
			return eris.Wrap(err, "write proto schema")
		}
		printer.Successf("Wrote .proto schema → %s", c.ProtoOut)
	}

	if c.GoOut != "" {
		printer.Successf("Generated %d commands → Go: %s", res.CommandCount(), c.GoOut)
		if c.CsOut != "" {
			printer.Successf("                       → C#: %s", c.CsOut)
		}
	} else {
		printer.Successf("Generated %d commands → C#: %s", res.CommandCount(), c.CsOut)
	}
	printer.NewLine(1)
	return nil
}

// writeOutputs writes the generated Go and/or C# to their output dirs, each from its own pass's work dir.
func (c *GenerateCmd) writeOutputs(goWork, csWork, goPkg string, res sdkgen.Result) error {
	if c.GoOut != "" && goWork != "" {
		if err := c.writeGo(goWork, goPkg, res); err != nil {
			return err
		}
	}
	if c.CsOut != "" && csWork != "" {
		if err := c.writeCSharp(csWork); err != nil {
			return err
		}
		// Emit WireName partials after the protoc copy (writeCSharp cleans stale .cs first, then re-copies,
		// so this file must be written last to survive). Gives each generated type its backend Name().
		if s := sdkgen.EmitWireNamesCS(res); s != "" {
			if err := os.WriteFile(filepath.Join(c.CsOut, "WireNames.gen.cs"), []byte(s), 0o600); err != nil {
				return eris.Wrap(err, "write WireName partials")
			}
		}
	}
	return nil
}

// ownProtos drops the files that exist only so the schema resolves.
func ownProtos(protos []sdkgen.ProtoFileOut) []sdkgen.ProtoFileOut {
	out := make([]sdkgen.ProtoFileOut, 0, len(protos))
	for _, p := range protos {
		if !p.ImportOnly {
			out = append(out, p)
		}
	}
	return out
}

// writeProtos persists the emitted .proto schema to dir (mirroring each file's module-relative path), so
// the schema is a committed, reviewable artifact — the basis for buf breaking-change checks and for other
// modules to import these types. This is the same .proto that's compiled into the Go output.
func writeProtos(dir string, protos []sdkgen.ProtoFileOut) error {
	for _, pf := range protos {
		dst := filepath.Join(dir, filepath.FromSlash(pf.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, []byte(pf.Content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// withTarget returns the messages carrying the given codegen target.
func withTarget(msgs []sdkgen.Message, t sdkgen.Target) []sdkgen.Message {
	out := make([]sdkgen.Message, 0, len(msgs))
	for _, m := range msgs {
		if slices.Contains(m.Targets, t) {
			out = append(out, m)
		}
	}
	return out
}

// resolveGoPackage computes the proto package + file identity ("<import-path>;<pkg>"). When --go-out is
// set it anchors to that module (so the backend's Go output and the C# client agree); otherwise (client-
// only, including any remote source) there's no Go output to anchor to, so it derives the identity from
// the source backend's own module. Only the --go-out path needs the output dir created and stubbed.
func (c *GenerateCmd) resolveGoPackage(dir string) (string, error) {
	if c.GoOut == "" {
		// Client-only: anchor the proto identity to the backend's own module, read from the source dir.
		goPkg, err := sdkgen.DeriveGoPackage(dir)
		if err != nil {
			return "", eris.Wrap(err, "resolve the backend's Go module from <source> — point it at your "+
				"backend directory (the one with go.mod)")
		}
		return goPkg, nil
	}
	// Resolve the module before creating the dir, so a --go-out outside any Go module fails cleanly
	// instead of leaving a stray empty directory behind.
	goPkg, err := sdkgen.DeriveGoPackage(c.GoOut)
	if err != nil {
		return "", eris.Wrap(err, "resolve --go-out's Go module — point --go-out at a path inside your "+
			"backend module, e.g. <backend>/gen")
	}
	if err := os.MkdirAll(c.GoOut, 0o755); err != nil {
		return "", eris.Wrap(err, "create go output dir")
	}
	// Bootstrap: source code blank-imports the generated package, so it must be a loadable package
	// before discovery type-checks the source. Write a stub if the output dir has no Go files yet
	// (first run, or after a clean).
	if err := ensureGenStub(c.GoOut, sdkgen.GoPackageName(goPkg)); err != nil {
		return "", eris.Wrap(err, "stub generated package")
	}
	return goPkg, nil
}

// writeGo copies the generated proto structs into --go-out and emits the in-place wire bridges. Only
// valid for a local source (a remote clone is read-only and gets deleted), enforced by Run's validation.
func (c *GenerateCmd) writeGo(work, _ string, res sdkgen.Result) error {
	// Remove stale generated proto outputs so a renamed descriptor file doesn't leave a duplicate
	// that double-registers with protobuf. Recursive: source_relative output nests each .pb.go under
	// a subdir mirroring its source path, so a top-level glob would miss (and never clean) any of them.
	if err := removeGlobRecursive(c.GoOut, ".pb.go"); err != nil {
		return eris.Wrap(err, "clean stale Go output")
	}
	if err := copyTree(filepath.Join(work, "gen", "go"), c.GoOut); err != nil {
		return eris.Wrap(err, "copy generated Go")
	}
	// Emit the wire layer in-place: per source package, a wire.gen.go giving each type its ToProto/
	// FromProto converters, its SizeWire/AppendWire direct encoders and (for top-level types)
	// MarshalWire/UnmarshalWire, so it satisfies schema.Serializable and the ECS column's snapshot
	// contract. No registry and no init() — the methods live on the type itself.
	if err := emitWireBridges(res.Messages); err != nil {
		return eris.Wrap(err, "emit wire layer")
	}
	return nil
}

// writeCSharp copies the generated C# client SDK into --cs-out. C# output is nested (base_namespace=
// mirrors each package's namespace into subdirs, so same-named leaf packages don't collide), so stale
// .cs are cleaned recursively rather than only at the top level.
func (c *GenerateCmd) writeCSharp(work string) error {
	if err := os.MkdirAll(c.CsOut, 0o755); err != nil {
		return eris.Wrap(err, "create C# output dir")
	}
	if err := removeGlobRecursive(c.CsOut, ".cs"); err != nil {
		return eris.Wrap(err, "clean stale C# output")
	}
	if err := copyTree(filepath.Join(work, "gen", "csharp"), c.CsOut); err != nil {
		return eris.Wrap(err, "copy generated C#")
	}
	return nil
}

// ensureGenStub writes a minimal valid package file into dir if it has no Go
// files yet, so source that blank-imports the generated package still type-checks
// during discovery. Overwritten by the real generated files in the same run.
func ensureGenStub(dir, pkg string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return nil // already a package
		}
	}
	stub := "// Code generated by world sdk generate. DO NOT EDIT.\npackage " + pkg + "\n"
	return os.WriteFile(filepath.Join(dir, "doc.gen.go"), []byte(stub), 0o600)
}

// emitWireBridges writes a wire.gen.go into each source package that declares commands/events/nested
// structs (grouped by Message.Dir). module is the backend's Go module (anchors each type's gen package);
// goPkg is DeriveGoPackage's result for the --go-out module ("<import-path>;<pkg>"), whose import path is
// the root the per-package gen packages hang off.
func emitWireBridges(msgs []sdkgen.Message) error {
	byDir := map[string][]sdkgen.Message{}
	// A mirrored type owns no directory, so it renders no file of its own — it is looked up by the
	// packages that reference it, which emit its converters as free functions. An imported type owns no
	// directory either, but needs no entry here: its owner already generated the methods.
	mirrored := map[string]sdkgen.Message{}
	for _, m := range msgs {
		if m.Own.Mirrored() {
			mirrored[m.PkgPath+"."+m.Name] = m
		}
		if m.Dir == "" {
			continue
		}
		byDir[m.Dir] = append(byDir[m.Dir], m)
	}
	for dir, group := range byDir {
		src, err := sdkgen.RenderGoWire(group, mirrored)
		if err != nil {
			return err
		}
		if src == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, sdkgen.WireFileName), []byte(src), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// worldEngineModule is the module whose API the generated code targets.
const worldEngineModule = "github.com/argus-labs/world-engine"

// printEngineCheck is a pre-generation sys check: does the backend's world-engine satisfy the API the
// generated code targets? Prints a green line when it matches, a loud warning when it's too old (so a stale
// pin is caught here, before the generated code fails to compile against it), and skips quietly when the
// comparison can't be made (no module, a local replace, no direct dependency, or world-cli itself built
// without a usable version).
//
// The comparison uses the version the backend actually builds against — the replace target wins over the
// (informational) require line — so a versioned fork replace is judged by its replace version, not the
// stale require hint it overrides, and a path-only local replace (effective version unknowable) skips
// rather than warns. This is what `go build` resolves; the previous require-only read produced false
// "older than target / won't compile" warnings for fork replaces and a dead local-replace skip branch.
func printEngineCheck(dir string) {
	// The target is the world-engine release this binary was built from: the CLI ships in that module,
	// so it is also the release world-cli scaffolds from and whose API sdkgen discovers.
	printEngineCheckForTarget(dir, version.WorldEngine())
}

// printEngineCheckForTarget is printEngineCheck with an explicit target, so the comparison logic can be
// tested against any target.
func printEngineCheckForTarget(dir, target string) {
	if !semver.IsValid(target) {
		// Stay quiet rather than compare against a bogus target and print a false verdict.
		return
	}
	modRoot, ok := modfile.FindModuleRoot(dir)
	if !ok {
		printEngineCheckMessage(engineCheckNoModule, target, "")
		return
	}
	outcome, ver := classifyEngineCheck(target, modRoot)
	printEngineCheckMessage(outcome, target, ver)
}

// engineCheckOutcome is the result of comparing the backend's world-engine version against the release
// world-cli targets. It carries the decision out of printEngineCheck so the comparison
// (and its replace-awareness) can be tested without the build-info lookup, the printer, or shelling out.
type engineCheckOutcome uint8

const (
	// engineCheckOK means the backend's effective world-engine is at least as new as the target.
	engineCheckOK engineCheckOutcome = iota
	// engineCheckOld means the backend's effective world-engine is older than the target — the generated
	// code may not compile. "Effective" accounts for replace directives.
	engineCheckOld
	// engineCheckNoComparableVersion means the backend's world-engine pin names a commit rather than a
	// release — a path-only local replace (e.g. "=> ./local"), or a pseudo-version reached through
	// either a replace or a require — so there is no API level to compare and the check skips.
	engineCheckNoComparableVersion
	// engineCheckNoModule means no enclosing Go module was found at the source directory.
	engineCheckNoModule
	// engineCheckNotFound means world-engine is not a direct dependency of the backend's module.
	engineCheckNotFound
	// engineCheckReadError means the backend's go.mod could not be read or parsed.
	engineCheckReadError
	// engineCheckNoTarget means the target world-engine version is not a valid semver — there is no valid
	// target to compare against.
	engineCheckNoTarget
)

// classifyEngineCheck determines the world-engine compatibility outcome for the backend module at
// moduleRoot against target (the release world-cli scaffolds from). It consults replace
// directives before require — the effective version a backend actually builds against —
// so a versioned fork replace is compared by its replace target (not the stale require hint it overrides).
// A version that names a commit rather than a release — a path-only local replace, or a pseudo-version
// from either a replace or a require — carries no comparable API level, so it skips. It reads go.mod
// only; it does not shell out. The returned version is the effective one for engineCheckOK and
// engineCheckOld, "" otherwise.
func classifyEngineCheck(target, moduleRoot string) (engineCheckOutcome, string) {
	if !semver.IsValid(target) {
		return engineCheckNoTarget, ""
	}
	repVer, replaced, err := modfile.ModuleReplace(moduleRoot, worldEngineModule)
	if err != nil {
		return engineCheckReadError, ""
	}
	if replaced && repVer == "" {
		return engineCheckNoComparableVersion, ""
	}
	ver, found, err := modfile.ModuleDependencyVersion(moduleRoot, worldEngineModule)
	if err != nil {
		return engineCheckReadError, ""
	}
	if !found || ver == "" {
		return engineCheckNotFound, ""
	}
	if module.IsPseudoVersion(ver) {
		return engineCheckNoComparableVersion, ""
	}
	if modfile.IsOlderThan(ver, target) {
		return engineCheckOld, ver
	}
	return engineCheckOK, ver
}

// printEngineCheckMessage renders the verdict for one outcome via the printer. engineCheckNoTarget is
// silent; the skip outcomes print an informational line; engineCheckOld prints the loud warning.
func printEngineCheckMessage(outcome engineCheckOutcome, target, ver string) {
	switch outcome {
	case engineCheckNoTarget:
		// No valid target — stay quiet (see printEngineCheckForTarget).
	case engineCheckNoModule:
		printer.Infof("· world-engine: could not locate backend module — skipping compatibility check")
		printer.NewLine(1)
	case engineCheckNoComparableVersion:
		printer.Infof("· world-engine: backend pin has no comparable version — skipping compatibility check")
		printer.NewLine(1)
	case engineCheckNotFound:
		printer.Infof("· world-engine: not a direct dependency of the backend — skipping compatibility check")
		printer.NewLine(1)
	case engineCheckReadError:
		printer.Infof("· world-engine: could not read backend go.mod — skipping compatibility check")
		printer.NewLine(1)
	case engineCheckOld:
		printer.Notificationf("⚠ world-engine %s is older than %s that world-cli targets — bump it "+
			"(`go get %s@%s`) or the backend won't compile", ver, target, worldEngineModule, target)
		printer.NewLine(1)
	case engineCheckOK:
		printer.Successf("✓ world-engine %s — compatible (world-cli targets %s)", ver, target)
		printer.NewLine(1)
	}
}

// reportItem is one row of the generation report, reduced to what the grouped view needs.
type reportItem struct {
	category, where, gotype string
}

// printReport renders the generation report: every issue grouped by category, each category showing its
// one-line suggested fix once, then the affected symbols. One section — every finding stops generation.
func printReport(violations []sdkgen.Violation, orphans []string) {
	if len(violations) == 0 && len(orphans) == 0 {
		return
	}
	printer.NewLine(1)
	// Factual counts, no clean/not-clean verdict.
	printer.Infof("Generation report — %d violation(s), %d orphaned\n", len(violations), len(orphans))

	printUnregisteredCategories(violations)

	if len(violations) > 0 {
		items := make([]reportItem, 0, len(violations))
		for _, v := range violations {
			items = append(items, reportItem{v.Category, v.Where, v.GoType})
		}
		printer.NewLine(1)
		printer.Errorln("VIOLATIONS — these must be fixed for generation to succeed:")
		printReportSection(items)
	}
	if len(orphans) > 0 {
		printer.NewLine(1)
		printer.Notificationln(
			"ORPHANS — declared with Name() but never registered (RegisterCommand/RegisterEvent/" +
				"RegisterComponent/RegisterSystemEvent) and not nested anywhere; NOT generated. Forgot to register one?",
		)
		for _, o := range orphans {
			printer.Infoln("    · " + o)
		}
	}
	printer.NewLine(1)
}

// printReportSection groups items by category (stable first-seen order) and prints each group with its fix.
// printUnregisteredCategories reports findings whose category has no guidance registered. That is a
// generator defect, not a problem with the backend: the finding is printed with no fix, so it is called
// out separately rather than leaving the reader to go looking for a mistake in code that is probably
// fine.
func printUnregisteredCategories(violations []sdkgen.Violation) {
	seen := map[string]bool{}
	var unknown []string
	for _, v := range violations {
		if seen[v.Category] {
			continue
		}
		seen[v.Category] = true
		if sdkgen.GuidanceFor(v.Category).Proper == "" {
			unknown = append(unknown, v.Category)
		}
	}
	if len(unknown) == 0 {
		return
	}
	sort.Strings(unknown)
	printer.NewLine(1)
	printer.Errorf(
		"GENERATOR BUG — no guidance is registered for %s. Findings in %s were reported "+
			"without a fix and may not be a real problem with your code. Please report this, quoting the "+
			"lines below and the Go types of the fields they name.\n",
		strings.Join(quoteAll(unknown), ", "),
		pluralCategory(len(unknown)),
	)
}

// quoteAll quotes each element for display in a sentence.
func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strconv.Quote(s)
	}
	return out
}

func pluralCategory(n int) string {
	if n == 1 {
		return "that category"
	}
	return "those categories"
}

func printReportSection(items []reportItem) {
	var order []string
	byCat := map[string][]reportItem{}
	for _, it := range items {
		if _, seen := byCat[it.category]; !seen {
			order = append(order, it.category)
		}
		byCat[it.category] = append(byCat[it.category], it)
	}
	for _, cat := range order {
		group := byCat[cat]
		printer.NewLine(1)
		printer.Infof("  %s · %d\n", cat, len(group))
		if g := sdkgen.GuidanceFor(cat); g.Proper != "" {
			printer.Infoln("    Fix: " + g.Proper)
		}
		for _, it := range group {
			line := "    · " + it.where
			if it.gotype != "" && it.gotype != it.where {
				line += "  (" + it.gotype + ")"
			}
			printer.Infoln(line)
		}
	}
}

// removeGlobRecursive deletes every file under root whose name ends with ext (used to clear stale
// generated C#, which lands in namespace-mirrored subdirs). A missing root is not an error.
func removeGlobRecursive(root, ext string) error {
	r, err := os.OpenRoot(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer r.Close()
	return fs.WalkDir(r.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ext) {
			return r.Remove(filepath.FromSlash(p))
		}
		return nil
	})
}

// copyTree copies every file under src into dst, preserving the relative layout.
func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	in, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	return fs.WalkDir(in.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.FromSlash(p)
		if d.IsDir() {
			return out.MkdirAll(rel, 0o755)
		}
		data, err := in.ReadFile(rel)
		if err != nil {
			return err
		}
		return out.WriteFile(rel, data, 0o644)
	})
}
