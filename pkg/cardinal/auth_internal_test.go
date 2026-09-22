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
	authenticator, err := newAuthenticatorArgus(server.URL + "/")
	require.NoError(t, err)

	token := signGameToken(t, privateKey, jwt.RegisteredClaims{
		Subject:   "player-123",
		Issuer:    server.URL + "/auth",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}, "game")
	player, err := authenticator.authenticate(context.Background(), requestWithBearer(t, token))
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-123"}, player)
}

func TestAuthenticatorArgusRejectsInvalidGameClaims(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, otherPrivateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	server := newAuthTestServer(t, publicKey)
	authenticator, err := newAuthenticatorArgus(server.URL)
	require.NoError(t, err)

	validClaims := jwt.RegisteredClaims{
		Subject:   "player-123",
		Issuer:    server.URL + "/auth",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}
	for _, test := range []struct {
		name     string
		key      ed25519.PrivateKey
		claims   jwt.RegisteredClaims
		tokenUse string
	}{
		{name: "untrusted signature", key: otherPrivateKey, claims: validClaims, tokenUse: "game"},
		{name: "account token", key: privateKey, claims: validClaims},
		{name: "wrong issuer", key: privateKey, claims: jwt.RegisteredClaims{Subject: "player-123", Issuer: "https://other.example/auth", ExpiresAt: validClaims.ExpiresAt}, tokenUse: "game"},
		{name: "missing expiry", key: privateKey, claims: jwt.RegisteredClaims{Subject: "player-123", Issuer: validClaims.Issuer}, tokenUse: "game"},
		{name: "expired", key: privateKey, claims: jwt.RegisteredClaims{Subject: "player-123", Issuer: validClaims.Issuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}, tokenUse: "game"},
		{name: "missing subject", key: privateKey, claims: jwt.RegisteredClaims{Issuer: validClaims.Issuer, ExpiresAt: validClaims.ExpiresAt}, tokenUse: "game"},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := signGameToken(t, test.key, test.claims, test.tokenUse)
			_, authErr := authenticator.authenticate(context.Background(), requestWithBearer(t, token))
			require.Error(t, authErr)
		})
	}
}

func TestAuthenticatorDevUsesPlayerID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Player-ID", " player-123 ")

	player, err := (authenticatorDev{}).authenticate(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-123"}, player)

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

func signGameToken(t *testing.T, privateKey ed25519.PrivateKey, claims jwt.RegisteredClaims, tokenUse string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, gameTokenClaims{
		RegisteredClaims: claims,
		TokenUse:         tokenUse,
	})
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
