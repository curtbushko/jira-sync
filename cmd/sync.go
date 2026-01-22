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
	"github.com/curtbushko/jira-sync/internal/application/sync"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// syncFlags holds all the parsed flags for the sync command.
type syncFlags struct {
	tasksDir    string
	project     string
	dryRun      bool
	skipConfirm bool
	createOnly  bool
	linkOnly    bool
	statusOnly  bool
}

// syncContext holds all the dependencies for sync operations.
type syncContext struct {
	repo       ports.TaskRepository
	jiraClient ports.JiraClient
	hasher     ports.HashComputer
	service    *sync.Service
	issueType  string
	linkType   string
}

var syncCmd = &cobra.Command{
	Use:   "sync [tasks-dir] (default: .)",
	Short: "Sync task files with Jira (default dir: .)",
	Long: `Synchronize all task files with Jira.

This command handles the full lifecycle:
- Creates tickets for 'pending' tasks
- Links dependencies for 'created' tasks
- Updates local files with Jira data

Arguments:
  tasks-dir   Directory containing task files (default: current directory)

Example:
  jira-sync sync --project GUARD
  jira-sync sync ./tasks/ --project GUARD --dry-run`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)

	syncCmd.Flags().StringP("project", "p", "", "Jira project key (e.g., GUARD)")
	syncCmd.Flags().Bool("dry-run", false, "Show what would happen without making changes")
	syncCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	syncCmd.Flags().Bool("create-only", false, "Only create tickets, don't link dependencies")
	syncCmd.Flags().Bool("link-only", false, "Only link dependencies, don't create tickets")
	syncCmd.Flags().Bool("status-only", false, "Only update status from Jira")

	_ = viper.BindPFlag("defaults.project", syncCmd.Flags().Lookup("project"))
}

var (
	errJiraURLRequired   = errors.New("jira.url is required (set JIRA_URL or use config file)")
	errJiraUserRequired  = errors.New("jira.user is required (set JIRA_USER or use config file)")
	errJiraTokenRequired = errors.New("JIRA_TOKEN environment variable is required")
)

func runSync(cmd *cobra.Command, args []string) error {
	flags := parseSyncFlags(cmd, args)

	repo := filesystem.NewFileTaskRepository()
	tasks, categorized, err := loadAndCategorizeTasks(flags.tasksDir, repo)
	if err != nil {
		return err
	}

	printSummary(tasks, categorized)

	if flags.statusOnly {
		color.Yellow("Status-only mode: No changes to Jira")
		return nil
	}

	if flags.dryRun {
		return handleDryRun(categorized, tasks)
	}

	if !flags.skipConfirm {
		if !confirmSync(len(tasks)) {
			color.Yellow("Cancelled")
			return nil
		}
	}

	syncCtx, err := createSyncContext(repo)
	if err != nil {
		return err
	}

	return executeSyncPhases(cmd.Context(), flags, syncCtx, categorized, tasks)
}

func parseSyncFlags(cmd *cobra.Command, args []string) syncFlags {
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

	return syncFlags{
		tasksDir:    tasksDir,
		project:     project,
		dryRun:      dryRun,
		skipConfirm: skipConfirm,
		createOnly:  createOnly,
		linkOnly:    linkOnly,
		statusOnly:  statusOnly,
	}
}

func loadAndCategorizeTasks(tasksDir string, repo ports.TaskRepository) ([]*domain.TaskFile, *sync.CategorizedTasks, error) {
	color.Cyan("Scanning %s...\n", tasksDir)

	tasks, err := repo.ListTasks(tasksDir)
	if err != nil {
		return nil, nil, fmt.Errorf("parse tasks: %w", err)
	}

	hasher := hashing.NewSHA256HashComputer()
	svc := sync.NewService(repo, nil, hasher)
	categorized := svc.CategorizeTasks(tasks)

	return tasks, categorized, nil
}

func printSummary(tasks []*domain.TaskFile, categorized *sync.CategorizedTasks) {
	fmt.Printf("Found %d task files:\n", len(tasks))
	fmt.Printf("  - %d pending (will create tickets)\n", len(categorized.Pending))
	fmt.Printf("  - %d created (will link dependencies)\n", len(categorized.Created))
	fmt.Printf("  - %d modified (will update Jira description)\n", len(categorized.NeedsUpdate))
	fmt.Printf("  - %d linked (up to date)\n", len(categorized.Linked))
	fmt.Println()
}

func handleDryRun(categorized *sync.CategorizedTasks, tasks []*domain.TaskFile) error {
	color.Yellow("Dry run - no changes will be made")
	showPendingTickets(categorized.Pending)
	showDependenciesToLink(categorized.Created, tasks)
	showModifiedTickets(categorized.NeedsUpdate)
	return nil
}

func confirmSync(taskCount int) bool {
	fmt.Printf("Sync %d tasks with Jira? [y/N] ", taskCount)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	response = strings.TrimSpace(response)
	return response == "y" || response == "Y"
}

func createSyncContext(repo ports.TaskRepository) (*syncContext, error) {
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
	service := sync.NewService(repo, jiraClient, hasher)

	issueType := viper.GetString("defaults.issue_type")
	if issueType == "" {
		issueType = domain.DefaultIssueType
	}

	linkType := viper.GetString("link_types.dependency")
	if linkType == "" {
		linkType = domain.DefaultLinkType
	}

	return &syncContext{
		repo:       repo,
		jiraClient: jiraClient,
		hasher:     hasher,
		service:    service,
		issueType:  issueType,
		linkType:   linkType,
	}, nil
}

