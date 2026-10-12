package worldscaffold

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/otiai10/copy"
	"github.com/rotisserie/eris"
	"golang.org/x/mod/modfile"

	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
	"github.com/argus-labs/world-engine/cli/pkg/version"
)

const worldEngineExamplePrefix = "github.com/argus-labs/world-engine/pkg/template/"

//go:embed templates/go_mod_template
var goModTemplate string

// Scaffold creates a new project at targetDir from the given template: it clones
// and instantiates the template (see InstantiateTemplate) at the World Engine
// release this binary was built from, then tidies dependencies. It is the single entry point for
// callers that don't manage the individual steps (e.g. the editor). On any
// failure it leaves nothing behind — targetDir is removed so the name stays
// retryable.
func Scaffold(ctx context.Context, tmpl GameTemplate, targetDir string) error {
	if err := InstantiateTemplate(ctx, tmpl.URL, version.WorldEngine(), targetDir, tmpl.Subdir); err != nil {
		return err
	}
	if err := Tidy(ctx, targetDir); err != nil {
		// InstantiateTemplate created targetDir; drop it so a retry starts clean.
		removeAll(ctx, targetDir)
		return err
	}
	return nil
}

// InstantiateTemplate clones a git template and turns it into a ready-to-build
// project named after targetDir's base: it writes a go.mod, rewrites example
// imports to the module path, and sets world.toml's project field. On any failure
// the partially-created targetDir is removed so the name stays retryable. All
// paths are explicit — no [os.Chdir].
func InstantiateTemplate(ctx context.Context, url, version, targetDir, subdir string) error {
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return eris.Errorf("Project named '%s' already exists in this directory, "+
				"please change the directory or use another name", targetDir)
		}
		return eris.Wrap(err, "failed to create project directory")
	}

	if err := instantiate(ctx, url, version, targetDir, subdir); err != nil {
		removeAll(ctx, targetDir)
		return err
	}
	return nil
}

// removeAll removes path best-effort, logging (rather than silently discarding)
// any failure.
func removeAll(ctx context.Context, path string) {
	if err := os.RemoveAll(path); err != nil {
		slog.Default().WarnContext(ctx, "worldscaffold: failed to clean up directory after failure",
			"path", path, "err", err)
	}
}

// instantiate does the clone + project setup into targetDir. InstantiateTemplate
// owns the existence check and the failure cleanup.
func instantiate(ctx context.Context, url, version, targetDir, subdir string) error {
	// Clone to temp directory
	tempDir, err := os.MkdirTemp("", "world-cli-clone-*")
	if err != nil {
		return eris.Wrap(err, "failed to create temp directory")
	}
	defer removeAll(ctx, tempDir)

	// Clone with tag/branch fallback (git --branch resolves both tags and branches)
	cloneOpts := &gogit.CloneOptions{
		URL:           url,
		Depth:         1,
		SingleBranch:  true,
		ReferenceName: plumbing.NewTagReferenceName(version),
	}
	_, err = gogit.PlainCloneContext(ctx, tempDir, false, cloneOpts)
	if err != nil {
		// Tag (or other ref) not found — retry as branch. go-git reports a missing
		// tag as NoMatchingRefSpecError, not plumbing.ErrReferenceNotFound, so match both
		// to cover dev builds where version.WorldEngine() is a branch name (e.g. "main").
		if eris.Is(err, plumbing.ErrReferenceNotFound) || errors.As(err, &gogit.NoMatchingRefSpecError{}) {
			cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(version)
			_, err = gogit.PlainCloneContext(ctx, tempDir, false, cloneOpts)
		}
	}
	if err != nil {
		return eris.Wrapf(err, "failed to clone repository from %s", url)
	}

	// Determine source path
	srcPath := tempDir
	if subdir != "" {
		srcPath = filepath.Join(tempDir, subdir)
		if _, err := os.Stat(srcPath); errors.Is(err, fs.ErrNotExist) {
			return eris.Errorf("subdirectory '%s' not found in repository", subdir)
		}
	}

	// Copy to target (using otiai10/copy with git artifact filtering)
	if err := copyTemplateDirectory(srcPath, targetDir); err != nil {
		return eris.Wrap(err, "failed to copy template")
	}

	// Setup Go module (all paths explicit, no chdir). The project name (world.toml
	// + folder) is the target's base; the module path used for import rewriting is
	// whatever go.mod actually declares, so the two stay consistent even if a
	// template ever ships its own go.mod.
	projectName := filepath.Base(targetDir)
	if err := createGoModIn(targetDir, projectName, version); err != nil {
		return eris.Wrap(err, "failed to create go.mod")
	}
	if err := rewriteImportsIn(targetDir, moduleNameFromGoMod(targetDir, projectName)); err != nil {
		return eris.Wrap(err, "failed to rewrite imports")
	}

	// Set world.toml's project field to the folder name.
	if err := worldtoml.WriteProject(filepath.Join(targetDir, worldtoml.FileName), projectName); err != nil {
		return eris.Wrap(err, "failed to set project in world.toml")
	}

	return nil
}

// copyTemplateDirectory copies src to dst, skipping .git directories and .gitkeep files.
func copyTemplateDirectory(src, dst string) error {
	return copy.Copy(src, dst, copy.Options{
		Skip: skipGitArtifacts,
	})
}

