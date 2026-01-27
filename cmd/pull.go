package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/curtbushko/jira-sync/internal/adapters/filesystem"
	"github.com/curtbushko/jira-sync/internal/adapters/hashing"
	"github.com/curtbushko/jira-sync/internal/adapters/jira"
	"github.com/curtbushko/jira-sync/internal/application/pull"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// pullFlags holds all the parsed flags for the pull command.
type pullFlags struct {
	tasksDir    string
	dryRun      bool
	skipConfirm bool
}

// pullContext holds all the dependencies for pull operations.
type pullContext struct {
	repo    ports.TaskRepository
	service *pull.Service
}

var pullCmd = &cobra.Command{
	Use:   "pull [tasks-dir] (default: .)",
	Short: "Pull Jira changes to local files (default dir: .)",
	Long: `Pull changes from Jira to local task files.

This command fetches updates from Jira and applies them locally:
- Updates jira-state from Jira status
- Updates title from Jira summary
- Updates description from Jira description
- Updates jira-dependencies from Jira issue links

Only tasks with a jira-number are processed.

Arguments:
  tasks-dir   Directory containing task files (default: current directory)

Example:
  jira-sync pull
  jira-sync pull ./tasks/ --dry-run`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPull,
}

func init() {
	rootCmd.AddCommand(pullCmd)

	pullCmd.Flags().Bool("dry-run", false, "Show what would happen without making changes")
	pullCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
}

func runPull(cmd *cobra.Command, args []string) error {
	flags := parsePullFlags(cmd, args)

	slog.Debug("pull command started",
		slog.String("tasks_dir", flags.tasksDir),
		slog.Bool("dry_run", flags.dryRun),
		slog.Bool("skip_confirm", flags.skipConfirm),
	)

	repo := filesystem.NewFileTaskRepository()
	tasks, err := loadPullableTasks(flags.tasksDir, repo)
	if err != nil {
		slog.Debug("failed to load pullable tasks", slog.String("error", err.Error()))
		return err
	}

	if len(tasks) == 0 {
		slog.Debug("no tasks with jira numbers found")
		color.Yellow("No tasks with Jira numbers found")
		return nil
	}

	slog.Debug("found pullable tasks", slog.Int("count", len(tasks)))
	printPullSummary(tasks)

	if flags.dryRun {
		return handlePullDryRun(cmd.Context(), tasks)
	}

	if !flags.skipConfirm {
		if !confirmPull(len(tasks)) {
			color.Yellow("Cancelled")
			return nil
		}
	}

	pullCtx, err := createPullContext(repo, tasks)
	if err != nil {
		return err
	}

	return executePull(cmd.Context(), pullCtx, tasks)
}

