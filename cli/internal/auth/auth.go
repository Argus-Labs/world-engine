// Package auth obtains an Argus JWT for calls to remote services, caching it
// between runs so a developer signs in roughly once a month.
package auth

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/pkg/browser"
	"github.com/rotisserie/eris"
	"golang.org/x/term"

	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/store"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/textinput"
)

const (
	// DefaultAuthURL is the Argus auth service. Override with WORLD_AUTH_URL.
	DefaultAuthURL = "https://api.argus.dev"

	sessionPath  = "/auth/service-auth-session"
	pollInterval = 3 * time.Second
	pollTimeout  = 11 * time.Minute

	// renewBefore treats a token as expired early, so a long-running command does
	// not start with one about to lapse.
	renewBefore = 5 * time.Minute
)

// Client runs the sign-in flow. Persistence belongs to store, so other
// commands can read the token without importing auth. The zero value is not
// usable; use New.
type Client struct {
	authURL string
	http    *http.Client

	// humanPresentFn reports whether anyone can finish the browser sign-in.
	// New leaves it nil and humanPresent falls back to the production check.
	// Tests set it to drive both branches of signIn without depending on the
	// test runner's own TTY.
	humanPresentFn func() bool

	// canPromptFn reports whether the confirmation prompt can run. New leaves
	// it nil and canPrompt falls back to the production check. Tests set it
	// alongside humanPresentFn to cover a piped run, where a human is present
	// but the prompt cannot be answered.
	canPromptFn func() bool

	// confirmBrowserFn asks whether to open the sign-in link and opens it on
	// confirmation; a non-nil error aborts sign-in (esc / ctrl+c). New leaves
	// it nil and confirmBrowser falls back to textinput.Confirm +
	// browser.OpenURL. Tests stub it so signIn never starts the Bubble Tea
	// program, which has no input in a test harness.
	confirmBrowserFn func(clientURL string) error
}

