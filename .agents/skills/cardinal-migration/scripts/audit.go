// Command audit scans a Cardinal game repository for the pre-migration world-engine API and,
// once code uses the new API, for types that are used without a matching Register* call.
//
// It parses source only (no type checking), so it runs before the game compiles against the
// new release. Findings are resolved through each file's imports, which keeps look-alike
// packages that merely reuse Cardinal's names out of the counts.
//
// Usage: go run audit.go [root]
package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	cardinalPath  = "github.com/argus-labs/world-engine/pkg/cardinal"
	pluginPrefix  = "github.com/argus-labs/world-engine/pkg/plugin/"
	dataPath      = pluginPrefix + "data"
	physics2dPath = pluginPrefix + "physics2d"
	maxExamples   = 3
)

// Old-API identifiers selected from the cardinal package (cardinal.X).
var oldCardinalSelectors = map[string]string{
	"BaseSystemState":         "state struct embeds cardinal.BaseSystemState",
	"WithCommand":             "cardinal.WithCommand[T] field",
	"WithEvent":               "cardinal.WithEvent[T] field",
	"WithSystemEventEmitter":  "cardinal.WithSystemEventEmitter[T] field",
	"WithSystemEventReceiver": "cardinal.WithSystemEventReceiver[T] field",
	"WithComponent":           "cardinal.WithComponent[T] field (v0.16.9)",
	"Ref":                     "cardinal.Ref[C] search field",
	"Contains":                "cardinal.Contains[...] search type",
	"Exact":                   "cardinal.Exact[...] search type",
	"RegisterSystem":          "cardinal.RegisterSystem(world, fn)",
	"RegisterSystemV2":        "cardinal.RegisterSystemV2(world, s)",
	"RegisterPlugin":          "cardinal.RegisterPlugin(world, p)",
}

// Methods that only existed in the v0.16.8/v0.16.9 interim API.
var interimMethods = map[string]string{
	"RegisterSystemV2":  "w.RegisterSystemV2(s) (v0.16.8/9)",
	"RegisterArchetype": "w.RegisterArchetype[A]() (v0.16.9)",
}

// physics2d package functions removed in v0.16.5 in favor of *Plugin methods.
var oldPhysicsFuncs = []string{
	"Raycast", "OverlapAABB", "CircleSweep", "ResetRuntime", "WorldID",
	"FlushBufferedContacts", "SetStepContactEmitter",
}

type kind string

const (
	kindComponent   kind = "component"
	kindCommand     kind = "command"
	kindEvent       kind = "event"
	kindSystemEvent kind = "system event"
)

var registerMethods = map[string]kind{
	"RegisterComponent":   kindComponent,
	"RegisterCommand":     kindCommand,
	"RegisterEvent":       kindEvent,
	"RegisterSystemEvent": kindSystemEvent,
}

// typeRef names a type by package import path (or directory for unresolved local packages).
type typeRef struct{ pkg, name string }

func (t typeRef) String() string { return t.pkg + "." + t.name }

type use struct {
	ref typeRef
	pos string
}

type archetypeUse struct {
	expr ast.Expr // struct type literal or named type
	file *fileInfo
	pos  string
}

type fileInfo struct {
	path     string
	pkg      string            // import path of the file's package
	imports  map[string]string // local name -> import path
	cardinal bool              // imports pkg/cardinal
}

type structDecl struct {
	st   *ast.StructType
	file *fileInfo // declaring file, whose imports resolve the field types
}

type audit struct {
	root     string
	fset     *token.FileSet
	findings map[string][]string // label -> positions
	lookRoot map[string]bool     // dirs with Cardinal look-alike names but no cardinal import
	stale    []string            // wire.gen.go files without AppendWire
	pinned   []string            // go.mod lines pinning world-engine / go

	registered map[kind]map[typeRef]bool
	used       map[kind][]use
	archetypes []archetypeUse
	structs    map[typeRef]structDecl
	aliases    map[typeRef]typeRef // type X = Y
	named      map[typeRef]bool    // types with a Name() method: components, commands, events
	plugins    map[string]bool     // plugin import paths passed to RegisterPlugin
	funcs      map[typeRef]bool    // package-level functions
	systemArgs []use               // named RegisterSystem arguments, checked against funcs
	newAPI     bool

	pkgVars       map[typeRef]map[string]bool // package-level var -> plugin import paths
	pluginVarArgs []use                       // RegisterPlugin(x) not bound in the function
}

