package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"

	"github.com/argus-labs/world-engine/cli/internal/store"
)

// jwtWithExp builds an unsigned JWT: only the payload is read here, and the
// operator is what verifies signatures.
func jwtWithExp(t *testing.T, exp time.Time, email string) string {
	t.Helper()
	payload, err := json.Marshal(Claims{Email: email, PersonaID: "p-1", Exp: exp.Unix()})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"
}

func TestParseClaims(t *testing.T) {
	exp := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	claims, err := ParseClaims(jwtWithExp(t, exp, "dev@argus.gg"))
	if err != nil {
		t.Fatalf("ParseClaims() error = %v", err)
	}
	if claims.Email != "dev@argus.gg" {
		t.Errorf("Email = %q, want dev@argus.gg", claims.Email)
	}
	if claims.PersonaID != "p-1" {
		t.Errorf("PersonaID = %q, want p-1", claims.PersonaID)
	}
	if !claims.ExpiresAt().Equal(exp) {
		t.Errorf("ExpiresAt() = %v, want %v", claims.ExpiresAt(), exp)
	}
}

func TestParseClaimsRejectsMalformed(t *testing.T) {
	for _, tt := range []struct{ name, jwt string }{
		{"not a jwt", "nonsense"},
		{"wrong segment count", "a.b"},
		{"payload is not base64", "h.!!!.s"},
		{"no expiry", "h." + base64.RawURLEncoding.EncodeToString([]byte(`{"email":"x"}`)) + ".s"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseClaims(tt.jwt); err == nil {
				t.Error("ParseClaims() error = nil, want an error")
			}
		})
	}
}

// An expired cached token must not be returned, or every call fails at the
// operator with an auth error instead of prompting a fresh sign-in.
func TestCachedRejectsExpiredToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := &Client{}

	for _, tt := range []struct {
		name    string
		exp     time.Time
		wantErr bool
	}{
		{"valid", time.Now().Add(24 * time.Hour), false},
		{"expired", time.Now().Add(-time.Hour), true},
		{"expiring inside the renew window", time.Now().Add(renewBefore / 2), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cred := store.Credential{JWT: jwtWithExp(t, tt.exp, "dev@argus.gg")}
			if err := store.PutCredential(cred); err != nil {
				t.Fatalf("PutCredential() error = %v", err)
			}
			_, err := c.cached()
			if (err != nil) != tt.wantErr {
				t.Errorf("cached() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCachedMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := &Client{}
	if _, err := c.cached(); err == nil {
		t.Error("cached() error = nil for a missing file, want an error")
	}
}

func TestCreateSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != sessionPath {
			t.Errorf("got %s %s, want POST %s", r.Method, r.URL.Path, sessionPath)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		_, _ = fmt.Fprint(w, `{"clientUrl":"https://auth/authorize?sessionId=1",`+
			`"callbackUrl":"https://auth/status?sessionId=1"}`)
	}))
	defer srv.Close()

	c := &Client{authURL: srv.URL, http: srv.Client()}
	session, err := c.createSession(context.Background())
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	if session.ClientURL == "" || session.CallbackURL == "" {
		t.Errorf("createSession() = %+v, want both URLs populated", session)
	}
}

func TestCreateSessionRejectsIncompleteResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"clientUrl":"https://auth/authorize"}`)
	}))
	defer srv.Close()

	c := &Client{authURL: srv.URL, http: srv.Client()}
	if _, err := c.createSession(context.Background()); err == nil {
		t.Error("createSession() error = nil for a response with no callbackUrl, want an error")
	}
}

// pending is the normal case while the user is still in the browser.
func TestPollWaitsThroughPending(t *testing.T) {
	want := jwtWithExp(t, time.Now().Add(time.Hour), "dev@argus.gg")
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			_, _ = fmt.Fprint(w, `{"status":"pending"}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"status":"success","jwt":%q}`, want)
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	got, err := c.pollEvery(context.Background(), srv.URL, time.Millisecond)
	if err != nil {
		t.Fatalf("poll() error = %v", err)
	}
	if got != want {
		t.Errorf("poll() returned the wrong token")
	}
	if calls != 3 {
		t.Errorf("polled %d times, want 3", calls)
	}
}

// Anything that is neither pending nor success is terminal: keeping the poll
// alive would hang until the timeout with no explanation.
func TestPollFailsOnUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"status":"denied"}`)
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	if _, err := c.pollEvery(context.Background(), srv.URL, time.Millisecond); err == nil {
		t.Error("poll() error = nil for status \"denied\", want an error")
	}
}

func TestPollHonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"status":"pending"}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &Client{http: srv.Client()}
	if _, err := c.pollEvery(ctx, srv.URL, time.Millisecond); err == nil {
		t.Error("poll() error = nil for a cancelled context, want an error")
	}
}

// signIn must not prompt and must not poll when there is no terminal to answer:
// the confirmation prompt drives a Bubble Tea program that blocks forever
// without a TTY, and the poll waits pollTimeout (11 minutes) for a human to
// finish the browser flow. Under no TTY (CI, cron, an ssh without one) there is
// no such human, so signIn must fail fast with a clear error instead of hanging
// ~11 minutes and then reporting a misleading "run the command again" timeout.
func TestSignInFailsFastWithoutTTY(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The guard runs before createSession, so an unattended run must not
		// reach the auth service at all: no throwaway server-side session, and
		// no 30s HTTP timeout on a firewalled host before the error explains
		// itself. A pending status would, under the bug, additionally keep poll
		// alive until the 11-minute ceiling.
		t.Errorf("signIn requested %s with no TTY; it must fail before any auth call", r.URL.Path)
		fmt.Fprint(w, `{"status":"pending"}`)
	}))
	defer srv.Close()

	promptCalls := 0
	c := &Client{
		authURL:        srv.URL,
		http:           srv.Client(),
		humanPresentFn: func() bool { return false },
		confirmBrowserFn: func(string) error {
			promptCalls++
			return nil
		},
	}

	start := time.Now()
	_, err := c.signIn(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("signIn() error = nil with no TTY, want the fast-fail error (the bug polled for ~11 minutes)")
	}
	if !strings.Contains(err.Error(), "requires a terminal") {
		t.Errorf("signIn() error = %q, want one that names the missing terminal/interactive shell", err)
	}
	if promptCalls != 0 {
		t.Errorf("confirmBrowser called %d time(s) with no TTY, want 0 (the prompt must be skipped)", promptCalls)
	}
	// Fast-fail should be near-instant; anything close to pollTimeout means the
	// bug is back. Keep a generous headroom over the no-network path.
	if elapsed > 2*time.Second {
		t.Errorf("signIn took %v with no TTY, want a fast failure (the bug hung ~11 minutes)", elapsed)
	}
}

// With a terminal present, signIn runs the browser confirmation (here stubbed
// to avoid the Bubble Tea program), then polls the callback URL until the JWT
// arrives. This is the interactive happy path; it guards the wiring between
// createSession, the prompt, poll, and the returned token against regressions
// from the no-TTY fast-fail guard above.
func TestSignInPollsAndCompletesWithTTY(t *testing.T) {
	want := jwtWithExp(t, time.Now().Add(time.Hour), "dev@argus.gg")

	var srv *httptest.Server
	statusCalls := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == sessionPath {
			fmt.Fprintf(w, `{"clientUrl":%q,"callbackUrl":%q}`,
				srv.URL+"/authorize", srv.URL+"/status")
			return
		}
		statusCalls++
		fmt.Fprintf(w, `{"status":"success","jwt":%q}`, want)
	}))
	defer srv.Close()

	promptCalls := 0
	opened := ""
	c := &Client{
		authURL:        srv.URL,
		http:           srv.Client(),
		humanPresentFn: func() bool { return true },
		canPromptFn:    func() bool { return true },
		confirmBrowserFn: func(clientURL string) error {
			promptCalls++
			opened = clientURL
			return nil
		},
	}

	done := make(chan struct{})
	var got string
	var err error
	go func() {
		defer close(done)
		got, err = c.signIn(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("signIn blocked with a TTY; the happy path should complete once poll returns success")
	}
	if err != nil {
		t.Fatalf("signIn() error = %v", err)
	}
	if got != want {
		t.Errorf("signIn() token = %q, want %q", got, want)
	}
	if promptCalls != 1 {
		t.Errorf("confirmBrowser called %d time(s) with a TTY, want 1", promptCalls)
	}
	if opened != srv.URL+"/authorize" {
		t.Errorf("confirmBrowser received %q, want the session client URL", opened)
	}
	if statusCalls == 0 {
		t.Error("status endpoint never polled with a TTY, want at least one poll")
	}
}

