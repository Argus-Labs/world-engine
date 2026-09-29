package sdkgen

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Default plugin versions — mirror proto/buf.gen.yaml. Overridable via the CLI
// flags (--go-version / --csharp-version).
const (
	DefaultGoVersion     = "v1.36.11" // protocolbuffers/go
	DefaultCSharpVersion = "33.2"     // protocolbuffers/csharp == protoc release
)

//go:embed buf/Dockerfile
var dockerfile []byte

//go:embed buf/buf.yaml
var bufYAML []byte

//go:embed buf/buf.gen.cs.yaml
var bufGenCSYAML []byte

//go:embed buf/buf.gen.go.yaml
var bufGenGoYAML []byte

// GenOptions pins the plugin versions baked into the buf image.
type GenOptions struct {
	GoVersion     string // protoc-gen-go, e.g. v1.36.11
	CSharpVersion string // protoc release for the C# backend, e.g. 33.2
}

func (o GenOptions) withDefaults() GenOptions {
	if o.GoVersion == "" {
		o.GoVersion = DefaultGoVersion
	}
	if o.CSharpVersion == "" {
		o.CSharpVersion = DefaultCSharpVersion
	}
	// The embedded Dockerfile downloads protoc from a URL that already prepends `v`
	// (.../v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-...), so PROTOC_VERSION must be the bare
	// form. protoc release tags are `v`-prefixed, so a `--csharp-version v33.2` override would
	// otherwise yield a `vv33.2` URL that 404s. Strip a leading `v` here so both the cache tag
	// and the build arg receive the same normalized value.
	o.CSharpVersion = strings.TrimPrefix(o.CSharpVersion, "v")
	return o
}

func (o GenOptions) imageTag() string {
	return fmt.Sprintf("worldcli-sdkgen-buf:go%s-protoc%s",
		strings.TrimPrefix(o.GoVersion, "v"), o.CSharpVersion)
}

// EnsureImage returns the buf image tag for these versions, building it (from the
// embedded Dockerfile) only if it isn't already cached. So the cost is one-time
// per version set; later runs reuse the cached image.
func EnsureImage(ctx context.Context, o GenOptions) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("docker is required to build the codegen toolchain but was not found on PATH: %w", err)
	}
	o = o.withDefaults()
	tag := o.imageTag()
	if exec.CommandContext(ctx, "docker", "image", "inspect", tag).Run() == nil {
		return tag, nil // cached
	}
	dir, err := os.MkdirTemp("", "sdkgen-img-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(
		filepath.Join(dir, "Dockerfile"),
		dockerfile,
		0o600,
	); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "docker", "build", "--load", "-t", tag,
		"--build-arg", "GO_PLUGIN_VERSION="+o.GoVersion,
		"--build-arg", "PROTOC_VERSION="+o.CSharpVersion, dir)
	if out, berr := cmd.CombinedOutput(); berr != nil {
		return "", fmt.Errorf("docker build buf image: %w\n%s", berr, out)
	}
	return tag, nil
}

// RunBufCSharp generates C# only. Used for dependency (plugin) types: the plugin ships its own Go wire
// code, so regenerating Go would duplicate/collide in the proto registry — only the C# is missing.
func RunBufCSharp(ctx context.Context, tag string, protos []ProtoFileOut) (string, error) {
	return runBufWith(ctx, tag, protos, bufGenCSYAML, []string{"gen/csharp"})
}

// RunBufGo generates Go only. Used for the Go pass over every locally-defined wire type — including
// system events, which are engine-internal (Go only) and so are excluded from the separate C# run.
func RunBufGo(ctx context.Context, tag string, protos []ProtoFileOut) (string, error) {
	return runBufWith(ctx, tag, protos, bufGenGoYAML, []string{"gen/go"})
}

