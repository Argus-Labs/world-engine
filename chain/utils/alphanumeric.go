package utils

import "regexp"

var alphanumeric = regexp.MustCompile("^[a-zA-Z0-9_]*$")

func IsAlphaNumeric(s string) bool {
	return alphanumeric.MatchString(s)
}