// An interactive sign-in where the user declines to open the browser still
// polls: they may complete the flow in a browser they opened themselves. This
// preserves the original behavior, which returned from the prompt without
// aborting and proceeded to poll.
func TestSignInProceedsToPollWhenUserDeclinesBrowserWithTTY(t *testing.T) {
	want := jwtWithExp(t, time.Now().Add(time.Hour), "dev@argus.gg")

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == sessionPath {
			fmt.Fprintf(w, `{"clientUrl":%q,"callbackUrl":%q}`,
				srv.URL+"/authorize", srv.URL+"/status")
			return
		}
		fmt.Fprintf(w, `{"status":"success","jwt":%q}`, want)
	}))
	defer srv.Close()

	opened := ""
	c := &Client{
		authURL:        srv.URL,
		http:           srv.Client(),
		humanPresentFn: func() bool { return true },
		canPromptFn:    func() bool { return true },
		confirmBrowserFn: func(clientURL string) error {
			// "n" (don't open): record the URL but report no error so signIn polls.
			opened = clientURL
			return nil
		},
	}

	got, err := c.signIn(context.Background())
	if err != nil {
		t.Fatalf("signIn() error = %v", err)
	}
	if got != want {
		t.Errorf("signIn() token = %q, want %q", got, want)
	}
	if opened == "" {
		t.Error("confirmBrowser never called, want one call")
	}
}

// An interactive sign-in where the user cancels the prompt (esc / ctrl+c) must
// abort immediately rather than polling on for pollTimeout.
func TestSignInAbortsWhenUserCancelsPromptWithTTY(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == sessionPath {
			fmt.Fprintf(w, `{"clientUrl":%q,"callbackUrl":%q}`,
				srv.URL+"/authorize", srv.URL+"/status")
			return
		}
		t.Errorf("signIn polled after the user cancelled the prompt")
		fmt.Fprint(w, `{"status":"pending"}`)
	}))
	defer srv.Close()

	cancelErr := errors.New("input canceled")
	c := &Client{
		authURL:          srv.URL,
		http:             srv.Client(),
		humanPresentFn:   func() bool { return true },
		canPromptFn:      func() bool { return true },
		confirmBrowserFn: func(string) error { return cancelErr },
	}

	_, err := c.signIn(context.Background())
	if !errors.Is(err, cancelErr) {
		t.Errorf("signIn() error = %v, want the prompt's cancel error", err)
	}
}

