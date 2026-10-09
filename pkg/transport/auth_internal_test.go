package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/goccy/go-json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatorArgusAcceptsGamePlayerToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	server := newAuthTestServer(t, publicKey)
	authenticator, err := NewArgusAuthenticator(server.URL, "argus", "rampage")
	require.NoError(t, err)

	token := signGameToken(t, privateKey, jwt.RegisteredClaims{
		Subject:   "player-123",
		Issuer:    server.URL + "/auth",
		Audience:  jwt.ClaimStrings{"argus/rampage"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	})
	player, err := authenticator.Authenticate(context.Background(), requestWithBearer(t, token))
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-123"}, player)
}

func TestAuthenticatorArgusRejectsInvalidGameClaims(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, otherPrivateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	server := newAuthTestServer(t, publicKey)
	authenticator, err := NewArgusAuthenticator(server.URL, "argus", "rampage")
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
		claims jwt.RegisteredClaims
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
			token := signGameToken(t, test.key, test.claims)
			_, authErr := authenticator.Authenticate(context.Background(), requestWithBearer(t, token))
			requireUnauthenticated(t, authErr)
		})
	}

	t.Run("non-EdDSA algorithm", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims)
		token.Header["kid"] = "test-key"
		signed, signErr := token.SignedString([]byte("test-secret"))
		require.NoError(t, signErr)
		_, authErr := authenticator.Authenticate(context.Background(), requestWithBearer(t, signed))
		requireUnauthenticated(t, authErr)
	})
}

// A JWKS may publish a key without "alg", which keyfunc then accepts for any token algorithm.
// Only the EdDSA allowlist stops a token signed by such a non-EdDSA key.
func TestAuthenticatorArgusRejectsNonEdDSAKeyFromJWKS(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	server := newAuthTestServer(t, publicKey, map[string]string{
		"kty": "RSA",
		"use": "sig",
		"kid": "rsa-key",
		"n":   base64.RawURLEncoding.EncodeToString(rsaKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(rsaKey.E)).Bytes()),
	})
	authenticator, err := NewArgusAuthenticator(server.URL, "argus", "rampage")
	require.NoError(t, err)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Subject:   "player-123",
		Audience:  jwt.ClaimStrings{"argus/rampage"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	})
	token.Header["kid"] = "rsa-key"
	signed, err := token.SignedString(rsaKey)
	require.NoError(t, err)

	_, err = authenticator.Authenticate(context.Background(), requestWithBearer(t, signed))
	requireUnauthenticated(t, err)
}

func TestNewArgusAuthenticatorRejectsInvalidGameID(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	server := newAuthTestServer(t, publicKey)

	for _, test := range []struct {
		name         string
		organization string
		project      string
	}{
		{name: "slash in organization", organization: "argus/games", project: "rampage"},
		{name: "slash in project", organization: "argus", project: "games/rampage"},
		{name: "empty organization", organization: "", project: "rampage"},
		{name: "empty project", organization: "argus", project: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewArgusAuthenticator(server.URL, test.organization, test.project)
			require.ErrorContains(t, err, "must not contain '/'")
		})
	}
}

func TestAuthenticatorDevUsesPlayerID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Player-Id", " player-123 ")

	player, err := (authenticatorDev{}).authenticate(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-123"}, player)

	_, err = (authenticatorDev{}).authenticate(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil))
	requireUnauthenticated(t, err)
}

func requireUnauthenticated(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "error: %v", err)
}

// newAuthTestServer serves a JWKS with publicKey as "test-key", followed by extraKeys.
func newAuthTestServer(t *testing.T, publicKey ed25519.PublicKey, extraKeys ...map[string]string) *httptest.Server {
	t.Helper()
	keys := append([]map[string]string{{
		"kty": "OKP",
		"crv": "Ed25519",
		"alg": "EdDSA",
		"use": "sig",
		"kid": "test-key",
		"x":   base64.RawURLEncoding.EncodeToString(publicKey),
	}}, extraKeys...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/auth/jwks" {
			http.NotFound(w, req)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(server.Close)
	return server
}

func signGameToken(t *testing.T, privateKey ed25519.PrivateKey, claims jwt.RegisteredClaims) string {
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
