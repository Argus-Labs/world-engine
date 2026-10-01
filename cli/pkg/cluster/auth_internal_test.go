package cluster

import "testing"

// A k3d operator is unauthenticated; sending a bearer to it is pointless, and
// requiring a token would make local development need an Argus login.
func TestIsLocalEndpoint(t *testing.T) {
	for _, tt := range []struct {
		endpoint string
		want     bool
	}{
		{"http://localhost:8090", true},
		{"http://127.0.0.1:8090", true},
		{"http://host.docker.internal:8090", true},
		{"https://cardinal-operator-usw2.argus.dev", false},
		{"https://cardinal-operator-eph-x.argus.dev/", false},
		{"https://localhost.argus.dev", false},
		// url.Parse would read a bare host's name as the scheme.
		{"localhost:8090", true},
		// ParseIP classifies all of 127/8 and ::1, which prefix matching missed.
		{"http://[::1]:8090", true},
		{"http://127.0.0.2:8090", true},
	} {
		t.Run(tt.endpoint, func(t *testing.T) {
			if got := isLocalEndpoint(tt.endpoint); got != tt.want {
				t.Errorf("isLocalEndpoint(%q) = %v, want %v", tt.endpoint, got, tt.want)
			}
		})
	}
}