// skipGitArtifacts returns true for .git directories and .gitkeep placeholder files.
func skipGitArtifacts(info os.FileInfo, src, _ string) (bool, error) {
	name := filepath.Base(src)

	// Skip .git directory
	if info.IsDir() && name == ".git" {
		return true, nil
	}

	// Skip .gitkeep placeholder files
	if !info.IsDir() && name == ".gitkeep" {
		return true, nil
	}

	return false, nil
}

// createGoModIn creates a go.mod in the specified directory if one doesn't exist, requiring World Engine
// at the version the template was cloned at. Without the requirement, go mod tidy resolves World Engine
// to its latest release, and after a breaking release that no longer compiles the cloned template.
//
// The go.mod also declares the World CLI as a Go tool. The CLI ships in the world-engine module, so
// `go tool world` always matches the World Engine release the project builds against.
func createGoModIn(dir, moduleName, worldEngineVersion string) error {
	goModPath := filepath.Join(dir, "go.mod")

	if _, err := os.Stat(goModPath); err == nil {
		return nil // already exists
	} else if !os.IsNotExist(err) {
		return eris.Wrap(err, "failed to check go.mod")
	}

	content := strings.NewReplacer(
		"{{MODULE_NAME}}", moduleName, "{{WORLD_ENGINE_VERSION}}", worldEngineVersion,
	).Replace(goModTemplate)
	// Use 0600 to satisfy gosec G306; go.mod is not sensitive, but stricter perms are acceptable.
	if err := os.WriteFile(goModPath, []byte(content), 0o600); err != nil {
		return eris.Wrap(err, "failed to write go.mod")
	}
	return nil
}

// moduleNameFromGoMod returns the module path declared in dir/go.mod, or fallback
// if it can't be read or parsed. This keeps import rewriting consistent with a
// go.mod the template may have shipped (createGoModIn leaves an existing one
// untouched); for the normal case (no go.mod) it equals fallback.
func moduleNameFromGoMod(dir, fallback string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return fallback
	}
	if mod := modfile.ModulePath(data); mod != "" {
		return mod
	}
	return fallback
}

// rewriteImportsIn rewrites example imports in all .go files in the directory.
// It rewrites imports that reference github.com/argus-labs/world-engine/pkg/template/<template>/<rest>
// to <moduleName>/<rest>. Engine imports (e.g., pkg/cardinal, pkg/ecs) are preserved.
func rewriteImportsIn(dir, moduleName string) error {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(worldEngineExamplePrefix) + `([^/]+)/([^"]+)"`)

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return eris.Wrapf(err, "failed to read %s", path)
		}

		original := string(data)
		replaced := re.ReplaceAllStringFunc(original, func(m string) string {
			submatches := re.FindStringSubmatch(m)
			if len(submatches) != 3 {
				return m
			}
			// submatches[1] is <template>, submatches[2] is <rest>
			return fmt.Sprintf("\"%s/%s\"", moduleName, submatches[2])
		})

		if replaced == original {
			return nil
		}

		if err := os.WriteFile(path, []byte(replaced), info.Mode()); err != nil {
			return eris.Wrapf(err, "failed to write %s", path)
		}
		return nil
	})
}

// Tidy runs go mod tidy in the specified project directory.
// Uses os/exec with explicit cmd.Dir — does not depend on or modify CWD.
func Tidy(ctx context.Context, projectDir string) error {
	select {
	case <-ctx.Done():
		return eris.Wrap(ctx.Err(), "context cancelled")
	default:
		// no-op
	}

	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return eris.Wrap(err, "failed to resolve project path")
	}

	out, err := goCommand(ctx, absDir, "mod", "tidy").CombinedOutput()
	if err == nil {
		return nil
	}

	// Check for world-engine module resolution issues
	outStr := string(out)
	if strings.Contains(outStr, "module for package github.com/argus-labs/world-engine/") ||
		strings.Contains(outStr, "github.com/argus-labs/world-engine/pkg/cardinal") {
		worldEngineURL := "github.com/argus-labs/world-engine/pkg/cardinal@" + version.WorldEngine()
		// #nosec G204 -- version.WorldEngine is this binary's own module version
		if _, getErr := goCommand(ctx, absDir, "get", worldEngineURL).CombinedOutput(); getErr != nil {
			return prettifyGoToolError(outStr)
		}

		// Retry tidy
		if retryOut, retryErr := goCommand(ctx, absDir, "mod", "tidy").CombinedOutput(); retryErr != nil {
			return prettifyGoToolError(string(retryOut))
		}
		return nil
	}

	return prettifyGoToolError(outStr)
}

// goCommand builds a `go` invocation rooted at dir. dir is set explicitly, so
// this neither depends on nor modifies the process CWD.
func goCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	hideConsoleWindow(cmd)
	return cmd
}

func prettifyGoToolError(output string) error {
	// Private module via HTTPS without creds or through proxy/sumdb
	if strings.Contains(output, "terminal prompts disabled") ||
		strings.Contains(output, "sum.golang.org/lookup") ||
		(strings.Contains(output, "invalid version") && strings.Contains(output, "github.com/argus-labs/")) {
		return eris.New("go mod tidy failed: unable to fetch private module")
	}

	// Default: keep it concise (first line only)
	if idx := strings.IndexByte(output, '\n'); idx > 0 {
		output = output[:idx]
	}
	return eris.Errorf("go mod tidy failed: %s", output)
}