// scope is what a function's identifiers are known to hold, from parameter and var types and
// simple assignments. Names are not tracked per block.
type scope struct {
	cardinal map[string]bool            // a Cardinal World or Entity
	plugins  map[string]map[string]bool // plugin import paths the name was assigned from
}

const unresolvedPlugin = "w.RegisterPlugin(x) with x not traced to a plugin constructor: confirm which plugin"

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	os.Exit(run(root, os.Stdout))
}

// run audits root, writes the report to out and returns the exit code: 0 clean, 1 findings,
// 2 error.
func run(root string, out io.Writer) int {
	root, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit:", err)
		return 2
	}
	a := &audit{
		root:       root,
		fset:       token.NewFileSet(),
		findings:   map[string][]string{},
		lookRoot:   map[string]bool{},
		registered: map[kind]map[typeRef]bool{},
		used:       map[kind][]use{},
		structs:    map[typeRef]structDecl{},
		aliases:    map[typeRef]typeRef{},
		named:      map[typeRef]bool{},
		plugins:    map[string]bool{},
		funcs:      map[typeRef]bool{},
		pkgVars:    map[typeRef]map[string]bool{},
	}
	for _, k := range []kind{kindComponent, kindCommand, kindEvent, kindSystemEvent} {
		a.registered[k] = map[typeRef]bool{}
	}
	if err := a.walk(); err != nil {
		fmt.Fprintln(os.Stderr, "audit:", err)
		return 2
	}
	a.resolveArchetypes()
	for _, u := range a.systemArgs {
		if a.funcs[u.ref] {
			a.add("w.RegisterSystem(fn) with a function (v0.16.8/9)", u.pos)
		}
	}
	for _, u := range a.pluginVarArgs {
		if !a.creditPlugin(a.pkgVars[u.ref]) {
			a.add(unresolvedPlugin, u.pos)
		}
	}
	if a.report(out) {
		return 1
	}
	return 0
}

// walk parses the module that encloses the root. Files under the root are audited. Files
// elsewhere in the module only contribute declarations (Name methods, structs, aliases,
// functions), so a shard's uses of shared packages still resolve.
func (a *audit) walk() error {
	modules := map[string]string{} // module root dir -> module path
	moduleRoot := a.root
	// The root may sit inside a module; resolve its enclosing go.mod too.
	for d := a.root; !fileExists(filepath.Join(d, "go.mod")) && filepath.Dir(d) != d; {
		d = filepath.Dir(d)
		if mod := readModulePath(filepath.Join(d, "go.mod"), a); mod != "" {
			modules[d] = mod
			moduleRoot = d
		}
	}
	return filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		audited := within(a.root, path)
		if d.IsDir() {
			// Skip what the go tool ignores, but never a directory that holds the root.
			skip := strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" ||
				name == "vendor" || name == "node_modules" || name == "gen"
			if skip && !within(path, a.root) {
				return filepath.SkipDir
			}
			pins := a
			if !audited {
				pins = nil
			}
			if mod := readModulePath(filepath.Join(path, "go.mod"), pins); mod != "" {
				modules[path] = mod
			}
			return nil
		}
		switch {
		case name == "wire.gen.go" && audited:
			if b, err := os.ReadFile(path); err == nil && !strings.Contains(string(b), "AppendWire(") {
				a.stale = append(a.stale, a.rel(path))
			}
			return nil
		case !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".pb.go"):
			return nil
		}
		a.parse(path, importPathOf(filepath.Dir(path), modules), audited)
		return nil
	})
}

