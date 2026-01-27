package pull

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// Action represents the action taken during a pull operation.
type Action string

const (
	// ActionUpdated indicates the task was updated from Jira.
	ActionUpdated Action = "updated"
	// ActionSkipped indicates no changes were needed.
	ActionSkipped Action = "skipped"
	// ActionConflict indicates both local and Jira changed.
	ActionConflict Action = "conflict"
	// ActionError indicates an error occurred.
	ActionError Action = "error"
)

// Result contains the result of pulling a task from Jira.
type Result struct {
	Task             *domain.TaskFile
	Action           Action
	Fields           []string
	DependencyResult *DependencyPullResult // nil if no dependency changes
	Error            error
}

// Option configures pull behavior.
type Option func(*pullOptions)

type pullOptions struct {
	force bool
}

// WithForce sets the force option to overwrite local changes.
func WithForce(force bool) Option {
	return func(o *pullOptions) {
		o.force = force
	}
}

// Service handles pulling from Jira to local files.
// This is a pull-only service - it does NOT push to Jira.
type Service struct {
	jira     ports.JiraClient
	hasher   ports.HashComputer
	detector *ChangeDetector
	allTasks []*domain.TaskFile // All tasks for dependency mapping
}

// NewService creates a new pull service.
// linkType specifies the Jira link type for dependencies (e.g., "Blocks", "Is Blocked By").
func NewService(jira ports.JiraClient, hasher ports.HashComputer, linkType string) *Service {
	slog.Debug("creating pull service", slog.String("link_type", linkType))
	return &Service{
		jira:     jira,
		hasher:   hasher,
		detector: NewChangeDetector(hasher, linkType),
	}
}

// SetAllTasks sets the list of all tasks for dependency mapping.
// Must be called before pulling tasks with jira-dependencies.
func (s *Service) SetAllTasks(tasks []*domain.TaskFile) {
	slog.Debug("setting all tasks for dependency mapping", slog.Int("count", len(tasks)))
	s.allTasks = tasks
}

// PullTask pulls Jira changes to a local task file.
// Returns a Result indicating what action was taken.
// Also pulls jira-dependencies if allTasks is set.
func (s *Service) PullTask(ctx context.Context, task *domain.TaskFile, opts ...Option) *Result {
	slog.Debug("pulling task",
		slog.String("task", task.TaskID()),
		slog.String("jira_key", task.Frontmatter.JiraNumber),
		slog.String("path", task.Path),
	)

	options := &pullOptions{}
	for _, opt := range opts {
		opt(options)
	}

	slog.Debug("pull options", slog.Bool("force", options.force))

	// Skip tasks without Jira number
	if task.Frontmatter.JiraNumber == "" {
		slog.Debug("skipping task without jira number", slog.String("task", task.TaskID()))
		return &Result{Task: task, Action: ActionSkipped}
	}

	// Get current Jira issue
	slog.Debug("fetching jira issue", slog.String("jira_key", task.Frontmatter.JiraNumber))
	jiraIssue, err := s.jira.GetIssue(ctx, task.Frontmatter.JiraNumber)
	if err != nil {
		slog.Debug("failed to fetch jira issue",
			slog.String("jira_key", task.Frontmatter.JiraNumber),
			slog.String("error", err.Error()),
		)
		return &Result{Task: task, Action: ActionError, Error: err}
	}

	slog.Debug("fetched jira issue",
		slog.String("jira_key", task.Frontmatter.JiraNumber),
		slog.String("summary", jiraIssue.Summary),
		slog.String("status", jiraIssue.Status),
		slog.Time("updated", jiraIssue.Updated),
	)

	// Detect changes
	changeResult := s.detector.Detect(task, jiraIssue)

	var result *Result

	slog.Debug("change detection result",
		slog.String("task", task.TaskID()),
		slog.Int("change_type", int(changeResult.Type)),
		slog.Any("changed_fields", changeResult.Fields),
	)

	// Handle based on change type
	switch changeResult.Type {
	case ChangeTypeJiraToLocal:
		// Pull Jira changes to local
		slog.Debug("pulling jira changes to local",
			slog.String("task", task.TaskID()),
			slog.Any("fields", changeResult.Fields),
		)
		s.pullFromJira(task, jiraIssue)
		s.updateLastSynced(task)
		result = &Result{
			Task:   task,
			Action: ActionUpdated,
			Fields: changeResult.Fields,
		}

	case ChangeTypeConflict:
		if options.force {
			// Force overwrite local with Jira
			slog.Debug("forcing jira changes to local (conflict override)",
				slog.String("task", task.TaskID()),
				slog.Any("fields", changeResult.Fields),
			)
			s.pullFromJira(task, jiraIssue)
			s.updateLastSynced(task)
			result = &Result{
				Task:   task,
				Action: ActionUpdated,
				Fields: changeResult.Fields,
			}
		} else {
			slog.Debug("conflict detected, not forcing",
				slog.String("task", task.TaskID()),
				slog.Any("fields", changeResult.Fields),
			)
			result = &Result{
				Task:   task,
				Action: ActionConflict,
				Fields: changeResult.Fields,
			}
		}

	case ChangeTypeLocalToJira, ChangeTypeNone:
		// No Jira changes to pull
		slog.Debug("no jira changes to pull",
			slog.String("task", task.TaskID()),
			slog.Int("change_type", int(changeResult.Type)),
		)
		result = &Result{Task: task, Action: ActionSkipped}
	}

	if result == nil {
		result = &Result{Task: task, Action: ActionSkipped}
	}

	// Also pull jira-dependencies (if allTasks is set and no conflict)
	if s.allTasks != nil && result.Action != ActionConflict {
		slog.Debug("pulling dependencies",
			slog.String("task", task.TaskID()),
			slog.String("jira_key", task.Frontmatter.JiraNumber),
		)
		depResult, err := s.pullDependencies(ctx, task)
		if err != nil {
			slog.Debug("failed to pull dependencies",
				slog.String("task", task.TaskID()),
				slog.String("error", err.Error()),
			)
			result.Error = err
			result.Action = ActionError
			return result
		}
		result.DependencyResult = depResult
		if depResult != nil {
			slog.Debug("dependency pull result",
				slog.String("task", task.TaskID()),
				slog.Bool("has_changes", depResult.HasChanges),
				slog.Any("local_deps", depResult.LocalDeps),
				slog.Any("jira_deps", depResult.JiraDeps),
			)
		}
	} else if s.allTasks == nil {
		slog.Debug("skipping dependency pull - allTasks not set",
			slog.String("task", task.TaskID()),
		)
	} else {
		slog.Debug("skipping dependency pull - conflict action",
			slog.String("task", task.TaskID()),
		)
	}

	slog.Debug("pull task completed",
		slog.String("task", task.TaskID()),
		slog.String("action", string(result.Action)),
	)

	return result
}

