package ecs

import (
	"encoding/binary"

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"github.com/shamaton/msgpack/v3"
)

// keyDirEntry holds the location of a component's data in the Bitcask file.
// Stored in the diskColumn's slice instead of the actual component data.
type keyDirEntry struct {
	offset int64  // byte offset in the data file
	size   uint32 // byte length of the serialized value
}

// diskColumn stores component data on disk via a Bitcask file.
// Implements abstractColumn with the same interface as column[T].
//
// Instead of storing []T in memory, it stores []keyDirEntry (12 bytes per row).
// The actual component data is serialized (MessagePack) and stored in the disk file.
// On Get, the entry's offset is used to ReadAt from the file, then deserialized.
// On Set, the data is buffered for the current tick and flushed at tick end.
type diskColumn[T Component] struct {
	compName string
	entries  []keyDirEntry

	// Reference to the shared disk store (file handle).
	store *diskStore

	// Per-tick buffer for modified rows. Maps row index to serialized bytes.
	// Cleared after flush.
	pendingWrites map[int][]byte
}

var _ abstractColumn = &diskColumn[Component]{}

func newDiskColumn[T Component](store *diskStore) *diskColumn[T] {
	var zero T
	return &diskColumn[T]{
		compName:      zero.Name(),
		entries:       make([]keyDirEntry, 0, columnCapacity),
		store:         store,
		pendingWrites: make(map[int][]byte),
	}
}

func newDiskColumnFactory[T Component](store *diskStore) columnFactory {
	return func() abstractColumn {
		return newDiskColumn[T](store)
	}
}

func (c *diskColumn[T]) len() int {
	return len(c.entries)
}

func (c *diskColumn[T]) name() string {
	return c.compName
}

// extend adds a new row with an empty entry (no data on disk yet).
func (c *diskColumn[T]) extend() {
	c.entries = append(c.entries, keyDirEntry{offset: -1, size: 0})
}

// set serializes the component and buffers it for flush.
func (c *diskColumn[T]) set(row int, component T) {
	assert.That(row < len(c.entries), "disk column: row out of bounds")

	data, err := msgpack.Marshal(component)
	assert.That(err == nil, "disk column: failed to serialize component")

	c.pendingWrites[row] = data
}

func (c *diskColumn[T]) setAbstract(row int, component Component) {
	concrete, ok := component.(T)
	assert.That(ok, "disk column: wrong component type")
	c.set(row, concrete)
}

// get returns the component for the given row.
// Checks pending writes first, then reads from the disk file.
//
// Disk I/O errors (file read, msgpack deserialize) are treated as invariant violations,
// same as out-of-bounds checks in column[T]. If the file is corrupted or the fd is closed,
// the game state is unrecoverable and panicking is the correct behavior.
func (c *diskColumn[T]) get(row int) T {
	assert.That(row < len(c.entries), "disk column: row out of bounds")

	// Check pending writes first (buffered this tick).
	if data, ok := c.pendingWrites[row]; ok {
		var val T
		err := msgpack.Unmarshal(data, &val)
		assert.That(err == nil, "disk column: failed to deserialize from buffer")
		return val
	}

	// Read from disk file.
	entry := c.entries[row]
	assert.That(entry.offset >= 0, "disk column: reading unwritten entry")

	value, err := c.store.readRecord(entry)
	assert.That(err == nil, "disk column: failed to read from file")

	var val T
	err = msgpack.Unmarshal(value, &val)
	assert.That(err == nil, "disk column: failed to deserialize from file")

	return val
}

func (c *diskColumn[T]) getAbstract(row int) Component {
	return c.get(row)
}

// remove swaps the last entry with the row to remove (same as column[T]).
func (c *diskColumn[T]) remove(row int) {
	assert.That(row < len(c.entries), "disk column: tried to remove out of bounds")

	lastIndex := len(c.entries) - 1

	if row != lastIndex {
		// Swap entry.
		c.entries[row] = c.entries[lastIndex]
		// Swap pending write if the last row has one.
		if data, ok := c.pendingWrites[lastIndex]; ok {
			c.pendingWrites[row] = data
			delete(c.pendingWrites, lastIndex)
		} else {
			delete(c.pendingWrites, row)
		}
	} else {
		delete(c.pendingWrites, row)
	}

	c.entries = c.entries[:lastIndex]
}

