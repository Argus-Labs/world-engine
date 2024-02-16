package persona

import (
	"regexp"
)

// IsValidPersonaTag checks that string is a valid persona tag: alphanumeric + underscore
func IsValidPersonaTag(s string) bool {
	var regexpObj = regexp.MustCompile("^[a-zA-Z0-9_]+$")
	return regexpObj.MatchString(s)
}
