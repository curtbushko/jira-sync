package cmd

import (
	"fmt"
	"os"

	"github.com/curtbushko/jira-sync/internal/adapters/filesystem"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// migrateFlags holds all the parsed flags for the migrate command.
type migrateFlags struct {
	tasksDir       string
	dryRun         bool
	defaultProject string
}

var migrateCmd = &cobra.Command{
	Use:   "migrate [tasks-dir]",
	Short: "Migrate task files to add missing frontmatter fields",
	Long: `Migrate older task files by adding missing frontmatter fields.

This command scans all task files and adds any missing frontmatter fields
with sensible defaults. This ensures backwards compatibility when new
fields are added to the schema.

Example:
  jira-sync migrate ./tasks/
  jira-sync migrate ./tasks/ --dry-run
  jira-sync migrate ./tasks/ --default-project GUARD`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMigrate,
}

func init() {
	rootCmd.AddCommand(migrateCmd)

	migrateCmd.Flags().Bool("dry-run", false, "Show what would be migrated without making changes")
	migrateCmd.Flags().String("default-project", "", "Default jira-project for tasks missing this field")
}

func runMigrate(cmd *cobra.Command, args []string) error {
	flags := parseMigrateFlags(cmd, args)

	// Check if tasks directory exists
	info, err := os.Stat(flags.tasksDir)
	if err != nil {
		return fmt.Errorf("tasks directory not found: %s", flags.tasksDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", flags.tasksDir)
	}

	repo := filesystem.NewFileTaskRepository()
	writer := filesystem.NewWriter()

	// List all .md files
	tasks, err := repo.ListTasks(flags.tasksDir)
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(tasks) == 0 {
		fmt.Println("No task files found")
		return nil
	}

	// Track migration results
	var migratedCount int
	var skippedCount int

	for _, task := range tasks {
		// Read original file content
		originalContent, err := os.ReadFile(task.Path)
		if err != nil {
			color.Red("Failed to read %s: %v", task.Path, err)
			continue
		}

		// Set default project if provided and task is missing it
		if flags.defaultProject != "" && task.Frontmatter.JiraProject == "" {
			task.Frontmatter.JiraProject = flags.defaultProject
		}

		// The task was already migrated during parsing by MigrateFrontmatter
		// Now marshal the migrated content and compare
		newContent, err := writer.Marshal(task)
		if err != nil {
			color.Red("Failed to marshal %s: %v", task.Path, err)
			continue
		}

		// Compare original and new content
		if newContent == string(originalContent) {
			skippedCount++
			continue
		}

		// File needs migration
		migratedCount++
		taskID := task.TaskID()
		if taskID == "" {
			taskID = task.Frontmatter.Title
		}

		if flags.dryRun {
			color.Yellow("Would migrate: %s (%s)", taskID, task.Path)
		} else {
			// Write back the migrated task
			if err := repo.WriteTask(task); err != nil {
				color.Red("Failed to write %s: %v", task.Path, err)
				continue
			}
			color.Green("Migrated: %s (%s)", taskID, task.Path)
		}
	}

	// Summary
	fmt.Println()
	if flags.dryRun {
		fmt.Printf("Dry run complete: %d files would be migrated, %d already up to date\n",
			migratedCount, skippedCount)
	} else {
		fmt.Printf("Migration complete: %d files migrated, %d already up to date\n",
			migratedCount, skippedCount)
	}

	return nil
}

func parseMigrateFlags(cmd *cobra.Command, args []string) migrateFlags {
	tasksDir := "."
	if len(args) > 0 {
		tasksDir = args[0]
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	defaultProject, _ := cmd.Flags().GetString("default-project")

	return migrateFlags{
		tasksDir:       tasksDir,
		dryRun:         dryRun,
		defaultProject: defaultProject,
	}
}
