package root

import "testing"

func TestRemoteOperatorEndpoint(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want string
	}{
		{"us-west1", "https://operator-usw1.argus.dev"},
		{"usw1", "https://operator-usw1.argus.dev"},
		{"usw2", "https://operator-usw1.argus.dev"},
		{"daim-eph-test", "https://operator-usw1.argus.dev/ephemeral/daim-eph-test"},
	} {
		t.Run(tc.env, func(t *testing.T) {
			if got := remoteOperatorEndpoint(tc.env); got != tc.want {
				t.Fatalf("remoteOperatorEndpoint(%q) = %q, want %q", tc.env, got, tc.want)
			}
		})
	}
}
