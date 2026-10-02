package transport

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

// -------------------------------------------------------------------------------------------------
// Authentication
// -------------------------------------------------------------------------------------------------

type User struct {
	jwt.RegisteredClaims

	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// AuthMode selects the authentication mode for the client-facing ConnectRPC service.
type AuthMode uint8

const (
	AuthModeUndefined AuthMode = iota
	AuthModeArgus
	AuthModeDev
)

const (
	argusAuthModeString     = "ARGUS"
	devAuthModeString       = "DEV"
	undefinedAuthModeString = "UNDEFINED"
)

func (a AuthMode) String() string {
	switch a {
	case AuthModeUndefined:
		return undefinedAuthModeString
	case AuthModeArgus:
		return argusAuthModeString
	case AuthModeDev:
		return devAuthModeString
	default:
		return undefinedAuthModeString
	}
}

func (a AuthMode) IsValid() bool {
	return a == AuthModeArgus || a == AuthModeDev
}

func ParseAuthMode(s string) (AuthMode, error) {
	switch strings.ToUpper(s) {
	case argusAuthModeString:
		return AuthModeArgus, nil
	case devAuthModeString:
		return AuthModeDev, nil
	default:
		return AuthModeUndefined, eris.Errorf("invalid auth mode: %s", s)
	}
}

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
// Argus Auth
// -------------------------------------------------------------------------------------------------

type authenticatorArgus struct {
	keyfunc keyfunc.Keyfunc
}

func newAuthenticatorArgus(argusAuthURL string) (*authenticatorArgus, error) {
	assert.That(argusAuthURL != "", "Should've validated the URL")

	jwksURL := argusAuthURL + "/auth/jwks"
	client := &http.Client{
		Timeout: 3 * time.Second,
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
