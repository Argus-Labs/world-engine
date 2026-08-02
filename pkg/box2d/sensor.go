// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/sensor.h, src/sensor.c.
//
// Sensor shapes need to
//   - detect begin and end overlap events
//   - events must be reported in deterministic order
//   - maintain an active list of overlaps for query
//
// Assumption
//   - sensors don't detect shapes on the same body
//
// Algorithm
//
//	Query all sensors for overlaps
//	Check against previous overlaps
//
// Data structures
//
//	Each sensor has a double buffered array of overlaps
//	These overlaps use a shape reference with index and generation
//
// Deviations from upstream:
//   - b2SensorTaskContext (a per-worker bitset) collapses to the single
//     World.sensorEventBits bitset: this port is single-threaded, so the
//     per-worker union in b2OverlapSensors is a no-op and is not ported.
//   - The parallel-for over sensors becomes a serial loop in sensor-array
//     index order. That is the same set of sensors upstream visits, walked in
//     a fixed order, so event order does not depend on worker scheduling.
//   - b2SensorQueryContext drops the taskContext field, which upstream never
//     reads inside b2SensorQueryCallback.

package box2d

import (
	"cmp"
	"slices"
)

// sensorHit tracks shapes that hit sensors using time of impact
// (upstream b2SensorHit).
type sensorHit struct {
	sensorID  int
	visitorID int
}

// visitor is a shape reference stored in sensor overlap arrays
// (upstream b2Visitor).
type visitor struct {
	shapeID    int
	generation uint16
}

// sensor holds the overlap state for one sensor shape (upstream b2Sensor).
// Sensors are shapes that live in the broad-phase but never have contacts.
// At the end of the time step all sensors are queried for overlap with any
// other shapes. Sensors ignore body type and sleeping.
type sensor struct {
	hits      []visitor
	overlaps1 []visitor
	overlaps2 []visitor
	shapeID   int
}

// sensorQueryContext is the callback context for the per-sensor broad-phase
// query (upstream struct b2SensorQueryContext).
type sensorQueryContext struct {
	world       *World
	sensor      *sensor
	sensorShape *shape
	transform   Transform
}

// compareVisitors orders sensor overlaps (upstream b2CompareVisitors).
//
// DETERMINISM: upstream's qsort comparator only orders by shapeId and returns
// 1 for every non-less pair, so ties are left in an unspecified order. This
// port compares the full (shapeID, generation) key, which is a total order:
// a shape id can appear in overlaps2 at most once per generation because both
// producers (the time-of-impact hits and the tree query) read the shape's
// current generation. The sorted result is therefore unique.
func compareVisitors(a, b visitor) int {
	return cmp.Or(
		cmp.Compare(a.shapeID, b.shapeID),
		cmp.Compare(a.generation, b.generation),
	)
}

// sensorQueryCallback records one candidate overlap for a sensor
// (upstream b2SensorQueryCallback).
func sensorQueryCallback(proxyID int, userData uint64, context any) bool {
	_ = proxyID

	shapeID := int(userData)

	queryContext, ok := context.(*sensorQueryContext)
	assert(ok)
	if !ok {
		return false
	}

	sensorShape := queryContext.sensorShape
	sensorShapeID := sensorShape.id

	if shapeID == sensorShapeID {
		return true
	}

	world := queryContext.world
	otherShape := &world.shapes[shapeID]

	// Are sensor events enabled on the other shape?
	if !otherShape.enableSensorEvents {
		return true
	}

	// Skip shapes on the same body
	if otherShape.bodyID == sensorShape.bodyID {
		return true
	}

	// Check filter
	if !shouldShapesCollide(sensorShape.filter, otherShape.filter) {
		return true
	}

	// Custom user filter
	if sensorShape.enableCustomFiltering || otherShape.enableCustomFiltering {
		customFilterFcn := world.customFilterFcn
		if customFilterFcn != nil {
			idA := ShapeID{
				index1:     int32(sensorShapeID + 1),
				world0:     world.worldID,
				generation: sensorShape.generation,
			}
			idB := ShapeID{
				index1:     int32(shapeID + 1),
				world0:     world.worldID,
				generation: otherShape.generation,
			}
			if !customFilterFcn(idA, idB, world.customFilterContext) {
				return true
			}
		}
	}

	otherTransform := world.getBodyTransform(otherShape.bodyID)

	var input DistanceInput
	input.ProxyA = makeShapeDistanceProxy(sensorShape)
	input.ProxyB = makeShapeDistanceProxy(otherShape)
	input.TransformA = queryContext.transform
	input.TransformB = otherTransform
	input.UseRadii = true

	var cache SimplexCache
	output := ShapeDistance(&input, &cache, nil)

	overlaps := output.Distance < 10.0*epsilon
	if !overlaps {
		return true
	}

	// Record the overlap
	sen := queryContext.sensor
	sen.overlaps2 = append(sen.overlaps2, visitor{
		shapeID:    shapeID,
		generation: otherShape.generation,
	})

	return true
}

