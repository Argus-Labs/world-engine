package transport

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/authn"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/goccy/go-json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rotisserie/eris"
)

// -------------------------------------------------------------------------------------------------
// Authentication
// -------------------------------------------------------------------------------------------------

// Player is the authenticated gameplay identity a client request carries.
type Player struct {
	ID string
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

func PlayerFromContext(ctx context.Context) *Player {
	info := authn.GetInfo(ctx)
	if info == nil {
		return nil
	}
	player, ok := info.(*Player)
	if !ok {
		return nil
	}
	return player
}

// -------------------------------------------------------------------------------------------------
// Argus Auth
// -------------------------------------------------------------------------------------------------

// ArgusAuthenticator authenticates game tokens issued by Argus Auth for one organization/project.
type ArgusAuthenticator struct {
	audience string
	keyfunc  keyfunc.Keyfunc
}

// NewArgusAuthenticator fetches Argus Auth's signing keys and returns an authenticator that
// accepts only EdDSA game tokens with aud equal to organization/project, an unexpired exp, and a
// non-empty sub. Pass its Authenticate method to authn.NewMiddleware.
func NewArgusAuthenticator(argusAuthURL, organization, project string) (*ArgusAuthenticator, error) {
	if argusAuthURL == "" {
		return nil, eris.New("argus auth URL cannot be empty")
	}
	// Argus Auth issues aud as exactly "organization/project", so a '/' in either part could never
	// match and every token would be rejected.
	if organization == "" || project == "" || strings.Contains(organization+project, "/") {
		return nil, eris.Errorf(
			"organization %q and project %q must be non-empty and must not contain '/' in ARGUS auth mode",
			organization, project,
		)
	}

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

	return &ArgusAuthenticator{
		audience: organization + "/" + project,
		keyfunc:  keyfn,
	}, nil
}

// Authenticate returns the *Player named by the request's bearer token. It satisfies
// authn.AuthFunc; every rejection is a connect.CodeUnauthenticated error.
func (a *ArgusAuthenticator) Authenticate(_ context.Context, req *http.Request) (any, error) {
	jwtString, ok := authn.BearerToken(req)
	if !ok {
		return nil, authn.Errorf("Authorization header must be in format: 'Bearer <JWT>'")
	}

	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(
		jwtString,
		claims,
		a.keyfunc.Keyfunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithAudience(a.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, authn.Errorf("invalid JWT: %v", err)
	}
	if !token.Valid {
		return nil, authn.Errorf("JWT token is invalid")
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return nil, authn.Errorf("JWT subject is required")
	}

	return &Player{ID: claims.Subject}, nil
}

// -------------------------------------------------------------------------------------------------
// Dev Auth
// -------------------------------------------------------------------------------------------------

// DevPlayerIDHeader names the caller's player ID in DEV auth mode.
const DevPlayerIDHeader = "X-Player-Id"

type authenticatorDev struct{}

func (a authenticatorDev) authenticate(_ context.Context, req *http.Request) (any, error) {
	playerID := strings.TrimSpace(req.Header.Get(DevPlayerIDHeader))
	if playerID == "" {
		return nil, authn.Errorf("%s header is required", DevPlayerIDHeader)
	}

	return &Player{ID: playerID}, nil
}
