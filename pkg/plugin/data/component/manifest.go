// Package component holds the Cardinal components the data plugin owns.
//
// The plugin keeps loaded configuration in an in-memory catalog (not in components), so this
// package is small: just the ConfigManifest singleton that records, per file, the content hash of
// the config the world is running. The reconcile system in pkg/plugin/data/system uses it to keep
// the catalog matched to whatever a snapshot restore brings back.
package component

import "github.com/argus-labs/world-engine/pkg/immutable"

// ConfigFileHash is one config file's content hash: the path a kind reports from JSONFile(), and
// the hash the source gave for the bytes behind it.
//
// The field numbers matter. A proto map<string, string> encodes as a repeated message of
// {key = 1, value = 2}, so Path and Hash in that order make this list byte-identical to the map
// this component used to hold — every snapshot written before the change still reads back.
type ConfigFileHash struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// ConfigManifest is the only snapshot-resident component the data plugin owns. It records, per
// config file, the content hash of the bytes loaded into the plugin's in-memory catalog.
//
// On a snapshot restore the reconcile system reads this back and uses it to re-fetch the exact
// config the world was running before the restart, so a crash never silently changes the rules of
// an in-flight game. The hashes themselves come from the source — a forge column in production,
// sha256 of the embedded bytes in local dev.
type ConfigManifest struct {
	// Files holds one entry per config file, ordered by path.
	//
	// The order is why this is a list rather than the map it used to be. Go randomises map
	// iteration, so one manifest encoded to a different byte sequence on every run and two identical
	// worlds wrote different snapshots. The data system builds it sorted. A snapshot written before
	// the change restores in whatever order it was written; the system's compare is order-sensitive,
	// so such a manifest reads as changed and is rewritten sorted on the next tick.
	Files immutable.Slice[ConfigFileHash]
}

// Name implements cardinal.Component.
func (ConfigManifest) Name() string { return "data_config_manifest" }

// Hash returns the hash recorded for path. A linear scan: a manifest holds a few dozen entries at
// most, and this runs only while a restored snapshot disagrees with the running config, never on
// the per-tick path.
func (c ConfigManifest) Hash(path string) (string, bool) {
	i := c.Files.IndexFunc(func(e ConfigFileHash) bool { return e.Path == path })
	if i < 0 {
		return "", false
	}
	return c.Files.At(i).Hash, true
}
