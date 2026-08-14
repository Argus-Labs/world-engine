package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{in: "ARGUS", want: ModeArgus},
		{in: "argus", want: ModeArgus},
		{in: "DEV", want: ModeDev},
		{in: "dev", want: ModeDev},
		{in: "UNDEFINED", wantErr: true},
		{in: "", wantErr: true},
		{in: "nope", wantErr: true},
	} {
		got, err := ParseMode(tc.in)
		if tc.wantErr {
			require.Error(t, err, "ParseMode(%q) should reject", tc.in)
			require.Equal(t, ModeUndefined, got, "rejected mode should be ModeUndefined")
			continue
		}
		require.NoError(t, err, "ParseMode(%q)", tc.in)
		require.Equal(t, tc.want, got, "ParseMode(%q)", tc.in)
	}
}

func TestMode_StringRoundTrips(t *testing.T) {
	t.Parallel()

	for _, mode := range []Mode{ModeArgus, ModeDev} {
		got, err := ParseMode(mode.String())
		require.NoError(t, err, "String() of a valid mode must parse back")
		require.Equal(t, mode, got)
	}

	require.Equal(t, undefinedModeString, ModeUndefined.String())
	require.Equal(t, undefinedModeString, Mode(99).String(), "unknown values must not claim a real mode")
}

func TestMode_IsValid(t *testing.T) {
	t.Parallel()

	require.True(t, ModeArgus.IsValid())
	require.True(t, ModeDev.IsValid())
	require.False(t, ModeUndefined.IsValid(), "undefined must never authenticate anything")
	require.False(t, Mode(99).IsValid())
}

// TestNewMiddleware_RejectsUnusableMode verifies an unset or unknown mode is a startup error rather
// than a service that silently accepts every request.
func TestNewMiddleware_RejectsUnusableMode(t *testing.T) {
	t.Parallel()

	for _, mode := range []Mode{ModeUndefined, Mode(99)} {
		mw, err := NewMiddleware(mode, "")
		require.Error(t, err, "mode %s must not produce middleware", mode)
		require.Nil(t, mw)
	}
}

// TestDevMiddleware_PopulatesUser is the contract handlers depend on: after a request passes the
// middleware, UserFromContext resolves the caller. A command's persona is read from there, so a
// regression here is an authentication bypass, not a cosmetic bug.
func TestDevMiddleware_PopulatesUser(t *testing.T) {
	t.Parallel()

	middleware, err := NewMiddleware(ModeDev, "")
	require.NoError(t, err)

	var got *User
	handler := middleware.Wrap(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = UserFromContext(r.Context())
	}))

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	request.Header.Set("X-Email", "  player@example.com  ")

	response, err := server.Client().Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)

	require.NotNil(t, got, "handler must see an authenticated user")
	require.Equal(t, "player@example.com", got.Email, "surrounding whitespace must be trimmed")
	require.Equal(t, "player@example.com", got.ID)
}

// TestDevMiddleware_RejectsMissingEmail verifies the handler never runs without an identity.
func TestDevMiddleware_RejectsMissingEmail(t *testing.T) {
	t.Parallel()

	middleware, err := NewMiddleware(ModeDev, "")
	require.NoError(t, err)

	reached := false
	handler := middleware.Wrap(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		reached = true
	}))

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	for _, email := range []string{"", "   "} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		if email != "" {
			request.Header.Set("X-Email", email)
		}

		response, err := server.Client().Do(request)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())

		require.Equal(t, http.StatusUnauthorized, response.StatusCode, "X-Email %q must be rejected", email)
		require.False(t, reached, "handler must not run for an unauthenticated request")
	}
}

// TestUserFromContext_NilWithoutMiddleware verifies an unauthenticated context yields nil rather
// than a zero-valued User that would read as a caller with an empty ID.
func TestUserFromContext_NilWithoutMiddleware(t *testing.T) {
	t.Parallel()

	require.Nil(t, UserFromContext(t.Context()))
}
