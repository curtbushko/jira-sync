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
	service    *sync.Service
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

	repo := filesystem.NewFileTaskRepository()
	tasks, categorized, err := loadAndCategorizePushTasks(flags.tasksDir, repo)
	if err != nil {
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

func loadAndCategorizePushTasks(tasksDir string, repo ports.TaskRepository) ([]*domain.TaskFile, *sync.CategorizedTasks, error) {
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

func printPushSummary(tasks []*domain.TaskFile, categorized *sync.CategorizedTasks) {
	fmt.Printf("Found %d task files:\n", len(tasks))
	fmt.Printf("  - %d pending (will create tickets)\n", len(categorized.Pending))
	fmt.Printf("  - %d created (will link dependencies)\n", len(categorized.Created))
	fmt.Printf("  - %d modified (will update Jira description)\n", len(categorized.NeedsUpdate))
	fmt.Printf("  - %d linked (up to date)\n", len(categorized.Linked))
	fmt.Println()
}

func handlePushDryRun(categorized *sync.CategorizedTasks, tasks []*domain.TaskFile) error {
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

	return &pushContext{
		repo:       repo,
		jiraClient: jiraClient,
		hasher:     hasher,
		service:    service,
		issueType:  issueType,
		linkType:   linkType,
	}, nil
}

func executePushPhases(ctx context.Context, flags pushFlags, pushCtx *pushContext, categorized *sync.CategorizedTasks, tasks []*domain.TaskFile) error {
	if err := executePushCreatePhase(ctx, flags, pushCtx, categorized); err != nil {
		return err
	}

	if err := executePushLinkPhase(ctx, flags, pushCtx, categorized, tasks); err != nil {
		return err
	}

	if err := executePushUpdatePhase(ctx, pushCtx, categorized); err != nil {
		return err
	}

	color.Green("\n[OK] Push complete")
	return nil
}

func executePushCreatePhase(ctx context.Context, flags pushFlags, pushCtx *pushContext, categorized *sync.CategorizedTasks) error {
	if flags.linkOnly || len(categorized.Pending) == 0 {
		return nil
	}

	color.Cyan("\nCreating tickets...\n")
	if err := pushCtx.service.CreateTickets(ctx, categorized.Pending, flags.project, pushCtx.issueType); err != nil {
		return fmt.Errorf("create tickets: %w", err)
	}

	for _, task := range categorized.Pending {
		if err := pushCtx.repo.WriteTask(task); err != nil {
			return fmt.Errorf("save task %s: %w", task.Path, err)
		}
		color.Green("[OK] %s -> %s", task.TaskID(), task.Frontmatter.JiraNumber)
	}

	categorized.Created = append(categorized.Created, categorized.Pending...)
	return nil
}

func executePushLinkPhase(ctx context.Context, flags pushFlags, pushCtx *pushContext, categorized *sync.CategorizedTasks, tasks []*domain.TaskFile) error {
	if flags.createOnly || len(categorized.Created) == 0 {
		return nil
	}

	color.Cyan("\nLinking dependencies...\n")
	if err := pushCtx.service.LinkDependencies(ctx, categorized.Created, pushCtx.linkType); err != nil {
		return fmt.Errorf("link dependencies: %w", err)
	}

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
	for _, dep := range task.Frontmatter.JiraDependencies {
		if depTask, ok := taskMap[dep]; ok {
			color.Green("[OK] %s blocked by %s", task.Frontmatter.JiraNumber, depTask.Frontmatter.JiraNumber)
		}
	}
}

func executePushUpdatePhase(ctx context.Context, pushCtx *pushContext, categorized *sync.CategorizedTasks) error {
	if len(categorized.NeedsUpdate) == 0 {
		return nil
	}

	color.Cyan("\nUpdating modified tickets...\n")
	if err := pushCtx.service.UpdateModified(ctx, categorized.NeedsUpdate); err != nil {
		return fmt.Errorf("update tickets: %w", err)
	}

	for _, task := range categorized.NeedsUpdate {
		if err := pushCtx.repo.WriteTask(task); err != nil {
			return fmt.Errorf("save task %s: %w", task.Path, err)
		}
		color.Green("[OK] Updated %s", task.Frontmatter.JiraNumber)
	}
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
		if len(task.Frontmatter.JiraDependencies) == 0 {
			continue
		}
		if !hasLinks {
			fmt.Println("\nJira-dependencies to link:")
			hasLinks = true
		}
		printPushDependencyLinks(task, idMap)
	}
}

func printPushDependencyLinks(task *domain.TaskFile, idMap map[string]*domain.TaskFile) {
	for _, depID := range task.Frontmatter.JiraDependencies {
		depTask := idMap[depID]
		if depTask != nil && depTask.Frontmatter.JiraNumber != "" {
			fmt.Printf("  - %s blocked by %s\n", task.Frontmatter.JiraNumber, depTask.Frontmatter.JiraNumber)
		} else {
			fmt.Printf("  - %s blocked by %s (pending)\n", task.TaskID(), depID)
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
