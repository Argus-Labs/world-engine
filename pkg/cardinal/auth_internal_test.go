package cardinal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
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
	authenticator, err := NewArgusAuthenticator(server.URL, "argus/rampage")
	require.NoError(t, err)
	t.Cleanup(authenticator.Close)

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
	authenticator, err := NewArgusAuthenticator(server.URL, "argus/rampage")
	require.NoError(t, err)
	t.Cleanup(authenticator.Close)

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
	authenticator, err := NewArgusAuthenticator(server.URL, "argus/rampage")
	require.NoError(t, err)
	t.Cleanup(authenticator.Close)

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

	for _, gameID := range []string{"", "rampage", "/rampage", "argus/", "argus/games/rampage", "argus/ram page"} {
		_, err := NewArgusAuthenticator(server.URL, gameID)
		require.ErrorContains(t, err, "must have the form <owner>/<name>", "game ID %q", gameID)
	}
}

// TestArgusAuthenticatorAcceptsNewKeyAfterRotation is the regression test for the original bug:
// the authenticator fetched the JWKS exactly once at startup, so a token signed by a key added
// afterward was rejected with "key not found". With the refresh-capable keyfunc, a token whose
// kid is unknown triggers an on-demand re-fetch, so the new key is accepted without a restart.
func TestArgusAuthenticatorAcceptsNewKeyAfterRotation(t *testing.T) {
	pubA, privA, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pubB, privB, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyA := ed25519JWK("key-a", pubA)
	keyB := ed25519JWK("key-b", pubB)

	server := newMutableJWKSServer(t, keyA)
	authenticator, err := NewArgusAuthenticator(server.URL, "argus/rampage")
	require.NoError(t, err)
	t.Cleanup(authenticator.Close)

	// The key present at startup is trusted.
	tokA := signGameTokenWithKID(t, privA, "key-a", gameClaims("player-a"))
	player, err := authenticator.Authenticate(context.Background(), requestWithBearer(t, tokA))
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-a"}, player)

	// Rotate: publish key-b alongside key-a (standard overlap window). Before the fix the
	// frozen key set never learned about key-b, so this token failed with "key not found".
	server.setKeys(keyA, keyB)
	tokB := signGameTokenWithKID(t, privB, "key-b", gameClaims("player-b"))
	player, err = authenticator.Authenticate(context.Background(), requestWithBearer(t, tokB))
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-b"}, player)
}