// parse records the file's declarations and, when audited, its old API, hazards and uses.
func (a *audit) parse(path, pkg string, audited bool) {
	f, err := parser.ParseFile(a.fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		if audited {
			a.add("unparseable file (fix syntax first)", a.rel(path))
		}
		return
	}
	fi := &fileInfo{path: path, pkg: pkg, imports: map[string]string{}}
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		local := p[strings.LastIndex(p, "/")+1:]
		if imp.Name != nil {
			local = imp.Name.Name
		}
		fi.imports[local] = p
		fi.cardinal = fi.cardinal || p == cardinalPath
	}
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil {
			a.funcs[typeRef{pkg, fd.Name.Name}] = true
			continue
		}
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv != nil && fd.Name.Name == "Name" {
			if ref, ok := fi.resolve(fd.Recv.List[0].Type); ok {
				a.named[ref] = true
			}
			continue
		}
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				sc := newScope()
				sc.bindSpec(vs, fi)
				for name, paths := range sc.plugins {
					ref := typeRef{pkg, name}
					if a.pkgVars[ref] == nil {
						a.pkgVars[ref] = map[string]bool{}
					}
					maps.Copy(a.pkgVars[ref], paths) // build-tagged files may disagree
				}
			}
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if st, ok := ts.Type.(*ast.StructType); ok {
				a.structs[typeRef{pkg, ts.Name.Name}] = structDecl{st, fi}
			} else if ts.Assign.IsValid() {
				if target, ok := fi.resolve(ts.Type); ok {
					a.aliases[typeRef{pkg, ts.Name.Name}] = target
				}
			}
		}
	}
	if audited {
		a.inspect(f, fi)
	}
}

func (a *audit) inspect(f *ast.File, fi *fileInfo) {
	cardinalName := fi.localName(cardinalPath)
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			a.selector(n, fi, cardinalName)
		case *ast.RangeStmt:
			if n.Key != nil && n.Value != nil && isMethodCall(n.X, "Iter") && cardinalName != "" {
				a.add("two-variable search iteration: for id, row := range s.Iter()", a.pos(n))
			}
		case *ast.AssignStmt:
			if len(n.Lhs) == 3 && len(n.Rhs) == 1 && isMethodCall(n.Rhs[0], "Single") && cardinalName != "" {
				a.add("three-value Single(): id, row, err := ...Single()", a.pos(n))
			}
		}
		return true
	})
	for _, decl := range f.Decls {
		sc := scopeOf(decl, fi)
		ast.Inspect(decl, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				a.call(call, fi, sc)
			}
			return true
		})
	}
}

// scopeOf binds the identifiers declared anywhere in decl, including nested function literals.
func scopeOf(decl ast.Decl, fi *fileInfo) scope {
	sc := newScope()
	if _, ok := decl.(*ast.FuncDecl); !ok {
		return sc
	}
	ast.Inspect(decl, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			for _, name := range n.Names {
				sc.bindType(name.Name, n.Type, fi)
			}
		case *ast.ValueSpec:
			sc.bindSpec(n, fi)
		case *ast.AssignStmt:
			switch {
			case len(n.Lhs) == len(n.Rhs):
				for i := range n.Lhs {
					sc.bindValue(n.Lhs[i], n.Rhs[i], fi)
				}
			case len(n.Rhs) == 1: // e, err := ...Single(); w, err := cardinal.NewWorld(opts)
				sc.bindValue(n.Lhs[0], n.Rhs[0], fi)
			}
		case *ast.RangeStmt:
			if id, ok := n.Key.(*ast.Ident); ok && isMethodCall(n.X, "Iter") && fi.cardinal {
				sc.cardinal[id.Name] = true
			}
		}
		return true
	})
	return sc
}

func newScope() scope {
	return scope{cardinal: map[string]bool{}, plugins: map[string]map[string]bool{}}
}

func (sc scope) bindSpec(vs *ast.ValueSpec, fi *fileInfo) {
	for i, name := range vs.Names {
		if vs.Type != nil {
			sc.bindType(name.Name, vs.Type, fi)
		}
		if i < len(vs.Values) {
			sc.bindValue(name, vs.Values[i], fi)
		}
	}
}

func (sc scope) bindType(name string, typ ast.Expr, fi *fileInfo) {
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if sel, ok := typ.(*ast.SelectorExpr); ok && fi.isCardinal(sel.X) &&
		slices.Contains([]string{"World", "TestWorld", "Entity"}, sel.Sel.Name) {
		sc.cardinal[name] = true
	}
	sc.bindPlugin(name, typ, fi)
}

