package store

import (
	"os"
	"path/filepath"

	"github.com/rotisserie/eris"
)

const (
	DefaultStoreDirName = ".worldcli"
)

// GetStoreDir returns the default store directory path (~/.worldcli).
func GetStoreDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, DefaultStoreDirName), nil
}

// SetupStoreDir initializes the store directory and returns its path. This is
// the only place ~/.worldcli is created, so its mode cannot depend on which
// caller ran first.
func SetupStoreDir() (string, error) {
	dir, err := GetStoreDir()
	if err != nil {
		return "", err
	}

	fs, err := os.Stat(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
	} else if !fs.IsDir() {
		return "", eris.Errorf("cannot create store directory: a file with the same name %s already exists", dir)
	}

	// 0700: the directory holds credentials.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// MkdirAll leaves an existing directory's mode alone, so installs created
	// under the previous 0755 need tightening explicitly.
	// #nosec G302 -- a directory needs its execute bit; 0700 is the tightest usable mode.
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}

	return dir, nil
}
