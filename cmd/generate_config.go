package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/spf13/cobra"
)

var generateConfigCmd = &cobra.Command{
	Use:   "generate-config",
	Short: "Generate a default config file",
	Long: `Generate a default configuration file at ~/.config/jira-sync/config.yaml.

The generated config includes all available settings with sensible defaults.
You will need to update the jira.url and jira.user values for your environment.

The Jira API token should be set via the JIRA_TOKEN environment variable
and not stored in the config file for security reasons.`,
	RunE: runGenerateConfig,
}

func init() {
	rootCmd.AddCommand(generateConfigCmd)

	generateConfigCmd.Flags().StringP("output", "o", "",
		"Output path (default: ~/.config/jira-sync/config.yaml)")
	generateConfigCmd.Flags().Bool("force", false,
		"Overwrite existing config file")
}

func runGenerateConfig(cmd *cobra.Command, _ []string) error {
	output, _ := cmd.Flags().GetString("output")
	force, _ := cmd.Flags().GetBool("force")

	// Determine output path
	configPath := output
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(home, ".config", domain.DefaultConfigDir, domain.DefaultConfigFile)
	}

	// Check if file exists
	if _, err := os.Stat(configPath); err == nil && !force {
		return fmt.Errorf("config file already exists: %s (use --force to overwrite)", configPath)
	}

	// Create parent directories
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Generate default config content
	content := generateDefaultConfig()

	// Write config file
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Config file created: %s\n", configPath)
	return nil
}

func generateDefaultConfig() string {
	return fmt.Sprintf(`# jira-sync configuration file
# See: https://github.com/curtbushko/jira-sync for documentation

# Jira connection settings
jira:
  # Your Jira instance URL (required)
  url: "https://your-company.atlassian.net"
  # Your Jira username/email (required)
  user: "your-email@company.com"
  # NOTE: Set your API token via JIRA_TOKEN environment variable
  # Do not store tokens in this file for security reasons

# Default values for new tasks
defaults:
  # Default Jira project key
  project: "PROJECT"
  # Default issue type for new tasks
  issue_type: "%s"

# Link type mappings
link_types:
  # Link type used for dependency relationships
  dependency: "%s"
`, domain.DefaultIssueType, domain.DefaultLinkType)
}
