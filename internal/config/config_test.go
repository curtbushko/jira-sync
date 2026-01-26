package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_LoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".jira-sync.yaml")
	configContent := `
jira:
  url: https://test.atlassian.net
  user: test@test.com
defaults:
  project: TEST
  issue_type: Story
  end_date_offset: 14
link_types:
  dependency: Blocks
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Set required env var
	t.Setenv("JIRA_TOKEN", "test-token")

	cfg, err := LoadFromFile(configPath)

	require.NoError(t, err)
	assert.Equal(t, "https://test.atlassian.net", cfg.Jira.URL)
	assert.Equal(t, "test@test.com", cfg.Jira.User)
	assert.Equal(t, "test-token", cfg.Jira.Token)
	assert.Equal(t, "TEST", cfg.Defaults.Project)
	assert.Equal(t, "Story", cfg.Defaults.IssueType)
	assert.Equal(t, 14, cfg.Defaults.EndDateOffset)
	assert.Equal(t, "Blocks", cfg.LinkTypes.Dependency)
}

func TestConfig_EnvironmentOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".jira-sync.yaml")
	configContent := `
jira:
  url: https://file.atlassian.net
  user: file@test.com
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Set environment variables to override
	t.Setenv("JIRA_URL", "https://env.atlassian.net")
	t.Setenv("JIRA_USER", "env@test.com")
	t.Setenv("JIRA_TOKEN", "env-token")

	cfg, err := LoadFromFile(configPath)

	require.NoError(t, err)
	// Environment should override file
	assert.Equal(t, "https://env.atlassian.net", cfg.Jira.URL)
	assert.Equal(t, "env@test.com", cfg.Jira.User)
	assert.Equal(t, "env-token", cfg.Jira.Token)
}

func TestConfig_Defaults(t *testing.T) {
	t.Setenv("JIRA_URL", "https://test.atlassian.net")
	t.Setenv("JIRA_USER", "test@test.com")
	t.Setenv("JIRA_TOKEN", "test-token")

	cfg, err := LoadFromEnv()

	require.NoError(t, err)
	// Check defaults are applied
	assert.Equal(t, "Story", cfg.Defaults.IssueType)
	assert.Equal(t, 7, cfg.Defaults.EndDateOffset)
	assert.Equal(t, "Blocks", cfg.LinkTypes.Dependency)
}

func TestConfig_MissingToken(t *testing.T) {
	// Clear JIRA_TOKEN if set
	originalToken := os.Getenv("JIRA_TOKEN")
	if originalToken != "" {
		require.NoError(t, os.Unsetenv("JIRA_TOKEN"))
		t.Cleanup(func() {
			_ = os.Setenv("JIRA_TOKEN", originalToken)
		})
	}

	t.Setenv("JIRA_URL", "https://test.atlassian.net")
	t.Setenv("JIRA_USER", "test@test.com")

	_, err := LoadFromEnv()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "JIRA_TOKEN")
}

func TestConfig_MissingURL(t *testing.T) {
	// Clear JIRA_URL if set
	originalURL := os.Getenv("JIRA_URL")
	if originalURL != "" {
		require.NoError(t, os.Unsetenv("JIRA_URL"))
		t.Cleanup(func() {
			_ = os.Setenv("JIRA_URL", originalURL)
		})
	}

	t.Setenv("JIRA_USER", "test@test.com")
	t.Setenv("JIRA_TOKEN", "test-token")

	_, err := LoadFromEnv()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "url")
}

func TestConfig_MissingUser(t *testing.T) {
	// Clear JIRA_USER if set
	originalUser := os.Getenv("JIRA_USER")
	if originalUser != "" {
		require.NoError(t, os.Unsetenv("JIRA_USER"))
		t.Cleanup(func() {
			_ = os.Setenv("JIRA_USER", originalUser)
		})
	}

	t.Setenv("JIRA_URL", "https://test.atlassian.net")
	t.Setenv("JIRA_TOKEN", "test-token")

	_, err := LoadFromEnv()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user")
}

func TestConfig_NestedEnvVars(t *testing.T) {
	t.Setenv("JIRA_URL", "https://test.atlassian.net")
	t.Setenv("JIRA_USER", "test@test.com")
	t.Setenv("JIRA_TOKEN", "test-token")
	t.Setenv("JIRA_DEFAULTS_PROJECT", "MYPROJ")
	t.Setenv("JIRA_DEFAULTS_ISSUE_TYPE", "Bug")
	t.Setenv("JIRA_DEFAULTS_END_DATE_OFFSET", "21")
	t.Setenv("JIRA_LINK_TYPES_DEPENDENCY", "Relates")

	cfg, err := LoadFromEnv()

	require.NoError(t, err)
	assert.Equal(t, "MYPROJ", cfg.Defaults.Project)
	assert.Equal(t, "Bug", cfg.Defaults.IssueType)
	assert.Equal(t, 21, cfg.Defaults.EndDateOffset)
	assert.Equal(t, "Relates", cfg.LinkTypes.Dependency)
}