func runBufWith(
	ctx context.Context,
	tag string,
	protos []ProtoFileOut,
	genYAML []byte,
	checkSubs []string,
) (string, error) {
	work, err := os.MkdirTemp("", "sdkgen-gen-")
	if err != nil {
		return "", err
	}
	// buf config at the root; each .proto at its module-relative path (subdirs created) so imports
	// between package files resolve.
	files := map[string][]byte{
		"buf.yaml":     bufYAML,
		"buf.gen.yaml": genYAML,
	}
	for _, pf := range protos {
		// The work dir is a map, so two files claiming one path would leave only the last — and every
		// message the other held would be missing from the output with nothing reported. Refuse instead:
		// callers assemble this slice from several passes, and a silent drop there is invisible until a
		// client is missing a type.
		if prev, ok := files[pf.Path]; ok && string(prev) != pf.Content {
			_ = os.RemoveAll(work)
			return "", fmt.Errorf("two different .proto files were both generated as %s — one would have "+
				"silently replaced the other", pf.Path)
		}
		files[pf.Path] = []byte(pf.Content)
	}
	for name, data := range files {
		dst := filepath.Join(work, filepath.FromSlash(name))
		if werr := os.MkdirAll(filepath.Dir(dst), 0o755); werr != nil {
			_ = os.RemoveAll(work)
			return "", werr
		}
		if werr := os.WriteFile(dst, data, 0o600); werr != nil {
			_ = os.RemoveAll(work)
			return "", werr
		}
	}
	// Limit generation to this backend's own protos via --path. An import-only file is written into the
	// work dir so the schema resolves, but left off the list: buf generates only what is named here, so
	// its types stay owned by the module that already generated them instead of being copied into this
	// one. (buf's default is the same — files reached through `import` are not generated — but every file
	// here is a work-dir input, so the distinction has to be made explicitly.)
	args := []string{"run", "--rm", "-v", work + ":/work"}
	args = append(args, userArgs(ctx)...)
	args = append(args, tag, "generate")
	var own int
	for _, pf := range protos {
		if pf.ImportOnly {
			continue
		}
		own++
		args = append(args, "--path", pf.Path)
	}
	// No --path at all does not mean "generate nothing" to buf, it means "generate everything" — so a run
	// that wrongly marked every file import-only would produce a full, plausible-looking output and hide
	// the mistake. There is no legitimate run with nothing of its own to generate; say so instead.
	if own == 0 {
		_ = os.RemoveAll(work)
		return "", fmt.Errorf("every one of the %d generated .proto files was marked import-only, so this "+
			"run owns nothing to generate", len(protos))
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	if out, rerr := cmd.CombinedOutput(); rerr != nil {
		_ = os.RemoveAll(work)
		return "", fmt.Errorf("buf generate failed: %w\n%s", rerr, out)
	}
	// buf can exit 0 yet emit nothing (e.g. a misconfigured buf.gen.yaml or a plugin that no-ops),
	// which would otherwise surface downstream as a cryptic "no such file" from copyTree. Fail here.
	for _, sub := range checkSubs {
		if _, serr := os.Stat(filepath.Join(work, sub)); serr != nil {
			_ = os.RemoveAll(work)
			return "", fmt.Errorf("buf generate produced no %s output (check buf.gen.yaml and the image plugins)", sub)
		}
	}
	return work, nil
}

// userArgs runs buf as the invoker on rootful Linux Docker, where container root is host root and would
// leave a work dir the caller can't remove. Rootless Docker already maps container root to the invoker
// (--user there maps to a different host uid), and Docker Desktop maps bind-mount ownership to the user.
func userArgs(ctx context.Context) []string {
	uid := os.Getuid()
	if runtime.GOOS != "linux" || uid == 0 {
		return nil
	}
	out, err := exec.CommandContext(ctx, "docker", "info", "-f", "{{println .SecurityOptions}}").Output()
	if err != nil || strings.Contains(string(out), "name=rootless") {
		return nil
	}
	// That uid has no home in the image, and buf needs a writable cache dir.
	return []string{"--user", fmt.Sprintf("%d:%d", uid, os.Getgid()), "-e", "HOME=/tmp"}
}
