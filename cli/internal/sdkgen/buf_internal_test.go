package sdkgen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWithDefaults pins the version-handling contract that EnsureImage relies on: empty fields fall back
// to the repo-pinned defaults, and a `v`-prefixed C# override is normalized to the bare form the embedded
// Dockerfile URL expects.
//
// The Dockerfile downloads protoc from
// .../v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-$A.zip — it prepends its own `v`, so
// PROTOC_VERSION must be bare. protoc's GitHub release tags are `v`-prefixed (v33.2), so a
// `--csharp-version v33.2` override (the spelling a user copies from the releases page) used to flow
// verbatim into PROTOC_VERSION and produce a `vv33.2` URL that 404s, aborting the image build.
// withDefaults strips the leading `v` so both the cache tag (imageTag) and the PROTOC_VERSION build arg
// receive the same normalized value. The Go version is left alone: the Dockerfile's
// `go install …@${GO_PLUGIN_VERSION}` needs the `v`, and imageTag trims it for the tag itself.
func TestWithDefaults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		in           GenOptions
		wantGo       string
		wantCSharp   string
		wantImageTag string
	}{
		{
			name:         "empty fields fall back to repo-pinned defaults",
			in:           GenOptions{},
			wantGo:       DefaultGoVersion,
			wantCSharp:   DefaultCSharpVersion,
			wantImageTag: "worldcli-sdkgen-buf:go1.36.11-protoc33.2",
		},
		{
			name:         "explicit bare C# override passes through unchanged",
			in:           GenOptions{CSharpVersion: "33.2"},
			wantGo:       DefaultGoVersion,
			wantCSharp:   "33.2",
			wantImageTag: "worldcli-sdkgen-buf:go1.36.11-protoc33.2",
		},
		{
			name:         "v-prefixed C# override normalized to bare form",
			in:           GenOptions{CSharpVersion: "v33.2"},
			wantGo:       DefaultGoVersion,
			wantCSharp:   "33.2",
			wantImageTag: "worldcli-sdkgen-buf:go1.36.11-protoc33.2",
		},
		{
			name:         "explicit Go override preserved with its v (go install needs it)",
			in:           GenOptions{GoVersion: "v1.40.0"},
			wantGo:       "v1.40.0",
			wantCSharp:   DefaultCSharpVersion,
			wantImageTag: "worldcli-sdkgen-buf:go1.40.0-protoc33.2",
		},
		{
			name:         "v-prefixed C# override alongside a Go override",
			in:           GenOptions{GoVersion: "v1.40.0", CSharpVersion: "v33.2"},
			wantGo:       "v1.40.0",
			wantCSharp:   "33.2",
			wantImageTag: "worldcli-sdkgen-buf:go1.40.0-protoc33.2",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := c.in.withDefaults()
			require.Equal(t, c.wantGo, got.GoVersion, "GoVersion")
			require.Equal(t, c.wantCSharp, got.CSharpVersion, "CSharpVersion (the value forwarded to PROTOC_VERSION)")
			require.Equal(t, c.wantImageTag, got.imageTag(), "imageTag (the docker cache tag)")
		})
	}
}

// TestProtocURLHasNoDoubledV is the regression guard for the original bug. It rebuilds the GitHub release
// URL exactly as the embedded Dockerfile does — prepending its own `v` to PROTOC_VERSION — using the
// value withDefaults produces, and asserts the URL never contains the broken `vv…` tag segment, whatever
// spelling the CLI user supplied. A passing assertion means the protoc download the image build performs
// targets a real release rather than a 404.
func TestProtocURLHasNoDoubledV(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"", "33.2", "v33.2", "v1.2.3"} {
		protoc := GenOptions{CSharpVersion: in}.withDefaults().CSharpVersion
		url := "https://github.com/protocolbuffers/protobuf/releases/download/v" + protoc +
			"/protoc-" + protoc + "-linux-x86_64.zip"
		require.Falsef(
			t,
			strings.Contains(url, "/vv"),
			"input %q -> PROTOC_VERSION %q -> URL %q contains /vv",
			in,
			protoc,
			url,
		)
	}
}
