package ecs

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"strings"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// Snapshot migration converts stored component values whose shape no longer matches the code.
//
// A migration is a struct whose fields declare what it reads and writes, plus a Migrate method that
// does the conversion. The engine reads those fields once at registration and never again:
//
//	type RenamedV1ToCurrent struct {
//	    Old ecs.In[earlyaccess.Renamed]
//	    New ecs.Out[fullrelease.Renamed]
//	}
//
//	func (m *RenamedV1ToCurrent) Migrate() { m.New.Set(fullrelease.Renamed{After: m.Old.Get().Before}) }
//
// What makes one run is the stored shape, nothing else. A save whose columns already carry current
// shapes runs no migrations at all, which is what an ordinary restart is, so restoring the same
// save twice can never convert it twice.

// -------------------------------------------------------------------------------------------------
// Declaring a migration
// -------------------------------------------------------------------------------------------------

// Migration is a declared conversion. The engine calls Migrate once per entity whose stored values
// match the declared inputs.
type Migration interface {
	Migrate()
}

// In declares a required input: a value the entity must hold, in the shape T describes.
//
// T is usually a retired struct — the shape as an older build wrote it — which is why it does not
// have to be a registered component. It exists to decode old bytes and nothing else.
type In[T Component] struct {
	value T
}

// Get returns the stored value, decoded into the declared shape.
func (i *In[T]) Get() T { return i.value }

func (i *In[T]) inputName() string {
	var zero T
	return zero.Name()
}

func (i *In[T]) inputShape() uint64 {
	var zero T
	return shapeHash(zero)
}

func (i *In[T]) inputRequired() bool { return true }

// inputReset clears the value between entities. A required input is always filled before its
// migration runs, so this only matters for keeping stale data out of a failed restore.
func (i *In[T]) inputReset() {
	var zero T
	i.value = zero
}

// inputSet takes a value straight from the previous migration's output, rather than from a stored
// payload. The assertion cannot fail in practice: the engine only pairs an output with an input
// when both declare the same name and the same shape, and matching shapes means matching types.
func (i *In[T]) inputSet(value any) error {
	typed, ok := value.(T)
	if !ok {
		var zero T
		return eris.Errorf("migration produced %T for %q, which expects %T", value, zero.Name(), zero)
	}
	i.value = typed
	return nil
}

func (i *In[T]) inputDecode(data []byte) error {
	var zero T
	decoded, err := zero.UnmarshalWire(data)
	if err != nil {
		return eris.Wrapf(err, "failed to decode stored %q", zero.Name())
	}
	value, ok := decoded.(T)
	if !ok {
		return eris.Errorf("decoding %q produced %T, not the declared shape", zero.Name(), decoded)
	}
	i.value = value
	return nil
}

// Optional declares an input the entity may lack.
//
// A merge usually needs one: the components being merged were added at different times, so an
// entity from before the later one has only some of them. A required input would skip that entity
// entirely; an optional one lets the migration decide what its absence means.
//
// It does not affect ordering. If something produces T, that still has to run first, because an
// optional input the engine could have filled but did not is a wrong answer, not a missing one.
type Optional[T Component] struct {
	value   T
	present bool
}

// Get returns the value and whether the entity had one. The value is the zero T when it did not,
// which is why the boolean matters: zero is a legitimate stored value.
func (o *Optional[T]) Get() (T, bool) { return o.value, o.present }

func (o *Optional[T]) inputName() string {
	var zero T
	return zero.Name()
}

func (o *Optional[T]) inputShape() uint64 {
	var zero T
	return shapeHash(zero)
}

func (o *Optional[T]) inputRequired() bool { return false }

func (o *Optional[T]) inputReset() {
	var zero T
	o.value, o.present = zero, false
}

func (o *Optional[T]) inputSet(value any) error {
	typed, ok := value.(T)
	if !ok {
		var zero T
		return eris.Errorf("migration produced %T for %q, which expects %T", value, zero.Name(), zero)
	}
	o.value, o.present = typed, true
	return nil
}

func (o *Optional[T]) inputDecode(data []byte) error {
	var zero T
	decoded, err := zero.UnmarshalWire(data)
	if err != nil {
		return eris.Wrapf(err, "failed to decode stored %q", zero.Name())
	}
	value, ok := decoded.(T)
	if !ok {
		return eris.Errorf("decoding %q produced %T, not the declared shape", zero.Name(), decoded)
	}
	o.value, o.present = value, true
	return nil
}

