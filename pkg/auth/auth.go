// Package auth authenticates client requests to a shard's ConnectRPC service.
//
// Two modes ship: ARGUS validates a bearer JWT against the Argus Auth service's JWKS, and DEV
// trusts an X-Email header. Both resolve a request to a *User, which downstream handlers read
// with UserFromContext — a command's persona comes from there, never from the request body.
//
// Callers wire it in one step:
//
//	middleware, err := auth.NewMiddleware(auth.ModeDev, "")
//	if err != nil {
//	    return err
//	}
//	mux.Handle(path, middleware.Wrap(handler))
//
// The package holds no world or tick state, so any runtime serving the client-facing protocol can
// use it, whether or not it has a simulation loop.
package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/authn"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/goccy/go-json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rotisserie/eris"
)

// jwksFetchTimeout bounds the JWKS fetch at startup. Failing fast beats hanging a boot.
const jwksFetchTimeout = 3 * time.Second

// User is the authenticated caller behind a request.
type User struct {
	jwt.RegisteredClaims

	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// UserFromContext returns the authenticated user for the request being served, or nil if the
// request did not pass through NewMiddleware's wrapper.
func UserFromContext(ctx context.Context) *User {
	info := authn.GetInfo(ctx)
	if info == nil {
		return nil
	}
	user, ok := info.(*User)
	if !ok {
		return nil
	}
	return user
}

// -------------------------------------------------------------------------------------------------
// Mode
// -------------------------------------------------------------------------------------------------

// Mode selects the authentication mode for the client-facing ConnectRPC service.
type Mode uint8

const (
	ModeUndefined Mode = iota
	ModeArgus
	ModeDev
)

const (
	argusModeString     = "ARGUS"
	devModeString       = "DEV"
	undefinedModeString = "UNDEFINED"
)

func (m Mode) String() string {
	switch m {
	case ModeUndefined:
		return undefinedModeString
	case ModeArgus:
		return argusModeString
	case ModeDev:
		return devModeString
	default:
		return undefinedModeString
	}
}

// IsValid reports whether m names a usable mode. ModeUndefined is not usable.
func (m Mode) IsValid() bool {
	return m == ModeArgus || m == ModeDev
}

// ParseMode converts a mode name (case-insensitive) to a Mode.
func ParseMode(s string) (Mode, error) {
	switch strings.ToUpper(s) {
	case argusModeString:
		return ModeArgus, nil
	case devModeString:
		return ModeDev, nil
	default:
		return ModeUndefined, eris.Errorf("invalid auth mode: %s", s)
	}
}

// -------------------------------------------------------------------------------------------------
// Middleware
// -------------------------------------------------------------------------------------------------

// NewMiddleware builds the authentication middleware for mode. argusAuthURL is required when mode
// is ModeArgus and ignored otherwise; in that mode this fetches the JWKS, so it performs network
// I/O and should be called once at startup, not per request.
func NewMiddleware(mode Mode, argusAuthURL string) (*authn.Middleware, error) {
	var authenticate func(context.Context, *http.Request) (any, error)

	switch mode {
	case ModeArgus:
		authenticator, err := newAuthenticatorArgus(argusAuthURL)
		if err != nil {
			return nil, eris.Wrap(err, "failed to create argus authenticator")
		}
		authenticate = authenticator.authenticate
	case ModeDev:
		authenticate = authenticatorDev{}.authenticate
	case ModeUndefined:
		fallthrough
	default:
		return nil, eris.Errorf("invalid service auth mode: %s", mode)
	}

	return authn.NewMiddleware(authenticate), nil
}

// -------------------------------------------------------------------------------------------------
// Argus Auth
// -------------------------------------------------------------------------------------------------

type authenticatorArgus struct {
	keyfunc keyfunc.Keyfunc
}

func newAuthenticatorArgus(argusAuthURL string) (*authenticatorArgus, error) {
	assert.That(argusAuthURL != "", "Should've validated the URL")

	jwksURL := argusAuthURL + "/auth/jwks"
	client := &http.Client{
		Timeout: jwksFetchTimeout,
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create JWKS request")
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, eris.Wrap(err, "failed to fetch JWKS")
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, eris.Errorf("HTTP error: %d - %s", response.StatusCode, response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, eris.Wrap(err, "failed to read response body")
	}

	keyfn, err := keyfunc.NewJWKSetJSON(json.RawMessage(body))
	if err != nil {
		return nil, eris.Wrap(err, "failed to create keyfunc")
	}

	return &authenticatorArgus{keyfunc: keyfn}, nil
}

func (a *authenticatorArgus) authenticate(_ context.Context, req *http.Request) (any, error) {
	jwtString, ok := authn.BearerToken(req)
	if !ok {
		return nil, authn.Errorf("Authorization header must be in format: 'Bearer <JWT>'")
	}

	user := &User{}
	token, err := jwt.ParseWithClaims(jwtString, user, a.keyfunc.Keyfunc)
	if err != nil {
		return nil, eris.Wrap(err, "JWT parse error")
	}
	if !token.Valid {
		return nil, eris.New("JWT token is invalid")
	}

	// TODO: Remove this comment once persona ID is removed from the JWT.
	// if u.PersonaID == "" {
	// 	return nil, authn.Errorf("JWT token is missing persona ID")
	// }

	return user, nil
}

// -------------------------------------------------------------------------------------------------
// Dev Auth
// -------------------------------------------------------------------------------------------------

type authenticatorDev struct{}

func (a authenticatorDev) authenticate(_ context.Context, req *http.Request) (any, error) {
	email := strings.TrimSpace(req.Header.Get("X-Email"))
	if email == "" {
		return nil, authn.Errorf("X-Email header is required")
	}

	return &User{ID: email, Email: email}, nil
}