// humanPresent's production check must consult every standard stream, not just
// the one the prompt happens to use. Each is the sole survivor of some ordinary
// redirection — `world logs </dev/null 2>/dev/null` leaves only stdout, which
// is exactly where printer writes the sign-in link — so missing one turns a
// working sign-in into the 11-minute poll this guard exists to prevent.
//
// Needs a real terminal to assert the positive cases; skips where there is no
// controlling terminal (CI), which still leaves the all-redirected case covered.
func TestHumanPresentConsultsEveryStandardStream(t *testing.T) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no controlling terminal to test against: %v", err)
	}
	t.Cleanup(func() { _ = tty.Close() })

	pipe, pw, err := os.Pipe() // stands in for any redirected stream
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { _ = pipe.Close(); _ = pw.Close() })

	for _, tt := range []struct {
		name                  string
		stdin, stdout, stderr *os.File
		want                  bool
	}{
		{"all redirected", pipe, pw, pw, false},
		{"only stdin attached", tty, pw, pw, true},
		{"only stdout attached", pipe, tty, pw, true},
		{"only stderr attached", pipe, pw, tty, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Swapping the process streams is the point: the bug this guards
			// against is humanPresent leaving one of them out of the check, so
			// the test has to exercise that exact line rather than a stand-in.
			// Restored immediately, and these tests do not run in parallel.
			inSave, outSave, errSave := os.Stdin, os.Stdout, os.Stderr
			//nolint:reassign // deliberate, scoped, and restored by the defer below
			os.Stdin, os.Stdout, os.Stderr = tt.stdin, tt.stdout, tt.stderr
			//nolint:reassign // restores what the line above swapped
			defer func() { os.Stdin, os.Stdout, os.Stderr = inSave, outSave, errSave }()

			if got := (&Client{}).humanPresent(); got != tt.want {
				t.Errorf("humanPresent() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Redirecting output does not remove the human. `world logs 2>&1 | tee run.log`
// leaves stderr as a pipe while the developer is still watching the terminal:
// the sign-in link is printed, they can open it, and the poll completes. Only
// the Bubble Tea prompt is unusable there, because program.NewTeaProgram starts
// it with no input reader when stderr is not a terminal. So the prompt is
// skipped and the sign-in still succeeds — gating the poll on stderr too would
// break this working flow in the name of fixing CI.
func TestSignInPollsWithoutPromptWhenOutputIsRedirected(t *testing.T) {
	want := jwtWithExp(t, time.Now().Add(time.Hour), "dev@argus.gg")

	var srv *httptest.Server
	statusCalls := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == sessionPath {
			fmt.Fprintf(w, `{"clientUrl":%q,"callbackUrl":%q}`,
				srv.URL+"/authorize", srv.URL+"/status")
			return
		}
		statusCalls++
		fmt.Fprintf(w, `{"status":"success","jwt":%q}`, want)
	}))
	defer srv.Close()

	promptCalls := 0
	c := &Client{
		authURL:        srv.URL,
		http:           srv.Client(),
		humanPresentFn: func() bool { return true },
		canPromptFn:    func() bool { return false },
		confirmBrowserFn: func(string) error {
			promptCalls++
			return nil
		},
	}

	got, err := c.signIn(context.Background())
	if err != nil {
		t.Fatalf("signIn() error = %v, want success: a redirected stderr does not mean nobody can authorize", err)
	}
	if got != want {
		t.Errorf("signIn() token = %q, want %q", got, want)
	}
	if promptCalls != 0 {
		t.Errorf("confirmBrowser called %d time(s) with stderr redirected, want 0 (the prompt has no input to read)",
			promptCalls)
	}
	if statusCalls == 0 {
		t.Error("status endpoint never polled with stderr redirected, want the poll to run")
	}
}

// Token is the user-facing entry point the operator interceptor calls on every
// RPC, so this reproduces the bug at the level it is observed: a
// non-interactive shell with no usable cached credential must fail fast, not
// poll for ~11 minutes. Covers both ways cached() can route to signIn — a
// missing credential file and an expired (or near-expiry) cached JWT, the two
// branches the bug report's reproduction exercised.
func TestTokenFailsFastWithoutTTY(t *testing.T) {
	for _, tt := range []struct {
		name string
		seed bool
		exp  time.Time
	}{
		{"missing credential", false, time.Time{}},
		{"expired credential", true, time.Now().Add(-time.Hour)},
		{"near-expiry credential", true, time.Now().Add(renewBefore / 2)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			if tt.seed {
				if err := store.PutCredential(store.Credential{
					JWT: jwtWithExp(t, tt.exp, "ci@argus.gg"),
				}); err != nil {
					t.Fatalf("PutCredential: %v", err)
				}
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// The guard runs before createSession, so no request should
				// reach the auth service at all. A pending status would, under
				// the bug, additionally keep poll alive until the 11-minute
				// ceiling.
				t.Errorf("Token() requested %s with no TTY; it must fail before any auth call", r.URL.Path)
				fmt.Fprint(w, `{"status":"pending"}`)
			}))
			defer srv.Close()

			c := &Client{
				authURL:        srv.URL,
				http:           srv.Client(),
				humanPresentFn: func() bool { return false },
			}

			start := time.Now()
			_, err := c.Token(context.Background())
			elapsed := time.Since(start)

			if err == nil {
				t.Fatal("Token() error = nil with no TTY, want the fast-fail error (the bug polled for ~11 minutes)")
			}
			if !strings.Contains(err.Error(), "requires a terminal") {
				t.Errorf("Token() error = %q, want one that names the missing terminal/interactive shell", err)
			}
			// Fast-fail should be near-instant; anything close to pollTimeout
			// means the bug is back. Keep generous headroom over the no-network
			// path (session POST + a single status GET at most).
			if elapsed > 2*time.Second {
				t.Errorf("Token() took %v with no TTY, want a fast failure (the bug hung ~11 minutes)", elapsed)
			}
		})
	}
}