func (sc scope) bindValue(lhs, rhs ast.Expr, fi *fileInfo) {
	name := bindingName(lhs)
	if name == "" {
		return
	}
	if fi.cardinalValue(rhs) {
		sc.cardinal[name] = true
	}
	sc.bindPlugin(name, rhs, fi)
}

// bindingName names a variable or a field of one (r.plugin), or returns "".
func bindingName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		if t.Name != "_" {
			return t.Name
		}
	case *ast.SelectorExpr:
		if x := bindingName(t.X); x != "" {
			return x + "." + t.Sel.Name
		}
	}
	return ""
}

func (sc scope) bindPlugin(name string, expr ast.Expr, fi *fileInfo) {
	if p := fi.pluginPkg(expr); p != "" {
		if sc.plugins[name] == nil {
			sc.plugins[name] = map[string]bool{}
		}
		sc.plugins[name][p] = true
	}
}

// cardinalValue reports whether expr evaluates to a Cardinal World or Entity: cardinal.NewWorld,
// cardinal.NewTestWorld, w.Create[A](), w.Entity(id) or ...Single().
func (fi *fileInfo) cardinalValue(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || !fi.cardinal {
		return false
	}
	fun := call.Fun
	if idx, ok := fun.(*ast.IndexExpr); ok {
		sel, ok := idx.X.(*ast.SelectorExpr)
		return ok && sel.Sel.Name == "Create" && !fi.isImport(sel.X)
	}
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if fi.isImport(sel.X) {
		return fi.isCardinal(sel.X) && (sel.Sel.Name == "NewWorld" || sel.Sel.Name == "NewTestWorld")
	}
	return sel.Sel.Name == "Entity" || sel.Sel.Name == "Single"
}

func (fi *fileInfo) isImport(expr ast.Expr) bool {
	x, ok := expr.(*ast.Ident)
	return ok && fi.imports[x.Name] != ""
}

func (fi *fileInfo) isCardinal(expr ast.Expr) bool {
	x, ok := expr.(*ast.Ident)
	return ok && fi.imports[x.Name] == cardinalPath
}

// pluginPkg returns the world-engine plugin package that expr is built from or typed as:
// lobby.NewPlugin(cfg), &physics2d.Plugin{}, *physics2d.Plugin.
func (fi *fileInfo) pluginPkg(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.CallExpr:
		return fi.pluginPkg(t.Fun)
	case *ast.UnaryExpr:
		return fi.pluginPkg(t.X)
	case *ast.StarExpr:
		return fi.pluginPkg(t.X)
	case *ast.CompositeLit:
		return fi.pluginPkg(t.Type)
	case *ast.IndexExpr:
		return fi.pluginPkg(t.X)
	case *ast.IndexListExpr:
		return fi.pluginPkg(t.X)
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			if p := fi.imports[x.Name]; strings.HasPrefix(p, pluginPrefix) {
				return p
			}
			return ""
		}
		return fi.pluginPkg(t.X) // lobby.NewPlugin(cfg).WithX()
	}
	return ""
}

// creditPlugin records the plugin when paths names exactly one.
func (a *audit) creditPlugin(paths map[string]bool) bool {
	if len(paths) != 1 {
		return false
	}
	for p := range paths {
		a.plugins[p] = true
	}
	return true
}

func (a *audit) selector(sel *ast.SelectorExpr, fi *fileInfo, cardinalName string) {
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		if label, ok := interimMethods[sel.Sel.Name]; ok && cardinalName != "" {
			a.add(label, a.pos(sel))
		}
		return
	}
	switch path := fi.imports[x.Name]; {
	case path == cardinalPath:
		if label, ok := oldCardinalSelectors[sel.Sel.Name]; ok {
			a.add(label, a.pos(sel))
		}
	case path == physics2dPath && slices.Contains(oldPhysicsFuncs, sel.Sel.Name):
		a.add("physics2d package function (now a *Plugin method)", a.pos(sel))
	case path == "" && cardinalName != "":
		if label, ok := interimMethods[sel.Sel.Name]; ok {
			a.add(label, a.pos(sel))
		}
	case path != "" && !strings.HasPrefix(path, "github.com/argus-labs/world-engine/") &&
		(sel.Sel.Name == "BaseSystemState" || sel.Sel.Name == "WithCommand"):
		a.lookRoot[a.rel(filepath.Dir(fi.path))] = true
	}
}