// Out declares an output: a value the migration produces, in the shape the current code declares.
type Out[T Component] struct {
	value T
	set   bool
}

// Set records the produced value. A migration that declares an output and never sets it has
// produced nothing, which the engine treats as an error rather than as a zero value.
func (o *Out[T]) Set(value T) {
	o.value, o.set = value, true
}

func (o *Out[T]) outputName() string {
	var zero T
	return zero.Name()
}

func (o *Out[T]) outputShape() uint64 {
	var zero T
	return shapeHash(zero)
}

// outputValue hands the produced value to whatever consumes it. A chained migration takes it
// through inputSet below, so an intermediate value never round-trips through bytes.
func (o *Out[T]) outputValue() any { return o.value }

func (o *Out[T]) outputWasSet() bool { return o.set }

func (o *Out[T]) outputReset() {
	var zero T
	o.value, o.set = zero, false
}

// outputApply writes the produced value onto the entity, which must already be in an archetype
// holding the component. T is known here, so this stays on the generic path rather than needing a
// dynamic setter.
func (o *Out[T]) outputApply(ws *worldState, eid EntityID) error {
	return ws.setComponent(eid, o.value)
}

// The engine reaches a migration's declared fields through these, which only the types above
// implement. Unexported methods keep them closed: a migration author can declare In and Out fields
// but cannot supply their own.
type (
	migrationInput interface {
		inputName() string
		inputShape() uint64
		inputRequired() bool
		inputReset()
		inputDecode(data []byte) error
		inputSet(value any) error
	}
	migrationOutput interface {
		outputName() string
		outputShape() uint64
		outputWasSet() bool
		outputReset()
		outputValue() any
		outputApply(ws *worldState, eid EntityID) error
	}
)

// boundMigration is a registered migration with its fields located. The struct instance is reused
// for every entity, so running one costs no allocation and no reflection.
type boundMigration struct {
	run     Migration
	inputs  []migrationInput
	outputs []migrationOutput
	name    string

	// A migration declaring All, Put or Create sees the whole world rather than one entity, so it
	// runs once after every entity is restored instead of once per entity.
	wholeWorld   bool
	worldInputs  []worldInput
	worldOutputs []worldOutput
}

// storedShape identifies a stored value: a component name plus the shape it was written in. It is
// what a migration's input declares and what a snapshot column resolves to.
type storedShape struct {
	name  string
	shape uint64
}

// migrationManager holds every registered migration and the indexes the planner needs.
//
// A shape has at most one producer and at most one consumer. Two producers would make the value an
// entity ends up with depend on which ran last, and two consumers would make it depend on which
// the engine picked, so both are refused at registration rather than resolved by luck.
type migrationManager struct {
	all      []*boundMigration
	byInput  map[storedShape]*boundMigration
	byOutput map[storedShape]*boundMigration

	// order is every migration in dependency order, computed on first use and cleared whenever a
	// migration is registered.
	order []*boundMigration
	// plans memoises the route for a set of stored shapes. Entities holding the same shapes take
	// the same route, so it is worked out once per distinct set rather than once per entity. Only
	// a restore writes here, and a restore holds the world lock.
	plans map[string]*routePlan
}

func newMigrationManager() migrationManager {
	return migrationManager{
		byInput:  map[storedShape]*boundMigration{},
		byOutput: map[storedShape]*boundMigration{},
		plans:    map[string]*routePlan{},
	}
}

// RegisterMigration binds a migration's declared fields and records the stored shape it consumes.
//
// Everything reflective happens here, once, before any save is read.
func (w *World) RegisterMigration(m Migration) error {
	bound, err := bindMigration(m)
	if err != nil {
		return err
	}

	mgr := &w.state.migrations
	for _, input := range bound.inputs {
		key := inputKey(input)
		if existing, taken := mgr.byInput[key]; taken {
			return eris.Errorf("migrations %s and %s both consume %q in the same shape",
				existing.name, bound.name, key.name)
		}
		mgr.byInput[key] = bound
	}
	for _, output := range bound.outputs {
		key := outputKey(output)
		if existing, taken := mgr.byOutput[key]; taken {
			return eris.Errorf("migrations %s and %s both produce %q in the same shape",
				existing.name, bound.name, key.name)
		}
		mgr.byOutput[key] = bound
	}

	mgr.all = append(mgr.all, bound)
	// Anything worked out before this migration existed may now be different.
	mgr.order = nil
	clear(mgr.plans)
	return nil
}

