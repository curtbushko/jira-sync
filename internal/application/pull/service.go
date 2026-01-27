package pull

import (
	"context"
	"fmt"
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
func NewService(jira ports.JiraClient, hasher ports.HashComputer) *Service {
	return &Service{
		jira:     jira,
		hasher:   hasher,
		detector: NewChangeDetector(hasher),
	}
}

// SetAllTasks sets the list of all tasks for dependency mapping.
// Must be called before pulling tasks with jira-dependencies.
func (s *Service) SetAllTasks(tasks []*domain.TaskFile) {
	s.allTasks = tasks
}

// PullTask pulls Jira changes to a local task file.
// Returns a Result indicating what action was taken.
// Also pulls jira-dependencies if allTasks is set.
func (s *Service) PullTask(ctx context.Context, task *domain.TaskFile, opts ...Option) *Result {
	options := &pullOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Skip tasks without Jira number
	if task.Frontmatter.JiraNumber == "" {
		return &Result{Task: task, Action: ActionSkipped}
	}

	// Get current Jira issue
	jiraIssue, err := s.jira.GetIssue(ctx, task.Frontmatter.JiraNumber)
	if err != nil {
		return &Result{Task: task, Action: ActionError, Error: err}
	}

	// Detect changes
	changeResult := s.detector.Detect(task, jiraIssue)

	var result *Result

	// Handle based on change type
	switch changeResult.Type {
	case ChangeTypeJiraToLocal:
		// Pull Jira changes to local
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
			s.pullFromJira(task, jiraIssue)
			s.updateLastSynced(task)
			result = &Result{
				Task:   task,
				Action: ActionUpdated,
				Fields: changeResult.Fields,
			}
		} else {
			result = &Result{
				Task:   task,
				Action: ActionConflict,
				Fields: changeResult.Fields,
			}
		}

	case ChangeTypeLocalToJira, ChangeTypeNone:
		// No Jira changes to pull
		result = &Result{Task: task, Action: ActionSkipped}
	}

	if result == nil {
		result = &Result{Task: task, Action: ActionSkipped}
	}

	// Also pull jira-dependencies (if allTasks is set and no conflict)
	if s.allTasks != nil && result.Action != ActionConflict {
		depResult, err := s.pullDependencies(ctx, task)
		if err != nil {
			result.Error = err
			result.Action = ActionError
			return result
		}
		result.DependencyResult = depResult
	}

	return result
}

// PullAll pulls Jira changes to all tasks.
// Returns a slice of Results, one for each task.
func (s *Service) PullAll(ctx context.Context, tasks []*domain.TaskFile, opts ...Option) []*Result {
	results := make([]*Result, 0, len(tasks))

	for _, task := range tasks {
		result := s.PullTask(ctx, task, opts...)
		results = append(results, result)
	}

	return results
}

// pullFromJira updates the local task with Jira values.
func (s *Service) pullFromJira(task *domain.TaskFile, jiraIssue *ports.Issue) {
	task.Frontmatter.Title = jiraIssue.Summary
	task.Description = jiraIssue.Description

	if jiraIssue.Status != "" {
		task.Frontmatter.JiraState = jiraIssue.Status
	}
}

// updateLastSynced updates the task's sync metadata.
func (s *Service) updateLastSynced(task *domain.TaskFile) {
	task.Frontmatter.LastSynced = time.Now().UTC().Format(time.RFC3339)
	task.Frontmatter.ContentHash = s.hasher.ComputeHash(task)
}

// pullDependencies pulls jira-dependencies from Jira to the local task.
// This is a pull-only operation - it does NOT create/delete links in Jira.
// Returns the dependency pull result, or nil if no dependencies to pull.
func (s *Service) pullDependencies(ctx context.Context, task *domain.TaskFile) (*DependencyPullResult, error) {
	// Skip if allTasks not set
	if s.allTasks == nil {
		return nil, nil
	}

	// Get current Jira links
	jiraLinks, err := s.jira.GetIssueLinks(ctx, task.Frontmatter.JiraNumber)
	if err != nil {
		return nil, fmt.Errorf("get issue links for %s: %w", task.Frontmatter.JiraNumber, err)
	}

	// Detect dependencies in Jira
	depResult := s.detector.DetectDependencies(task, jiraLinks, s.allTasks)

	// Update local task with Jira dependencies (pull direction only)
	// Jira is authoritative - local file should reflect what Jira has
	if len(depResult.JiraDeps) > 0 {
		task.Frontmatter.JiraDependencies = depResult.JiraDeps
	} else {
		// Clear local deps if Jira has none
		task.Frontmatter.JiraDependencies = nil
	}

	// NOTE: We do NOT create/delete links in Jira here.
	// That's the responsibility of the push service.

	return &depResult, nil
}