// call records registrations and uses of the new API.
func (a *audit) call(call *ast.CallExpr, fi *fileInfo, sc scope) {
	// Generic method calls: w.RegisterComponent[T](), w.Commands[T](), e.Get[T](), w.Create[A]().
	if idx, ok := call.Fun.(*ast.IndexExpr); ok {
		sel, ok := idx.X.(*ast.SelectorExpr)
		if !ok {
			return
		}
		if x, ok := sel.X.(*ast.Ident); ok && fi.imports[x.Name] != "" {
			// Package-level generic function, e.g. data.Get[T]().
			if fi.imports[x.Name] == dataPath && sel.Sel.Name == "Get" && len(call.Args) == 0 {
				a.add("data.Get[T]() without the plugin argument", a.pos(call))
			}
			return
		}
		name := sel.Sel.Name
		if k, ok := registerMethods[name]; ok {
			a.newAPI = true
			if ref, ok := fi.resolve(idx.Index); ok {
				a.registered[k][ref] = true
			}
			return
		}
		switch name {
		case "Commands":
			a.useType(kindCommand, idx.Index, fi, call)
		case "SystemEvents":
			a.useType(kindSystemEvent, idx.Index, fi, call)
		case "Get", "Has", "Remove":
			a.useType(kindComponent, idx.Index, fi, call)
		case "Contains", "Exact", "Create":
			if fi.cardinal {
				a.archetypes = append(a.archetypes, archetypeUse{idx.Index, fi, a.pos(call)})
			}
		}
		return
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	if x, ok := sel.X.(*ast.Ident); ok && fi.imports[x.Name] != "" {
		return // package function, not a World or Entity method
	}
	var arg ast.Expr
	var k kind
	switch sel.Sel.Name {
	case "RegisterSystem":
		if len(call.Args) == 0 {
			return
		}
		if _, ok := call.Args[0].(*ast.FuncLit); ok {
			a.add("w.RegisterSystem(fn) with a function (v0.16.8/9)", a.pos(call))
		} else if ref, ok := fi.resolve(call.Args[0]); ok {
			a.systemArgs = append(a.systemArgs, use{ref, a.pos(call)})
		}
		return
	case "RegisterPlugin":
		if len(call.Args) == 1 {
			a.registerPlugin(call.Args[0], fi, sc, call)
		}
		return
	case "Set":
		k = kindComponent
	case "Broadcast":
		k = kindEvent
	case "EmitSystemEvent":
		k = kindSystemEvent
	case "SendTo":
		k = kindEvent
		if len(call.Args) != 2 {
			return
		}
		arg = call.Args[1]
	default:
		return
	}
	if arg == nil {
		if len(call.Args) != 1 {
			return
		}
		arg = call.Args[0]
	}
	if u, ok := arg.(*ast.UnaryExpr); ok && u.Op == token.AND {
		// Only where the receiver is known to be a World or Entity: big.Int.Set(&x) is valid.
		if recv, ok := sel.X.(*ast.Ident); (ok && sc.cardinal[recv.Name]) || fi.cardinalValue(sel.X) {
			a.add(fmt.Sprintf("pointer passed to %s (pass the value)", sel.Sel.Name), a.pos(call))
		}
		return
	}
	// Only composite literals name their type. A variable (w.SendTo(p, ev)) is not a recorded use.
	if lit, ok := arg.(*ast.CompositeLit); ok && lit.Type != nil {
		a.useType(k, lit.Type, fi, call)
	}
}

func (a *audit) useType(k kind, expr ast.Expr, fi *fileInfo, at ast.Node) {
	if !fi.cardinal {
		return // look-alike code with its own Get/Set/Broadcast
	}
	if ref, ok := fi.resolve(expr); ok {
		a.used[k] = append(a.used[k], use{ref, a.pos(at)})
	}
}

// resolveArchetypes turns each Contains/Exact/Create type argument into component uses.
func (a *audit) resolveArchetypes() {
	for _, au := range a.archetypes {
		var st *ast.StructType
		fi := au.file
		switch t := au.expr.(type) {
		case *ast.StructType:
			st = t
		default:
			ref, ok := fi.resolve(t)
			if !ok {
				continue
			}
			ref = a.canonical(ref)
			// A component in archetype position panics, or for an empty tag silently matches
			// every entity. Components are the types that declare Name().
			if a.named[ref] {
				a.add("component passed where an archetype belongs: wrap it in struct{ C }", au.pos)
				continue
			}
			decl, ok := a.structs[ref]
			if !ok {
				continue
			}
			st, fi = decl.st, decl.file
		}
		for _, field := range st.Fields.List {
			if ref, ok := fi.resolve(field.Type); ok {
				a.used[kindComponent] = append(a.used[kindComponent], use{ref, au.pos})
			}
		}
	}
}

var builtin = map[string]bool{
	"bool": true, "string": true, "int": true, "int8": true, "int16": true, "int32": true,
	"int64": true, "uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true, "byte": true, "rune": true,
}

func (a *audit) report(out io.Writer) (failed bool) {
	w := bufio.NewWriter(out)
	defer w.Flush()

	fmt.Fprintf(w, "# Cardinal migration audit: %s\n\n", a.root)
	fmt.Fprintln(w, "## Pinned versions")
	if len(a.pinned) == 0 {
		fmt.Fprintln(w, "- no go.mod requires github.com/argus-labs/world-engine")
	}
	for _, p := range a.pinned {
		fmt.Fprintln(w, "-", p)
	}

	fmt.Fprintln(w, "\n## Look-alike packages (not Cardinal; do not rewrite)")
	if len(a.lookRoot) == 0 {
		fmt.Fprintln(w, "- none")
	}
	for _, d := range sortedKeys(a.lookRoot) {
		fmt.Fprintln(w, "-", d)
	}

	fmt.Fprintln(w, "\n## Old API and hazards")
	if len(a.findings) == 0 {
		fmt.Fprintln(w, "- none")
	}
	labels := sortedKeys(a.findings)
	sort.SliceStable(labels, func(i, j int) bool { return len(a.findings[labels[i]]) > len(a.findings[labels[j]]) })
	for _, label := range labels {
		failed = true
		pos := a.findings[label]
		fmt.Fprintf(w, "- %4d  %s\n", len(pos), label)
		for _, p := range examples(pos) {
			fmt.Fprintf(w, "        %s\n", p)
		}
	}

	fmt.Fprintln(w, "\n## Stale generated wire code (run world sdk generate)")
	if len(a.stale) == 0 {
		fmt.Fprintln(w, "- none")
	}
	for _, s := range a.stale {
		failed = true
		fmt.Fprintln(w, "-", s)
	}

	fmt.Fprintln(w, "\n## Used but never registered")
	if !a.newAPI {
		fmt.Fprintln(w, "- skipped: no RegisterComponent/Command/Event/SystemEvent calls found yet")
		return failed
	}
	missing := 0
	for _, k := range []kind{kindComponent, kindCommand, kindEvent, kindSystemEvent} {
		registered := map[typeRef]bool{}
		for ref := range a.registered[k] {
			registered[a.canonical(ref)] = true
		}
		seen := map[typeRef]bool{}
		for _, u := range a.used[k] {
			ref := a.canonical(u.ref)
			file, _, _ := strings.Cut(u.pos, ":")
			if registered[ref] || seen[ref] || a.pluginOwned(ref) || a.lookRoot[filepath.Dir(file)] {
				continue
			}
			seen[ref] = true
			missing++
			fmt.Fprintf(w, "- %s %s (first use %s)\n", k, u.ref, u.pos)
		}
	}
	if missing == 0 {
		fmt.Fprintln(w, "- none")
	}
	fmt.Fprintln(w, "\nNotes: the registration check covers the directory given and is source-only. In a")
	fmt.Fprintln(w, "monorepo, run it once per shard directory. It counts a use only where a composite")
	fmt.Fprintln(w, "literal or type argument names the type, not values passed through variables or")
	fmt.Fprintln(w, "helpers; RunDST per shard is the backstop.")
	return failed || missing > 0
}

// examples returns up to maxExamples positions, at most one per file.
func examples(pos []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range pos {
		file, _, _ := strings.Cut(p, ":")
		if seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, p)
		if len(out) == maxExamples {
			break
		}
	}
	return out
}

