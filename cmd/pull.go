package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/curtbushko/jira-sync/internal/adapters/filesystem"
	"github.com/curtbushko/jira-sync/internal/adapters/hashing"
	"github.com/curtbushko/jira-sync/internal/adapters/jira"
	"github.com/curtbushko/jira-sync/internal/application/fullsync"
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
	force       bool
}

// pullContext holds all the dependencies for pull operations.
type pullContext struct {
	repo    ports.TaskRepository
	service *fullsync.Service
}

var pullCmd = &cobra.Command{
	Use:   "pull [tasks-dir] (default: .)",
	Short: "Pull Jira changes to local files (default dir: .)",
	Long: `Pull changes from Jira to local task files.

This command fetches updates from Jira and applies them locally:
- Updates jira-state from Jira status
- Updates title from Jira summary
- Updates description from Jira description

Only tasks with a jira-number are processed.

Arguments:
  tasks-dir   Directory containing task files (default: current directory)

Example:
  jira-sync pull
  jira-sync pull ./tasks/ --dry-run
  jira-sync pull ./tasks/ --force  # Overwrite local changes`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPull,
}

func init() {
	rootCmd.AddCommand(pullCmd)

	pullCmd.Flags().Bool("dry-run", false, "Show what would happen without making changes")
	pullCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	pullCmd.Flags().Bool("force", false, "Overwrite local changes even if there are conflicts")
}

func runPull(cmd *cobra.Command, args []string) error {
	flags := parsePullFlags(cmd, args)

	repo := filesystem.NewFileTaskRepository()
	tasks, err := loadPullableTasks(flags.tasksDir, repo)
	if err != nil {
		return err
	}

	if len(tasks) == 0 {
		color.Yellow("No tasks with Jira numbers found")
		return nil
	}

	printPullSummary(tasks)

	if flags.dryRun {
		return handlePullDryRun(cmd.Context(), tasks, flags)
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

	return executePull(cmd.Context(), pullCtx, tasks, flags)
}

func parsePullFlags(cmd *cobra.Command, args []string) pullFlags {
	tasksDir := "."
	if len(args) > 0 {
		tasksDir = args[0]
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	force, _ := cmd.Flags().GetBool("force")

	return pullFlags{
		tasksDir:    tasksDir,
		dryRun:      dryRun,
		skipConfirm: skipConfirm,
		force:       force,
	}
}

func loadPullableTasks(tasksDir string, repo ports.TaskRepository) ([]*domain.TaskFile, error) {
	color.Cyan("Scanning %s...\n", tasksDir)

	allTasks, err := repo.ListTasks(tasksDir)
	if err != nil {
		return nil, fmt.Errorf("parse tasks: %w", err)
	}

	// Filter to only tasks with Jira numbers
	var tasks []*domain.TaskFile
	for _, task := range allTasks {
		if task.Frontmatter.JiraNumber != "" {
			tasks = append(tasks, task)
		}
	}

	return tasks, nil
}

func printPullSummary(tasks []*domain.TaskFile) {
	fmt.Printf("Found %d tasks with Jira numbers to check for updates\n\n", len(tasks))
}

func handlePullDryRun(ctx context.Context, tasks []*domain.TaskFile, flags pullFlags) error {
	color.Yellow("Dry run - no changes will be made\n")

	pullCtx, err := createPullContext(nil, tasks)
	if err != nil {
		return err
	}

	var opts []fullsync.PullOption
	if flags.force {
		opts = append(opts, fullsync.WithForce(true))
	}

	results := pullCtx.service.PullAll(ctx, tasks, opts...)

	var updated, skipped, conflicts, errors int
	for _, result := range results {
		switch result.Action {
		case fullsync.PullActionUpdated:
			updated++
			color.Green("  [WOULD UPDATE] %s (%s)", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
			for _, field := range result.Fields {
				fmt.Printf("    - %s\n", field)
			}
		case fullsync.PullActionConflict:
			conflicts++
			color.Yellow("  [CONFLICT] %s (%s) - both local and Jira changed", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
		case fullsync.PullActionError:
			errors++
			color.Red("  [ERROR] %s: %v", result.Task.Frontmatter.JiraNumber, result.Error)
		case fullsync.PullActionSkipped:
			skipped++
		}
	}

	fmt.Println()
	fmt.Printf("Summary: %d would update, %d up-to-date, %d conflicts, %d errors\n", updated, skipped, conflicts, errors)
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
	jiraURL := viper.GetString("jira.url")
	jiraUser := viper.GetString("jira.user")
	jiraToken := viper.GetString("token")

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
		return nil, fmt.Errorf("create jira client: %w", err)
	}

	hasher := hashing.NewSHA256HashComputer()
	service := fullsync.NewService(jiraClient, hasher)

	// Set all tasks for dependency mapping
	service.SetAllTasks(allTasks)

	return &pullContext{
		repo:    repo,
		service: service,
	}, nil
}

func executePull(ctx context.Context, pullCtx *pullContext, tasks []*domain.TaskFile, flags pullFlags) error {
	color.Cyan("Pulling updates from Jira...\n")

	var opts []fullsync.PullOption
	if flags.force {
		opts = append(opts, fullsync.WithForce(true))
	}

	results := pullCtx.service.PullAll(ctx, tasks, opts...)

	var updated, skipped, conflicts, errors int
	for _, result := range results {
		switch result.Action {
		case fullsync.PullActionUpdated:
			updated++
			if err := pullCtx.repo.WriteTask(result.Task); err != nil {
				color.Red("[ERROR] Failed to save %s: %v", result.Task.Path, err)
				errors++
				continue
			}
			color.Green("[OK] Updated %s (%s)", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
			for _, field := range result.Fields {
				fmt.Printf("    - %s\n", field)
			}
		case fullsync.PullActionConflict:
			conflicts++
			color.Yellow("[CONFLICT] %s (%s) - use --force to overwrite local", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
		case fullsync.PullActionError:
			errors++
			color.Red("[ERROR] %s: %v", result.Task.Frontmatter.JiraNumber, result.Error)
		case fullsync.PullActionSkipped:
			skipped++
		}
	}

	fmt.Println()
	if conflicts > 0 {
		color.Yellow("Pull completed with conflicts: %d updated, %d up-to-date, %d conflicts, %d errors", updated, skipped, conflicts, errors)
		return nil
	}
	if errors > 0 {
		color.Red("Pull completed with errors: %d updated, %d up-to-date, %d errors", updated, skipped, errors)
		return nil
	}

	color.Green("[OK] Pull complete: %d updated, %d up-to-date", updated, skipped)
	return nil
}
