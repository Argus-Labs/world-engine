package harness

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"google.golang.org/protobuf/proto"
)

// CaptureRow is the complete physics state of one body: its own components plus
// the shape entities its slots reference, resolved at capture time so two worlds
// can be compared shape by shape.
type CaptureRow struct {
	Transform physics.Transform2D
	Velocity  physics.Velocity2D
	Body      physics.PhysicsBody2D
	Shapes    []CapturedShape
	Entity    cardinal.EntityID
}

// CapturedShape is one slot and the shape entity it points at. Kind is the
// geometry component found on that entity, or "" when the entity was missing;
// only the geometry field matching Kind is set.
type CapturedShape struct {
	Slot    physics.ShapeSlot
	Kind    string
	Common  physics.ShapeCommon
	Circle  physics.CircleGeom
	Box     physics.BoxGeom
	Polygon physics.PolygonGeom
	Chain   physics.ChainGeom
	Edge    physics.EdgeGeom
	Capsule physics.CapsuleGeom
}

// resolveShape looks the slot's shape entity up through the six searches.
func resolveShape(sh *ShapeSearches, slot physics.ShapeSlot) CapturedShape {
	out := CapturedShape{Slot: slot}
	id := slot.Shape
	if row, err := sh.Circles.GetByID(id); err == nil {
		out.Kind, out.Common, out.Circle = "circle", row.Common.Get(), row.Geom.Get()
		return out
	}
	if row, err := sh.Boxes.GetByID(id); err == nil {
		out.Kind, out.Common, out.Box = "box", row.Common.Get(), row.Geom.Get()
		return out
	}
	if row, err := sh.Polygons.GetByID(id); err == nil {
		out.Kind, out.Common, out.Polygon = "polygon", row.Common.Get(), row.Geom.Get()
		return out
	}
	if row, err := sh.Chains.GetByID(id); err == nil {
		out.Kind, out.Common, out.Chain = "chain", row.Common.Get(), row.Geom.Get()
		out.Chain.Points = slices.Clone(out.Chain.Points)
		return out
	}
	if row, err := sh.Edges.GetByID(id); err == nil {
		out.Kind, out.Common, out.Edge = "edge", row.Common.Get(), row.Geom.Get()
		return out
	}
	if row, err := sh.Capsules.GetByID(id); err == nil {
		out.Kind, out.Common, out.Capsule = "capsule", row.Common.Get(), row.Geom.Get()
	}
	return out
}

// SingletonRow is the plugin's own bookkeeping entity. It carries ActiveContacts,
// the persisted record of which pairs have had a Begin emitted and not yet an End.
// The physics step diffs it against Box2D's live contact list after a rebuild, so
// if it does not survive a restore the rebuilt world replays every existing
// overlap as a new contact.
type SingletonRow struct {
	Tag            cardinal.Ref[physics.PhysicsSingletonTag]
	ActiveContacts cardinal.Ref[physics.ActiveContacts]
}

// Capture is every body in a world, keyed by its probe label, plus the plugin's
// singleton state. Labels are used rather than entity IDs so a capture stays
// comparable across two worlds.
type Capture struct {
	Rows map[string]CaptureRow
	// Contacts is the singleton's ActiveContacts, normalised and sorted.
	Contacts []physics.ContactPairEntry
	// Singletons is how many physics singleton entities exist. Anything but one
	// is a bug: the plugin panics on two and loses its dedupe baseline on none.
	Singletons int
}

