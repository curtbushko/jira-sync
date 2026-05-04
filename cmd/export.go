package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/curtbushko/jira-sync/internal/adapters/filesystem"
	"github.com/curtbushko/jira-sync/internal/adapters/hashing"
	"github.com/curtbushko/jira-sync/internal/adapters/jira"
	"github.com/curtbushko/jira-sync/internal/application/export"
)

var issueKeyRegex = regexp.MustCompile(`^[A-Z][A-Z0-9]*-\d+$`)

var exportCmd = &cobra.Command{
	Use:   "export <jira-id>",
	Short: "Export a Jira issue to a local task file",
	Long: `Export an existing Jira issue to a local markdown task file.

The filename is generated from the issue's creation date in zettelkasten format.
All relevant fields are mapped to frontmatter, and blocking dependencies are
extracted from issue links.

Arguments:
  jira-id   Jira issue key (e.g., GUARD-123, CRE-456)

Example:
  jira-sync export GUARD-123
  jira-sync export CRE-456 --output ./tasks/
  jira-sync export GUARD-123 --parent GUARD-100 --force`,
	Args: cobra.ExactArgs(1),
	RunE: runExport,
}

func init() {
	rootCmd.AddCommand(exportCmd)

	exportCmd.Flags().StringP("output", "o", ".", "Output directory for task file")
	exportCmd.Flags().StringP("parent", "p", "", "Override jira-parent value")
	exportCmd.Flags().BoolP("force", "f", false, "Overwrite existing file")
}

func runExport(cmd *cobra.Command, args []string) error {
	issueKey := args[0]

	// Get flags
	outputDir, _ := cmd.Flags().GetString("output")
	parentOverride, _ := cmd.Flags().GetString("parent")
	force, _ := cmd.Flags().GetBool("force")

	slog.Debug("export command started",
		slog.String("issue_key", issueKey),
		slog.String("output_dir", outputDir),
		slog.String("parent_override", parentOverride),
		slog.Bool("force", force),
	)

	// Validate issue key format
	if !issueKeyRegex.MatchString(issueKey) {
		slog.Debug("invalid issue key format", slog.String("issue_key", issueKey))
		return fmt.Errorf("invalid issue key format: %s (expected PROJECT-NUMBER, e.g., GUARD-123)", issueKey)
	}

	// Get Jira credentials from config
	jiraURL := viper.GetString("jira.url")
	jiraUser := viper.GetString("jira.user")
	jiraToken := viper.GetString("token")

	slog.Debug("jira config",
		slog.String("jira_url", jiraURL),
		slog.String("jira_user", jiraUser),
		slog.Bool("has_token", jiraToken != ""),
	)

	if jiraURL == "" {
		return errJiraURLRequired
	}
	if jiraUser == "" {
		return errJiraUserRequired
	}
	if jiraToken == "" {
		return errJiraTokenRequired
	}

	// Create Jira client
	jiraClient, err := jira.NewClient(jiraURL, jiraUser, jiraToken)
	if err != nil {
		slog.Debug("failed to create jira client", slog.String("error", err.Error()))
		return fmt.Errorf("create jira client: %w", err)
	}

	// Create dependencies
	repo := filesystem.NewFileTaskRepository()
	hasher := hashing.NewSHA256HashComputer()

	// Load existing tasks for dependency mapping (ignore error, may be empty)
	existingTasks, _ := repo.ListTasks(outputDir)
	slog.Debug("loaded existing tasks for dependency mapping", slog.Int("count", len(existingTasks)))

	// Create export service
	svc := export.NewService(jiraClient, hasher, existingTasks)

	// Export the issue
	slog.Debug("fetching issue from jira", slog.String("issue_key", issueKey))
	color.Cyan("Fetching %s...\n", issueKey)

	result, err := svc.Export(cmd.Context(), issueKey, export.Options{
		ParentOverride: parentOverride,
	})
	if err != nil {
		slog.Debug("failed to export issue", slog.String("issue_key", issueKey), slog.String("error", err.Error()))
		return fmt.Errorf("export %s: %w", issueKey, err)
	}

	slog.Debug("issue exported successfully",
		slog.String("issue_key", issueKey),
		slog.String("title", result.Task.Frontmatter.Title),
		slog.String("status", result.Task.Frontmatter.JiraState),
		slog.String("filename", result.Filename),
	)

	// Generate output path
	outputPath := filepath.Join(outputDir, result.Filename)

	// Check if file exists
	if _, err := os.Stat(outputPath); err == nil && !force {
		slog.Debug("file already exists", slog.String("path", outputPath))
		return fmt.Errorf("file already exists: %s (use --force to overwrite)", outputPath)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		slog.Debug("failed to create output directory", slog.String("dir", outputDir), slog.String("error", err.Error()))
		return fmt.Errorf("create output directory: %w", err)
	}

	// Set the path and write
	result.Task.Path = outputPath
	slog.Debug("writing task file", slog.String("path", outputPath))
	if err := repo.WriteTask(result.Task); err != nil {
		slog.Debug("failed to write task file", slog.String("path", outputPath), slog.String("error", err.Error()))
		return fmt.Errorf("write task file: %w", err)
	}

	// Print success
	color.Green("Found: %s", result.Task.Frontmatter.Title)
	fmt.Printf("  Jira Key: %s\n", result.Task.Frontmatter.JiraNumber)
	fmt.Printf("  Status: %s\n", result.Task.Frontmatter.JiraState)
	fmt.Printf("  Created: %s\n", result.Task.Frontmatter.CreatedDate)
	if result.Task.Frontmatter.JiraParent != "" {
		fmt.Printf("  Parent: %s\n", result.Task.Frontmatter.JiraParent)
	}

	if len(result.Task.Frontmatter.JiraBlocks) > 0 {
		fmt.Printf("  Blocks: %s\n", strings.Join(result.Task.Frontmatter.JiraBlocks, ", "))
	}
	if len(result.Task.Frontmatter.JiraIsBlockedBy) > 0 {
		fmt.Printf("  Is Blocked By: %s\n", strings.Join(result.Task.Frontmatter.JiraIsBlockedBy, ", "))
	}

	slog.Debug("export completed successfully", slog.String("path", outputPath))
	color.Green("\nExported: %s", outputPath)

	return nil
}
