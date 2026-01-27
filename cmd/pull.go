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
	force       bool
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

	slog.Debug("pull command started",
		slog.String("tasks_dir", flags.tasksDir),
		slog.Bool("dry_run", flags.dryRun),
		slog.Bool("skip_confirm", flags.skipConfirm),
		slog.Bool("force", flags.force),
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
		} else {
			slog.Debug("task has no jira number, skipping",
				slog.String("task", task.TaskID()),
			)
		}
	}

	slog.Debug("filtered to pullable tasks", slog.Int("count", len(tasks)))
	return tasks, nil
}

func printPullSummary(tasks []*domain.TaskFile) {
	fmt.Printf("Found %d tasks with Jira numbers to check for updates\n\n", len(tasks))
}

func handlePullDryRun(ctx context.Context, tasks []*domain.TaskFile, flags pullFlags) error {
	slog.Debug("starting dry run", slog.Int("task_count", len(tasks)))
	color.Yellow("Dry run - no changes will be made\n")

	pullCtx, err := createPullContext(nil, tasks)
	if err != nil {
		slog.Debug("failed to create pull context", slog.String("error", err.Error()))
		return err
	}

	var opts []pull.Option
	if flags.force {
		slog.Debug("force option enabled")
		opts = append(opts, pull.WithForce(true))
	}

	slog.Debug("pulling all tasks in dry run mode")
	results := pullCtx.service.PullAll(ctx, tasks, opts...)

	var updated, skipped, conflicts, errCount int
	for _, result := range results {
		slog.Debug("dry run result",
			slog.String("task", result.Task.TaskID()),
			slog.String("jira_key", result.Task.Frontmatter.JiraNumber),
			slog.String("action", string(result.Action)),
			slog.Any("fields", result.Fields),
		)
		switch result.Action {
		case pull.ActionUpdated:
			updated++
			color.Green("  [WOULD UPDATE] %s (%s)", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
			for _, field := range result.Fields {
				fmt.Printf("    - %s\n", field)
			}
		case pull.ActionConflict:
			conflicts++
			color.Yellow("  [CONFLICT] %s (%s) - both local and Jira changed", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
		case pull.ActionError:
			errCount++
			slog.Debug("dry run error",
				slog.String("task", result.Task.TaskID()),
				slog.String("error", result.Error.Error()),
			)
			color.Red("  [ERROR] %s: %v", result.Task.Frontmatter.JiraNumber, result.Error)
		case pull.ActionSkipped:
			skipped++
		}
	}

	slog.Debug("dry run summary",
		slog.Int("would_update", updated),
		slog.Int("up_to_date", skipped),
		slog.Int("conflicts", conflicts),
		slog.Int("errors", errCount),
	)

	fmt.Println()
	fmt.Printf("Summary: %d would update, %d up-to-date, %d conflicts, %d errors\n", updated, skipped, conflicts, errCount)
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

	// Get link type from config (defaults to "Blocks")
	linkType := viper.GetString("link_types.dependency")
	if linkType == "" {
		linkType = "Blocks"
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

func executePull(ctx context.Context, pullCtx *pullContext, tasks []*domain.TaskFile, flags pullFlags) error {
	slog.Debug("starting pull execution", slog.Int("task_count", len(tasks)))
	color.Cyan("Pulling updates from Jira...\n")

	var opts []pull.Option
	if flags.force {
		slog.Debug("force option enabled for pull")
		opts = append(opts, pull.WithForce(true))
	}

	results := pullCtx.service.PullAll(ctx, tasks, opts...)
	slog.Debug("pull completed", slog.Int("result_count", len(results)))

	var updated, skipped, conflicts, errCount int
	for _, result := range results {
		// Check if dependencies changed (Jira differs from local, in either direction)
		depsUpdated := result.DependencyResult != nil && result.DependencyResult.HasChanges

		slog.Debug("processing pull result",
			slog.String("task", result.Task.TaskID()),
			slog.String("jira_key", result.Task.Frontmatter.JiraNumber),
			slog.String("action", string(result.Action)),
			slog.Bool("has_dependency_result", result.DependencyResult != nil),
			slog.Bool("deps_updated", depsUpdated),
		)

		if result.DependencyResult != nil {
			slog.Debug("dependency result details",
				slog.String("task", result.Task.TaskID()),
				slog.Any("local_deps", result.DependencyResult.LocalDeps),
				slog.Any("jira_deps", result.DependencyResult.JiraDeps),
				slog.Bool("has_changes", result.DependencyResult.HasChanges),
			)
		}

		switch result.Action {
		case pull.ActionUpdated:
			updated++
			slog.Debug("writing updated task",
				slog.String("task", result.Task.TaskID()),
				slog.String("path", result.Task.Path),
				slog.Any("fields", result.Fields),
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
			color.Green("[OK] Updated %s (%s)", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
			for _, field := range result.Fields {
				fmt.Printf("    - %s\n", field)
			}
			if depsUpdated {
				fmt.Printf("    - jira-dependencies\n")
			}
		case pull.ActionConflict:
			conflicts++
			slog.Debug("conflict detected",
				slog.String("task", result.Task.TaskID()),
				slog.String("jira_key", result.Task.Frontmatter.JiraNumber),
				slog.Any("fields", result.Fields),
			)
			color.Yellow("[CONFLICT] %s (%s) - use --force to overwrite local", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
		case pull.ActionError:
			errCount++
			slog.Debug("error during pull",
				slog.String("task", result.Task.TaskID()),
				slog.String("jira_key", result.Task.Frontmatter.JiraNumber),
				slog.String("error", result.Error.Error()),
			)
			color.Red("[ERROR] %s: %v", result.Task.Frontmatter.JiraNumber, result.Error)
		case pull.ActionSkipped:
			// Even if content didn't change, write if dependencies were pulled from Jira
			if depsUpdated {
				updated++
				slog.Debug("writing task with updated dependencies",
					slog.String("task", result.Task.TaskID()),
					slog.String("path", result.Task.Path),
					slog.Any("jira_deps", result.DependencyResult.JiraDeps),
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
				color.Green("[OK] Updated %s (%s)", result.Task.Frontmatter.JiraNumber, result.Task.Frontmatter.Title)
				fmt.Printf("    - jira-dependencies\n")
			} else {
				skipped++
				slog.Debug("task skipped - no changes",
					slog.String("task", result.Task.TaskID()),
					slog.String("jira_key", result.Task.Frontmatter.JiraNumber),
				)
			}
		}
	}

	slog.Debug("pull execution summary",
		slog.Int("updated", updated),
		slog.Int("skipped", skipped),
		slog.Int("conflicts", conflicts),
		slog.Int("errors", errCount),
	)

	fmt.Println()
	if conflicts > 0 {
		color.Yellow("Pull completed with conflicts: %d updated, %d up-to-date, %d conflicts, %d errors", updated, skipped, conflicts, errCount)
		return nil
	}
	if errCount > 0 {
		color.Red("Pull completed with errors: %d updated, %d up-to-date, %d errors", updated, skipped, errCount)
		return nil
	}

	color.Green("[OK] Pull complete: %d updated, %d up-to-date", updated, skipped)
	return nil
}
