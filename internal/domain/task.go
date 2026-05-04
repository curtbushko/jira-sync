// Package domain contains core domain types for jira-sync.
package domain

import (
	"strings"
)

// TaskFile represents a markdown task file with frontmatter and description.
type TaskFile struct {
	Path        string
	Frontmatter Frontmatter
	Description string // Body content (becomes Jira description)
}

// Frontmatter contains the YAML frontmatter fields of a task file.
type Frontmatter struct {
	Title              string   `yaml:"title"`
	JiraNumber         string   `yaml:"jira-number"`
	JiraProject        string   `yaml:"jira-project"`
	JiraType           string   `yaml:"jira-type"`
	JiraState          string   `yaml:"jira-state"`
	JiraAssignee       string   `yaml:"jira-assignee"`
	JiraResolutionDate string   `yaml:"jira-resolution-date"`
	CreatedDate        string   `yaml:"created-date"`
	JiraURL            string   `yaml:"jira-url"`
	SyncStatus         string   `yaml:"sync-status"`
	JiraParent         string   `yaml:"jira-parent"`
	JiraBlocks         []string `yaml:"jira-blocks"`        // Issues this task blocks (OutwardIssue)
	JiraIsBlockedBy    []string `yaml:"jira-is-blocked-by"` // Issues that block this task (InwardIssue)
	ContentHash        string   `yaml:"content-hash"`
}

// TaskID extracts the task ID prefix from the title (e.g., "KB-1" from "KB-1: Title").
func (t *TaskFile) TaskID() string {
	for i, c := range t.Frontmatter.Title {
		if c == ':' {
			return t.Frontmatter.Title[:i]
		}
	}
	return t.Frontmatter.Title
}

// MigrateFrontmatter adds missing fields with default values.
// Returns true if any fields were migrated, false if no changes were needed.
func (t *TaskFile) MigrateFrontmatter() bool {
	migrated := false

	// Set default JiraType if empty
	if t.Frontmatter.JiraType == "" {
		t.Frontmatter.JiraType = DefaultIssueType
		migrated = true
	}

	// Set default JiraState if empty
	if t.Frontmatter.JiraState == "" {
		t.Frontmatter.JiraState = DefaultJiraState
		migrated = true
	}

	// Set default SyncStatus if empty
	if t.Frontmatter.SyncStatus == "" {
		t.Frontmatter.SyncStatus = SyncStatusPending
		migrated = true
	}

	// Initialize nil slices to empty slices
	if t.Frontmatter.JiraBlocks == nil {
		t.Frontmatter.JiraBlocks = []string{}
		migrated = true
	}

	if t.Frontmatter.JiraIsBlockedBy == nil {
		t.Frontmatter.JiraIsBlockedBy = []string{}
		migrated = true
	}

	return migrated
}

// IsEpic returns true if this task's jira-type is "Epic".
func (t *TaskFile) IsEpic() bool {
	return strings.EqualFold(t.Frontmatter.JiraType, "Epic")
}
