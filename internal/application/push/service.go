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
// Creates links based on:
//   - jira-blocks: issues this task blocks (we are the blocker)
//   - jira-is-blocked-by: issues that block this task (we are blocked)
func (s *Service) LinkDependencies(ctx context.Context, tasks []*domain.TaskFile, linkType string) error {
	// Build task ID to Jira key map
	idMap := s.BuildTaskIDMap(tasks)

	// Create links for each task
	for _, task := range tasks {
		thisIssue := task.Frontmatter.JiraNumber
		hasLinks := false

		// Process jira-blocks: this task blocks other issues
		// CreateLink(inward=blocked, outward=blocker) -> this task is blocker
		for _, blockedID := range task.Frontmatter.JiraBlocks {
			blockedIssue, err := s.resolveJiraKey(blockedID, idMap)
			if err != nil {
				return fmt.Errorf("%w: %s not found for %s", domain.ErrDependencyNotFound, blockedID, task.Frontmatter.Title)
			}

			// Create link: blockedIssue is blocked by thisIssue
			if err := s.jira.CreateLink(ctx, blockedIssue, thisIssue, linkType); err != nil {
				return fmt.Errorf("link %s blocks %s: %w", thisIssue, blockedIssue, err)
			}
			hasLinks = true
		}

		// Process jira-is-blocked-by: this task is blocked by other issues
		// CreateLink(inward=blocked, outward=blocker) -> this task is blocked
		for _, blockerID := range task.Frontmatter.JiraIsBlockedBy {
			blockerIssue, err := s.resolveJiraKey(blockerID, idMap)
			if err != nil {
				return fmt.Errorf("%w: %s not found for %s", domain.ErrDependencyNotFound, blockerID, task.Frontmatter.Title)
			}

			// Create link: thisIssue is blocked by blockerIssue
			if err := s.jira.CreateLink(ctx, thisIssue, blockerIssue, linkType); err != nil {
				return fmt.Errorf("link %s is blocked by %s: %w", thisIssue, blockerIssue, err)
			}
			hasLinks = true
		}

		// Mark as linked if we processed any links or there were none to process
		if hasLinks || (len(task.Frontmatter.JiraBlocks) == 0 && len(task.Frontmatter.JiraIsBlockedBy) == 0) {
			task.Frontmatter.SyncStatus = domain.SyncStatusLinked
		}
	}

	return nil
}

// resolveJiraKey resolves an ID to a Jira key.
// Handles multiple formats:
//   - Plain task ID: "KB-1"
//   - Wiki link format: "[KB-1: Title](filename.md)"
//   - Jira key: "GUARD-123"
func (s *Service) resolveJiraKey(depID string, idMap map[string]string) (string, error) {
	// Parse wiki link format if present: [Title](file.md)
	resolvedID := parseWikiLink(depID)

	// Check if it's in the local task map
	if jiraKey, ok := idMap[resolvedID]; ok {
		return jiraKey, nil
	}

	// Check if it looks like a Jira key already
	if isJiraKey(resolvedID) {
		return resolvedID, nil
	}

	return "", fmt.Errorf("cannot resolve %s to Jira key", depID)
}

// parseWikiLink extracts the task ID from a wiki link format.
// "[KB-1: Title](file.md)" -> "KB-1"
// Returns the original string if not a wiki link.
func parseWikiLink(wikiLink string) string {
	if !strings.HasPrefix(wikiLink, "[") {
		return wikiLink
	}

	// Find the end of the title part
	closeIdx := strings.Index(wikiLink, "]")
	if closeIdx == -1 {
		return wikiLink
	}

	title := wikiLink[1:closeIdx]
	// Extract task ID from title
	return extractTaskID(title)
}

// isJiraKey checks if a string looks like a Jira issue key (PROJECT-NUMBER).
func isJiraKey(issueKey string) bool {
	// Jira keys are typically uppercase letters followed by dash and numbers
	// e.g., GUARD-1519, PROJ-123
	idx := strings.Index(issueKey, "-")
	if idx <= 0 || idx >= len(issueKey)-1 {
		return false
	}
	project := issueKey[:idx]
	number := issueKey[idx+1:]

	// Project part should be all uppercase letters
	for _, c := range project {
		if c < 'A' || c > 'Z' {
			return false
		}
	}

	// Number part should be all digits
	for _, c := range number {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
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
