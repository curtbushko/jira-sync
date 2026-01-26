package filesystem

import (
	"testing"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriter_Marshal_ValidTask(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Test",
			JiraNumber:       "",
			JiraProject:      "GUARD",
			JiraState:        "Todo",
			CreatedDate:      "2026-01-16",
			JiraURL:          "",
			SyncStatus:       "pending",
			JiraParent:       "GUARD-100",
			SyncDependencies: []string{},
			JiraDependencies: []string{},
			ContentHash:      "",
			LastSynced:       "",
		},
		Description: "Task description",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	assert.Contains(t, content, "KB-1: Test") // YAML may use single or double quotes
	assert.Contains(t, content, "sync-status: pending")
	assert.Contains(t, content, "jira-parent: GUARD-100")
	assert.Contains(t, content, "jira-project: GUARD")
	assert.Contains(t, content, "jira-state: Todo")
	assert.Contains(t, content, "Task description")
	assert.True(t, hasProperFrontmatterDelimiters(content))
}

func TestWriter_Marshal_WithDependencies(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:            "ERR-2: Detection",
			JiraNumber:       "GUARD-102",
			JiraProject:      "GUARD",
			JiraState:        "In Progress",
			CreatedDate:      "2026-01-16",
			JiraURL:          "https://company.atlassian.net/browse/GUARD-102",
			SyncStatus:       "linked",
			JiraParent:       "GUARD-100",
			SyncDependencies: []string{"KB-3", "ERR-1"},
			JiraDependencies: []string{"KB-3", "ERR-1"},
			ContentHash:      "abc123",
			LastSynced:       "2026-01-16T10:00:00Z",
		},
		Description: "Implement detection.",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	assert.Contains(t, content, "jira-number: GUARD-102")
	assert.Contains(t, content, "jira-url: https://company.atlassian.net/browse/GUARD-102")
	// Dependencies are written in flow style [KB-3, ERR-1]
	assert.Contains(t, content, "sync-dependencies:")
	assert.Contains(t, content, "jira-dependencies:")
	assert.Contains(t, content, "KB-3")
	assert.Contains(t, content, "ERR-1")
	assert.Contains(t, content, "content-hash: abc123")
	assert.Contains(t, content, "last-synced:")
	assert.Contains(t, content, "2026-01-16T10:00:00Z")
}

func TestWriter_Marshal_SeparateDependencies(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:            "ERR-3: Separate Deps",
			JiraProject:      "GUARD",
			JiraState:        "Todo",
			SyncStatus:       "pending",
			JiraParent:       "GUARD-100",
			SyncDependencies: []string{"KB-1"},
			JiraDependencies: []string{"KB-1", "KB-2"},
		},
		Description: "Test",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	// Verify both dependency types are present
	assert.Contains(t, content, "sync-dependencies: [KB-1]")
	assert.Contains(t, content, "jira-dependencies: [KB-1, KB-2]")
}

func TestWriter_Marshal_EmptyDependencies(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Test",
			JiraProject:      "GUARD",
			JiraState:        "Todo",
			SyncStatus:       "pending",
			JiraParent:       "GUARD-100",
			SyncDependencies: []string{},
			JiraDependencies: []string{},
		},
		Description: "Test",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	assert.Contains(t, content, "sync-dependencies: []")
	assert.Contains(t, content, "jira-dependencies: []")
}

func TestWriter_Marshal_NilDependencies(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Test",
			JiraProject:      "GUARD",
			JiraState:        "Todo",
			SyncStatus:       "pending",
			JiraParent:       "GUARD-100",
			SyncDependencies: nil,
			JiraDependencies: nil,
		},
		Description: "Test",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	assert.Contains(t, content, "sync-dependencies: []")
	assert.Contains(t, content, "jira-dependencies: []")
}

func TestWriter_Marshal_MultilineDescription(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Test",
			JiraProject: "GUARD",
			JiraState:   "Todo",
			SyncStatus:  "pending",
			JiraParent:  "GUARD-100",
		},
		Description: `First paragraph.

Second paragraph.

## Acceptance Criteria

- Item 1
- Item 2`,
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	assert.Contains(t, content, "First paragraph.")
	assert.Contains(t, content, "## Acceptance Criteria")
	assert.Contains(t, content, "- Item 1")
}

func hasProperFrontmatterDelimiters(content string) bool {
	lines := splitLines(content)
	if len(lines) < 2 {
		return false
	}
	if lines[0] != "---" {
		return false
	}
	// Find closing delimiter
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			return true
		}
	}
	return false
}

func splitLines(content string) []string {
	var lines []string
	start := 0
	for idx := 0; idx < len(content); idx++ {
		if content[idx] == '\n' {
			lines = append(lines, content[start:idx])
			start = idx + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}
