package persona

import (
	"regexp"
)

const (
	MinimumPersonaTagLength = 3
	MaximumPersonaTagLength = 16
)

var (
	// Regexp syntax is described here: https://github.com/google/re2/wiki/Syntax
	personaTagRegexp = regexp.MustCompile("^[a-zA-Z0-9_]+$")
)

// IsValidPersonaTag checks that string is a valid persona tag: alphanumeric + underscore
func IsValidPersonaTag(s string) bool {
	if length := len(s); length < MinimumPersonaTagLength || length > MaximumPersonaTagLength {
		return false
	}
	return personaTagRegexp.MatchString(s)
}