// PullAll pulls Jira changes to all tasks.
// Returns a slice of Results, one for each task.
func (s *Service) PullAll(ctx context.Context, tasks []*domain.TaskFile, opts ...Option) []*Result {
	slog.Debug("pulling all tasks", slog.Int("count", len(tasks)))
	results := make([]*Result, 0, len(tasks))

	for i, task := range tasks {
		slog.Debug("pulling task",
			slog.Int("index", i),
			slog.Int("total", len(tasks)),
			slog.String("task", task.TaskID()),
		)
		result := s.PullTask(ctx, task, opts...)
		results = append(results, result)
	}

	slog.Debug("pull all completed", slog.Int("result_count", len(results)))
	return results
}

// pullFromJira updates the local task with Jira values.
func (s *Service) pullFromJira(task *domain.TaskFile, jiraIssue *ports.Issue) {
	slog.Debug("updating task from jira",
		slog.String("task", task.TaskID()),
		slog.String("old_title", task.Frontmatter.Title),
		slog.String("new_title", jiraIssue.Summary),
		slog.String("old_status", task.Frontmatter.JiraState),
		slog.String("new_status", jiraIssue.Status),
		slog.Bool("description_changed", task.Description != jiraIssue.Description),
	)
	task.Frontmatter.Title = jiraIssue.Summary
	task.Description = jiraIssue.Description

	if jiraIssue.Status != "" {
		task.Frontmatter.JiraState = jiraIssue.Status
	}
}

// updateLastSynced updates the task's sync metadata.
func (s *Service) updateLastSynced(task *domain.TaskFile) {
	newLastSynced := time.Now().UTC().Format(time.RFC3339)
	newContentHash := s.hasher.ComputeHash(task)
	slog.Debug("updating sync metadata",
		slog.String("task", task.TaskID()),
		slog.String("last_synced", newLastSynced),
		slog.String("content_hash", newContentHash),
	)
	task.Frontmatter.LastSynced = newLastSynced
	task.Frontmatter.ContentHash = newContentHash
}

// pullDependencies pulls jira-dependencies from Jira to the local task.
// This is a pull-only operation - it does NOT create/delete links in Jira.
// Returns the dependency pull result, or nil if no dependencies to pull.
func (s *Service) pullDependencies(ctx context.Context, task *domain.TaskFile) (*DependencyPullResult, error) {
	slog.Debug("pullDependencies called",
		slog.String("task", task.TaskID()),
		slog.String("jira_key", task.Frontmatter.JiraNumber),
	)

	// Skip if allTasks not set
	if s.allTasks == nil {
		slog.Debug("allTasks not set, skipping dependency pull")
		return nil, nil
	}

	// Get current Jira links
	slog.Debug("fetching jira issue links", slog.String("jira_key", task.Frontmatter.JiraNumber))
	jiraLinks, err := s.jira.GetIssueLinks(ctx, task.Frontmatter.JiraNumber)
	if err != nil {
		slog.Debug("failed to fetch jira issue links",
			slog.String("jira_key", task.Frontmatter.JiraNumber),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("get issue links for %s: %w", task.Frontmatter.JiraNumber, err)
	}

	slog.Debug("fetched jira issue links",
		slog.String("jira_key", task.Frontmatter.JiraNumber),
		slog.Int("link_count", len(jiraLinks)),
	)

	// Detect dependencies in Jira
	depResult := s.detector.DetectDependencies(task, jiraLinks, s.allTasks)

	slog.Debug("dependency detection result",
		slog.String("task", task.TaskID()),
		slog.Bool("has_changes", depResult.HasChanges),
		slog.Any("local_deps", depResult.LocalDeps),
		slog.Any("jira_deps", depResult.JiraDeps),
	)

	// Update local task with Jira dependencies (pull direction only)
	// Only update if there are actual changes to avoid clearing unpushed local deps
	if depResult.HasChanges {
		if len(depResult.JiraDeps) > 0 {
			slog.Debug("updating local task with jira dependencies",
				slog.String("task", task.TaskID()),
				slog.Any("new_deps", depResult.JiraDeps),
			)
			task.Frontmatter.JiraDependencies = depResult.JiraDeps
		} else {
			// Set to empty slice (not nil) when Jira has no deps
			slog.Debug("clearing local task dependencies (jira has none)",
				slog.String("task", task.TaskID()),
			)
			task.Frontmatter.JiraDependencies = []string{}
		}
	} else {
		slog.Debug("no dependency changes to apply",
			slog.String("task", task.TaskID()),
		)
	}

	// NOTE: We do NOT create/delete links in Jira here.
	// That's the responsibility of the push service.

	return &depResult, nil
}
