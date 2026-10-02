//nolint:testpackage // exercises unexported uniqueTag directly
package cluster

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUniqueTag_BackToBackCallsDiffer is the regression test for the reported
// silent no-op redeploy: uniqueTag() used time.Now().Unix() (1-second
// granularity), so two Deploy calls whose tag computations landed in the same
// wall-clock second produced an identical image:tag. The operator rolls pods
// only on an image-string change, so a colliding tag made the Deploy RPC a
// no-op while the CLI reported success. Sub-second resolution must keep two
// back-to-back calls distinct.
func TestUniqueTag_BackToBackCallsDiffer(t *testing.T) {
	t.Parallel()

	first := uniqueTag()
	second := uniqueTag()
	require.NotEqual(t, first, second,
		"two back-to-back uniqueTag() calls must not collide — a colliding tag makes the "+
			"operator's Deploy RPC a silent no-op (it rolls only on image-string change)")
}

// TestUniqueTag_HasLocalPrefix locks the wire format the retag/import/operator
// path depends on: the tag is appended to the registry ref as "<ref>:<tag>" and
// sent verbatim in DeployRequest.image_tag, and purgeK8sImages lists registry
// push tags by the "<ref>:*" filter. A prefix/shape change would break the k3d
// import and the operator's observed-imageTag diff, so the 'local-' prefix and a
// non-empty suffix must be preserved.
func TestUniqueTag_HasLocalPrefix(t *testing.T) {
	t.Parallel()

	tag := uniqueTag()
	require.True(t, strings.HasPrefix(tag, "local-"),
		"uniqueTag() must keep the 'local-' prefix used by the retag/import/operator path, got %q", tag)
	require.Greater(t, len(tag), len("local-"),
		"uniqueTag() must carry a non-empty suffix after 'local-', got %q", tag)
}
