package pull

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// Result contains the result of pulling a task from Jira.
type Result struct {
	Task         *domain.TaskFile
	UpdatedDeps  bool     // Whether dependencies were updated
	Dependencies []string // The dependencies pulled from Jira
	Error        error
}

// Service handles pulling from Jira to local files.
type Service struct {
	jira     ports.JiraClient
	hasher   ports.HashComputer
	depDetector *DependencyDetector
	allTasks []*domain.TaskFile
}

// NewService creates a new pull service.
func NewService(jira ports.JiraClient, hasher ports.HashComputer, linkType string) *Service {
	slog.Debug("creating pull service", slog.String("link_type", linkType))
	return &Service{
		jira:        jira,
		hasher:      hasher,
		depDetector: NewDependencyDetector(linkType),
	}
}

// SetAllTasks sets the list of all tasks for dependency mapping.
func (s *Service) SetAllTasks(tasks []*domain.TaskFile) {
	slog.Debug("setting all tasks for dependency mapping", slog.Int("count", len(tasks)))
	s.allTasks = tasks
}

// PullTask pulls a task from Jira and updates the local file.
// Always syncs from Jira - no change detection.
func (s *Service) PullTask(ctx context.Context, task *domain.TaskFile) *Result {
	slog.Debug("pulling task",
		slog.String("task", task.TaskID()),
		slog.String("jira_key", task.Frontmatter.JiraNumber),
		slog.String("path", task.Path),
	)

	// Skip tasks without Jira number
	if task.Frontmatter.JiraNumber == "" {
		slog.Debug("skipping task without jira number", slog.String("task", task.TaskID()))
		return &Result{Task: task}
	}

	// Fetch Jira issue
	slog.Debug("fetching jira issue", slog.String("jira_key", task.Frontmatter.JiraNumber))
	jiraIssue, err := s.jira.GetIssue(ctx, task.Frontmatter.JiraNumber)
	if err != nil {
		slog.Debug("failed to fetch jira issue",
			slog.String("jira_key", task.Frontmatter.JiraNumber),
			slog.String("error", err.Error()),
		)
		return &Result{Task: task, Error: err}
	}

	slog.Debug("fetched jira issue",
		slog.String("jira_key", task.Frontmatter.JiraNumber),
		slog.String("summary", jiraIssue.Summary),
		slog.String("status", jiraIssue.Status),
	)

	// Update local task with Jira values
	s.syncFromJira(task, jiraIssue)

	// Fetch and update dependencies
	result := &Result{Task: task}
	if s.allTasks != nil {
		deps, err := s.syncDependencies(ctx, task)
		if err != nil {
			slog.Debug("failed to sync dependencies",
				slog.String("task", task.TaskID()),
				slog.String("error", err.Error()),
			)
			result.Error = err
			return result
		}
		result.Dependencies = deps
		result.UpdatedDeps = true
	}

	slog.Debug("pull task completed",
		slog.String("task", task.TaskID()),
		slog.Any("dependencies", result.Dependencies),
	)

	return result
}

// PullAll pulls all tasks from Jira.
func (s *Service) PullAll(ctx context.Context, tasks []*domain.TaskFile) []*Result {
	slog.Debug("pulling all tasks", slog.Int("count", len(tasks)))
	results := make([]*Result, 0, len(tasks))

	for i, task := range tasks {
		slog.Debug("pulling task",
			slog.Int("index", i),
			slog.Int("total", len(tasks)),
			slog.String("task", task.TaskID()),
		)
		result := s.PullTask(ctx, task)
		results = append(results, result)
	}

	slog.Debug("pull all completed", slog.Int("result_count", len(results)))
	return results
}

// syncFromJira updates the local task with Jira values.
func (s *Service) syncFromJira(task *domain.TaskFile, jiraIssue *ports.Issue) {
	slog.Debug("syncing task from jira",
		slog.String("task", task.TaskID()),
		slog.String("old_title", task.Frontmatter.Title),
		slog.String("new_title", jiraIssue.Summary),
		slog.String("old_status", task.Frontmatter.JiraState),
		slog.String("new_status", jiraIssue.Status),
	)

	task.Frontmatter.Title = jiraIssue.Summary
	task.Description = jiraIssue.Description

	if jiraIssue.Status != "" {
		task.Frontmatter.JiraState = jiraIssue.Status
	}

	// Update sync metadata
	task.Frontmatter.LastSynced = time.Now().UTC().Format(time.RFC3339)
	task.Frontmatter.ContentHash = s.hasher.ComputeHash(task)
}

// syncDependencies fetches and updates dependencies from Jira.
func (s *Service) syncDependencies(ctx context.Context, task *domain.TaskFile) ([]string, error) {
	slog.Debug("syncing dependencies",
		slog.String("task", task.TaskID()),
		slog.String("jira_key", task.Frontmatter.JiraNumber),
	)

	// Fetch Jira links
	jiraLinks, err := s.jira.GetIssueLinks(ctx, task.Frontmatter.JiraNumber)
	if err != nil {
		return nil, fmt.Errorf("get issue links for %s: %w", task.Frontmatter.JiraNumber, err)
	}

	slog.Debug("fetched jira links",
		slog.String("jira_key", task.Frontmatter.JiraNumber),
		slog.Int("link_count", len(jiraLinks)),
	)

	// Extract dependencies
	deps := s.depDetector.ExtractDependencies(task, jiraLinks, s.allTasks)

	// Update task
	if len(deps) > 0 {
		task.Frontmatter.JiraDependencies = deps
	} else {
		task.Frontmatter.JiraDependencies = []string{}
	}

	slog.Debug("dependencies synced",
		slog.String("task", task.TaskID()),
		slog.Any("dependencies", deps),
	)

	return deps, nil
}
