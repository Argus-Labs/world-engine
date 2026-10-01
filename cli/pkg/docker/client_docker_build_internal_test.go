package docker

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// TestBuildContextIgnorePatterns_Ordering pins the precedence contract:
// user .dockerignore patterns come first and defaultBuildIgnores come last, so
// patternmatcher's last-wins semantics make the defaults non-overridable. This
// guards against reintroducing the bug where defaults were concatenated
// *before* user patterns, letting a "!"-negation re-include default-ignored
// paths such as .git, node_modules, dist, or *.exe.
func TestBuildContextIgnorePatterns_Ordering(t *testing.T) {
	t.Parallel()

	user := []string{"foo", "!bar"}
	got := buildContextIgnorePatterns(user)

	want := slices.Concat(user, defaultBuildIgnores)
	if !slices.Equal(got, want) {
		t.Fatalf("buildContextIgnorePatterns(%v)\n  got  = %v\n  want = %v", user, got, want)
	}

	// Every default pattern must come after every user pattern so a user
	// "!"-negation can never win over a default exclusion.
	lastUser := user[len(user)-1]
	lastDefault := defaultBuildIgnores[len(defaultBuildIgnores)-1]
	userIdx := slices.Index(got, lastUser)
	defaultIdx := slices.Index(got, lastDefault)
	if userIdx < 0 || defaultIdx < 0 {
		t.Fatalf("could not locate anchors user=%q default=%q in %v", lastUser, lastDefault, got)
	}
	if userIdx > defaultIdx {
		t.Fatalf("expected every user pattern to precede defaults; got user %q at %d, default %q at %d (full: %v)",
			lastUser, userIdx, lastDefault, defaultIdx, got)
	}
}

// TestBuildContextIgnorePatterns_DefaultsNonOverridable verifies end-to-end that
// the build-context tarball never contains default-ignored paths, even when a
// consumer .dockerignore uses "!"-negations that target them. It exercises the
// real readDockerignore, buildContextIgnorePatterns, and
// tarWithEmbeddedDockerfile against a temp project tree containing one file
// under each default-ignored path plus ordinary source files.
func TestBuildContextIgnorePatterns_DefaultsNonOverridable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		dockerignore string
		wantPresent  []string
		wantAbsent   []string
		wantNoPrefix []string
		wantNoSuffix []string
	}{
		{
			// The reported bug: a "!"-negation targeting a default-ignored
			// path must not re-include it.
			name:         "negate_git",
			dockerignore: "!.git\n",
			wantPresent:  []string{"main.go", "__cardinal.Dockerfile", ".dockerignore"},
			wantNoPrefix: []string{".git/"},
		},
		{
			// Anti-overfix guard: a "!"-negation targeting a NON-default
			// path must still re-include it. The fix must only block
			// negations against defaults, not all negations.
			name:         "legitimate_negation_nondefault_path",
			dockerignore: "README.md\n!README.md\n",
			wantPresent:  []string{"README.md", "main.go", "__cardinal.Dockerfile", ".dockerignore"},
			wantNoPrefix: []string{".git/", "node_modules/", "dist/"},
			wantNoSuffix: []string{".exe"},
		},
		{
			// Regression guard: user *additional* ignores still work
			// alongside the non-overridable defaults.
			name:         "user_ignore_plus_default_negation",
			dockerignore: "README.md\n!.git\n",
			wantPresent:  []string{"main.go", "__cardinal.Dockerfile", ".dockerignore"},
			wantAbsent:   []string{"README.md"},
			wantNoPrefix: []string{".git/", "node_modules/", "dist/"},
			wantNoSuffix: []string{".exe"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeBuildContextFixture(t, root)

			if tt.dockerignore != "" {
				if err := os.WriteFile(filepath.Join(root, ".dockerignore"),
					[]byte(tt.dockerignore), 0o644); err != nil {
					t.Fatalf("write .dockerignore: %v", err)
				}
			}

			c := &Client{cfg: &service.Config{RootDir: root}, logger: slog.New(slog.DiscardHandler)}
			userIgnores, err := c.readDockerignore(context.Background(), root)
			if err != nil {
				t.Fatalf("readDockerignore: %v", err)
			}
			patterns := buildContextIgnorePatterns(userIgnores)

			rc, err := tarWithEmbeddedDockerfile(root, patterns, cardinalDockerfile, cardinalDockerfileName)
			if err != nil {
				t.Fatalf("tarWithEmbeddedDockerfile: %v", err)
			}
			defer rc.Close()

			entries := readBuildContextTarEntries(t, rc)

			for _, w := range tt.wantPresent {
				if !slices.Contains(entries, w) {
					t.Errorf("expected entry %q present; entries=%v", w, entries)
				}
			}
			for _, w := range tt.wantAbsent {
				if slices.Contains(entries, w) {
					t.Errorf("expected entry %q absent; entries=%v", w, entries)
				}
			}
			for _, p := range tt.wantNoPrefix {
				for _, e := range entries {
					if strings.HasPrefix(e, p) {
						t.Errorf("expected no entry with prefix %q; got %q; entries=%v", p, e, entries)
					}
				}
			}
			for _, s := range tt.wantNoSuffix {
				for _, e := range entries {
					if strings.HasSuffix(e, s) {
						t.Errorf("expected no entry with suffix %q; got %q; entries=%v", s, e, entries)
					}
				}
			}
		})
	}
}

// writeBuildContextFixture creates a deterministic project tree in root
// containing one file under each defaultBuildIgnores path plus ordinary
// source files, so the build-context tar can be checked for default-ignored
// paths regardless of .dockerignore contents.
func writeBuildContextFixture(t *testing.T, root string) {
	t.Helper()

	files := map[string]string{
		".git/config":         "",
		"node_modules/lib.js": "",
		"dist/bundle.js":      "",
		"bin/tool.exe":        "",
		"main.go":             "package main\n",
		"README.md":           "# project\n",
	}
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", full, err)
		}
	}
}

// readBuildContextTarEntries reads every entry name from a tar stream (the
// output of tarWithEmbeddedDockerfile), stripping any leading "./" for easy
// comparison.
func readBuildContextTarEntries(t *testing.T, r io.Reader) []string {
	t.Helper()

	tr := tar.NewReader(r)
	var names []string
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("reading tar entry: %v", err)
		}
		names = append(names, strings.TrimPrefix(hdr.Name, "./"))
	}
	return names
}
