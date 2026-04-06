package ecs

// Bitcask-style append-only data file for disk-backed components.
//
// The diskStore only manages the file. It does not know about entities, components,
// or indexes. The diskColumn[T] handles all indexing and serialization.
//
// References:
//   - Bitcask paper: https://riak.com/assets/bitcask-intro.pdf
//   - Bitcask architecture: https://arpitbhayani.me/blogs/bitcask/

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"

	"github.com/rotisserie/eris"
)

// record header size: valueSize(4) = 4 bytes
// Key is omitted because each diskColumn manages its own records (key is always the component name).
const recordHeaderSize = 4

// diskStore manages the append-only data file for disk-backed components.
type diskStore struct {
	dataFile    *os.File
	writeOffset int64
	mu          sync.Mutex
}

// newDiskStore opens (or creates) the data file.
func newDiskStore(basePath string) (*diskStore, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, eris.Wrap(err, "failed to create disk store directory")
	}

	dataPath := filepath.Join(basePath, "components.dat")
	f, err := os.OpenFile(dataPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, eris.Wrap(err, "failed to open data file")
	}

	// Get current file size for write offset.
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, eris.Wrap(err, "failed to stat data file")
	}

	return &diskStore{
		dataFile:    f,
		writeOffset: info.Size(),
	}, nil
}

// appendRecord appends a value to the data file and returns the offset.
// Record format: [valueSize: 4 bytes][value bytes]
func (ds *diskStore) appendRecord(value []byte) (int64, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	recordOffset := ds.writeOffset

	record := make([]byte, recordHeaderSize+len(value))
	binary.LittleEndian.PutUint32(record[0:4], uint32(len(value)))
	copy(record[recordHeaderSize:], value)

	if _, err := ds.dataFile.WriteAt(record, ds.writeOffset); err != nil {
		return 0, err
	}
	ds.writeOffset += int64(len(record))

	return recordOffset, nil
}

// readRecord reads a value from the data file at the given entry's offset.
// Record format: [valueSize: 4 bytes][value bytes]
// Validates that the on-disk valueSize matches entry.size for corruption detection.
func (ds *diskStore) readRecord(entry keyDirEntry) ([]byte, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	header := make([]byte, recordHeaderSize)
	if _, err := ds.dataFile.ReadAt(header, entry.offset); err != nil {
		return nil, err
	}

	valueSize := binary.LittleEndian.Uint32(header[0:4])

	if valueSize > 100*1024*1024 {
		return nil, eris.Errorf("record value size %d exceeds maximum", valueSize)
	}

	// Validate against the in-memory entry size for corruption detection.
	if entry.size > 0 && valueSize != entry.size {
		return nil, eris.Errorf("record size mismatch: header says %d, index says %d (possible corruption)", valueSize, entry.size)
	}

	dataOffset := entry.offset + int64(recordHeaderSize)
	value := make([]byte, valueSize)
	if _, err := ds.dataFile.ReadAt(value, dataOffset); err != nil {
		return nil, err
	}

	return value, nil
}

// compact rewrites the data file using only the live entries provided.
// Called by World.CompactDisk which collects live entries from all diskColumns.
func (ds *diskStore) compact(liveRecords []compactRecord) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	basePath := filepath.Dir(ds.dataFile.Name())
	compactPath := filepath.Join(basePath, "components.compact.dat")

	compactFile, err := os.OpenFile(compactPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return eris.Wrap(err, "failed to create compaction file")
	}

	var newOffset int64
	for i := range liveRecords {
		rec := &liveRecords[i]
		record := make([]byte, recordHeaderSize+len(rec.value))
		binary.LittleEndian.PutUint32(record[0:4], uint32(len(rec.value)))
		copy(record[recordHeaderSize:], rec.value)

		if _, err := compactFile.WriteAt(record, newOffset); err != nil {
			compactFile.Close()
			os.Remove(compactPath)
			return eris.Wrap(err, "failed to write compacted record")
		}

		// Write new offset/size back into the record so the caller can apply them.
		rec.offset = newOffset
		rec.size = uint32(len(rec.value))

		newOffset += int64(len(record))
	}

	if err := compactFile.Sync(); err != nil {
		compactFile.Close()
		os.Remove(compactPath)
		return eris.Wrap(err, "failed to fsync compacted file")
	}
	compactFile.Close()

	oldPath := ds.dataFile.Name()
	if err := os.Rename(compactPath, oldPath); err != nil {
		return eris.Wrap(err, "failed to rename compacted file")
	}

	ds.dataFile.Close()
	f, err := os.OpenFile(oldPath, os.O_RDWR, 0o644)
	if err != nil {
		return eris.Wrap(err, "failed to reopen compacted file")
	}

	ds.dataFile = f
	ds.writeOffset = newOffset
	return nil
}

// compactRecord is passed to compact() with the data to write.
// compact() updates offset and size in place after writing.
// The row field identifies which entry in the source column to update.
type compactRecord struct {
	value  []byte
	row    int    // Row index in the source diskColumn's entries slice.
	offset int64  // Updated by compact() with the new offset in the compacted file.
	size   uint32 // Updated by compact() with the value size.
}

// readAll reads the entire data file as a byte blob.
func (ds *diskStore) readAll() ([]byte, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	data := make([]byte, ds.writeOffset)
	if ds.writeOffset == 0 {
		return data, nil
	}
	_, err := ds.dataFile.ReadAt(data, 0)
	if err != nil {
		return nil, eris.Wrap(err, "failed to read data file")
	}
	return data, nil
}

// writeAll replaces the data file contents with the given blob.
func (ds *diskStore) writeAll(data []byte) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if err := ds.dataFile.Truncate(0); err != nil {
		return eris.Wrap(err, "failed to truncate data file")
	}
	if len(data) > 0 {
		if _, err := ds.dataFile.WriteAt(data, 0); err != nil {
			return eris.Wrap(err, "failed to write data file")
		}
	}
	ds.writeOffset = int64(len(data))
	return nil
}

// close closes the data file.
func (ds *diskStore) close() error {
	if ds.dataFile != nil {
		return ds.dataFile.Close()
	}
	return nil
}

// reset truncates the data file. Panics if truncation fails, since a failed truncate
// means the filesystem is in a bad state and continuing would mix stale and new data.
func (ds *diskStore) reset() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if err := ds.dataFile.Truncate(0); err != nil {
		panic("disk store: failed to truncate data file on reset: " + err.Error())
	}
	ds.writeOffset = 0
}