func inputKey(i migrationInput) storedShape {
	return storedShape{name: i.inputName(), shape: i.inputShape()}
}

func outputKey(o migrationOutput) storedShape {
	return storedShape{name: o.outputName(), shape: o.outputShape()}
}

// bindMigration locates the In and Out fields of a migration struct.
func bindMigration(m Migration) (*boundMigration, error) {
	value := reflect.ValueOf(m)
	if value.Kind() != reflect.Pointer || value.Elem().Kind() != reflect.Struct {
		return nil, eris.Errorf("migration %T must be a pointer to a struct", m)
	}

	bound := &boundMigration{run: m, name: reflect.TypeOf(m).Elem().String()}
	fields := value.Elem()
	for i := range fields.NumField() {
		field := fields.Field(i)
		if !field.CanAddr() || !field.Addr().CanInterface() {
			continue // an unexported field cannot be a declaration
		}
		switch declared := field.Addr().Interface().(type) {
		case migrationInput:
			bound.inputs = append(bound.inputs, declared)
		case migrationOutput:
			bound.outputs = append(bound.outputs, declared)
		case worldInput:
			bound.worldInputs = append(bound.worldInputs, declared)
			bound.wholeWorld = true
		case worldOutput:
			bound.worldOutputs = append(bound.worldOutputs, declared)
			bound.wholeWorld = true
		}
	}

	if bound.wholeWorld {
		if len(bound.inputs) > 0 {
			// The two kinds are filled differently and run at different times, so a migration has
			// to be one or the other.
			return nil, eris.Errorf(
				"migration %s mixes per-entity inputs with whole-world fields", bound.name)
		}
		if len(bound.worldInputs) == 0 {
			return nil, eris.Errorf("migration %s writes the world but reads nothing", bound.name)
		}
		return bound, nil
	}

	if len(bound.inputs) == 0 {
		// A migration with no input would match every entity in the world.
		return nil, eris.Errorf("migration %s declares no input", bound.name)
	}
	for _, input := range bound.inputs {
		if input.inputRequired() {
			return bound, nil
		}
	}
	// Optional inputs alone say nothing about which entities the migration is for.
	return nil, eris.Errorf("migration %s declares only optional inputs", bound.name)
}

// -------------------------------------------------------------------------------------------------
// Resolving a snapshot against this build
// -------------------------------------------------------------------------------------------------
//
// Before an entity can be restored, each stored value has to be matched against what this build
// declares: which component it is, and whether its shape still fits. That is what the rest of this
// file plans over, which is why it lives here rather than in the restore path.

// resolvedName is one snapshot name table entry, matched against this build.
type resolvedName struct {
	name  string
	shape uint64      // the shape the value was stored in
	cid   ComponentID // invalidComponentID when this build does not register the name
	stale bool        // the stored shape is not the one this build declares
}

// resolveNameTable matches each name table entry against this build's components.
//
// Neither an unresolved name nor a changed shape is an error here. A component this build stopped
// registering, or one whose struct changed, only matters for the entities that carry a value, so
// both are recorded and judged per entity.
func (ws *worldState) resolveNameTable(table []string, storedShapes []uint64) ([]resolvedName, error) {
	if len(storedShapes) > 0 && len(storedShapes) != len(table) {
		return nil, eris.Errorf("snapshot has %d component shapes for %d name table entries",
			len(storedShapes), len(table))
	}

	resolved := make([]resolvedName, len(table))
	var seen bitmap.Bitmap
	for i, name := range table {
		entry := resolvedName{name: name, cid: invalidComponentID}
		if len(storedShapes) > 0 {
			entry.shape = storedShapes[i]
		}

		cid, err := ws.components.getID(name)
		if err != nil {
			// A component this build stopped registering. There is no current shape to compare
			// against, so the value cannot be carried over: a migration has to consume it or the
			// restore stops. That is case 2e, dropping a component.
			entry.stale = true
			resolved[i] = entry
			continue
		}
		if seen.Contains(cid) {
			return nil, eris.Errorf("snapshot name table repeats component %q", name)
		}
		seen.Set(cid)

		entry.cid = cid
		entry.stale = !shapesMatch(entry.shape, ws.components.shapes[cid])
		resolved[i] = entry
	}
	return resolved, nil
}