// Labels returns the capture's labels in sorted order.
func (c Capture) Labels() []string {
	out := make([]string, 0, len(c.Rows))
	for k := range c.Rows {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// preCaptureState and postCaptureState are the two capture systems. They are
// separate flat types on purpose: Cardinal names a system after its state type,
// so two systems sharing one type would collide, and initSystemFields only walks
// a state struct's top-level fields, so a shared embedded struct would leave
// Probes uninitialised and the search would fault on first use.
type preCaptureState struct {
	cardinal.BaseSystemState
	Probes    Probes
	Circles   physics.CircleShapes
	Boxes     physics.BoxShapes
	Polygons  physics.PolygonShapes
	Chains    physics.ChainShapes
	Edges     physics.EdgeShapes
	Capsules  physics.CapsuleShapes
	Singleton cardinal.Contains[SingletonRow]
}

func (s *preCaptureState) shapes() *ShapeSearches {
	return &ShapeSearches{&s.Circles, &s.Boxes, &s.Polygons, &s.Chains, &s.Edges, &s.Capsules}
}

type postCaptureState struct {
	cardinal.BaseSystemState
	Probes    Probes
	Circles   physics.CircleShapes
	Boxes     physics.BoxShapes
	Polygons  physics.PolygonShapes
	Chains    physics.ChainShapes
	Edges     physics.EdgeShapes
	Capsules  physics.CapsuleShapes
	Singleton cardinal.Contains[SingletonRow]
}

func (s *postCaptureState) shapes() *ShapeSearches {
	return &ShapeSearches{&s.Circles, &s.Boxes, &s.Polygons, &s.Chains, &s.Edges, &s.Capsules}
}

// capture copies every body's components (and the shape entities its slots
// reference) into into, replacing whatever was there. A fresh map is allocated
// each time, so a caller that copies the Capture struct keeps that tick's state
// even as later ticks overwrite the field.
func capture(probes *Probes, shapes *ShapeSearches, singleton *cardinal.Contains[SingletonRow], into *Capture) {
	rows := make(map[string]CaptureRow, len(into.Rows))
	for eid, row := range probes.Iter() {
		p := row.Probe.Get()
		body := CloneBody(row.Body.Get())
		resolved := make([]CapturedShape, len(body.Shapes))
		for i, slot := range body.Shapes {
			resolved[i] = resolveShape(shapes, slot)
		}
		rows[p.Label] = CaptureRow{
			Entity:    eid,
			Transform: row.Transform.Get(),
			Velocity:  row.Velocity.Get(),
			Body:      body,
			Shapes:    resolved,
		}
	}

	var pairs []physics.ContactPairEntry
	count := 0
	for _, row := range singleton.Iter() {
		count++
		pairs = append(pairs, row.ActiveContacts.Get().Pairs...)
	}
	// Entry order is an implementation detail of the plugin's map iteration, so
	// sort before comparing two worlds.
	sort.Slice(pairs, func(i, j int) bool { return contactKey(pairs[i]) < contactKey(pairs[j]) })

	into.Rows = rows
	into.Contacts = pairs
	into.Singletons = count
}

// contactKey renders a contact pair as a sortable, comparable string.
func contactKey(p physics.ContactPairEntry) string {
	return fmt.Sprintf("%d:%d/%d:%d/sensor=%v/fa=%#x:%#x:%d/fb=%#x:%#x:%d",
		p.EntityA, p.ShapeIndexA, p.EntityB, p.ShapeIndexB, p.IsSensor,
		p.FilterACategoryBits, p.FilterAMaskBits, p.FilterAGroupIndex,
		p.FilterBCategoryBits, p.FilterBMaskBits, p.FilterBGroupIndex)
}

// CompareContacts reports differences between two worlds' ActiveContacts.
func CompareContacts(want, got Capture) []Diff {
	var diffs []Diff
	if got.Singletons != want.Singletons {
		diffs = append(diffs, Diff{"<singleton>", "count",
			strconv.Itoa(got.Singletons), strconv.Itoa(want.Singletons)})
	}

	seen := map[string]bool{}
	for _, p := range got.Contacts {
		seen[contactKey(p)] = true
	}
	for _, p := range want.Contacts {
		key := contactKey(p)
		if !seen[key] {
			diffs = append(diffs, Diff{"<active-contacts>", key, "missing", "present"})
		}
		delete(seen, key)
	}
	for key := range seen {
		diffs = append(diffs, Diff{"<active-contacts>", key, "present", "missing"})
	}
	return diffs
}

// RegisterPreCapture registers a capture that runs before the physics plugin.
// Call it before RegisterPlugin. After a snapshot restore, the first tick's
// pre-capture is the deserialized ECS state with nothing else having touched it.
func RegisterPreCapture(world *cardinal.World, into *Capture) {
	cardinal.RegisterSystem(world, func(state *preCaptureState) {
		capture(&state.Probes, state.shapes(), &state.Singleton, into)
	}, cardinal.WithHook(cardinal.PreUpdate))
}

// RegisterPostCapture registers a capture that runs after the physics pipeline.
func RegisterPostCapture(world *cardinal.World, into *Capture) {
	cardinal.RegisterSystem(world, func(state *postCaptureState) {
		capture(&state.Probes, state.shapes(), &state.Singleton, into)
	}, cardinal.WithHook(cardinal.PostUpdate))
}

// -----------------------------------------------------------------------------
// Snapshot plumbing
// -----------------------------------------------------------------------------

// innerWorld reaches cardinal.World's unexported ecs.World. Everything the
// restore path needs — Init, ToProto, FromProto — is an exported method on an
// unexported field of an internal type, reachable only this way from outside the
// world-engine module. See InitECS for why this shim exists at all.
func innerWorld(world *cardinal.World) reflect.Value {
	v := reflect.ValueOf(world).Elem()
	f := v.FieldByName("world")
	if !f.IsValid() {
		panic("cardinal.World: no 'world' field; the snapshot shim needs updating")
	}
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
}

// EncodeSnapshot marshals a world state to protobuf bytes, the form a snapshot
// actually takes on its way to JetStream or S3. Going through bytes is what makes
// a two-process restore a real one.
func EncodeSnapshot(state any) ([]byte, error) {
	msg, ok := state.(*cardinalv1.WorldState)
	if !ok {
		return nil, fmt.Errorf("expected *cardinalv1.WorldState, got %T", state)
	}
	return proto.Marshal(msg)
}

// DecodeSnapshot is the inverse of EncodeSnapshot.
func DecodeSnapshot(raw []byte) (any, error) {
	var msg cardinalv1.WorldState
	if err := proto.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// SnapshotWorld serializes a world exactly the way Cardinal's snapshot writer
// does, component bytes and all.
func SnapshotWorld(world *cardinal.World) (any, error) {
	m := innerWorld(world).MethodByName("ToProto")
	if !m.IsValid() {
		panic("ecs.World: no ToProto method; the snapshot shim needs updating")
	}
	out := m.Call(nil)
	// ecs.World.ToProto returns the state alone; tolerate a trailing error if one
	// is ever added so the shim keeps working across that change.
	if len(out) == 2 {
		if err, _ := out[1].Interface().(error); err != nil {
			return nil, err
		}
	}
	return out[0].Interface(), nil
}

// RestoreWorld loads a serialized world state, the way World.restore does after
// a crash. Note the ordering Cardinal uses: Init systems run first and only then
// is their state thrown away by this call.
func RestoreWorld(world *cardinal.World, state any) error {
	m := innerWorld(world).MethodByName("FromProto")
	if !m.IsValid() {
		panic("ecs.World: no FromProto method; the snapshot shim needs updating")
	}
	out := m.Call([]reflect.Value{reflect.ValueOf(state)})
	if err, _ := out[0].Interface().(error); err != nil {
		return err
	}
	return nil
}

// -----------------------------------------------------------------------------
// Comparison
// -----------------------------------------------------------------------------

// Diff describes one field that differs between two captures.
type Diff struct {
	Label string
	Field string
	Got   string
	Want  string
}

func (d Diff) String() string {
	return fmt.Sprintf("%s.%s: got %s, want %s", d.Label, d.Field, d.Got, d.Want)
}

// CompareCaptures reports every field of every body that differs between want
// and got, within tol on floats. Bodies present in one capture and not the other
// are reported too, because a body lost in a rebuild is the worst failure of all.
func CompareCaptures(want, got Capture, tol float64) []Diff {
	var diffs []Diff

	for _, label := range want.Labels() {
		w := want.Rows[label]
		g, ok := got.Rows[label]
		if !ok {
			diffs = append(diffs, Diff{label, "<body>", "missing", "present"})
			continue
		}
		diffs = append(diffs, compareRow(label, w, g, tol)...)
	}
	for _, label := range got.Labels() {
		if _, ok := want.Rows[label]; !ok {
			diffs = append(diffs, Diff{label, "<body>", "present", "missing"})
		}
	}
	return diffs
}

func compareRow(label string, w, g CaptureRow, tol float64) []Diff {
	var diffs []Diff
	add := func(field string, got, want any) {
		diffs = append(diffs, Diff{label, field, fmt.Sprint(got), fmt.Sprint(want)})
	}
	num := func(field string, got, want float64) {
		if math.Abs(got-want) > tol || math.IsNaN(got) != math.IsNaN(want) {
			add(field, got, want)
		}
	}
	boolean := func(field string, got, want bool) {
		if got != want {
			add(field, got, want)
		}
	}

	num("Transform.Position.X", g.Transform.Position.X, w.Transform.Position.X)
	num("Transform.Position.Y", g.Transform.Position.Y, w.Transform.Position.Y)
	num("Transform.Rotation", g.Transform.Rotation, w.Transform.Rotation)
	num("Velocity.Linear.X", g.Velocity.Linear.X, w.Velocity.Linear.X)
	num("Velocity.Linear.Y", g.Velocity.Linear.Y, w.Velocity.Linear.Y)
	num("Velocity.Angular", g.Velocity.Angular, w.Velocity.Angular)

	if g.Body.BodyType != w.Body.BodyType {
		add("Body.BodyType", g.Body.BodyType, w.Body.BodyType)
	}
	num("Body.LinearDamping", g.Body.LinearDamping, w.Body.LinearDamping)
	num("Body.AngularDamping", g.Body.AngularDamping, w.Body.AngularDamping)
	num("Body.GravityScale", g.Body.GravityScale, w.Body.GravityScale)
	boolean("Body.Active", g.Body.Active, w.Body.Active)
	boolean("Body.Awake", g.Body.Awake, w.Body.Awake)
	boolean("Body.SleepingAllowed", g.Body.SleepingAllowed, w.Body.SleepingAllowed)
	boolean("Body.Bullet", g.Body.Bullet, w.Body.Bullet)
	boolean("Body.FixedRotation", g.Body.FixedRotation, w.Body.FixedRotation)

	if len(g.Shapes) != len(w.Shapes) {
		add("Body.Shapes<len>", len(g.Shapes), len(w.Shapes))
		return diffs
	}
	for i := range w.Shapes {
		diffs = append(diffs, compareShape(label, i, w.Shapes[i], g.Shapes[i], tol)...)
	}
	return diffs
}

func compareShape(label string, i int, w, g CapturedShape, tol float64) []Diff {
	var diffs []Diff
	field := func(name string) string { return fmt.Sprintf("Body.Shapes[%d].%s", i, name) }
	add := func(name string, got, want any) {
		diffs = append(diffs, Diff{label, field(name), fmt.Sprint(got), fmt.Sprint(want)})
	}
	num := func(name string, got, want float64) {
		if math.Abs(got-want) > tol {
			add(name, got, want)
		}
	}
	pt := func(name string, got, want physics.Vec2) {
		if math.Abs(got.X-want.X) > tol || math.Abs(got.Y-want.Y) > tol {
			add(name, got, want)
		}
	}

	if g.Slot.Shape != w.Slot.Shape {
		add("Shape", g.Slot.Shape, w.Slot.Shape)
	}
	pt("LocalOffset", g.Slot.LocalOffset, w.Slot.LocalOffset)
	num("LocalRotation", g.Slot.LocalRotation, w.Slot.LocalRotation)
	if g.Kind != w.Kind {
		add("Kind", g.Kind, w.Kind)
		return diffs
	}
	if g.Common.IsSensor != w.Common.IsSensor {
		add("IsSensor", g.Common.IsSensor, w.Common.IsSensor)
	}
	num("Friction", g.Common.Friction, w.Common.Friction)
	num("Restitution", g.Common.Restitution, w.Common.Restitution)
	num("Density", g.Common.Density, w.Common.Density)
	if g.Common.CategoryBits != w.Common.CategoryBits {
		add("CategoryBits", fmt.Sprintf("%#x", g.Common.CategoryBits), fmt.Sprintf("%#x", w.Common.CategoryBits))
	}
	if g.Common.MaskBits != w.Common.MaskBits {
		add("MaskBits", fmt.Sprintf("%#x", g.Common.MaskBits), fmt.Sprintf("%#x", w.Common.MaskBits))
	}
	if g.Common.GroupIndex != w.Common.GroupIndex {
		add("GroupIndex", g.Common.GroupIndex, w.Common.GroupIndex)
	}

	num("Radius", g.Circle.Radius, w.Circle.Radius)
	pt("HalfExtents", g.Box.HalfExtents, w.Box.HalfExtents)
	if g.Polygon.Count != w.Polygon.Count {
		add("Vertices<count>", g.Polygon.Count, w.Polygon.Count)
	} else {
		for k := range int(w.Polygon.Count) {
			pt(fmt.Sprintf("Vertices[%d]", k), g.Polygon.Vertices[k], w.Polygon.Vertices[k])
		}
	}
	if g.Chain.Loop != w.Chain.Loop {
		add("Loop", g.Chain.Loop, w.Chain.Loop)
	}
	if len(g.Chain.Points) != len(w.Chain.Points) {
		add("ChainPoints<len>", len(g.Chain.Points), len(w.Chain.Points))
	} else {
		for k := range w.Chain.Points {
			pt(fmt.Sprintf("ChainPoints[%d]", k), g.Chain.Points[k], w.Chain.Points[k])
		}
	}
	pt("Edge.A", g.Edge.A, w.Edge.A)
	pt("Edge.B", g.Edge.B, w.Edge.B)
	pt("Capsule.A", g.Capsule.A, w.Capsule.A)
	pt("Capsule.B", g.Capsule.B, w.Capsule.B)
	num("Capsule.Radius", g.Capsule.Radius, w.Capsule.Radius)
	return diffs
}
