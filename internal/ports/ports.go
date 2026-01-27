// Package ports defines interfaces for external dependencies.
package ports

import (
	"context"
	"time"

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
	Updated     time.Time // Last updated timestamp from Jira
}

// IssueWithLinks represents a Jira issue with expanded link information.
// Used by the export command to fetch all issue details in one request.
type IssueWithLinks struct {
	Key         string
	URL         string // Full URL to the issue (e.g., https://jira.example.com/browse/PROJ-123)
	Project     string // Project key (e.g., "PROJ")
	Summary     string
	Description string
	Status      string
	IssueType   string      // Issue type (e.g., "Story", "Task", "Epic", "Bug")
	Parent      string      // Parent issue key (e.g., "PROJ-100"), empty if no parent
	Created     string      // Issue creation datetime in Jira format
	StartDate   string      // Start date field (may be empty)
	EndDate     string      // End date field (may be empty)
	Links       []IssueLink // Issue links
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

// Transition represents a Jira workflow transition.
type Transition struct {
	ID   string // Transition ID used by Jira API
	Name string // Human-readable transition name (e.g., "In Progress", "Done")
}

// IssueLink represents a link between two Jira issues.
type IssueLink struct {
	ID           string // Link ID (used for deletion)
	Type         string // Link type name (e.g., "Blocks")
	InwardIssue  string // Inward issue key (e.g., blocked issue)
	OutwardIssue string // Outward issue key (e.g., blocker)
}

// IssueReader handles reading Jira issues.
type IssueReader interface {
	// GetIssue fetches an issue by key.
	GetIssue(ctx context.Context, key string) (*Issue, error)

	// GetIssueWithLinks fetches an issue with expanded links for export.
	// Returns all fields needed to create a local task file.
	GetIssueWithLinks(ctx context.Context, key string) (*IssueWithLinks, error)
}

// IssueWriter handles creating and updating Jira issues.
type IssueWriter interface {
	// CreateIssue creates a new issue and returns the created issue with key.
	CreateIssue(ctx context.Context, req CreateIssueRequest) (*Issue, error)

	// UpdateIssue updates an existing issue.
	UpdateIssue(ctx context.Context, key string, req UpdateIssueRequest) error
}

// LinkManager handles Jira issue link operations.
type LinkManager interface {
	// CreateLink creates a dependency link between two issues.
	// inward is the blocked issue, outward is the blocker.
	CreateLink(ctx context.Context, inward, outward, linkType string) error

	// GetIssueLinks returns all links for an issue.
	GetIssueLinks(ctx context.Context, key string) ([]IssueLink, error)

	// DeleteLink removes an issue link by ID.
	DeleteLink(ctx context.Context, linkID string) error
}

// TransitionManager handles Jira workflow transitions.
type TransitionManager interface {
	// GetTransitions returns available transitions for an issue.
	GetTransitions(ctx context.Context, key string) ([]Transition, error)

	// DoTransition performs a workflow transition on an issue.
	DoTransition(ctx context.Context, key, transitionID string) error
}

// JiraClient handles all Jira API operations.
// It composes smaller interfaces for better interface segregation.
type JiraClient interface {
	IssueReader
	IssueWriter
	LinkManager
	TransitionManager

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