// storedColumn is one component value an entity holds in the snapshot.
type storedColumn struct {
	name    string
	payload []byte
	shape   uint64
	cid     ComponentID
	stale   bool
}

// storedColumns reads one entity's component list, checking the snapshot's own ordering rules.
func storedColumns(eid EntityID, ent *cardinalv1.Entity, resolved []resolvedName) ([]storedColumn, error) {
	idxs := ent.GetComponents()
	payloads := ent.GetPayloads()
	if len(idxs) != len(payloads) {
		return nil, eris.Errorf("snapshot entity %d has %d component indices but %d payloads",
			eid, len(idxs), len(payloads))
	}

	columns := make([]storedColumn, 0, len(idxs))
	last := int64(-1)
	for k, idx := range idxs {
		if int64(idx) <= last {
			return nil, eris.Errorf("snapshot entity %d component indices not strictly ascending", eid)
		}
		if int(idx) >= len(resolved) {
			return nil, eris.Errorf("snapshot entity %d component index %d outside the name table", eid, idx)
		}
		last = int64(idx)

		entry := resolved[idx]
		columns = append(columns, storedColumn{
			name:    entry.name,
			payload: payloads[k],
			shape:   entry.shape,
			cid:     entry.cid,
			stale:   entry.stale,
		})
	}
	return columns, nil
}

// -------------------------------------------------------------------------------------------------
// Planning and running
// -------------------------------------------------------------------------------------------------
//
// A save may be several releases behind, so reaching the current shapes can take more than one
// migration, and a migration may need a value another one produces. The order comes from the
// declared fields, never from registration order or from the data, which is what lets a game and
// its plugins declare migrations separately without any of them knowing the whole picture.
//
// It is worked out in two steps. Kahn's algorithm runs once over every registered migration and
// yields a global order. Per entity the engine walks that order and keeps the migrations whose
// inputs the entity actually has, so a save that already holds a later shape simply skips the
// steps before it.

// migrationOrder returns every registered migration in dependency order, computing it on first use.
func (ws *worldState) migrationOrder() ([]*boundMigration, error) {
	mgr := &ws.migrations
	if mgr.order != nil {
		return mgr.order, nil
	}

	graph := ws.migrationGraph()
	pending := graph.inDegrees()

	ready := make([]*boundMigration, 0, len(mgr.all))
	for _, m := range mgr.all {
		if pending[m] == 0 {
			ready = append(ready, m)
		}
	}

	order := make([]*boundMigration, 0, len(mgr.all))
	for len(ready) > 0 {
		// Keyed by the migration's full Go type name so every boot produces the same order. It
		// only ever decides between migrations that do not depend on each other, but it has to be
		// fixed: two shards restoring the same save with the same build must agree, because an
		// entity a migration creates takes the next free ID.
		slices.SortFunc(ready, func(a, b *boundMigration) int { return strings.Compare(a.name, b.name) })
		next := ready[0]
		ready = ready[1:]
		order = append(order, next)

		for _, consumer := range graph.consumersOf(next) {
			pending[consumer]--
			if pending[consumer] == 0 {
				ready = append(ready, consumer)
			}
		}
	}

	if len(order) != len(mgr.all) {
		// Kahn's algorithm proves a cycle exists by leaving migrations behind, but does not say
		// which. A walk over the leftovers names it.
		return nil, ws.cycleError(order)
	}

	mgr.order = order
	return order, nil
}

// migrationGraph is who produces and who consumes each shape. Per-entity and whole-world fields
// are both edges: a migration reading every Player has to run after whatever produced the Players.
type migrationGraph struct {
	all       []*boundMigration
	producers map[storedShape][]*boundMigration
	consumers map[storedShape][]*boundMigration
}