func parsePullFlags(cmd *cobra.Command, args []string) pullFlags {
	tasksDir := "."
	if len(args) > 0 {
		tasksDir = args[0]
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	return pullFlags{
		tasksDir:    tasksDir,
		dryRun:      dryRun,
		skipConfirm: skipConfirm,
	}
}

func loadPullableTasks(tasksDir string, repo ports.TaskRepository) ([]*domain.TaskFile, error) {
	slog.Debug("scanning directory for tasks", slog.String("tasks_dir", tasksDir))
	color.Cyan("Scanning %s...\n", tasksDir)

	allTasks, err := repo.ListTasks(tasksDir)
	if err != nil {
		return nil, fmt.Errorf("parse tasks: %w", err)
	}

	slog.Debug("found total tasks", slog.Int("count", len(allTasks)))

	// Filter to only tasks with Jira numbers
	var tasks []*domain.TaskFile
	for _, task := range allTasks {
		if task.Frontmatter.JiraNumber != "" {
			slog.Debug("task has jira number",
				slog.String("task", task.TaskID()),
				slog.String("jira_key", task.Frontmatter.JiraNumber),
			)
			tasks = append(tasks, task)
		}
	}

	slog.Debug("filtered to pullable tasks", slog.Int("count", len(tasks)))
	return tasks, nil
}

func printPullSummary(tasks []*domain.TaskFile) {
	fmt.Printf("Found %d tasks with Jira numbers to sync\n\n", len(tasks))
}

func handlePullDryRun(ctx context.Context, tasks []*domain.TaskFile) error {
	slog.Debug("starting dry run", slog.Int("task_count", len(tasks)))
	color.Yellow("Dry run - no changes will be made\n")

	pullCtx, err := createPullContext(nil, tasks)
	if err != nil {
		slog.Debug("failed to create pull context", slog.String("error", err.Error()))
		return err
	}

	results := pullCtx.service.PullAll(ctx, tasks)

	var synced, errCount int
	for _, result := range results {
		if result.Error != nil {
			errCount++
			color.Red("  [ERROR] %s: %v", result.Task.Frontmatter.JiraNumber, result.Error)
		} else if result.Task.Frontmatter.JiraNumber != "" {
			synced++
			color.Green("  [WOULD SYNC] %s (%s)", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
			if len(result.Dependencies) > 0 {
				fmt.Printf("    - dependencies: %v\n", result.Dependencies)
			}
		}
	}

	fmt.Println()
	fmt.Printf("Summary: %d would sync, %d errors\n", synced, errCount)
	return nil
}

func confirmPull(taskCount int) bool {
	fmt.Printf("Pull updates for %d tasks from Jira? [y/N] ", taskCount)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	response = strings.TrimSpace(response)
	return response == "y" || response == "Y"
}

func createPullContext(repo ports.TaskRepository, allTasks []*domain.TaskFile) (*pullContext, error) {
	slog.Debug("creating pull context")

	jiraURL := viper.GetString("jira.url")
	jiraUser := viper.GetString("jira.user")
	jiraToken := viper.GetString("token")

	slog.Debug("jira config",
		slog.String("jira_url", jiraURL),
		slog.String("jira_user", jiraUser),
		slog.Bool("has_token", jiraToken != ""),
	)

	if jiraURL == "" {
		return nil, errJiraURLRequired
	}
	if jiraUser == "" {
		return nil, errJiraUserRequired
	}
	if jiraToken == "" {
		return nil, errJiraTokenRequired
	}

	jiraClient, err := jira.NewClient(jiraURL, jiraUser, jiraToken)
	if err != nil {
		slog.Debug("failed to create jira client", slog.String("error", err.Error()))
		return nil, fmt.Errorf("create jira client: %w", err)
	}

	// Get link type from config (defaults to "Blocking")
	linkType := viper.GetString("link_types.dependency")
	if linkType == "" {
		linkType = "Blocking"
	}
	slog.Debug("using link type for dependencies", slog.String("link_type", linkType))

	hasher := hashing.NewSHA256HashComputer()
	service := pull.NewService(jiraClient, hasher, linkType)

	// Set all tasks for dependency mapping
	slog.Debug("setting all tasks for dependency mapping", slog.Int("task_count", len(allTasks)))
	service.SetAllTasks(allTasks)

	return &pullContext{
		repo:    repo,
		service: service,
	}, nil
}

func executePull(ctx context.Context, pullCtx *pullContext, tasks []*domain.TaskFile) error {
	slog.Debug("starting pull execution", slog.Int("task_count", len(tasks)))
	color.Cyan("Pulling updates from Jira...\n")

	results := pullCtx.service.PullAll(ctx, tasks)
	slog.Debug("pull completed", slog.Int("result_count", len(results)))

	var synced, errCount int
	for _, result := range results {
		slog.Debug("processing pull result",
			slog.String("task", result.Task.TaskID()),
			slog.String("jira_key", result.Task.Frontmatter.JiraNumber),
			slog.Bool("has_error", result.Error != nil),
			slog.Any("dependencies", result.Dependencies),
		)

		if result.Error != nil {
			errCount++
			slog.Debug("error during pull",
				slog.String("task", result.Task.TaskID()),
				slog.String("error", result.Error.Error()),
			)
			color.Red("[ERROR] %s: %v", result.Task.Frontmatter.JiraNumber, result.Error)
			continue
		}

		// Skip tasks without Jira number (shouldn't happen, but be safe)
		if result.Task.Frontmatter.JiraNumber == "" {
			continue
		}

		// Write the updated task
		slog.Debug("writing task",
			slog.String("task", result.Task.TaskID()),
			slog.String("path", result.Task.Path),
		)
		if err := pullCtx.repo.WriteTask(result.Task); err != nil {
			slog.Debug("failed to write task",
				slog.String("task", result.Task.TaskID()),
				slog.String("error", err.Error()),
			)
			color.Red("[ERROR] Failed to save %s: %v", result.Task.Path, err)
			errCount++
			continue
		}

		synced++
		color.Green("[OK] Synced %s (%s)", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
		if len(result.Dependencies) > 0 {
			fmt.Printf("    - dependencies: %v\n", result.Dependencies)
		}
	}

	slog.Debug("pull execution summary",
		slog.Int("synced", synced),
		slog.Int("errors", errCount),
	)

	fmt.Println()
	if errCount > 0 {
		color.Red("Pull completed with errors: %d synced, %d errors", synced, errCount)
		return nil
	}

	color.Green("[OK] Pull complete: %d synced", synced)
	return nil
}
