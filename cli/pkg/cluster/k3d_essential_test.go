package cluster

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// TestK3dEssentialFormatter verifies the noise-vs-signal contract documented
// on the formatter. Keeping this in a test pins the contract — if a future
// k3d version starts emitting a new "Pulling X" pattern we want the test to
// remind us to expand the allowlist.
func TestK3dEssentialFormatter(t *testing.T) {
	t.Parallel()
	f := k3dEssentialFormatter{}

	tests := []struct {
		name      string
		level     logrus.Level
		message   string
		wantBytes string
	}{
		{
			name:      "INFO 'Pulling image' is kept (slow op signal)",
			level:     logrus.InfoLevel,
			message:   "Pulling image 'docker.io/rancher/k3s:v1.31.5-k3s1'",
			wantBytes: "  Pulling image 'docker.io/rancher/k3s:v1.31.5-k3s1'\n",
		},
		{
			name:      "INFO boilerplate is dropped (Created network)",
			level:     logrus.InfoLevel,
			message:   "Created network 'k3d-world-engine'",
			wantBytes: "",
		},
		{
			name:      "INFO boilerplate is dropped (Starting node)",
			level:     logrus.InfoLevel,
			message:   "Starting node 'k3d-world-engine-server-0'",
			wantBytes: "",
		},
		{
			name:      "INFO boilerplate is dropped (image-import tools chatter)",
			level:     logrus.InfoLevel,
			message:   "Importing images from tarball '/k3d/images/...' into node 'k3d-world-engine-server-0'",
			wantBytes: "",
		},
		{
			name:      "WARN passes through with level visible",
			level:     logrus.WarnLevel,
			message:   "Healthcheck failed",
			wantBytes: "  [WARNING] Healthcheck failed\n",
		},
		{
			name:      "ERROR passes through with level visible",
			level:     logrus.ErrorLevel,
			message:   "boom",
			wantBytes: "  [ERROR] boom\n",
		},
		{
			name:      "DEBUG is dropped (below the WARN threshold but doesn't match Pulling prefix)",
			level:     logrus.DebugLevel,
			message:   "DOCKER_SOCK=/var/run/docker.sock",
			wantBytes: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := f.Format(&logrus.Entry{Level: tt.level, Message: tt.message})
			require.NoError(t, err)
			require.Equal(t, tt.wantBytes, string(out))
		})
	}
}
