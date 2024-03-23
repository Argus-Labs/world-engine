package cardinal

import (
	"regexp"

	"github.com/rotisserie/eris"
)

// Namespace is a unique identifier for a world used for posting to the data availability layer and to prevent
// signature replay attacks across multiple worlds.
type Namespace string

func (n Namespace) String() string {
	return string(n)
}

// Validate validates that the namespace is alphanumeric and not the default namespace in production mode.
func (n Namespace) Validate(mode RunMode) error {
	if !regexp.MustCompile(`^[a-zA-Z0-9-]+$`).MatchString(n.String()) {
		return eris.New("Invalid namespace. A namespace must be alphanumeric.")
	}
	if mode == RunModeProd {
		if n.String() == DefaultNamespace {
			return eris.New("Default namespace is not allowed in production mode.")
		}
	}
	return nil
}