// sensorTask rebuilds the current overlap set for sensors in
// [startIndex, endIndex) and flags the ones whose overlap set changed
// (upstream b2SensorTask). The upstream parallel-for becomes a serial call
// over the whole sensor array; see the file header.
func (w *World) sensorTask(startIndex, endIndex int) {
	assert(startIndex < endIndex)

	trees := &w.broadPhase.trees
	for sensorIndex := startIndex; sensorIndex < endIndex; sensorIndex++ {
		sen := &w.sensors[sensorIndex]
		sensorShape := &w.shapes[sen.shapeID]

		// Swap overlap arrays
		sen.overlaps1, sen.overlaps2 = sen.overlaps2, sen.overlaps1
		sen.overlaps2 = sen.overlaps2[:0]

		// Append sensor hits
		sen.overlaps2 = append(sen.overlaps2, sen.hits...)

		// Clear the hits
		sen.hits = sen.hits[:0]

		b := &w.bodies[sensorShape.bodyID]
		if b.setIndex == disabledSet || !sensorShape.enableSensorEvents {
			if len(sen.overlaps1) != 0 {
				// This sensor is dropping all overlaps because it has been
				// disabled.
				setBit(&w.sensorEventBits, uint32(sensorIndex))
			}

			continue
		}

		transform := w.getBodyTransformQuick(b)

		queryContext := sensorQueryContext{
			world:       w,
			sensor:      sen,
			sensorShape: sensorShape,
			transform:   transform,
		}

		assert(sensorShape.sensorIndex == sensorIndex)
		queryBounds := sensorShape.aabb

		// Query all trees
		for treeIndex := range int(BodyTypeCount) {
			_ = trees[treeIndex].Query(
				queryBounds, sensorShape.filter.MaskBits, sensorQueryCallback, &queryContext,
			)
		}

		// Sort the overlaps to enable finding begin and end events. This is
		// the only sort in the simulation path; see doc.go and
		// compareVisitors for why the comparator is a total order.
		slices.SortFunc(sen.overlaps2, compareVisitors)

		// Remove duplicates from overlaps2 (sorted). Duplicates are possible
		// due to the hit events appended earlier.
		uniqueCount := 0
		overlapData := sen.overlaps2
		for i := range overlapData {
			if uniqueCount == 0 || overlapData[i].shapeID != overlapData[uniqueCount-1].shapeID {
				overlapData[uniqueCount] = overlapData[i]
				uniqueCount++
			}
		}
		sen.overlaps2 = overlapData[:uniqueCount]

		count1 := len(sen.overlaps1)
		count2 := len(sen.overlaps2)
		if count1 != count2 {
			// something changed
			setBit(&w.sensorEventBits, uint32(sensorIndex))

			continue
		}

		for i := range count1 {
			s1 := &sen.overlaps1[i]
			s2 := &sen.overlaps2[i]

			if s1.shapeID != s2.shapeID || s1.generation != s2.generation {
				// something changed
				setBit(&w.sensorEventBits, uint32(sensorIndex))

				break
			}
		}
	}
}

// pushSensorBeginEvent appends a begin touch event for one visitor.
func (w *World) pushSensorBeginEvent(sensorID ShapeID, ref *visitor) {
	w.sensorBeginEvents = append(w.sensorBeginEvents, SensorBeginTouchEvent{
		SensorShapeID: sensorID,
		VisitorShapeID: ShapeID{
			index1:     int32(ref.shapeID + 1),
			world0:     w.worldID,
			generation: ref.generation,
		},
	})
}

// pushSensorEndEvent appends an end touch event for one visitor. End events
// are double buffered so the user can read them after the next step.
func (w *World) pushSensorEndEvent(sensorID ShapeID, ref *visitor) {
	event := SensorEndTouchEvent{
		SensorShapeID: sensorID,
		VisitorShapeID: ShapeID{
			index1:     int32(ref.shapeID + 1),
			world0:     w.worldID,
			generation: ref.generation,
		},
	}

	w.sensorEndEvents[w.endEventArrayIndex] = append(w.sensorEndEvents[w.endEventArrayIndex], event)
}

