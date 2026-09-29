package worldscaffold_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/worldscaffold"
)

func TestGetTemplateByName(t *testing.T) {
	t.Run("finds basic template", func(t *testing.T) {
		template, err := worldscaffold.GetTemplateByName("basic")
		require.NoError(t, err)
		assert.Equal(t, "Basic Example", template.Name)
		assert.Equal(t, "basic", template.ShortName)
	})

	t.Run("finds demo template", func(t *testing.T) {
		template, err := worldscaffold.GetTemplateByName("demo")
		require.NoError(t, err)
		assert.Equal(t, "Demo Game", template.Name)
		assert.Equal(t, "demo", template.ShortName)
	})

	t.Run("finds bare template", func(t *testing.T) {
		template, err := worldscaffold.GetTemplateByName("bare")
		require.NoError(t, err)
		assert.Equal(t, "Bare Bones", template.Name)
		assert.Equal(t, "bare", template.ShortName)
	})

	t.Run("case insensitive matching", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{"BASIC", "basic"},
			{"Basic", "basic"},
			{"BaSiC", "basic"},
			{"DEMO", "demo"},
			{"Demo", "demo"},
			{"BARE", "bare"},
		}

		for _, tt := range tests {
			template, err := worldscaffold.GetTemplateByName(tt.input)
			require.NoError(t, err, "input: %s", tt.input)
			assert.Equal(t, tt.expected, template.ShortName, "input: %s", tt.input)
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		template, err := worldscaffold.GetTemplateByName("  basic  ")
		require.NoError(t, err)
		assert.Equal(t, "basic", template.ShortName)
	})

	t.Run("returns error for unknown template", func(t *testing.T) {
		_, err := worldscaffold.GetTemplateByName("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown template 'nonexistent'")
		assert.Contains(t, err.Error(), "Valid options:")
		assert.Contains(t, err.Error(), "basic")
		assert.Contains(t, err.Error(), "demo")
		assert.Contains(t, err.Error(), "bare")
	})

	t.Run("returns error for empty string", func(t *testing.T) {
		_, err := worldscaffold.GetTemplateByName("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown template ''")
	})

	t.Run("returns error for whitespace only", func(t *testing.T) {
		_, err := worldscaffold.GetTemplateByName("   ")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown template ''")
	})
}

func TestGetAvailableTemplates(t *testing.T) {
	templates := worldscaffold.GetAvailableTemplates()

	t.Run("returns expected number of templates", func(t *testing.T) {
		assert.Len(t, templates, 3)
	})

	t.Run("all templates have required fields", func(t *testing.T) {
		for _, tmpl := range templates {
			assert.NotEmpty(t, tmpl.Name, "Name should not be empty")
			assert.NotEmpty(t, tmpl.ShortName, "ShortName should not be empty")
			assert.NotEmpty(t, tmpl.Description, "Description should not be empty")
			assert.NotEmpty(t, tmpl.URL, "URL should not be empty")
			assert.NotEmpty(t, tmpl.Subdir, "Subdir should not be empty")
		}
	})

	t.Run("short names are unique", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, tmpl := range templates {
			assert.False(t, seen[tmpl.ShortName], "Duplicate ShortName: %s", tmpl.ShortName)
			seen[tmpl.ShortName] = true
		}
	})
}
