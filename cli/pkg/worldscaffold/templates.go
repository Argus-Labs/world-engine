package worldscaffold

import (
	"fmt"
	"strings"
)

const exampleImportPrefix = "https://github.com/Argus-Labs/world-engine.git"

// GameTemplate represents a game template that can be cloned.
type GameTemplate struct {
	Name        string
	ShortName   string // Short name for CLI flag usage (e.g., "basic", "demo", "bare")
	Description string
	URL         string
	Subdir      string
}

// GetAvailableTemplates returns all available game templates.
func GetAvailableTemplates() []GameTemplate {
	return []GameTemplate{
		{
			Name:        "Basic Example",
			ShortName:   "basic",
			Description: "Simple example demonstrating core Cardinal concepts",
			URL:         exampleImportPrefix,
			Subdir:      "pkg/template/basic",
		},
		{
			Name:        "Demo Game",
			ShortName:   "demo",
			Description: "More complex game demonstration with multiple features",
			URL:         exampleImportPrefix,
			Subdir:      "pkg/template/multi-shard",
		},
		{
			Name:        "Bare Bones",
			ShortName:   "bare",
			Description: "Empty template with only the bare essentials",
			URL:         exampleImportPrefix,
			Subdir:      "pkg/template/bare-bone",
		},
	}
}

// GetTemplateByName finds a template by its short name (case-insensitive).
// Returns an error if the template is not found.
func GetTemplateByName(name string) (*GameTemplate, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	templates := GetAvailableTemplates()

	for _, t := range templates {
		if strings.ToLower(t.ShortName) == name {
			return &t, nil
		}
	}

	// Build list of valid template names for error message
	validNames := make([]string, len(templates))
	for i, t := range templates {
		validNames[i] = t.ShortName
	}

	return nil, fmt.Errorf("unknown template '%s'. Valid options: %s", name, strings.Join(validNames, ", "))
}
