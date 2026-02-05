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
	"github.com/curtbushko/jira-sync/internal/application/push"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Common Jira configuration errors.
var (
	errJiraURLRequired   = errors.New("jira.url is required (set JIRA_URL or use config file)")
	errJiraUserRequired  = errors.New("jira.user is required (set JIRA_USER or use config file)")
	errJiraTokenRequired = errors.New("JIRA_TOKEN environment variable is required")
)

// pushFlags holds all the parsed flags for the push command.
type pushFlags struct {
	tasksDir    string
	project     string
	dryRun      bool
	skipConfirm bool
	createOnly  bool
	linkOnly    bool
	statusOnly  bool
}

// pushContext holds all the dependencies for push operations.
type pushContext struct {
	repo       ports.TaskRepository
	jiraClient ports.JiraClient
	hasher     ports.HashComputer
	service    *push.Service
	issueType  string
	linkType   string
}

var pushCmd = &cobra.Command{
	Use:   "push [tasks-dir] (default: .)",
	Short: "Push local changes to Jira (default dir: .)",
	Long: `Push all local task file changes to Jira.

This command handles pushing local changes:
- Creates tickets for 'pending' tasks
- Links dependencies for 'created' tasks
- Updates Jira with modified task descriptions

Arguments:
  tasks-dir   Directory containing task files (default: current directory)

Example:
  jira-sync push --project GUARD
  jira-sync push ./tasks/ --project GUARD --dry-run`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPush,
}

func init() {
	rootCmd.AddCommand(pushCmd)

	pushCmd.Flags().StringP("project", "p", "", "Jira project key (e.g., GUARD)")
	pushCmd.Flags().Bool("dry-run", false, "Show what would happen without making changes")
	pushCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	pushCmd.Flags().Bool("create-only", false, "Only create tickets, don't link dependencies")
	pushCmd.Flags().Bool("link-only", false, "Only link dependencies, don't create tickets")
	pushCmd.Flags().Bool("status-only", false, "Only show status, don't make changes")

	_ = viper.BindPFlag("defaults.project", pushCmd.Flags().Lookup("project"))
}

func runPush(cmd *cobra.Command, args []string) error {
	flags := parsePushFlags(cmd, args)

	slog.Debug("push command started",
		slog.String("tasks_dir", flags.tasksDir),
		slog.String("project", flags.project),
		slog.Bool("dry_run", flags.dryRun),
		slog.Bool("skip_confirm", flags.skipConfirm),
		slog.Bool("create_only", flags.createOnly),
		slog.Bool("link_only", flags.linkOnly),
		slog.Bool("status_only", flags.statusOnly),
	)

	repo := filesystem.NewFileTaskRepository()
	tasks, categorized, err := loadAndCategorizePushTasks(flags.tasksDir, repo)
	if err != nil {
		slog.Debug("failed to load and categorize tasks", slog.String("error", err.Error()))
		return err
	}

	printPushSummary(tasks, categorized)

	if flags.statusOnly {
		color.Yellow("Status-only mode: No changes to Jira")
		return nil
	}

	if flags.dryRun {
		return handlePushDryRun(categorized, tasks)
	}

	if !flags.skipConfirm {
		if !confirmPush(len(tasks)) {
			color.Yellow("Cancelled")
			return nil
		}
	}

	pushCtx, err := createPushContext(repo)
	if err != nil {
		return err
	}

	return executePushPhases(cmd.Context(), flags, pushCtx, categorized, tasks)
}

