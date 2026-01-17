// Package ports defines interfaces for external dependencies.
package ports

import (
	"context"

	"github.com/curtbushko/jira-sync/internal/domain"
)

// TaskRepository handles reading/writing task files (file system abstraction).
type TaskRepository interface {
	// ReadTask reads a single task file and parses frontmatter.
	ReadTask(path string) (*domain.TaskFile, error)

	// WriteTask writes a task file with frontmatter and description.
	WriteTask(task *domain.TaskFile) error

	// ListTasks returns all task files in a directory.
	ListTasks(dir string) ([]*domain.TaskFile, error)

	// GenerateFilename creates a zettelkasten filename (YYYYMMDD-HHMMSS.md).
	GenerateFilename() string
}

// Issue represents a Jira issue.
type Issue struct {
	Key         string
	Self        string // URL to the issue
	Summary     string
	Description string
	Status      string
}

// CreateIssueRequest contains the data needed to create a Jira issue.
type CreateIssueRequest struct {
	Project     string
	Summary     string
	Description string
	IssueType   string
	Parent      string
}

// UpdateIssueRequest contains the data needed to update a Jira issue.
type UpdateIssueRequest struct {
	Summary     string
	Description string
}

// JiraClient handles all Jira API operations.
type JiraClient interface {
	// CreateIssue creates a new issue and returns the created issue with key.
	CreateIssue(ctx context.Context, req CreateIssueRequest) (*Issue, error)

	// UpdateIssue updates an existing issue.
	UpdateIssue(ctx context.Context, key string, req UpdateIssueRequest) error

	// CreateLink creates a dependency link between two issues.
	// inward is the blocked issue, outward is the blocker.
	CreateLink(ctx context.Context, inward, outward, linkType string) error

	// GetIssue fetches an issue by key.
	GetIssue(ctx context.Context, key string) (*Issue, error)

	// BaseURL returns the Jira instance base URL.
	BaseURL() string
}

// HashComputer computes content hashes for change detection.
type HashComputer interface {
	// ComputeHash returns SHA256 hash of task content.
	ComputeHash(task *domain.TaskFile) string
}

// UserPrompter handles user interaction (confirmations, etc.).
type UserPrompter interface {
	// Confirm asks user yes/no question, returns true if yes.
	Confirm(message string) bool
}
