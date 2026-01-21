package fullsync

import (
	"context"
	"time"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// SyncResult represents the result of syncing a single task.
type SyncResult struct {
	Task   *domain.TaskFile
	Type   ChangeType
	Fields []string
	Error  error
}

// Service handles bidirectional sync between local files and Jira.
type Service struct {
	jira     ports.JiraClient
	hasher   ports.HashComputer
	detector *ChangeDetector
}

// NewService creates a new fullsync service.
func NewService(jira ports.JiraClient, hasher ports.HashComputer) *Service {
	return &Service{
		jira:     jira,
		hasher:   hasher,
		detector: NewChangeDetector(hasher),
	}
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

	return &SyncResult{
		Task:   task,
		Type:   changeResult.Type,
		Fields: changeResult.Fields,
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