func parsePushFlags(cmd *cobra.Command, args []string) pushFlags {
	tasksDir := "."
	if len(args) > 0 {
		tasksDir = args[0]
	}

	project := viper.GetString("defaults.project")
	if projectFlag, _ := cmd.Flags().GetString("project"); projectFlag != "" {
		project = projectFlag
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	createOnly, _ := cmd.Flags().GetBool("create-only")
	linkOnly, _ := cmd.Flags().GetBool("link-only")
	statusOnly, _ := cmd.Flags().GetBool("status-only")

	return pushFlags{
		tasksDir:    tasksDir,
		project:     project,
		dryRun:      dryRun,
		skipConfirm: skipConfirm,
		createOnly:  createOnly,
		linkOnly:    linkOnly,
		statusOnly:  statusOnly,
	}
}

func loadAndCategorizePushTasks(tasksDir string, repo ports.TaskRepository) ([]*domain.TaskFile, *push.CategorizedTasks, error) {
	slog.Debug("scanning directory for tasks", slog.String("tasks_dir", tasksDir))
	color.Cyan("Scanning %s...\n", tasksDir)

	tasks, err := repo.ListTasks(tasksDir)
	if err != nil {
		slog.Debug("failed to list tasks", slog.String("error", err.Error()))
		return nil, nil, fmt.Errorf("parse tasks: %w", err)
	}

	slog.Debug("found total tasks", slog.Int("count", len(tasks)))

	hasher := hashing.NewSHA256HashComputer()
	svc := push.NewService(repo, nil, hasher)
	categorized := svc.CategorizeTasks(tasks)

	slog.Debug("categorized tasks",
		slog.Int("pending", len(categorized.Pending)),
		slog.Int("created", len(categorized.Created)),
		slog.Int("linked", len(categorized.Linked)),
		slog.Int("needs_update", len(categorized.NeedsUpdate)),
	)

	// Topologically sort pending tasks by jira-dependencies
	if len(categorized.Pending) > 0 {
		slog.Debug("sorting pending tasks topologically", slog.Int("count", len(categorized.Pending)))
		sorted, err := push.TopologicalSort(categorized.Pending, tasks)
		if err != nil {
			slog.Debug("topological sort failed", slog.String("error", err.Error()))
			return nil, nil, fmt.Errorf("dependency error: %w", err)
		}
		categorized.Pending = sorted
	}

	return tasks, categorized, nil
}

func printPushSummary(tasks []*domain.TaskFile, categorized *push.CategorizedTasks) {
	fmt.Printf("Found %d task files:\n", len(tasks))
	fmt.Printf("  - %d pending (will create tickets)\n", len(categorized.Pending))
	fmt.Printf("  - %d created (will link dependencies)\n", len(categorized.Created))
	fmt.Printf("  - %d modified (will update Jira description)\n", len(categorized.NeedsUpdate))
	fmt.Printf("  - %d linked (up to date)\n", len(categorized.Linked))
	fmt.Println()
}

func handlePushDryRun(categorized *push.CategorizedTasks, tasks []*domain.TaskFile) error {
	color.Yellow("Dry run - no changes will be made")
	showPushPendingTickets(categorized.Pending)
	showPushDependenciesToLink(categorized.Created, tasks)
	showPushModifiedTickets(categorized.NeedsUpdate)
	return nil
}

func confirmPush(taskCount int) bool {
	fmt.Printf("Push %d tasks to Jira? [y/N] ", taskCount)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	response = strings.TrimSpace(response)
	return response == "y" || response == "Y"
}

func createPushContext(repo ports.TaskRepository) (*pushContext, error) {
	slog.Debug("creating push context")

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

	hasher := hashing.NewSHA256HashComputer()
	service := push.NewService(repo, jiraClient, hasher)

	issueType := viper.GetString("defaults.issue_type")
	if issueType == "" {
		issueType = domain.DefaultIssueType
	}

	linkType := viper.GetString("link_types.dependency")
	if linkType == "" {
		linkType = domain.DefaultLinkType
	}

	slog.Debug("push context created",
		slog.String("issue_type", issueType),
		slog.String("link_type", linkType),
	)

	return &pushContext{
		repo:       repo,
		jiraClient: jiraClient,
		hasher:     hasher,
		service:    service,
		issueType:  issueType,
		linkType:   linkType,
	}, nil
}

func executePushPhases(ctx context.Context, flags pushFlags, pushCtx *pushContext, categorized *push.CategorizedTasks, tasks []*domain.TaskFile) error {
	slog.Debug("starting push execution phases")

	if err := executePushCreatePhase(ctx, flags, pushCtx, categorized); err != nil {
		return err
	}

	if err := executePushLinkPhase(ctx, flags, pushCtx, categorized, tasks); err != nil {
		return err
	}

	if err := executePushUpdatePhase(ctx, pushCtx, categorized, tasks); err != nil {
		return err
	}

	slog.Debug("push execution completed successfully")
	color.Green("\n[OK] Push complete")
	return nil
}

func executePushCreatePhase(ctx context.Context, flags pushFlags, pushCtx *pushContext, categorized *push.CategorizedTasks) error {
	if flags.linkOnly || len(categorized.Pending) == 0 {
		slog.Debug("skipping create phase",
			slog.Bool("link_only", flags.linkOnly),
			slog.Int("pending_count", len(categorized.Pending)),
		)
		return nil
	}

	slog.Debug("starting create phase", slog.Int("pending_count", len(categorized.Pending)))
	color.Cyan("\nCreating tickets...\n")
	if err := pushCtx.service.CreateTickets(ctx, categorized.Pending, flags.project, pushCtx.issueType); err != nil {
		slog.Debug("failed to create tickets", slog.String("error", err.Error()))
		return fmt.Errorf("create tickets: %w", err)
	}

	// Transition newly created issues to match local jira-state
	slog.Debug("transitioning newly created issues")
	transitioned, err := pushCtx.service.TransitionIssues(ctx, categorized.Pending)
	if err != nil {
		slog.Debug("failed to transition issues", slog.String("error", err.Error()))
		return fmt.Errorf("transition issues: %w", err)
	}
	if transitioned > 0 {
		slog.Debug("transitioned issues", slog.Int("count", transitioned))
		color.Cyan("Transitioned %d issue(s) to target state\n", transitioned)
	}

	for _, task := range categorized.Pending {
		slog.Debug("saving created task",
			slog.String("task", task.TaskID()),
			slog.String("jira_key", task.Frontmatter.JiraNumber),
			slog.String("path", task.Path),
		)
		if err := pushCtx.repo.WriteTask(task); err != nil {
			slog.Debug("failed to save task", slog.String("path", task.Path), slog.String("error", err.Error()))
			return fmt.Errorf("save task %s: %w", task.Path, err)
		}
		color.Green("[OK] %s -> %s", task.TaskID(), task.Frontmatter.JiraNumber)
	}

	categorized.Created = append(categorized.Created, categorized.Pending...)
	slog.Debug("create phase completed", slog.Int("created_count", len(categorized.Pending)))
	return nil
}

func executePushLinkPhase(ctx context.Context, flags pushFlags, pushCtx *pushContext, categorized *push.CategorizedTasks, tasks []*domain.TaskFile) error {
	if flags.createOnly || len(categorized.Created) == 0 {
		slog.Debug("skipping link phase",
			slog.Bool("create_only", flags.createOnly),
			slog.Int("created_count", len(categorized.Created)),
		)
		return nil
	}

	slog.Debug("starting link phase", slog.Int("created_count", len(categorized.Created)))
	color.Cyan("\nLinking dependencies...\n")
	if err := pushCtx.service.LinkDependencies(ctx, categorized.Created, tasks, pushCtx.linkType); err != nil {
		slog.Debug("failed to link dependencies", slog.String("error", err.Error()))
		return fmt.Errorf("link dependencies: %w", err)
	}

	slog.Debug("link phase completed")
	return savePushLinkedTasks(pushCtx, categorized.Created, tasks)
}

func savePushLinkedTasks(pushCtx *pushContext, created []*domain.TaskFile, allTasks []*domain.TaskFile) error {
	taskMap := buildPushTaskMap(allTasks)

	for _, task := range created {
		task.Frontmatter.ContentHash = pushCtx.hasher.ComputeHash(task)
		if err := pushCtx.repo.WriteTask(task); err != nil {
			return fmt.Errorf("save task %s: %w", task.Path, err)
		}
		printPushLinkedDependencies(task, taskMap)
	}
	return nil
}

func buildPushTaskMap(tasks []*domain.TaskFile) map[string]*domain.TaskFile {
	taskMap := make(map[string]*domain.TaskFile, len(tasks))
	for _, task := range tasks {
		taskMap[task.TaskID()] = task
	}
	return taskMap
}

func printPushLinkedDependencies(task *domain.TaskFile, taskMap map[string]*domain.TaskFile) {
	// Print issues that this task blocks
	for _, blockedID := range task.Frontmatter.JiraBlocks {
		if blockedTask, ok := taskMap[blockedID]; ok {
			color.Green("[OK] %s blocks %s", task.Frontmatter.JiraNumber, blockedTask.Frontmatter.JiraNumber)
		}
	}
	// Print issues that block this task
	for _, blockerID := range task.Frontmatter.JiraIsBlockedBy {
		if blockerTask, ok := taskMap[blockerID]; ok {
			color.Green("[OK] %s is blocked by %s", task.Frontmatter.JiraNumber, blockerTask.Frontmatter.JiraNumber)
		}
	}
}

func executePushUpdatePhase(ctx context.Context, pushCtx *pushContext, categorized *push.CategorizedTasks, tasks []*domain.TaskFile) error {
	if len(categorized.NeedsUpdate) == 0 {
		slog.Debug("skipping update phase", slog.Int("needs_update_count", 0))
		return nil
	}

	slog.Debug("starting update phase", slog.Int("needs_update_count", len(categorized.NeedsUpdate)))
	color.Cyan("\nUpdating modified tickets...\n")
	if err := pushCtx.service.UpdateModified(ctx, categorized.NeedsUpdate); err != nil {
		slog.Debug("failed to update tickets", slog.String("error", err.Error()))
		return fmt.Errorf("update tickets: %w", err)
	}

	// Link any new dependencies (Jira API is idempotent for existing links)
	slog.Debug("linking dependencies for updated tasks")
	if err := pushCtx.service.LinkDependencies(ctx, categorized.NeedsUpdate, tasks, pushCtx.linkType); err != nil {
		slog.Debug("failed to link dependencies", slog.String("error", err.Error()))
		return fmt.Errorf("link dependencies: %w", err)
	}

	// Transition issues to match local jira-state
	slog.Debug("transitioning updated issues")
	transitioned, err := pushCtx.service.TransitionIssues(ctx, categorized.NeedsUpdate)
	if err != nil {
		slog.Debug("failed to transition issues", slog.String("error", err.Error()))
		return fmt.Errorf("transition issues: %w", err)
	}
	if transitioned > 0 {
		slog.Debug("transitioned issues", slog.Int("count", transitioned))
		color.Cyan("Transitioned %d issue(s)\n", transitioned)
	}

	taskMap := buildPushTaskMap(tasks)
	for _, task := range categorized.NeedsUpdate {
		slog.Debug("saving updated task",
			slog.String("task", task.TaskID()),
			slog.String("jira_key", task.Frontmatter.JiraNumber),
			slog.String("path", task.Path),
		)
		task.Frontmatter.ContentHash = pushCtx.hasher.ComputeHash(task)
		if err := pushCtx.repo.WriteTask(task); err != nil {
			slog.Debug("failed to save task", slog.String("path", task.Path), slog.String("error", err.Error()))
			return fmt.Errorf("save task %s: %w", task.Path, err)
		}
		color.Green("[OK] Updated %s", task.Frontmatter.JiraNumber)
		printPushLinkedDependencies(task, taskMap)
	}

	slog.Debug("update phase completed", slog.Int("updated_count", len(categorized.NeedsUpdate)))
	return nil
}

func showPushPendingTickets(pending []*domain.TaskFile) {
	if len(pending) == 0 {
		return
	}
	fmt.Println("\nPending tickets to create:")
	for _, task := range pending {
		fmt.Printf("  - %s\n", task.Frontmatter.Title)
	}
}

func showPushDependenciesToLink(created []*domain.TaskFile, allTasks []*domain.TaskFile) {
	idMap := make(map[string]*domain.TaskFile, len(allTasks))
	for _, task := range allTasks {
		idMap[task.TaskID()] = task
	}

	var hasLinks bool
	for _, task := range created {
		if len(task.Frontmatter.JiraBlocks) == 0 && len(task.Frontmatter.JiraIsBlockedBy) == 0 {
			continue
		}
		if !hasLinks {
			fmt.Println("\nBlocking relationships to link:")
			hasLinks = true
		}
		printPushBlockingLinks(task, idMap)
	}
}

func printPushBlockingLinks(task *domain.TaskFile, idMap map[string]*domain.TaskFile) {
	// Print issues this task blocks
	for _, blockedID := range task.Frontmatter.JiraBlocks {
		blockedTask := idMap[blockedID]
		if blockedTask != nil && blockedTask.Frontmatter.JiraNumber != "" {
			fmt.Printf("  - %s blocks %s\n", task.Frontmatter.JiraNumber, blockedTask.Frontmatter.JiraNumber)
		} else {
			fmt.Printf("  - %s blocks %s (pending)\n", task.TaskID(), blockedID)
		}
	}
	// Print issues that block this task
	for _, blockerID := range task.Frontmatter.JiraIsBlockedBy {
		blockerTask := idMap[blockerID]
		if blockerTask != nil && blockerTask.Frontmatter.JiraNumber != "" {
			fmt.Printf("  - %s is blocked by %s\n", task.Frontmatter.JiraNumber, blockerTask.Frontmatter.JiraNumber)
		} else {
			fmt.Printf("  - %s is blocked by %s (pending)\n", task.TaskID(), blockerID)
		}
	}
}

func showPushModifiedTickets(modified []*domain.TaskFile) {
	if len(modified) == 0 {
		return
	}
	fmt.Println("\nModified tickets to update:")
	for _, task := range modified {
		fmt.Printf("  - %s (%s)\n", task.Frontmatter.JiraNumber, task.Frontmatter.Title)
	}
}