// pluginOwned reports whether ref belongs to a plugin that some world registers. The plugin
// registers its own types; games only register plugin commands they receive without the plugin.
// canonical follows type aliases to the declared type.
func (a *audit) canonical(ref typeRef) typeRef {
	for range 10 {
		target, ok := a.aliases[ref]
		if !ok {
			break
		}
		ref = target
	}
	return ref
}

func (a *audit) pluginOwned(ref typeRef) bool {
	for p := range a.plugins {
		if strings.HasPrefix(ref.pkg, p) {
			return true
		}
	}
	return false
}

func (a *audit) add(label, pos string) { a.findings[label] = append(a.findings[label], pos) }

func (a *audit) pos(n ast.Node) string {
	p := a.fset.Position(n.Pos())
	return fmt.Sprintf("%s:%d", a.rel(p.Filename), p.Line)
}

func (a *audit) rel(path string) string {
	if r, err := filepath.Rel(a.root, path); err == nil {
		return r
	}
	return path
}

func (fi *fileInfo) localName(path string) string {
	for local, p := range fi.imports {
		if p == path {
			return local
		}
	}
	return ""
}

// resolve names the type in expr, stripping pointers and generic arguments.
func (fi *fileInfo) resolve(expr ast.Expr) (typeRef, bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		if builtin[t.Name] {
			return typeRef{}, false
		}
		return typeRef{fi.pkg, t.Name}, true
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok && fi.imports[x.Name] != "" {
			return typeRef{fi.imports[x.Name], t.Sel.Name}, true
		}
	case *ast.StarExpr:
		return fi.resolve(t.X)
	case *ast.IndexExpr:
		return fi.resolve(t.X)
	case *ast.IndexListExpr:
		return fi.resolve(t.X)
	}
	return typeRef{}, false
}

