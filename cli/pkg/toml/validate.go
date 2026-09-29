package toml

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

// normalizeAndValidate normalizes a string by removing spaces and validates it contains only standard characters.
// Modifies the string in place and returns an error if the string contains special characters.
func normalizeAndValidateString(s *string, fieldName string) error {
	if s == nil {
		return eris.New(fmt.Sprintf("%s cannot be nil", fieldName))
	}

	// Remove all spaces
	normalized := strings.ReplaceAll(*s, " ", "")

	if normalized == "" {
		return eris.New(fmt.Sprintf("%s cannot be empty after removing spaces", fieldName))
	}

	// Allow only alphanumeric characters, hyphens, and underscores
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validPattern.MatchString(normalized) {
		return eris.New(
			fmt.Sprintf(
				"%s contains invalid characters. Only alphanumeric characters, hyphens, and underscores are allowed",
				fieldName,
			))
	}

	*s = normalized
	return nil
}

func validateLogLevel(logLevel, fieldName string) error {
	switch strings.ToLower(logLevel) {
	case zerolog.TraceLevel.String(),
		zerolog.DebugLevel.String(),
		zerolog.InfoLevel.String(),
		zerolog.WarnLevel.String(),
		zerolog.ErrorLevel.String():
		return nil
	}

	return eris.New(fmt.Sprintf("log level must be one of: trace, debug, info, warn, error for %s", fieldName))
}

func validatePath(path string) error {
	if path == "" {
		return nil
	}

	trimmed := strings.TrimSpace(path)
	if trimmed != path {
		return eris.New("path cannot have spaces")
	}

	cleaned := filepath.ToSlash(filepath.Clean(trimmed))

	// Allow characters commonly safe in POSIX paths: letters, numbers, '/', '.', '_', '-'
	// This permits values like ".", "./shards/game", "/", "/var/data", "shards/chat", etc.
	validPattern := regexp.MustCompile(`^[A-Za-z0-9._/\-]+$`)
	if !validPattern.MatchString(cleaned) {
		return eris.New("path contains invalid characters; allowed: letters, numbers, '/', '.', '_', '-'")
	}

	return nil
}
