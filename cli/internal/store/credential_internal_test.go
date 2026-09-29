package store

import (
	"os"
	"path/filepath"
	"testing"
)

// The credential is a bearer token with operator access: the file must not be
// readable by anyone else, and neither must the directory holding it.
func TestPutCredentialIsPrivate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := PutCredential(Credential{JWT: "header.payload.sig"}); err != nil {
		t.Fatalf("PutCredential() error = %v", err)
	}

	dir := filepath.Join(home, DefaultStoreDirName)
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("store directory mode = %o, want 700", perm)
	}

	fileInfo, err := os.Stat(filepath.Join(dir, credentialFile))
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("credential file mode = %o, want 600", perm)
	}
}

// SetupStoreDir must tighten a directory left at 0755 by an older CLI, which
// MkdirAll alone would not do.
func TestSetupStoreDirTightensExistingDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, DefaultStoreDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("seed dir: %v", err)
	}

	if _, err := SetupStoreDir(); err != nil {
		t.Fatalf("SetupStoreDir() error = %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("mode = %o, want 700 — an existing 0755 dir was left loose", perm)
	}
}

func TestGetCredentialRoundTrips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := PutCredential(Credential{JWT: "a.b.c"}); err != nil {
		t.Fatalf("PutCredential() error = %v", err)
	}
	got, err := GetCredential()
	if err != nil {
		t.Fatalf("GetCredential() error = %v", err)
	}
	if got.JWT != "a.b.c" {
		t.Errorf("JWT = %q, want a.b.c", got.JWT)
	}
}

func TestGetCredentialMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := GetCredential(); !os.IsNotExist(err) {
		t.Errorf("GetCredential() error = %v, want a not-exist error", err)
	}
}