func executeSyncPhases(ctx context.Context, flags syncFlags, syncCtx *syncContext, categorized *sync.CategorizedTasks, tasks []*domain.TaskFile) error {
	if err := executeCreatePhase(ctx, flags, syncCtx, categorized); err != nil {
		return err
	}

	if err := executeLinkPhase(ctx, flags, syncCtx, categorized, tasks); err != nil {
		return err
	}

	if err := executeUpdatePhase(ctx, syncCtx, categorized); err != nil {
		return err
	}

	color.Green("\n✓ Sync complete")
	return nil
}

func executeCreatePhase(ctx context.Context, flags syncFlags, syncCtx *syncContext, categorized *sync.CategorizedTasks) error {
	if flags.linkOnly || len(categorized.Pending) == 0 {
		return nil
	}

	color.Cyan("\nCreating tickets...\n")
	if err := syncCtx.service.CreateTickets(ctx, categorized.Pending, flags.project, syncCtx.issueType); err != nil {
		return fmt.Errorf("create tickets: %w", err)
	}

	for _, task := range categorized.Pending {
		if err := syncCtx.repo.WriteTask(task); err != nil {
			return fmt.Errorf("save task %s: %w", task.Path, err)
		}
		color.Green("✓ %s → %s", task.TaskID(), task.Frontmatter.JiraNumber)
	}

	categorized.Created = append(categorized.Created, categorized.Pending...)
	return nil
}

func executeLinkPhase(ctx context.Context, flags syncFlags, syncCtx *syncContext, categorized *sync.CategorizedTasks, tasks []*domain.TaskFile) error {
	if flags.createOnly || len(categorized.Created) == 0 {
		return nil
	}

	color.Cyan("\nLinking dependencies...\n")
	if err := syncCtx.service.LinkDependencies(ctx, categorized.Created, syncCtx.linkType); err != nil {
		return fmt.Errorf("link dependencies: %w", err)
	}

	return saveLinkedTasks(syncCtx, categorized.Created, tasks)
}

func saveLinkedTasks(syncCtx *syncContext, created []*domain.TaskFile, allTasks []*domain.TaskFile) error {
	taskMap := buildTaskMap(allTasks)

	for _, task := range created {
		task.Frontmatter.ContentHash = syncCtx.hasher.ComputeHash(task)
		if err := syncCtx.repo.WriteTask(task); err != nil {
			return fmt.Errorf("save task %s: %w", task.Path, err)
		}
		printLinkedDependencies(task, taskMap)
	}
	return nil
}

func buildTaskMap(tasks []*domain.TaskFile) map[string]*domain.TaskFile {
	taskMap := make(map[string]*domain.TaskFile, len(tasks))
	for _, task := range tasks {
		taskMap[task.TaskID()] = task
	}
	return taskMap
}

func printLinkedDependencies(task *domain.TaskFile, taskMap map[string]*domain.TaskFile) {
	for _, dep := range task.Frontmatter.JiraDependencies {
		if depTask, ok := taskMap[dep]; ok {
			color.Green("✓ %s blocked by %s", task.Frontmatter.JiraNumber, depTask.Frontmatter.JiraNumber)
		}
	}
}

func executeUpdatePhase(ctx context.Context, syncCtx *syncContext, categorized *sync.CategorizedTasks) error {
	if len(categorized.NeedsUpdate) == 0 {
		return nil
	}

	color.Cyan("\nUpdating modified tickets...\n")
	if err := syncCtx.service.UpdateModified(ctx, categorized.NeedsUpdate); err != nil {
		return fmt.Errorf("update tickets: %w", err)
	}

	for _, task := range categorized.NeedsUpdate {
		if err := syncCtx.repo.WriteTask(task); err != nil {
			return fmt.Errorf("save task %s: %w", task.Path, err)
		}
		color.Green("✓ Updated %s", task.Frontmatter.JiraNumber)
	}
	return nil
}

func showPendingTickets(pending []*domain.TaskFile) {
	if len(pending) == 0 {
		return
	}
	fmt.Println("\nPending tickets to create:")
	for _, task := range pending {
		fmt.Printf("  - %s\n", task.Frontmatter.Title)
	}
}

func showDependenciesToLink(created []*domain.TaskFile, allTasks []*domain.TaskFile) {
	idMap := make(map[string]*domain.TaskFile, len(allTasks))
	for _, task := range allTasks {
		idMap[task.TaskID()] = task
	}

	var hasLinks bool
	for _, task := range created {
		if len(task.Frontmatter.JiraDependencies) == 0 {
			continue
		}
		if !hasLinks {
			fmt.Println("\nJira-dependencies to link:")
			hasLinks = true
		}
		printDependencyLinks(task, idMap)
	}
}

func printDependencyLinks(task *domain.TaskFile, idMap map[string]*domain.TaskFile) {
	for _, depID := range task.Frontmatter.JiraDependencies {
		depTask := idMap[depID]
		if depTask != nil && depTask.Frontmatter.JiraNumber != "" {
			fmt.Printf("  - %s blocked by %s\n", task.Frontmatter.JiraNumber, depTask.Frontmatter.JiraNumber)
		} else {
			fmt.Printf("  - %s blocked by %s (pending)\n", task.TaskID(), depID)
		}
	}
}

func showModifiedTickets(modified []*domain.TaskFile) {
	if len(modified) == 0 {
		return
	}
	fmt.Println("\nModified tickets to update:")
	for _, task := range modified {
		fmt.Printf("  - %s (%s)\n", task.Frontmatter.JiraNumber, task.Frontmatter.Title)
	}
}
