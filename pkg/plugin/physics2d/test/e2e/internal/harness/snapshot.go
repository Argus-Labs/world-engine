package harness

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"google.golang.org/protobuf/proto"
)

// CaptureRow is the complete physics state of one body.
type CaptureRow struct {
	Transform physics.Transform2D
	Velocity  physics.Velocity2D
	Body      physics.PhysicsBody2D
	Entity    cardinal.EntityID
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
	Singleton cardinal.Contains[SingletonRow]
}

type postCaptureState struct {
	cardinal.BaseSystemState
	Probes    Probes
	Singleton cardinal.Contains[SingletonRow]
}

// capture copies every body's components into into, replacing whatever was
// there. A fresh map is allocated each time, so a caller that copies the Capture
// struct keeps that tick's state even as later ticks overwrite the field.
func capture(probes *Probes, singleton *cardinal.Contains[SingletonRow], into *Capture) {
	rows := make(map[string]CaptureRow, len(into.Rows))
	for eid, row := range probes.Iter() {
		p := row.Probe.Get()
		rows[p.Label] = CaptureRow{
			Entity:    eid,
			Transform: row.Transform.Get(),
			Velocity:  row.Velocity.Get(),
			Body:      CloneBody(row.Body.Get()),
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
		capture(&state.Probes, &state.Singleton, into)
	}, cardinal.WithHook(cardinal.PreUpdate))
}

// RegisterPostCapture registers a capture that runs after the physics pipeline.
func RegisterPostCapture(world *cardinal.World, into *Capture) {
	cardinal.RegisterSystem(world, func(state *postCaptureState) {
		capture(&state.Probes, &state.Singleton, into)
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

	if len(g.Body.Shapes) != len(w.Body.Shapes) {
		add("Body.Shapes<len>", len(g.Body.Shapes), len(w.Body.Shapes))
		return diffs
	}
	for i := range w.Body.Shapes {
		diffs = append(diffs, compareShape(label, i, w.Body.Shapes[i], g.Body.Shapes[i], tol)...)
	}
	return diffs
}

func compareShape(label string, i int, w, g physics.ColliderShape, tol float64) []Diff {
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

	if g.ShapeType != w.ShapeType {
		add("ShapeType", g.ShapeType, w.ShapeType)
	}
	if g.IsSensor != w.IsSensor {
		add("IsSensor", g.IsSensor, w.IsSensor)
	}
	pt("LocalOffset", g.LocalOffset, w.LocalOffset)
	num("LocalRotation", g.LocalRotation, w.LocalRotation)
	num("Radius", g.Radius, w.Radius)
	pt("HalfExtents", g.HalfExtents, w.HalfExtents)
	pt("CapsuleCenter1", g.CapsuleCenter1, w.CapsuleCenter1)
	pt("CapsuleCenter2", g.CapsuleCenter2, w.CapsuleCenter2)
	num("Friction", g.Friction, w.Friction)
	num("Restitution", g.Restitution, w.Restitution)
	num("Density", g.Density, w.Density)

	if g.CategoryBits != w.CategoryBits {
		add("CategoryBits", fmt.Sprintf("%#x", g.CategoryBits), fmt.Sprintf("%#x", w.CategoryBits))
	}
	if g.MaskBits != w.MaskBits {
		add("MaskBits", fmt.Sprintf("%#x", g.MaskBits), fmt.Sprintf("%#x", w.MaskBits))
	}
	if g.GroupIndex != w.GroupIndex {
		add("GroupIndex", g.GroupIndex, w.GroupIndex)
	}

	if len(g.Vertices) != len(w.Vertices) {
		add("Vertices<len>", len(g.Vertices), len(w.Vertices))
	} else {
		for k := range w.Vertices {
			pt(fmt.Sprintf("Vertices[%d]", k), g.Vertices[k], w.Vertices[k])
		}
	}
	if len(g.ChainPoints) != len(w.ChainPoints) {
		add("ChainPoints<len>", len(g.ChainPoints), len(w.ChainPoints))
	} else {
		for k := range w.ChainPoints {
			pt(fmt.Sprintf("ChainPoints[%d]", k), g.ChainPoints[k], w.ChainPoints[k])
		}
	}
	for k := range w.EdgeVertices {
		pt(fmt.Sprintf("EdgeVertices[%d]", k), g.EdgeVertices[k], w.EdgeVertices[k])
	}
	return diffs
}