// TestArgusAuthenticatorRejectsRetiredKeyAfterRotation covers the stale-trust half of the bug:
// once Argus Auth retires a key and the authenticator re-fetches, tokens signed by the retired
// key must stop being accepted. A short on-demand re-fetch interval keeps the test fast.
func TestArgusAuthenticatorRejectsRetiredKeyAfterRotation(t *testing.T) {
	pubA, privA, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pubB, privB, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyA := ed25519JWK("key-a", pubA)
	keyB := ed25519JWK("key-b", pubB)

	server := newMutableJWKSServer(t, keyA)
	authenticator, err := newArgusAuthenticator(server.URL, "argus/rampage", refreshOptions{
		refreshUnknownKIDEvery: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	t.Cleanup(authenticator.Close)

	// key-a is trusted while it is the only published key.
	tokA := signGameTokenWithKID(t, privA, "key-a", gameClaims("player-a"))
	player, err := authenticator.Authenticate(context.Background(), requestWithBearer(t, tokA))
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-a"}, player)

	// Retire key-a: publish key-b only. A key-b token forces a re-fetch that replaces the local
	// key set with whatever the JWKS serves (now just key-b).
	server.setKeys(keyB)
	tokB := signGameTokenWithKID(t, privB, "key-b", gameClaims("player-b"))
	player, err = authenticator.Authenticate(context.Background(), requestWithBearer(t, tokB))
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-b"}, player)

	// The retired key-a is gone from the local key set, so its tokens are now rejected. Before
	// the fix the static key set kept key-a forever and this token was still accepted.
	tokA2 := signGameTokenWithKID(t, privA, "key-a", gameClaims("player-a"))
	_, err = authenticator.Authenticate(context.Background(), requestWithBearer(t, tokA2))
	requireUnauthenticated(t, err)
}

// TestArgusAuthenticatorPeriodicRefreshRetiresKeys verifies the background refresh goroutine
// (not just the on-demand unknown-kid re-fetch) replaces the local key set, so a retired key
// stops being trusted even when no token with the new key ID arrives. A short refresh interval
// keeps the test fast.
func TestArgusAuthenticatorPeriodicRefreshRetiresKeys(t *testing.T) {
	pubA, privA, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pubB, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyA := ed25519JWK("key-a", pubA)
	keyB := ed25519JWK("key-b", pubB)

	server := newMutableJWKSServer(t, keyA)
	authenticator, err := newArgusAuthenticator(server.URL, "argus/rampage", refreshOptions{
		refreshInterval:        50 * time.Millisecond,
		refreshUnknownKIDEvery: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	t.Cleanup(authenticator.Close)

	tokA := signGameTokenWithKID(t, privA, "key-a", gameClaims("player-a"))
	player, err := authenticator.Authenticate(context.Background(), requestWithBearer(t, tokA))
	require.NoError(t, err)
	require.Equal(t, &Player{ID: "player-a"}, player)

	// Retire key-a and publish key-b only, then wait for the periodic refresh to replace the
	// local key set. Before the fix there was no refresh goroutine, so key-a was trusted forever.
	server.setKeys(keyB)
	require.Eventually(t, func() bool {
		_, authErr := authenticator.Authenticate(context.Background(), requestWithBearer(t, tokA))
		return authErr != nil
	}, 2*time.Second, 20*time.Millisecond, "key-a should be rejected after the background refresh retires it")
	_, err = authenticator.Authenticate(context.Background(), requestWithBearer(t, tokA))
	requireUnauthenticated(t, err)
}

// TestNewArgusAuthenticatorFailsFastWhenJWKSUnreachable ensures the refresh-capable constructor
// still fails at startup when the JWKS cannot be fetched, instead of silently starting with an
// empty key set (keyfunc.NewDefault does not return this error by default).
func TestNewArgusAuthenticatorFailsFastWhenJWKSUnreachable(t *testing.T) {
	t.Run("non-200 response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "down", http.StatusInternalServerError)
		}))
		t.Cleanup(server.Close)

		_, err := NewArgusAuthenticator(server.URL, "argus/rampage")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to create keyfunc")
	})

	t.Run("connection refused", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"keys":[]}`))
		}))
		server.Close() // Stop the server so the JWKS fetch is unreachable.

		_, err := NewArgusAuthenticator(server.URL, "argus/rampage")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to create keyfunc")
	})
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
	return signGameTokenWithKID(t, privateKey, "test-key", claims)
}

func signGameTokenWithKID(t *testing.T, privateKey ed25519.PrivateKey, kid string, claims jwt.RegisteredClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = kid
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

// gameClaims builds a valid Argus Auth game-token claim set for the given subject. The audience is
// the fixed game ID used by these tests.
func gameClaims(subject string) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Subject:   subject,
		Audience:  jwt.ClaimStrings{"argus/rampage"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
}

// ed25519JWK returns the public JWK JSON object for an Ed25519 key with the given kid.
func ed25519JWK(kid string, publicKey ed25519.PublicKey) map[string]string {
	return map[string]string{
		"kty": "OKP",
		"crv": "Ed25519",
		"alg": "EdDSA",
		"use": "sig",
		"kid": kid,
		"x":   base64.RawURLEncoding.EncodeToString(publicKey),
	}
}

// mutableJWKSServer is an httptest JWKS endpoint whose served keys can be rotated at runtime,
// so tests can simulate Argus Auth adding or retiring a signing key after startup.
type mutableJWKSServer struct {
	*httptest.Server

	mu   sync.Mutex
	keys []map[string]string
}

func newMutableJWKSServer(t *testing.T, keys ...map[string]string) *mutableJWKSServer {
	t.Helper()
	server := &mutableJWKSServer{keys: keys}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		server.mu.Lock()
		snapshot := append([]map[string]string(nil), server.keys...)
		server.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": snapshot})
	}))
	t.Cleanup(server.Close)
	return server
}

func (s *mutableJWKSServer) setKeys(keys ...map[string]string) {
	s.mu.Lock()
	s.keys = keys
	s.mu.Unlock()
}