// publishSensorEvents emits the begin/end touch events for one sensor whose
// overlap set changed this step (upstream: the body of the set-bit loop in
// b2OverlapSensors). Both overlap arrays are sorted by compareVisitors, so a
// single merge pass finds the difference.
func (w *World) publishSensorEvents(sensorIndex int) {
	sen := &w.sensors[sensorIndex]
	sensorShape := &w.shapes[sen.shapeID]
	sensorID := ShapeID{
		index1:     int32(sen.shapeID + 1),
		world0:     w.worldID,
		generation: sensorShape.generation,
	}

	refs1 := sen.overlaps1
	refs2 := sen.overlaps2
	count1 := len(refs1)
	count2 := len(refs2)

	// overlaps1 can have overlaps that end
	// overlaps2 can have overlaps that begin
	index1, index2 := 0, 0
	for index1 < count1 && index2 < count2 {
		r1 := &refs1[index1]
		r2 := &refs2[index2]

		switch {
		case r1.shapeID != r2.shapeID:
			if r1.shapeID < r2.shapeID {
				// end
				w.pushSensorEndEvent(sensorID, r1)
				index1++
			} else {
				// begin
				w.pushSensorBeginEvent(sensorID, r2)
				index2++
			}
		case r1.generation < r2.generation:
			// end
			w.pushSensorEndEvent(sensorID, r1)
			index1++
		case r1.generation > r2.generation:
			// begin
			w.pushSensorBeginEvent(sensorID, r2)
			index2++
		default:
			// persisted
			index1++
			index2++
		}
	}

	for index1 < count1 {
		// end
		w.pushSensorEndEvent(sensorID, &refs1[index1])
		index1++
	}

	for index2 < count2 {
		// begin
		w.pushSensorBeginEvent(sensorID, &refs2[index2])
		index2++
	}
}

// overlapSensors updates every sensor's overlap set and publishes the
// resulting begin/end touch events (upstream b2OverlapSensors). Called at the
// end of World.Step, after the solve.
func (w *World) overlapSensors() {
	sensorCount := len(w.sensors)
	if sensorCount == 0 {
		return
	}

	setBitCountAndClear(&w.sensorEventBits, uint32(sensorCount))

	// Upstream runs b2SensorTask as a parallel-for over the sensor array and
	// then unions the per-worker event bitsets. Single-threaded: one serial
	// pass, no union.
	w.sensorTask(0, sensorCount)

	// Iterate sensor bits and publish events. Process sensor state changes by
	// iterating over set bits, which visits sensors in ascending index order.
	bits := w.sensorEventBits.bits
	blockCount := w.sensorEventBits.blockCount

	for k := range blockCount {
		word := bits[k]
		for word != 0 {
			sensorIndex := int(64*k + ctz64(word))
			w.publishSensorEvents(sensorIndex)

			// Clear the smallest set bit
			word &= word - 1
		}
	}
}

// destroySensor removes a sensor from the world sensor array, emitting end
// touch events for its current overlaps (upstream b2DestroySensor).
func (w *World) destroySensor(sensorShape *shape) {
	s := &w.sensors[sensorShape.sensorIndex]
	for i := range s.overlaps2 {
		ref := &s.overlaps2[i]
		event := SensorEndTouchEvent{
			SensorShapeID: ShapeID{
				index1:     int32(sensorShape.id + 1),
				world0:     w.worldID,
				generation: sensorShape.generation,
			},
			VisitorShapeID: ShapeID{
				index1:     int32(ref.shapeID + 1),
				world0:     w.worldID,
				generation: ref.generation,
			},
		}

		w.sensorEndEvents[w.endEventArrayIndex] = append(w.sensorEndEvents[w.endEventArrayIndex], event)
	}

	// Destroy sensor
	s.hits = nil
	s.overlaps1 = nil
	s.overlaps2 = nil

	movedIndex := removeSwap(&w.sensors, sensorShape.sensorIndex)
	if movedIndex != NullIndex {
		// Fixup moved sensor
		movedSensor := &w.sensors[sensorShape.sensorIndex]
		otherSensorShape := &w.shapes[movedSensor.shapeID]
		otherSensorShape.sensorIndex = sensorShape.sensorIndex
	}
}