func (ws *worldState) migrationGraph() migrationGraph {
	g := migrationGraph{
		all:       ws.migrations.all,
		producers: map[storedShape][]*boundMigration{},
		consumers: map[storedShape][]*boundMigration{},
	}
	for _, m := range g.all {
		for _, shape := range m.produces() {
			g.producers[shape] = append(g.producers[shape], m)
		}
		for _, shape := range m.consumes() {
			g.consumers[shape] = append(g.consumers[shape], m)
		}
	}
	return g
}

// inDegrees counts, per migration, how many of its inputs something else produces. An input that
// only ever comes from a save has no producer and no edge, which is what makes stored shapes the
// starting points of the graph.
func (g migrationGraph) inDegrees() map[*boundMigration]int {
	pending := make(map[*boundMigration]int, len(g.all))
	for _, m := range g.all {
		for _, shape := range m.consumes() {
			for _, producer := range g.producers[shape] {
				if producer != m {
					pending[m]++
				}
			}
		}
	}
	return pending
}

// consumersOf lists the migrations that read what m produces. A migration reading and writing the
// same component, as a ranking does, is not a dependency on itself.
func (g migrationGraph) consumersOf(m *boundMigration) []*boundMigration {
	var found []*boundMigration
	for _, shape := range m.produces() {
		for _, consumer := range g.consumers[shape] {
			if consumer != m {
				found = append(found, consumer)
			}
		}
	}
	return found
}

// consumes lists every shape the migration reads, per-entity and whole-world alike.
func (b *boundMigration) consumes() []storedShape {
	shapes := make([]storedShape, 0, len(b.inputs)+len(b.worldInputs))
	for _, input := range b.inputs {
		shapes = append(shapes, inputKey(input))
	}
	for _, input := range b.worldInputs {
		shapes = append(shapes, storedShape{name: input.worldInputName(), shape: input.worldInputShape()})
	}
	return shapes
}

// produces lists every shape the migration writes.
func (b *boundMigration) produces() []storedShape {
	shapes := make([]storedShape, 0, len(b.outputs)+len(b.worldOutputs))
	for _, output := range b.outputs {
		shapes = append(shapes, outputKey(output))
	}
	for _, output := range b.worldOutputs {
		shapes = append(shapes, storedShape{name: output.worldOutputName(), shape: output.worldOutputShape()})
	}
	return shapes
}

// cycleError names a loop among the migrations Kahn's algorithm could not place.
func (ws *worldState) cycleError(placed []*boundMigration) error {
	settled := make(map[*boundMigration]bool, len(placed))
	for _, m := range placed {
		settled[m] = true
	}

	stuck := make([]*boundMigration, 0, len(ws.migrations.all)-len(placed))
	for _, m := range ws.migrations.all {
		if !settled[m] {
			stuck = append(stuck, m)
		}
	}
	slices.SortFunc(stuck, func(a, b *boundMigration) int { return strings.Compare(a.name, b.name) })

	hunt := &loopHunt{ws: ws, settled: settled, onPath: map[*boundMigration]bool{}}
	for _, m := range stuck {
		if loop := hunt.from(m); loop != nil {
			return eris.Errorf("migrations form a loop: %s. A route has to end at a shape this "+
				"build declares, and this one never does", strings.Join(loop, " then "))
		}
	}
	return eris.Errorf("%d migrations could not be ordered", len(stuck))
}

// loopHunt walks the migrations Kahn's algorithm left behind, following each output to whatever
// consumes it, until it re-enters something already on its path.
type loopHunt struct {
	ws      *worldState
	settled map[*boundMigration]bool
	onPath  map[*boundMigration]bool
	path    []*boundMigration
}

// from returns the loop closing at m, or nil if none passes through it.
func (h *loopHunt) from(m *boundMigration) []string {
	if h.onPath[m] {
		return h.loopAt(m)
	}

	h.onPath[m] = true
	h.path = append(h.path, m)
	defer func() {
		h.onPath[m] = false
		h.path = h.path[:len(h.path)-1]
	}()

	for _, consumer := range h.consumers(m) {
		if loop := h.from(consumer); loop != nil {
			return loop
		}
	}
	return nil
}

// consumers returns the unplaced migrations that read what m produces.
func (h *loopHunt) consumers(m *boundMigration) []*boundMigration {
	var found []*boundMigration
	for _, consumer := range h.ws.migrationGraph().consumersOf(m) {
		if !h.settled[consumer] {
			found = append(found, consumer)
		}
	}
	return found
}

