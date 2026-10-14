// Package dnslabel normalizes arbitrary strings into DNS-label-safe segments.
// world.toml validation requires project and shard IDs to already be canonical, so
// the names the cardinal-shard chart turns into Kubernetes objects and route paths
// are used as-is on both the cluster and the local edge.
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