func New() *Client {
	authURL := os.Getenv("WORLD_AUTH_URL")
	if authURL == "" {
		authURL = DefaultAuthURL
	}
	return &Client{
		authURL: strings.TrimSuffix(authURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Token returns a cached JWT, running the browser sign-in when there is none or
// it has expired. Blocks on the user completing sign-in.
func (c *Client) Token(ctx context.Context) (string, error) {
	if tok, err := c.cached(); err == nil && tok != "" {
		return tok, nil
	}

	tok, err := c.signIn(ctx)
	if err != nil {
		return "", err
	}
	if err := store.PutCredential(store.Credential{JWT: tok}); err != nil {
		// A working token is more useful than a failed write; say so and continue.
		printer.Infof("could not cache credentials: %v\n", err)
	}
	return tok, nil
}

type sessionResponse struct {
	ClientURL   string `json:"clientUrl"`
	CallbackURL string `json:"callbackUrl"`
}

type statusResponse struct {
	Status string `json:"status"`
	JWT    string `json:"jwt"`
}

func (c *Client) signIn(ctx context.Context) (string, error) {
	// The poll below waits up to pollTimeout (11 minutes) for someone to finish
	// the browser flow, so a run with nobody attached can only ever time out and
	// then report a misleading "run the command again". Fail fast instead, and
	// do it before createSession: an unattended run should neither open a
	// throwaway server-side session nor spend the 30s HTTP timeout on a
	// firewalled host before it learns why it cannot sign in.
	if !c.humanPresent() {
		return "", eris.New("sign-in requires a terminal; run a world command " +
			"interactively to sign in once — the cached credential is then " +
			"reused by unattended runs sharing the same home directory")
	}

	session, err := c.createSession(ctx)
	if err != nil {
		return "", err
	}

	printer.Headerln("Sign in to continue")
	printer.Infof("  %s\n\n", session.ClientURL)

	// Confirm runs a Bubble Tea program that program.NewTeaProgram starts with
	// no input reader when stderr is not a terminal, leaving it with no way to
	// receive the keypress that dismisses it. Skip the prompt in that case: the
	// link is printed above either way and the poll still completes the sign-in.
	if c.canPrompt() {
		if err := c.confirmBrowser(session.ClientURL); err != nil {
			// esc / ctrl+c: abort instead of polling on for another 11 minutes.
			return "", err
		}
	}

	printer.Infoln("  Waiting for authorization...")
	tok, err := c.poll(ctx, session.CallbackURL)
	if err != nil {
		return "", err
	}

	if claims, err := ParseClaims(tok); err == nil && claims.Email != "" {
		printer.Successf("Signed in as %s\n", claims.Email)
	} else {
		printer.Successln("Signed in")
	}
	return tok, nil
}

// humanPresent reports whether anyone can complete the browser sign-in. A
// terminal on any of the three standard streams is enough: it means a
// developer is at a keyboard, and only a run with none of them — CI, cron, ssh
// without a TTY — has nobody to authorize and must be kept away from the poll.
//
// All three are checked because the sign-in flow spreads across all three, and
// redirection rarely takes them together: signIn prints the link through
// printer, which writes to [os.Stdout]; the prompt reads [os.Stdin]; and
// program.NewTeaProgram keys off [os.Stderr]. `world logs 2>&1 | tee run.log`
// leaves only stdin attached, `world logs </dev/null 2>/dev/null` only stdout.
//
// Redirecting all three from a terminal (`world logs >out.log 2>err.log`) still
// polls. That is deliberate: someone is at the keyboard, the link is sitting in
// the file they asked for, and failing their command outright is worse than a
// wait they can read the link out of — or Ctrl+C.
func (c *Client) humanPresent() bool {
	if c.humanPresentFn != nil {
		return c.humanPresentFn()
	}
	return isTerminal(os.Stdin) || isTerminal(os.Stdout) || isTerminal(os.Stderr)
}

// canPrompt reports whether the confirmation prompt can run. It is deliberately
// stricter than humanPresent and matches program.NewTeaProgram, which drops the
// input reader when stderr is not a terminal: a prompt started there would hang
// with no way to answer it. canPrompt true implies humanPresent true.
func (c *Client) canPrompt() bool {
	if c.canPromptFn != nil {
		return c.canPromptFn()
	}
	return isTerminal(os.Stderr)
}

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// confirmBrowser asks the user whether to open the sign-in link and opens it on
// confirmation. A non-nil error aborts sign-in (esc / ctrl+c).
func (c *Client) confirmBrowser(clientURL string) error {
	if c.confirmBrowserFn != nil {
		return c.confirmBrowserFn(clientURL)
	}
	open, cerr := textinput.Confirm("Open this link in your browser?", "y")
	if cerr != nil {
		return cerr
	}
	if open {
		if err := browser.OpenURL(clientURL); err != nil {
			printer.Infoln("  Could not open a browser; use the link above.")
		}
	}
	return nil
}

func (c *Client) createSession(ctx context.Context) (*sessionResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.authURL+sessionPath, strings.NewReader("{}"))
	if err != nil {
		return nil, eris.Wrap(err, "building auth session request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, eris.Wrapf(err, "reaching the auth service at %s", c.authURL)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, eris.Errorf("auth service returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var session sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, eris.Wrap(err, "decoding auth session response")
	}
	if session.ClientURL == "" || session.CallbackURL == "" {
		return nil, eris.New("auth service returned an incomplete session")
	}
	return &session, nil
}

// poll asks the callback URL for the token until the user finishes in the
// browser. The session expires server-side, so this cannot wait forever.
func (c *Client) poll(ctx context.Context, callbackURL string) (string, error) {
	return c.pollEvery(ctx, callbackURL, pollInterval)
}

func (c *Client) pollEvery(ctx context.Context, callbackURL string, interval time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		status, err := c.status(ctx, callbackURL)
		switch {
		case err != nil:
			return "", err
		case status.Status == "success" && status.JWT != "":
			return status.JWT, nil
		case status.Status != "pending" && status.Status != "":
			return "", eris.Errorf("authorization failed: %s", status.Status)
		}

		select {
		case <-ctx.Done():
			if eris.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", eris.New("timed out waiting for authorization; run the command again")
			}
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Client) status(ctx context.Context, callbackURL string) (*statusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, callbackURL, nil)
	if err != nil {
		return nil, eris.Wrap(err, "building auth status request")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, eris.Wrap(err, "polling authorization status")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, eris.Errorf("auth status returned %s", resp.Status)
	}
	var status statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, eris.Wrap(err, "decoding auth status response")
	}
	return &status, nil
}

// cached returns the stored token, or an error if it is missing or expired.
func (c *Client) cached() (string, error) {
	cred, err := store.GetCredential()
	if err != nil {
		return "", err
	}
	claims, err := ParseClaims(cred.JWT)
	if err != nil {
		return "", err
	}
	if time.Now().Add(renewBefore).After(claims.ExpiresAt()) {
		return "", eris.New("cached token has expired")
	}
	return cred.JWT, nil
}

// Claims is the subset of the Argus JWT this CLI reads.
type Claims struct {
	Email     string `json:"email"`
	PersonaID string `json:"personaID"`
	Exp       int64  `json:"exp"`
}

func (c Claims) ExpiresAt() time.Time { return time.Unix(c.Exp, 0) }

// ParseClaims reads the payload without verifying the signature: the operator
// verifies it, and the CLI only needs the expiry to decide whether to re-use it.
func ParseClaims(jwt string) (Claims, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return Claims{}, eris.New("malformed JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, eris.Wrap(err, "decoding JWT payload")
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, eris.Wrap(err, "decoding JWT claims")
	}
	if claims.Exp == 0 {
		return Claims{}, eris.New("JWT has no expiry")
	}
	return claims, nil
}
