package toml

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"

	"github.com/argus-labs/world-engine/cli/pkg/dnslabel"
)

// normalizeAndValidateString normalizes a string by removing spaces and validates it contains only standard
// characters. Modifies the string in place and returns an error if the string contains special characters.
//
// This is a deliberately loose check used for fields that do NOT become Kubernetes object names (e.g.
// organization, which flows into env vars and identity strings only). Fields that become Kubernetes
// object names (project, shardID, serviceID) must use [normalizeAndValidateCanonicalName] instead so they
// are rejected before reaching the apiserver with a DNS-1123 violation.
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

// normalizeAndValidateCanonicalName normalizes a string by removing spaces and validates it is a
// dnslabel-canonical name (lowercase alphanumeric characters and hyphens, start/end alphanumeric).
// This is the same contract [world setup] enforces for the project field, ensuring values that flow
// into Kubernetes object names ({project}-db, {project}-{id}-service, metadata.name=shardID) are valid
// DNS-1123 labels and rejected at config load rather than later at k8s apply. Modifies the string in
// place and returns an error if the string is not canonical.
//
// [world setup]: https://github.com/argus-labs/world-engine/blob/main/cli/commands/root/tui/setup_update.go
func normalizeAndValidateCanonicalName(s *string, fieldName string) error {
	if s == nil {
		return eris.New(fmt.Sprintf("%s cannot be nil", fieldName))
	}

	// Remove all spaces so "rampage game" still normalizes to "rampagegame" (matching
	// normalizeAndValidateString's behavior) rather than the dnslabel-canonical "rampage-game".
	normalized := strings.ReplaceAll(*s, " ", "")

	if normalized == "" {
		return eris.New(fmt.Sprintf("%s cannot be empty after removing spaces", fieldName))
	}

	// dnslabel.IsCanonical returns true iff Sanitize(v) == v, i.e. the value is already
	// lowercase-alnum + hyphens with no leading/trailing/inner separator runs. This rejects
	// uppercase letters, underscores, leading hyphens, and other characters that k8s object
	// names (DNS-1123 subdomain) forbid — the same gate world setup applies.
	if !dnslabel.IsCanonical(normalized) {
		return eris.New(fmt.Sprintf(
			"%s contains invalid characters: name must contain only lowercase alphanumeric characters and hyphens "+
				"(DNS-1123 label, e.g. my-game)",
			fieldName,
		))
	}

	*s = normalized
	return nil
}

func validateLogLevel(logLevel, fieldName string) error {
	switch logLevel {
	case zerolog.DebugLevel.String():
		return nil
	case zerolog.InfoLevel.String():
		return nil
	case zerolog.WarnLevel.String():
		return nil
	case zerolog.ErrorLevel.String():
		return nil
	}

	return eris.New(fmt.Sprintf("log level must be one of: debug, info, warn, error for %s", fieldName))
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
