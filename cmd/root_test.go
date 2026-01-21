package cmd

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestInitConfig_ReadsEnvironmentVariables(t *testing.T) {
	// Reset viper for test isolation
	viper.Reset()

	// Set environment variables
	t.Setenv("JIRA_URL", "https://test.atlassian.net")
	t.Setenv("JIRA_USER", "test@test.com")
	t.Setenv("JIRA_TOKEN", "test-token")
	t.Setenv("JIRA_DEFAULTS_PROJECT", "TESTPROJ")
	t.Setenv("JIRA_DEFAULTS_ISSUE_TYPE", "Bug")
	t.Setenv("JIRA_LINK_TYPES_DEPENDENCY", "Relates")

	// Initialize config (simulates what happens at CLI startup)
	initConfig()

	// Verify environment variables are read correctly
	// These are the keys used in sync.go's createSyncContext()
	assert.Equal(t, "https://test.atlassian.net", viper.GetString("jira.url"),
		"JIRA_URL should be accessible via 'jira.url' key")
	assert.Equal(t, "test@test.com", viper.GetString("jira.user"),
		"JIRA_USER should be accessible via 'jira.user' key")
	assert.Equal(t, "test-token", viper.GetString("token"),
		"JIRA_TOKEN should be accessible via 'token' key")

	// These are used in parseSyncFlags() and createSyncContext()
	assert.Equal(t, "TESTPROJ", viper.GetString("defaults.project"),
		"JIRA_DEFAULTS_PROJECT should be accessible via 'defaults.project' key")
	assert.Equal(t, "Bug", viper.GetString("defaults.issue_type"),
		"JIRA_DEFAULTS_ISSUE_TYPE should be accessible via 'defaults.issue_type' key")
	assert.Equal(t, "Relates", viper.GetString("link_types.dependency"),
		"JIRA_LINK_TYPES_DEPENDENCY should be accessible via 'link_types.dependency' key")
}
