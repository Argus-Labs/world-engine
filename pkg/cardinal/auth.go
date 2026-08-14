package cardinal

import (
	"context"

	"github.com/argus-labs/world-engine/pkg/auth"
)

// Authentication lives in pkg/auth so runtimes without a world can serve the same client-facing
// protocol. These aliases keep the long-standing cardinal.* spelling working for game code; new
// callers should prefer the auth package directly.

type (
	// User is the authenticated caller behind a request. See auth.User.
	User = auth.User

	// AuthMode selects the authentication mode for the client-facing ConnectRPC service.
	// See auth.Mode.
	AuthMode = auth.Mode
)

const (
	AuthModeUndefined = auth.ModeUndefined
	AuthModeArgus     = auth.ModeArgus
	AuthModeDev       = auth.ModeDev
)

// ParseAuthMode converts a mode name (case-insensitive) to an AuthMode. See auth.ParseMode.
func ParseAuthMode(s string) (AuthMode, error) {
	return auth.ParseMode(s)
}

// UserFromContext returns the authenticated user for the request being served, or nil if the
// request did not pass through the authentication middleware. See auth.UserFromContext.
func UserFromContext(ctx context.Context) *User {
	return auth.UserFromContext(ctx)
}
