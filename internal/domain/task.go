// Package domain contains core domain types for jira-sync.
package domain

import (
	"regexp"
	"strings"
)

// wikiLinkRegex matches wiki-style links: [Title](filename.md)
var wikiLinkRegex = regexp.MustCompile(`^\[([^\]]+)\]\(([^)]+\.md)\)$`)

// TaskFile represents a markdown task file with frontmatter and description.
type TaskFile struct {
	Path        string
	Frontmatter Frontmatter
	Description string // Body content (becomes Jira description)
}

// Frontmatter contains the YAML frontmatter fields of a task file.
type Frontmatter struct {
	Title            string   `yaml:"title"`
	JiraNumber       string   `yaml:"jira-number"`
	JiraProject      string   `yaml:"jira-project"`
	JiraType         string   `yaml:"jira-type"`
	JiraState        string   `yaml:"jira-state"`
	CreatedDate      string   `yaml:"created-date"`
	JiraURL          string   `yaml:"jira-url"`
	SyncStatus       string   `yaml:"sync-status"`
	JiraParent       string   `yaml:"jira-parent"`
	SyncDependencies []string `yaml:"sync-dependencies"`
	JiraDependencies []string `yaml:"jira-dependencies"`
	ContentHash      string   `yaml:"content-hash"`
	LastSynced       string   `yaml:"last-synced"`
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

// SyncDependencyIDs extracts task IDs from sync-dependencies.
// Handles both wiki link format and legacy plain task IDs.
func (t *TaskFile) SyncDependencyIDs() []string {
	return extractDependencyIDs(t.Frontmatter.SyncDependencies)
}

// JiraDependencyIDs extracts task IDs from jira-dependencies.
// Handles both wiki link format and legacy plain task IDs.
func (t *TaskFile) JiraDependencyIDs() []string {
	return extractDependencyIDs(t.Frontmatter.JiraDependencies)
}

// extractDependencyIDs extracts task IDs from a dependency list.
// Handles both wiki link format "[KB-1: Title](file.md)" and legacy "KB-1".
func extractDependencyIDs(deps []string) []string {
	var ids []string
	for _, dep := range deps {
		taskID := parseWikiLinkTaskID(dep)
		if taskID != "" {
			ids = append(ids, taskID)
		}
	}
	return ids
}

// parseWikiLinkTaskID extracts the task ID from a wiki link or legacy format.
// Supports both formats:
//   - Wiki link: "[KB-1: Initialize Project](20260116-103001.md)" -> "KB-1"
//   - Legacy: "KB-1" -> "KB-1"
func parseWikiLinkTaskID(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}

	matches := wikiLinkRegex.FindStringSubmatch(link)
	if len(matches) == 3 {
		// Wiki link format: extract task ID from title (before colon)
		title := matches[1]
		if idx := strings.Index(title, ":"); idx != -1 {
			return strings.TrimSpace(title[:idx])
		}
		// No colon, use whole title as task ID
		return strings.TrimSpace(title)
	}

	// Legacy format: plain task ID
	return strings.TrimSpace(link)
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
	if t.Frontmatter.SyncDependencies == nil {
		t.Frontmatter.SyncDependencies = []string{}
		migrated = true
	}

	if t.Frontmatter.JiraDependencies == nil {
		t.Frontmatter.JiraDependencies = []string{}
		migrated = true
	}

	return migrated
}

// IsEpic returns true if this task's jira-type is "Epic".
func (t *TaskFile) IsEpic() bool {
	return strings.EqualFold(t.Frontmatter.JiraType, "Epic")
}
