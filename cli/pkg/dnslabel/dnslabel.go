// Package dnslabel normalizes arbitrary strings into DNS-label-safe path
// segments. Shard ingress paths are assembled from these segments in two
// independent places — the cardinal-operator (the Ingress path it programs) and
// pkg/cluster (the URL used to address a shard through Traefik) — and the two
// MUST produce identical segments or shard requests 404. This package is the
// single source of that normalization so those callers can't drift apart.
package dnslabel

import "strings"

// Sanitize lowercases value and collapses every run of non-alphanumeric
// characters into a single hyphen, trimming leading and trailing hyphens. Empty
// or all-invalid input yields "unknown" so a path segment is never empty.
func Sanitize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastHyphen := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if b.Len() > 0 && !lastHyphen {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	if result := strings.Trim(b.String(), "-"); result != "" {
		return result
	}
	return "unknown"
}

// IsCanonical reports whether value is already in Sanitize's normal form.
func IsCanonical(value string) bool {
	return Sanitize(value) == value
}
