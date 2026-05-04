package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/curtbushko/jira-sync/internal/domain"
)

func TestGenerateConfig_CreatesFileAtDefaultLocation(t *testing.T) {
	// Arrange: Create temp directory to act as home
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	expectedPath := filepath.Join(tempHome, ".config", domain.DefaultConfigDir, domain.DefaultConfigFile)

	// Act: Run generate-config command
	err := runGenerateConfig(generateConfigCmd, []string{})

	// Assert: File should be created at expected location
	require.NoError(t, err)
	assert.FileExists(t, expectedPath)
}

func TestGenerateConfig_CreatesParentDirectories(t *testing.T) {
	// Arrange: Create temp directory with no .config subdirectory
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	expectedDir := filepath.Join(tempHome, ".config", domain.DefaultConfigDir)

	// Act
	err := runGenerateConfig(generateConfigCmd, []string{})

	// Assert: Directory should be created
	require.NoError(t, err)
	assert.DirExists(t, expectedDir)
}

func TestGenerateConfig_ContainsAllDefaults(t *testing.T) {
	// Arrange
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	configPath := filepath.Join(tempHome, ".config", domain.DefaultConfigDir, domain.DefaultConfigFile)

	// Act
	err := runGenerateConfig(generateConfigCmd, []string{})
	require.NoError(t, err)

	// Assert: Read and parse the generated config
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var config map[string]interface{}
	err = yaml.Unmarshal(content, &config)
	require.NoError(t, err)

	// Check jira section exists with placeholder values
	jira, jiraExists := config["jira"].(map[string]interface{})
	require.True(t, jiraExists, "jira section should exist")
	assert.Contains(t, jira, "url")
	assert.Contains(t, jira, "user")

	// Check defaults section has proper default values
	defaults, defaultsExist := config["defaults"].(map[string]interface{})
	require.True(t, defaultsExist, "defaults section should exist")
	assert.Equal(t, domain.DefaultIssueType, defaults["issue_type"])
	assert.Contains(t, defaults, "project")

	// Check link_types section
	linkTypes, linkTypesExist := config["link_types"].(map[string]interface{})
	require.True(t, linkTypesExist, "link_types section should exist")
	assert.Equal(t, domain.DefaultLinkType, linkTypes["dependency"])
}

func TestGenerateConfig_RespectsOutputFlag(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	customPath := filepath.Join(tempDir, "custom", "my-config.yaml")

	// Reset flags for this test
	require.NoError(t, generateConfigCmd.Flags().Set("output", customPath))
	defer func() {
		_ = generateConfigCmd.Flags().Set("output", "")
	}()

	// Act
	err := runGenerateConfig(generateConfigCmd, []string{})

	// Assert
	require.NoError(t, err)
	assert.FileExists(t, customPath)
}

func TestGenerateConfig_FailsIfFileExistsWithoutForce(t *testing.T) {
	// Arrange: Create existing config file
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	configDir := filepath.Join(tempHome, ".config", domain.DefaultConfigDir)
	err := os.MkdirAll(configDir, 0o755)
	require.NoError(t, err)
	configPath := filepath.Join(configDir, domain.DefaultConfigFile)
	err = os.WriteFile(configPath, []byte("existing: config"), 0o644)
	require.NoError(t, err)

	// Reset flags - error ignored as flag is known to exist
	_ = generateConfigCmd.Flags().Set("force", "false")

	// Act
	err = runGenerateConfig(generateConfigCmd, []string{})

	// Assert: Should fail with file exists error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exists")
}

func TestGenerateConfig_SucceedsWithForceWhenFileExists(t *testing.T) {
	// Arrange: Create existing config file
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	configDir := filepath.Join(tempHome, ".config", domain.DefaultConfigDir)
	err := os.MkdirAll(configDir, 0o755)
	require.NoError(t, err)
	configPath := filepath.Join(configDir, domain.DefaultConfigFile)
	err = os.WriteFile(configPath, []byte("existing: config"), 0o644)
	require.NoError(t, err)

	// Set force flag - errors ignored as flags are known to exist
	_ = generateConfigCmd.Flags().Set("force", "true")
	defer func() {
		_ = generateConfigCmd.Flags().Set("force", "false")
	}()

	// Act
	err = runGenerateConfig(generateConfigCmd, []string{})

	// Assert: Should succeed and overwrite
	require.NoError(t, err)

	// Verify content was overwritten with new defaults
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "existing: config")
	assert.Contains(t, string(content), "jira:")
}

func TestGenerateConfig_OutputIncludesComments(t *testing.T) {
	// Arrange
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	configPath := filepath.Join(tempHome, ".config", domain.DefaultConfigDir, domain.DefaultConfigFile)

	// Act
	err := runGenerateConfig(generateConfigCmd, []string{})
	require.NoError(t, err)

	// Assert: Config should include helpful comments
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	contentStr := string(content)

	// Should have comment about JIRA_TOKEN env var
	assert.True(t, strings.Contains(contentStr, "JIRA_TOKEN") || strings.Contains(contentStr, "token"),
		"Config should mention JIRA_TOKEN environment variable")
}
