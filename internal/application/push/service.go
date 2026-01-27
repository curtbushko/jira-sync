// Package push provides push-only sync from local files to Jira.
package push

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// CategorizedTasks holds tasks grouped by their sync status.
type CategorizedTasks struct {
	Pending     []*domain.TaskFile // Not yet created in Jira
	Created     []*domain.TaskFile // Created but dependencies not linked
	Linked      []*domain.TaskFile // Fully synced and up-to-date
	NeedsUpdate []*domain.TaskFile // Content changed since last sync
}

// Service handles pushing local changes to Jira.
// This is a push-only service - it does NOT pull from Jira.
type Service struct {
	repo      ports.TaskRepository
	jira      ports.JiraClient
	hasher    ports.HashComputer
	validator *domain.FieldValidator
}

// NewService creates a new push service.
func NewService(repo ports.TaskRepository, jira ports.JiraClient, hasher ports.HashComputer) *Service {
	return &Service{
		repo:      repo,
		jira:      jira,
		hasher:    hasher,
		validator: domain.NewFieldValidator(nil), // No logging by default
	}
}

// NewServiceWithLogger creates a new push service with a logger for validation warnings.
func NewServiceWithLogger(repo ports.TaskRepository, jira ports.JiraClient, hasher ports.HashComputer, logger io.Writer) *Service {
	return &Service{
		repo:      repo,
		jira:      jira,
		hasher:    hasher,
		validator: domain.NewFieldValidator(logger),
	}
}

// CategorizeTasks groups tasks by their sync status and change detection.
func (s *Service) CategorizeTasks(tasks []*domain.TaskFile) *CategorizedTasks {
	result := &CategorizedTasks{}

	for _, task := range tasks {
		switch task.Frontmatter.SyncStatus {
		case domain.SyncStatusPending:
			result.Pending = append(result.Pending, task)
		case domain.SyncStatusCreated:
			result.Created = append(result.Created, task)
		case domain.SyncStatusLinked:
			if s.needsResync(task) {
				result.NeedsUpdate = append(result.NeedsUpdate, task)
			} else {
				result.Linked = append(result.Linked, task)
			}
		default:
			// Treat unknown status as pending
			result.Pending = append(result.Pending, task)
		}
	}

	return result
}

// needsResync checks if a task's content has changed since last sync.
func (s *Service) needsResync(task *domain.TaskFile) bool {
	if task.Frontmatter.ContentHash == "" {
		return true // Never synced
	}
	if s.hasher == nil {
		return false
	}
	currentHash := s.hasher.ComputeHash(task)
	return currentHash != task.Frontmatter.ContentHash
}

// CreateTickets creates Jira tickets for pending tasks.
// Fields are validated and truncated if they exceed Jira limits.
// Uses task's jira-project if set, otherwise falls back to defaultProject.
// Uses task's jira-type if set, otherwise falls back to defaultIssueType.
func (s *Service) CreateTickets(ctx context.Context, tasks []*domain.TaskFile, defaultProject, defaultIssueType string) error {
	for _, task := range tasks {
		// Use task's jira-project if set, otherwise use default
		project := task.Frontmatter.JiraProject
		if project == "" {
			project = defaultProject
		}

		// Use task's jira-type if set, otherwise use default
		issueType := task.Frontmatter.JiraType
		if issueType == "" {
			issueType = defaultIssueType
		}

		// Validate and truncate fields before sending to Jira
		summary, description := s.validateTaskFields(task)

		// For Epics, don't set a parent (Epics are top-level)
		parent := task.Frontmatter.JiraParent
		if strings.EqualFold(issueType, "Epic") && parent == "" {
			// Explicitly empty parent is OK for Epics
			parent = ""
		}

		issue, err := s.jira.CreateIssue(ctx, ports.CreateIssueRequest{
			Project:     project,
			Summary:     summary,
			Description: description,
			IssueType:   issueType,
			Parent:      parent,
		})
		if err != nil {
			return fmt.Errorf("create ticket for %s: %w", task.Frontmatter.Title, err)
		}

		// Update task with Jira data
		task.Frontmatter.JiraNumber = issue.Key
		task.Frontmatter.JiraURL = s.jira.BaseURL() + "/browse/" + issue.Key
		task.Frontmatter.SyncStatus = domain.SyncStatusCreated
	}

	return nil
}

// LinkDependencies creates dependency links in Jira for all tasks.
// Uses jira-dependencies (not sync-dependencies) since those define Jira "blocks" links.
func (s *Service) LinkDependencies(ctx context.Context, tasks []*domain.TaskFile, linkType string) error {
	// Build task ID to Jira key map
	idMap := s.BuildTaskIDMap(tasks)

	// Create links for each task based on jira-dependencies
	for _, task := range tasks {
		// Extract task IDs from jira-dependencies (handles wiki link format)
		depIDs := task.GetJiraDependencyIDs()
		if len(depIDs) == 0 {
			task.Frontmatter.SyncStatus = domain.SyncStatusLinked
			continue
		}

		blockedIssue := task.Frontmatter.JiraNumber

		for _, depID := range depIDs {
			blockerIssue, ok := idMap[depID]
			if !ok {
				return fmt.Errorf("%w: %s not found for %s", domain.ErrDependencyNotFound, depID, task.Frontmatter.Title)
			}

			// Create link: blockedIssue is blocked by blockerIssue
			if err := s.jira.CreateLink(ctx, blockedIssue, blockerIssue, linkType); err != nil {
				return fmt.Errorf("link %s -> %s: %w", blockerIssue, blockedIssue, err)
			}
		}

		task.Frontmatter.SyncStatus = domain.SyncStatusLinked
	}

	return nil
}

// UpdateModified updates Jira tickets for tasks with content changes.
// Fields are validated and truncated if they exceed Jira limits.
func (s *Service) UpdateModified(ctx context.Context, tasks []*domain.TaskFile) error {
	for _, task := range tasks {
		// Validate and truncate fields before sending to Jira
		summary, description := s.validateTaskFields(task)

		err := s.jira.UpdateIssue(ctx, task.Frontmatter.JiraNumber, ports.UpdateIssueRequest{
			Summary:     summary,
			Description: description,
		})
		if err != nil {
			return fmt.Errorf("update ticket %s: %w", task.Frontmatter.JiraNumber, err)
		}

		// Update content hash
		if s.hasher != nil {
			task.Frontmatter.ContentHash = s.hasher.ComputeHash(task)
		}
	}

	return nil
}

// validateTaskFields validates and returns truncated summary and description.
// It logs warnings if fields are truncated and the service has a logger configured.
func (s *Service) validateTaskFields(task *domain.TaskFile) (summary, description string) {
	// Use validator to validate and truncate, which also logs warnings
	s.validator.ValidateTask(task)
	return task.Frontmatter.Title, task.Description
}

// BuildTaskIDMap creates a map from task ID (e.g., "KB-1") to Jira key (e.g., "GUARD-101").
func (s *Service) BuildTaskIDMap(tasks []*domain.TaskFile) map[string]string {
	idMap := make(map[string]string, len(tasks))
	for _, task := range tasks {
		taskID := extractTaskID(task.Frontmatter.Title)
		idMap[taskID] = task.Frontmatter.JiraNumber
	}
	return idMap
}

// extractTaskID extracts the task ID from the title (e.g., "KB-1" from "KB-1: Title").
func extractTaskID(title string) string {
	idx := strings.Index(title, ":")
	if idx == -1 {
		return title
	}
	id := strings.TrimSpace(title[:idx])
	if id == "" {
		return title
	}
	return id
}
