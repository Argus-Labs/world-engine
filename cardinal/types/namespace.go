package types

import (
	"regexp"

	"github.com/rotisserie/eris"
)

var (
	regexAlphanumeric = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
)

// Namespace is a unique identifier for a world used for posting to the data availability layer and to prevent
// signature replay attacks across multiple worlds.
type Namespace string

func (n Namespace) String() string {
	return string(n)
}

// Validate validates that the namespace is alphanumeric or - (hyphen).
func (n Namespace) Validate() error {
	if !regexAlphanumeric.MatchString(n.String()) {
		return eris.New("Invalid namespace. A namespace must be alphanumeric.")
	}
	return nil
}
