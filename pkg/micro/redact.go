package micro

import (
	"net/url"
	"strings"
)

// invalidNATSURL replaces a NATS URL that cannot be parsed, since the raw string may hold credentials.
const invalidNATSURL = "<invalid NATS URL>"

// redactNATSURL returns a NATS server URL, or a comma-separated list of them, safe to log.
// It keeps only each server's scheme and host, so userinfo (user, password, or token) never
// reaches the output. It splits and defaults the scheme the way nats.go does.
func redactNATSURL(raw string) string {
	var servers []string

	for s := range strings.SplitSeq(raw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}

		servers = append(servers, redactNATSServerURL(s))
	}

	return strings.Join(servers, ",")
}

func redactNATSServerURL(s string) string {
	// Without this, url.Parse reads "user:pass@host" as scheme "user" with an opaque remainder.
	if !strings.Contains(s, "://") {
		s = "nats://" + s
	}

	u, err := url.Parse(s)
	if err != nil || u.Opaque != "" || u.Host == "" {
		return invalidNATSURL
	}

	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}
