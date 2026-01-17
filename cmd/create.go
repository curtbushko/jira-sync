package cmd

import (
	"fmt"
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
  jira-sync create --title "KB-1: Initialize Project" --parent GUARD-100 --description "Initialize kubebuilder"
  jira-sync create -t "ERR-1: Detector Stub" -p GUARD-100 -d "Create stub" --dependencies "KB-3"`,
	RunE: runCreate,
}

func init() {
	rootCmd.AddCommand(createCmd)

	// Required flags
	createCmd.Flags().StringP("title", "t", "", "Task ID and title (e.g., 'KB-1: Initialize Project')")
	createCmd.Flags().StringP("parent", "p", "", "Parent epic/story key (e.g., 'GUARD-100')")
	createCmd.Flags().StringP("description", "d", "", "Task description including acceptance criteria (becomes Jira description)")

	// Optional flags
	createCmd.Flags().String("dependencies", "", "Comma-separated task IDs (e.g., 'KB-1,ERR-1')")
	createCmd.Flags().StringP("output", "o", "./tasks", "Output directory for task files")

	// Mark required - errors ignored as flags are defined above
	_ = createCmd.MarkFlagRequired("title")
	_ = createCmd.MarkFlagRequired("parent")
	_ = createCmd.MarkFlagRequired("description")

	// Bind output to viper for config file support
	_ = viper.BindPFlag("defaults.output_dir", createCmd.Flags().Lookup("output"))
}

func runCreate(cmd *cobra.Command, _ []string) error {
	// Get flag values
	title, _ := cmd.Flags().GetString("title")
	parent, _ := cmd.Flags().GetString("parent")
	description, _ := cmd.Flags().GetString("description")
	depsStr, _ := cmd.Flags().GetString("dependencies")
	outputDir, _ := cmd.Flags().GetString("output")

	// Parse dependencies
	var deps []string
	if depsStr != "" {
		for _, d := range strings.Split(depsStr, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				deps = append(deps, d)
			}
		}
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Generate zettelkasten filename
	repo := filesystem.NewFileTaskRepository()
	filename := repo.GenerateFilename()
	filePath := filepath.Join(outputDir, filename)

	// Check if file already exists (unlikely but safe)
	if _, err := os.Stat(filePath); err == nil {
		// Add milliseconds to make unique
		time.Sleep(time.Millisecond)
		filename = repo.GenerateFilename()
		filePath = filepath.Join(outputDir, filename)
	}

	// Create task file struct
	now := time.Now()
	task := &domain.TaskFile{
		Path: filePath,
		Frontmatter: domain.Frontmatter{
			Title:        title,
			JiraNumber:   "",
			CreatedDate:  now.Format("2006-01-02"),
			StartDate:    "",
			EndDate:      "",
			JiraURL:      "",
			SyncStatus:   domain.SyncStatusPending,
			Parent:       parent,
			Dependencies: deps,
			ContentHash:  "",
		},
		Description: description,
	}

	// Write the file
	if err := repo.WriteTask(task); err != nil {
		return fmt.Errorf("write task file: %w", err)
	}

	color.Green("✓ Created: %s", filePath)
	fmt.Printf("  Title: %s\n", title)
	fmt.Printf("  Parent: %s\n", parent)
	if len(deps) > 0 {
		fmt.Printf("  Dependencies: %s\n", strings.Join(deps, ", "))
	}

	return nil
}
