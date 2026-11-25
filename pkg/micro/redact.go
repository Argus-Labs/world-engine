package micro

import (
	"net/url"
	"strings"
)

// invalidNATSURL replaces a NATS URL that cannot be parsed, since the raw string may hold credentials.
const invalidNATSURL = "<invalid NATS URL>"

// redactNATSURL returns a configured NATS URL, or a comma-separated list of them, safe to log.
// It splits the list the way nats.go does and redacts each server with redactNATSServerURL.
// Use it only on configured URLs: a connected URL can hold a literal comma inside its userinfo.
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

// redactNATSServerURL returns a single NATS server URL safe to log. It keeps only the scheme and
// host, so userinfo (user, password, or token) never reaches the output. Use it on
// nats.Conn.ConnectedUrl, whose [url.URL.String] writes a percent-encoded comma in userinfo as a
// literal comma, which a list split would cut inside the credential.
func redactNATSServerURL(s string) string {
	if s == "" {
		return ""
	}

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
