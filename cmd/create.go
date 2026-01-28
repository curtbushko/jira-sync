package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/curtbushko/jira-sync/internal/adapters/filesystem"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new task file",
	Long: `Create a new markdown task file with proper frontmatter.

This command generates a task file that can later be synced to Jira.
Designed for easy use by Claude when generating tickets.

Example:
  jira-sync create --title "KB-1: Initialize Project" --jira-parent GUARD-100 --description "Initialize kubebuilder"
  jira-sync create -t "ERR-1: Detector Stub" -p GUARD-100 -d "Create stub" --deps "KB-3"`,
	RunE: runCreate,
}

func init() {
	rootCmd.AddCommand(createCmd)

	// Required flags
	createCmd.Flags().StringP("title", "t", "", "Task ID and title (e.g., 'KB-1: Initialize Project')")
	createCmd.Flags().StringP("jira-parent", "p", "", "Parent epic/story key (e.g., 'GUARD-100'). Required except for Epics.")
	createCmd.Flags().StringP("description", "d", "", "Task description including acceptance criteria (becomes Jira description)")

	// Optional flags
	createCmd.Flags().String("jira-project", "", "Jira project key (e.g., 'GUARD')")
	createCmd.Flags().String("type", domain.DefaultIssueType, "Jira issue type (Task, Story, Bug, Epic)")
	createCmd.Flags().String("deps", "", "Comma-separated task IDs for Jira 'blocks' links (e.g., 'KB-1,ERR-1')")
	createCmd.Flags().StringP("output", "o", ".", "Output directory for task files")

	// Mark required - errors ignored as flags are defined above
	_ = createCmd.MarkFlagRequired("title")
	// Note: jira-parent is NOT required for Epics (validated in runCreate)
	_ = createCmd.MarkFlagRequired("description")

	// Bind output to viper for config file support
	_ = viper.BindPFlag("defaults.output_dir", createCmd.Flags().Lookup("output"))
}

// parseDeps parses a comma-separated dependency string into a slice
func parseDeps(depsStr string) []string {
	var deps []string
	if depsStr != "" {
		for _, d := range strings.Split(depsStr, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				deps = append(deps, d)
			}
		}
	}
	return deps
}

func runCreate(cmd *cobra.Command, _ []string) error {
	// Get flag values
	title, _ := cmd.Flags().GetString("title")
	jiraParent, _ := cmd.Flags().GetString("jira-parent")
	jiraType, _ := cmd.Flags().GetString("type")
	description, _ := cmd.Flags().GetString("description")
	jiraProject, _ := cmd.Flags().GetString("jira-project")
	depsStr, _ := cmd.Flags().GetString("deps")
	outputDir, _ := cmd.Flags().GetString("output")

	slog.Debug("create command started",
		slog.String("title", title),
		slog.String("jira_parent", jiraParent),
		slog.String("jira_type", jiraType),
		slog.String("jira_project", jiraProject),
		slog.String("deps", depsStr),
		slog.String("output_dir", outputDir),
	)

	// Validate jira-parent requirement (required for all types except Epic)
	if !strings.EqualFold(jiraType, "Epic") && jiraParent == "" {
		slog.Debug("jira-parent required but not provided", slog.String("jira_type", jiraType))
		return fmt.Errorf("--jira-parent is required for issue type %q (only Epics can omit parent)", jiraType)
	}

	// Parse dependencies
	jiraDeps := parseDeps(depsStr)
	slog.Debug("parsed dependencies", slog.Int("count", len(jiraDeps)), slog.Any("deps", jiraDeps))

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		slog.Debug("failed to create output directory", slog.String("dir", outputDir), slog.String("error", err.Error()))
		return fmt.Errorf("create output directory: %w", err)
	}

	// Generate zettelkasten filename
	repo := filesystem.NewFileTaskRepository()
	filename := repo.GenerateFilename()
	filePath := filepath.Join(outputDir, filename)
	slog.Debug("generated filename", slog.String("filename", filename), slog.String("path", filePath))

	// Check if file already exists (unlikely but safe)
	if _, err := os.Stat(filePath); err == nil {
		slog.Debug("file already exists, generating new filename", slog.String("path", filePath))
		// Add milliseconds to make unique
		time.Sleep(time.Millisecond)
		filename = repo.GenerateFilename()
		filePath = filepath.Join(outputDir, filename)
		slog.Debug("new filename generated", slog.String("filename", filename), slog.String("path", filePath))
	}

	// Create task file struct
	now := time.Now()
	task := &domain.TaskFile{
		Path: filePath,
		Frontmatter: domain.Frontmatter{
			Title:           title,
			JiraNumber:      "",
			JiraProject:     jiraProject,
			JiraType:        jiraType,
			JiraState:       domain.DefaultJiraState,
			CreatedDate:     now.Format("2006-01-02"),
			JiraURL:         "",
			SyncStatus:      domain.SyncStatusPending,
			JiraParent:      jiraParent,
			JiraBlocks:      []string{},
			JiraIsBlockedBy: jiraDeps, // deps = issues that block this task
			ContentHash:     "",
			LastSynced:      "",
		},
		Description: description,
	}

	// Write the file
	slog.Debug("writing task file", slog.String("path", filePath))
	if err := repo.WriteTask(task); err != nil {
		slog.Debug("failed to write task file", slog.String("path", filePath), slog.String("error", err.Error()))
		return fmt.Errorf("write task file: %w", err)
	}

	slog.Debug("create completed successfully",
		slog.String("path", filePath),
		slog.String("title", title),
		slog.String("jira_type", jiraType),
	)

	color.Green("✓ Created: %s", filePath)
	fmt.Printf("  Title: %s\n", title)
	fmt.Printf("  Type: %s\n", jiraType)
	if jiraParent != "" {
		fmt.Printf("  Jira-Parent: %s\n", jiraParent)
	}
	if jiraProject != "" {
		fmt.Printf("  Jira-Project: %s\n", jiraProject)
	}
	if len(jiraDeps) > 0 {
		fmt.Printf("  Jira-Is-Blocked-By: %s\n", strings.Join(jiraDeps, ", "))
	}

	return nil
}