// loopAt renders the path from m's first appearance round to m.
func (h *loopHunt) loopAt(m *boundMigration) []string {
	names := make([]string, 0, len(h.path)+1)
	for _, step := range h.path {
		if len(names) > 0 || step == m {
			names = append(names, step.name)
		}
	}
	return append(names, m.name)
}

// routePlan is the set of migrations that run for one set of stored shapes, in the order they run.
type routePlan struct {
	steps []routeStep
	// produces is what the route adds to an entity: the outputs no migration on this route
	// consumes. They are what the entity ends up holding, so each has to be a registered component.
	produces []ComponentID
}

// routeStep is one migration on a route, paired with where each of its outputs goes.
type routeStep struct {
	run *boundMigration
	// sinks[i] is the migration consuming output i on this route. A nil entry means nothing does,
	// so the value lands on the entity.
	sinks []*boundMigration
}

// routeKey identifies a set of stored shapes, so entities holding the same shapes share a route.
func routeKey(stale []storedShape) string {
	sorted := slices.Clone(stale)
	slices.SortFunc(sorted, func(a, b storedShape) int {
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		return cmp.Compare(a.shape, b.shape)
	})

	var key strings.Builder
	for _, s := range sorted {
		fmt.Fprintf(&key, "%s:%x;", s.name, s.shape)
	}
	return key.String()
}

// routeFor works out which migrations run for a set of stored shapes, and in what order.
func (ws *worldState) routeFor(stale []storedShape) (*routePlan, error) {
	key := routeKey(stale)
	if cached, ok := ws.migrations.plans[key]; ok {
		return cached, nil
	}

	order, err := ws.migrationOrder()
	if err != nil {
		return nil, err
	}

	// A shape is available once the entity stores it or an earlier migration has produced it.
	available := make(map[storedShape]bool, len(stale))
	for _, shape := range stale {
		available[shape] = true
	}

	route := &routePlan{}
	planned := map[*boundMigration]bool{}
	for _, m := range order {
		if m.wholeWorld || !readyFor(m, available) {
			continue
		}
		planned[m] = true
		route.steps = append(route.steps, routeStep{run: m, sinks: make([]*boundMigration, len(m.outputs))})
		for _, output := range m.outputs {
			available[outputKey(output)] = true
		}
	}

	// An output another migration on this route consumes is handed to it. Anything else is what
	// the entity ends up with.
	for i, step := range route.steps {
		for j, output := range step.run.outputs {
			consumer, consumed := ws.migrations.byInput[outputKey(output)]
			if consumed && planned[consumer] {
				route.steps[i].sinks[j] = consumer
				continue
			}
			cid, err := ws.components.getID(output.outputName())
			if err != nil {
				return nil, eris.Wrapf(err, "migration %s produces unregistered component %q",
					step.run.name, output.outputName())
			}
			route.produces = append(route.produces, cid)
		}
	}

	ws.migrations.plans[key] = route
	return route, nil
}

// readyFor reports whether every required input of m is available. Optional inputs never hold a
// migration back; the entity simply may not have one.
func readyFor(m *boundMigration, available map[storedShape]bool) bool {
	for _, input := range m.inputs {
		if input.inputRequired() && !available[inputKey(input)] {
			return false
		}
	}
	return true
}

// plan is what one entity's stored components resolve to.
type plan struct {
	route *routePlan
	// stored holds this entity's stale payloads, keyed by the shape they were written in. The
	// route is shared between entities; these values are not.
	stored map[storedShape][]byte
	// carried holds the stored payloads that still match the current code, keyed by component ID.
	carried map[ComponentID][]byte
}

