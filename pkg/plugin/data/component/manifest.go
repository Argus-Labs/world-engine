// Package component holds the Cardinal components the data plugin owns.
//
// The plugin keeps loaded configuration in an in-memory catalog (not in components), so this
// package is small: just the ConfigManifest singleton that records, per file, the content hash of
// the config the world is running. The reconcile system in pkg/plugin/data/system uses it to keep
// the catalog matched to whatever a snapshot restore brings back.
package component

// ConfigManifest is the only snapshot-resident component the data plugin owns. It records, per
// config file, the content hash of the bytes loaded into the plugin's in-memory catalog.
//
// On a snapshot restore the reconcile system reads this back and uses it to re-fetch the exact
// config the world was running before the restart, so a crash never silently changes the rules of
// an in-flight game. The hashes themselves come from the source — a forge column in production,
// sha256 of the embedded bytes in local dev.
type ConfigManifest struct {
	// Files maps each kind's JSONFile() path to the content hash the source reported when those
	// bytes were last loaded into the catalog.
	Files map[string]string
}

// Name implements cardinal.Component.
func (ConfigManifest) Name() string { return "data_config_manifest" }
