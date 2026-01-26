package fullsync

import (
	"context"
	"fmt"
	"time"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// LinkTypeBlocks is the Jira link type for blocking relationships.
const LinkTypeBlocks = "Blocks"

// PullAction represents the action taken during a pull operation.
type PullAction string

const (
	// PullActionUpdated indicates the task was updated from Jira.
	PullActionUpdated PullAction = "updated"
	// PullActionSkipped indicates no changes were needed.
	PullActionSkipped PullAction = "skipped"
	// PullActionConflict indicates both local and Jira changed.
	PullActionConflict PullAction = "conflict"
	// PullActionError indicates an error occurred.
	PullActionError PullAction = "error"
)

// PullResult contains the result of pulling a task from Jira.
type PullResult struct {
	Task             *domain.TaskFile
	Action           PullAction
	Fields           []string
	DependencyResult *DependencyChangeResult // nil if no dependency changes
	Error            error
}

// PullOption configures pull behavior.
type PullOption func(*pullOptions)

type pullOptions struct {
	force bool
}

// WithForce sets the force option to overwrite local changes.
func WithForce(force bool) PullOption {
	return func(o *pullOptions) {
		o.force = force
	}
}

// SyncResult represents the result of syncing a single task.
type SyncResult struct {
	Task             *domain.TaskFile
	Type             ChangeType
	Fields           []string
	DependencyResult *DependencyChangeResult // nil if no dependency changes
	Error            error
}

// Service handles bidirectional sync between local files and Jira.
type Service struct {
	jira     ports.JiraClient
	hasher   ports.HashComputer
	detector *ChangeDetector
	allTasks []*domain.TaskFile // All tasks for dependency mapping
}

// NewService creates a new fullsync service.
func NewService(jira ports.JiraClient, hasher ports.HashComputer) *Service {
	return &Service{
		jira:     jira,
		hasher:   hasher,
		detector: NewChangeDetector(hasher),
	}
}

// SetAllTasks sets the list of all tasks for dependency mapping.
// Must be called before syncing tasks with jira-dependencies.
func (s *Service) SetAllTasks(tasks []*domain.TaskFile) {
	s.allTasks = tasks
}

// SyncTask syncs a single task bidirectionally.
// Returns the change type and any error.
func (s *Service) SyncTask(ctx context.Context, task *domain.TaskFile) (*SyncResult, error) {
	// Skip tasks without Jira number
	if task.Frontmatter.JiraNumber == "" {
		return &SyncResult{Task: task, Type: ChangeTypeNone}, nil
	}

	// Get current Jira issue
	jiraIssue, err := s.jira.GetIssue(ctx, task.Frontmatter.JiraNumber)
	if err != nil {
		return nil, err
	}

	// Detect changes
	changeResult := s.detector.Detect(task, jiraIssue)

	switch changeResult.Type {
	case ChangeTypeLocalToJira:
		// Push local changes to Jira
		err := s.pushToJira(ctx, task)
		if err != nil {
			return nil, err
		}
		s.updateLastSynced(task)

	case ChangeTypeJiraToLocal:
		// Pull Jira changes to local
		s.pullFromJira(task, jiraIssue)
		s.updateLastSynced(task)

	case ChangeTypeConflict:
		// Don't auto-resolve conflicts
		// Return result so caller can handle

	case ChangeTypeNone:
		// Nothing to do
	}

	// Also sync jira-dependencies (if allTasks is set and no conflict)
	var depResult *DependencyChangeResult
	if s.allTasks != nil && changeResult.Type != ChangeTypeConflict {
		depResult, err = s.syncDependencies(ctx, task)
		if err != nil {
			return nil, err
		}
	}

	return &SyncResult{
		Task:             task,
		Type:             changeResult.Type,
		Fields:           changeResult.Fields,
		DependencyResult: depResult,
	}, nil
}

// SyncAllTasks syncs all tasks and returns results.
func (s *Service) SyncAllTasks(ctx context.Context, tasks []*domain.TaskFile) ([]*SyncResult, error) {
	var results []*SyncResult

	for _, task := range tasks {
		result, err := s.SyncTask(ctx, task)
		if err != nil {
			results = append(results, &SyncResult{
				Task:  task,
				Type:  ChangeTypeNone,
				Error: err,
			})
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// pushToJira updates the Jira issue with local changes.
func (s *Service) pushToJira(ctx context.Context, task *domain.TaskFile) error {
	return s.jira.UpdateIssue(ctx, task.Frontmatter.JiraNumber, ports.UpdateIssueRequest{
		Summary:     task.Frontmatter.Title,
		Description: task.Description,
	})
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

// syncDependencies syncs jira-dependencies between local and Jira.
// Returns the dependency change result, or nil if no dependencies to sync.
func (s *Service) syncDependencies(ctx context.Context, task *domain.TaskFile) (*DependencyChangeResult, error) {
	// Skip if allTasks not set
	if s.allTasks == nil {
		return nil, nil
	}

	// Get current Jira links
	jiraLinks, err := s.jira.GetIssueLinks(ctx, task.Frontmatter.JiraNumber)
	if err != nil {
		return nil, fmt.Errorf("get issue links for %s: %w", task.Frontmatter.JiraNumber, err)
	}

	// Detect dependency changes
	depResult := s.detector.DetectDependencyChanges(task, jiraLinks, s.allTasks)

	// Update local task with Jira dependencies (pull direction)
	// This ensures the local file reflects what's in Jira
	if len(depResult.JiraDeps) > 0 {
		task.Frontmatter.JiraDependencies = depResult.JiraDeps
	} else if len(depResult.LocalDeps) == 0 {
		// Clear local deps if Jira has none and local has none
		task.Frontmatter.JiraDependencies = nil
	}

	if !depResult.HasChanges {
		return &depResult, nil
	}

	// Apply changes: remove stale links
	for _, linkID := range depResult.ToRemove {
		if err := s.jira.DeleteLink(ctx, linkID); err != nil {
			return nil, fmt.Errorf("delete link %s: %w", linkID, err)
		}
	}

	// Apply changes: add new links
	for _, blockerKey := range depResult.ToAdd {
		// Create "Blocks" link: blockerKey blocks task.JiraNumber
		if err := s.jira.CreateLink(ctx, task.Frontmatter.JiraNumber, blockerKey, LinkTypeBlocks); err != nil {
			return nil, fmt.Errorf("create link %s -> %s: %w", blockerKey, task.Frontmatter.JiraNumber, err)
		}
	}

	return &depResult, nil
}

// SyncDependenciesOnly syncs only jira-dependencies for a task.
// Useful when you want to sync dependencies without syncing other fields.
func (s *Service) SyncDependenciesOnly(ctx context.Context, task *domain.TaskFile) (*DependencyChangeResult, error) {
	if task.Frontmatter.JiraNumber == "" {
		return nil, nil
	}
	return s.syncDependencies(ctx, task)
}

// PullTask pulls Jira changes to a local task file.
// Returns a PullResult indicating what action was taken.
// Also syncs jira-dependencies if allTasks is set.
func (s *Service) PullTask(ctx context.Context, task *domain.TaskFile, opts ...PullOption) *PullResult {
	options := &pullOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Skip tasks without Jira number
	if task.Frontmatter.JiraNumber == "" {
		return &PullResult{Task: task, Action: PullActionSkipped}
	}

	// Get current Jira issue
	jiraIssue, err := s.jira.GetIssue(ctx, task.Frontmatter.JiraNumber)
	if err != nil {
		return &PullResult{Task: task, Action: PullActionError, Error: err}
	}

	// Detect changes
	changeResult := s.detector.Detect(task, jiraIssue)

	var result *PullResult

	// Handle based on change type
	switch changeResult.Type {
	case ChangeTypeJiraToLocal:
		// Pull Jira changes to local
		s.pullFromJira(task, jiraIssue)
		s.updateLastSynced(task)
		result = &PullResult{
			Task:   task,
			Action: PullActionUpdated,
			Fields: changeResult.Fields,
		}

	case ChangeTypeConflict:
		if options.force {
			// Force overwrite local with Jira
			s.pullFromJira(task, jiraIssue)
			s.updateLastSynced(task)
			result = &PullResult{
				Task:   task,
				Action: PullActionUpdated,
				Fields: changeResult.Fields,
			}
		} else {
			result = &PullResult{
				Task:   task,
				Action: PullActionConflict,
				Fields: changeResult.Fields,
			}
		}

	case ChangeTypeLocalToJira, ChangeTypeNone:
		// No Jira changes to pull
		result = &PullResult{Task: task, Action: PullActionSkipped}
	}

	if result == nil {
		result = &PullResult{Task: task, Action: PullActionSkipped}
	}

	// Also sync jira-dependencies (if allTasks is set and no conflict)
	if s.allTasks != nil && result.Action != PullActionConflict {
		depResult, err := s.syncDependencies(ctx, task)
		if err != nil {
			result.Error = err
			result.Action = PullActionError
			return result
		}
		result.DependencyResult = depResult
	}

	return result
}

// PullAll pulls Jira changes to all tasks.
// Returns a slice of PullResults, one for each task.
func (s *Service) PullAll(ctx context.Context, tasks []*domain.TaskFile, opts ...PullOption) []*PullResult {
	results := make([]*PullResult, 0, len(tasks))

	for _, task := range tasks {
		result := s.PullTask(ctx, task, opts...)
		results = append(results, result)
	}

	return results
}