// planEntity resolves one entity's stored components against the current code.
//
// A value whose shape still matches is carried over untouched. A value whose shape has changed, or
// whose component this build no longer registers, needs a migration that consumes it; a stored
// value with no such migration stops the restore, because decoding it as the current struct would
// read fields that moved or changed type.
func (ws *worldState) planEntity(eid EntityID, columns []storedColumn) (plan, error) {
	result := plan{carried: make(map[ComponentID][]byte, len(columns))}

	var stale []storedShape
	for _, column := range columns {
		if !column.stale {
			result.carried[column.cid] = column.payload
			continue
		}
		shape := storedShape{name: column.name, shape: column.shape}
		if result.stored == nil {
			result.stored = make(map[storedShape][]byte, len(columns))
		}
		result.stored[shape] = column.payload
		stale = append(stale, shape)
	}

	if len(stale) == 0 {
		result.route = &routePlan{}
		return result, nil
	}

	route, err := ws.routeFor(stale)
	if err != nil {
		return plan{}, err
	}
	result.route = route

	// Every stale value has to be consumed by something on the route. One that is not has no way
	// to reach a shape this build declares.
	consumed := map[storedShape]bool{}
	for _, step := range route.steps {
		for _, input := range step.run.inputs {
			consumed[inputKey(input)] = true
		}
	}
	for _, column := range columns {
		if !column.stale {
			continue
		}
		if consumed[storedShape{name: column.name, shape: column.shape}] {
			continue
		}
		if column.cid == invalidComponentID {
			return plan{}, eris.Errorf(
				"snapshot entity %d holds component %q, which this build does not register, "+
					"and no migration consumes it", eid, column.name)
		}
		return plan{}, eris.Errorf(
			"snapshot entity %d holds component %q in a shape this build does not declare, "+
				"and no migration consumes it", eid, column.name)
	}

	return result, nil
}

// planComponents returns the component IDs an entity ends up with: everything carried over, plus
// everything the route produces. Whether a produced component is registered was settled when the
// route was worked out, so nothing here can fail.
func (ws *worldState) planComponents(p plan) []ComponentID {
	ids := make([]ComponentID, 0, len(p.carried)+len(p.route.produces))
	for cid := range p.carried {
		ids = append(ids, cid)
	}
	return append(ids, p.route.produces...)
}

// runMigrations runs one entity's route in order.
func (ws *worldState) runMigrations(eid EntityID, p plan) error {
	// Inputs are cleared for the whole route before any of it runs, not per step: a step's inputs
	// may have been filled by an earlier step, and clearing them then would undo that. Without
	// this, an optional input left set by the previous entity would read as present.
	for _, step := range p.route.steps {
		for _, input := range step.run.inputs {
			input.inputReset()
		}
	}

	for _, step := range p.route.steps {
		if err := ws.runStep(eid, step, p.stored); err != nil {
			return err
		}
	}
	return nil
}

func (ws *worldState) runStep(eid EntityID, step routeStep, stored map[storedShape][]byte) error {
	m := step.run
	for _, output := range m.outputs {
		output.outputReset()
	}

	// Inputs an earlier migration produced were filled when it ran. Inputs the entity stores are
	// decoded now. An optional input that is neither stays absent.
	for _, input := range m.inputs {
		payload, isStored := stored[inputKey(input)]
		if !isStored {
			continue
		}
		if err := input.inputDecode(payload); err != nil {
			return eris.Wrapf(err, "migration %s failed to read %q", m.name, input.inputName())
		}
	}

	m.run.Migrate()

	for i, output := range m.outputs {
		if !output.outputWasSet() {
			return eris.Errorf("migration %s declared output %q but produced no value",
				m.name, output.outputName())
		}

		ws.markProduced(outputKey(output))

		sink := step.sinks[i]
		if sink == nil {
			if err := output.outputApply(ws, eid); err != nil {
				return eris.Wrapf(err, "migration %s failed to write %q", m.name, output.outputName())
			}
			continue
		}
		if err := sink.bindInputValue(outputKey(output), output.outputValue()); err != nil {
			return eris.Wrapf(err, "migration %s failed to hand %q to %s",
				m.name, output.outputName(), sink.name)
		}
	}
	return nil
}

// markProduced records that this restore converted something into the given shape. It is what
// decides whether a whole-world migration has any work to do.
func (ws *worldState) markProduced(shape storedShape) {
	if ws.restoreProduced == nil {
		ws.restoreProduced = map[storedShape]bool{}
	}
	ws.restoreProduced[shape] = true
}

// bindInputValue hands a value straight from one migration's output to another's input, so an
// intermediate shape never round-trips through bytes.
func (b *boundMigration) bindInputValue(key storedShape, value any) error {
	for _, input := range b.inputs {
		if inputKey(input) != key {
			continue
		}
		return input.inputSet(value)
	}
	return eris.Errorf("migration %s has no input for %q", b.name, key.name)
}
