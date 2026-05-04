package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/curtbushko/jira-sync/internal/adapters/filesystem"
)

// migrateFlags holds all the parsed flags for the migrate command.
type migrateFlags struct {
	tasksDir       string
	dryRun         bool
	defaultProject string
}

var migrateCmd = &cobra.Command{
	Use:   "migrate [tasks-dir] (default: .)",
	Short: "Migrate task files to add missing frontmatter fields (default dir: .)",
	Long: `Migrate older task files by adding missing frontmatter fields.

This command scans all task files and adds any missing frontmatter fields
with sensible defaults. This ensures backwards compatibility when new
fields are added to the schema.

Arguments:
  tasks-dir   Directory containing task files (default: current directory)

Example:
  jira-sync migrate
  jira-sync migrate --dry-run
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

	slog.Debug("migrate command started",
		slog.String("tasks_dir", flags.tasksDir),
		slog.Bool("dry_run", flags.dryRun),
		slog.String("default_project", flags.defaultProject),
	)

	// Check if tasks directory exists
	info, err := os.Stat(flags.tasksDir)
	if err != nil {
		slog.Debug("tasks directory not found", slog.String("dir", flags.tasksDir))
		return fmt.Errorf("tasks directory not found: %s", flags.tasksDir)
	}
	if !info.IsDir() {
		slog.Debug("path is not a directory", slog.String("path", flags.tasksDir))
		return fmt.Errorf("not a directory: %s", flags.tasksDir)
	}

	repo := filesystem.NewFileTaskRepository()
	writer := filesystem.NewWriter()

	// List all .md files
	slog.Debug("listing tasks", slog.String("dir", flags.tasksDir))
	tasks, err := repo.ListTasks(flags.tasksDir)
	if err != nil {
		slog.Debug("failed to list tasks", slog.String("error", err.Error()))
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	slog.Debug("found tasks", slog.Int("count", len(tasks)))

	if len(tasks) == 0 {
		fmt.Println("No task files found")
		return nil
	}

	// Track migration results
	var migratedCount int
	var skippedCount int

	for _, task := range tasks {
		slog.Debug("processing task",
			slog.String("task", task.TaskID()),
			slog.String("path", task.Path),
		)

		// Read original file content
		originalContent, err := os.ReadFile(task.Path)
		if err != nil {
			slog.Debug("failed to read task file", slog.String("path", task.Path), slog.String("error", err.Error()))
			color.Red("Failed to read %s: %v", task.Path, err)
			continue
		}

		// Set default project if provided and task is missing it
		if flags.defaultProject != "" && task.Frontmatter.JiraProject == "" {
			slog.Debug("setting default project",
				slog.String("task", task.TaskID()),
				slog.String("project", flags.defaultProject),
			)
			task.Frontmatter.JiraProject = flags.defaultProject
		}

		// The task was already migrated during parsing by MigrateFrontmatter
		// Now marshal the migrated content and compare
		newContent, err := writer.Marshal(task)
		if err != nil {
			slog.Debug("failed to marshal task", slog.String("path", task.Path), slog.String("error", err.Error()))
			color.Red("Failed to marshal %s: %v", task.Path, err)
			continue
		}

		// Compare original and new content
		if newContent == string(originalContent) {
			slog.Debug("task already up to date", slog.String("task", task.TaskID()))
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
			slog.Debug("would migrate task (dry run)", slog.String("task", taskID), slog.String("path", task.Path))
			color.Yellow("Would migrate: %s (%s)", taskID, task.Path)
		} else {
			// Write back the migrated task
			slog.Debug("writing migrated task", slog.String("task", taskID), slog.String("path", task.Path))
			if err := repo.WriteTask(task); err != nil {
				slog.Debug("failed to write task", slog.String("path", task.Path), slog.String("error", err.Error()))
				color.Red("Failed to write %s: %v", task.Path, err)
				continue
			}
			color.Green("Migrated: %s (%s)", taskID, task.Path)
		}
	}

	slog.Debug("migrate completed",
		slog.Int("migrated", migratedCount),
		slog.Int("skipped", skippedCount),
		slog.Bool("dry_run", flags.dryRun),
	)

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