// flush writes all pending data to the disk file and updates the entries.
func (c *diskColumn[T]) flush() error {

	for row, data := range c.pendingWrites {
		offset, err := c.store.appendRecord(data)
		if err != nil {
			return eris.Wrapf(err, "disk column: failed to flush row %d", row)
		}
		c.entries[row] = keyDirEntry{
			offset: offset,
			size:   uint32(len(data)),
		}
	}
	clear(c.pendingWrites)
	return nil
}

// collectLiveRecords returns compact records for all entries with data on disk.
// Each record includes the row index so applyCompactedOffsets can write back
// the new offsets after compaction. Called at tick boundary (after systems finish).
func (c *diskColumn[T]) collectLiveRecords() ([]compactRecord, error) {
	var records []compactRecord
	for i := range c.entries {
		if c.entries[i].offset < 0 {
			continue
		}

		value, err := c.store.readRecord(c.entries[i])
		if err != nil {
			return nil, eris.Wrapf(err, "failed to read record at row %d during compaction", i)
		}

		records = append(records, compactRecord{
			value: value,
			row:   i,
		})
	}
	return records, nil
}

// applyCompactedOffsets writes the new offsets from compacted records back into entries.
// Called after compact() has updated the offset/size fields in each record.
func (c *diskColumn[T]) applyCompactedOffsets(records []compactRecord) {
	for _, rec := range records {
		c.entries[rec.row] = keyDirEntry{
			offset: rec.offset,
			size:   rec.size,
		}
	}
}

// toProto serializes the disk column's keyDirEntries for snapshot.
// The actual component data lives in the Bitcask file, which is snapshotted separately
// as a raw blob. This method only stores the index (offset + size per row).
//
// Note: the Bitcask file may contain dead data from previous updates/deletes.
// This does not affect correctness on restore because the keyDirEntries point to the
// correct live offsets. Dead data is wasted bytes that the next compaction cleans up.
// For smaller snapshots, trigger compaction before snapshot.
func (c *diskColumn[T]) toProto() (*cardinalv1.Column, error) {

	// Serialize keyDirEntries as the "components" field.
	// Each entry is encoded as 12 bytes: offset(8) + size(4).
	entryData := make([][]byte, len(c.entries))
	for i, entry := range c.entries {
		buf := make([]byte, 12)
		binary.LittleEndian.PutUint64(buf[0:8], uint64(entry.offset))
		binary.LittleEndian.PutUint32(buf[8:12], entry.size)
		entryData[i] = buf
	}

	return &cardinalv1.Column{
		ComponentName: c.compName,
		Components:    entryData,
	}, nil
}

// fromProto restores the disk column's keyDirEntries from a snapshot.
// The Bitcask file must already be restored before calling this.
// The offsets in the entries point directly into the restored file.
func (c *diskColumn[T]) fromProto(pb *cardinalv1.Column) error {
	if pb == nil {
		return eris.New("protobuf column is nil")
	}
	if pb.GetComponentName() != c.compName {
		return eris.Errorf("component name mismatch: expected %s, got %s", c.compName, pb.GetComponentName())
	}


	c.entries = make([]keyDirEntry, len(pb.GetComponents()))
	for i, buf := range pb.GetComponents() {
		if len(buf) < 12 {
			return eris.Errorf("invalid keyDirEntry at index %d: expected 12 bytes, got %d", i, len(buf))
		}
		offset := int64(binary.LittleEndian.Uint64(buf[0:8]))
		size := binary.LittleEndian.Uint32(buf[8:12])

		c.entries[i] = keyDirEntry{
			offset: offset,
			size:   size,
		}
	}

	return nil
}