// registerPlugin credits the plugin package of a RegisterPlugin argument: lobby.NewPlugin(cfg),
// or a variable or field assigned from one in the same function, or a package-level variable.
func (a *audit) registerPlugin(arg ast.Expr, fi *fileInfo, sc scope, call *ast.CallExpr) {
	if p := fi.pluginPkg(arg); p != "" {
		a.plugins[p] = true
		return
	}
	if u, ok := arg.(*ast.UnaryExpr); ok {
		arg = u.X
	}
	if _, ok := arg.(*ast.CompositeLit); ok {
		return // the game's own plugin type
	}
	name := bindingName(arg)
	switch {
	case sc.plugins[name] != nil:
		if !a.creditPlugin(sc.plugins[name]) {
			a.add(unresolvedPlugin, a.pos(call))
		}
	case name != "" && !strings.Contains(name, "."):
		a.pluginVarArgs = append(a.pluginVarArgs, use{typeRef{fi.pkg, name}, a.pos(call)})
	default:
		a.add(unresolvedPlugin, a.pos(call))
	}
}

// within reports whether path is dir or lies under it.
func within(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isMethodCall(expr ast.Expr, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name
}

func readModulePath(gomod string, a *audit) string {
	b, err := os.ReadFile(gomod)
	if err != nil {
		return ""
	}
	var mod string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "module "):
			mod = strings.TrimSpace(strings.TrimPrefix(line, "module "))
		case a == nil:
		case strings.HasPrefix(line, "go "):
			a.pinned = append(a.pinned, fmt.Sprintf("%s: %s", a.rel(gomod), line))
		case strings.Contains(line, "github.com/argus-labs/world-engine "):
			a.pinned = append(a.pinned, fmt.Sprintf("%s: %s", a.rel(gomod), strings.TrimPrefix(line, "require ")))
		}
	}
	return mod
}

// importPathOf maps a directory to its import path using the nearest enclosing module.
func importPathOf(dir string, modules map[string]string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if mod, ok := modules[d]; ok {
			rel, _ := filepath.Rel(d, dir)
			if rel == "." {
				return mod
			}
			return mod + "/" + filepath.ToSlash(rel)
		}
		if parent := filepath.Dir(d); parent == d {
			return filepath.ToSlash(dir)
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
