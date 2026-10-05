package cardinal

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatorArgusAcceptsGamePlayerToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	server := newAuthTestServer(t, publicKey)
	authenticator, err := newAuthenticatorArgus(server.URL, "argus", "rampage")
	require.NoError(t, err)

	token := signToken(t, privateKey, jwt.RegisteredClaims{
		Subject:   "player-123",
		Issuer:    server.URL + "/auth",
		Audience:  jwt.ClaimStrings{"argus/rampage"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	})
	player, err := authenticator.authenticate(context.Background(), requestWithBearer(t, token))
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-123"}, player)
}

// Clients on Unity SDK 0.4 and earlier still send legacy login tokens, which Cardinal must keep accepting.
func TestAuthenticatorArgusAcceptsLegacyLoginToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	server := newAuthTestServer(t, publicKey)
	authenticator, err := newAuthenticatorArgus(server.URL, "argus", "rampage")
	require.NoError(t, err)

	// The claims Argus Auth puts in a legacy login token: the account fields, a separate persona ID, and
	// its own origin as the audience. The player is the account ID, as before game tokens.
	token := signToken(t, privateKey, jwt.MapClaims{
		"id":        "account-123",
		"email":     "player@example.com",
		"personaID": "persona-456",
		"sub":       "account-123",
		"iss":       server.URL,
		"aud":       server.URL,
		"exp":       time.Now().Add(time.Minute).Unix(),
	})
	player, err := authenticator.authenticate(context.Background(), requestWithBearer(t, token))
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "account-123"}, player)
}

func TestAuthenticatorArgusRejectsInvalidGameClaims(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, otherPrivateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	server := newAuthTestServer(t, publicKey)
	authenticator, err := newAuthenticatorArgus(server.URL, "argus", "rampage")
	require.NoError(t, err)

	validClaims := jwt.RegisteredClaims{
		Subject:   "player-123",
		Issuer:    server.URL + "/auth",
		Audience:  jwt.ClaimStrings{"argus/rampage"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}
	for _, test := range []struct {
		name   string
		key    ed25519.PrivateKey
		claims jwt.Claims
	}{
		{name: "untrusted signature", key: otherPrivateKey, claims: validClaims},
		{
			name: "wrong audience", key: privateKey,
			claims: jwt.RegisteredClaims{
				Subject: validClaims.Subject, Issuer: validClaims.Issuer,
				Audience: jwt.ClaimStrings{"argus/other"}, ExpiresAt: validClaims.ExpiresAt,
			},
		},
		{
			name: "missing audience", key: privateKey,
			claims: jwt.RegisteredClaims{
				Subject: validClaims.Subject, Issuer: validClaims.Issuer, ExpiresAt: validClaims.ExpiresAt,
			},
		},
		{
			// An ordinary account token from Argus Auth has the account ID but is not a legacy login token.
			name: "account token", key: privateKey,
			claims: jwt.MapClaims{
				"id": "account-123", "sub": "account-123", "aud": server.URL,
				"exp": time.Now().Add(time.Minute).Unix(),
			},
		},
		{
			name: "missing expiry",
			key:  privateKey,
			claims: jwt.RegisteredClaims{
				Subject: validClaims.Subject, Issuer: validClaims.Issuer, Audience: validClaims.Audience,
			},
		},
		{
			name: "expired",
			key:  privateKey,
			claims: jwt.RegisteredClaims{
				Subject:  validClaims.Subject,
				Issuer:   validClaims.Issuer,
				Audience: validClaims.Audience,
				ExpiresAt: jwt.NewNumericDate(
					time.Now().Add(-time.Minute),
				),
			},
		},
		{
			name: "missing subject",
			key:  privateKey,
			claims: jwt.RegisteredClaims{
				Issuer: validClaims.Issuer, Audience: validClaims.Audience, ExpiresAt: validClaims.ExpiresAt,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := signToken(t, test.key, test.claims)
			_, authErr := authenticator.authenticate(context.Background(), requestWithBearer(t, token))
			require.Error(t, authErr)
		})
	}

	t.Run("non-EdDSA algorithm", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims)
		token.Header["kid"] = "test-key"
		signed, signErr := token.SignedString([]byte("test-secret"))
		require.NoError(t, signErr)
		_, authErr := authenticator.authenticate(context.Background(), requestWithBearer(t, signed))
		require.Error(t, authErr)
	})
}

func TestAuthenticatorDevUsesPlayerID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Player-Id", " player-123 ")

	player, err := (authenticatorDev{}).authenticate(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-123"}, player)

	// Unity SDK 0.4 and earlier send the dev player ID as X-Email.
	legacyReq := httptest.NewRequest(http.MethodGet, "/", nil)
	legacyReq.Header.Set("X-Email", "dev@example.com")
	player, err = (authenticatorDev{}).authenticate(context.Background(), legacyReq)
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "dev@example.com"}, player)

	_, err = (authenticatorDev{}).authenticate(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil))
	require.Error(t, err)
}

func newAuthTestServer(t *testing.T, publicKey ed25519.PublicKey) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/auth/jwks" {
			http.NotFound(w, req)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "OKP",
			"crv": "Ed25519",
			"alg": "EdDSA",
			"use": "sig",
			"kid": "test-key",
			"x":   base64.RawURLEncoding.EncodeToString(publicKey),
		}}})
	}))
	t.Cleanup(server.Close)
	return server
}

func signToken(t *testing.T, privateKey ed25519.PrivateKey, claims jwt.Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(privateKey)
	require.NoError(t, err)
	return signed
}

func requestWithBearer(t *testing.T, token string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}
