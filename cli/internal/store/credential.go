package store

import (
	"os"
	"path/filepath"

	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
)

// credentialFile is unchanged from when auth owned it, so existing sign-ins
// survive.
const credentialFile = "auth.json"

// Credential is the cached Argus token. Only the JWT is stored; identity and
// expiry are claims inside it. Commands that call remote services read it from
// here rather than reimplementing the file layout.
type Credential struct {
	JWT string `json:"jwt"`
}

// PutCredential writes the credential into the store directory.
func PutCredential(c Credential) error {
	dir, err := SetupStoreDir()
	if err != nil {
		return eris.Wrap(err, "creating the credential directory")
	}
	data, err := json.Marshal(c)
	if err != nil {
		return eris.Wrap(err, "encoding credentials")
	}
	// 0600: this is a bearer token with operator access.
	return os.WriteFile(filepath.Join(dir, credentialFile), data, 0o600)
}

// GetCredential reads the cached credential. A missing file surfaces as an
// os.IsNotExist error, which callers treat as "not signed in".
func GetCredential() (Credential, error) {
	dir, err := GetStoreDir()
	if err != nil {
		return Credential{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, credentialFile))
	if err != nil {
		return Credential{}, err
	}
	var c Credential
	if err := json.Unmarshal(data, &c); err != nil {
		return Credential{}, eris.Wrap(err, "reading cached credentials")
	}
	return c, nil
}
