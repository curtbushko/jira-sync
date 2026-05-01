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
			JiraIsBlockedBy: []string{},
			ContentHash:      "",
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
			JiraIsBlockedBy: []string{"KB-3", "ERR-1"},
			ContentHash:      "abc123",
		},
		Description: "Implement detection.",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	assert.Contains(t, content, "jira-number: GUARD-102")
	assert.Contains(t, content, "jira-url: https://company.atlassian.net/browse/GUARD-102")
	assert.Contains(t, content, "jira-is-blocked-by:")
	assert.Contains(t, content, "KB-3")
	assert.Contains(t, content, "ERR-1")
	assert.Contains(t, content, "content-hash: abc123")
}

func TestWriter_Marshal_WithJiraIsBlockedBy(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:            "ERR-3: Deps Test",
			JiraProject:      "GUARD",
			JiraState:        "Todo",
			SyncStatus:       "pending",
			JiraParent:       "GUARD-100",
			JiraIsBlockedBy: []string{"KB-1", "KB-2"},
		},
		Description: "Test",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	assert.Contains(t, content, "jira-is-blocked-by: [KB-1, KB-2]")
}

func TestWriter_Marshal_EmptyDependencies(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:           "KB-1: Test",
			JiraProject:     "GUARD",
			JiraState:       "Todo",
			SyncStatus:      "pending",
			JiraParent:      "GUARD-100",
			JiraBlocks:      []string{},
			JiraIsBlockedBy: []string{},
		},
		Description: "Test",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	assert.Contains(t, content, "jira-blocks: []")
	assert.Contains(t, content, "jira-is-blocked-by: []")
}

func TestWriter_Marshal_NilDependencies(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:           "KB-1: Test",
			JiraProject:     "GUARD",
			JiraState:       "Todo",
			SyncStatus:      "pending",
			JiraParent:      "GUARD-100",
			JiraBlocks:      nil,
			JiraIsBlockedBy: nil,
		},
		Description: "Test",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	// Nil slices should be output as empty arrays
	assert.Contains(t, content, "jira-blocks: []")
	assert.Contains(t, content, "jira-is-blocked-by: []")
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

func TestWriter_Marshal_WithAssignee(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:        "KB-1: Assigned Task",
			JiraNumber:   "GUARD-101",
			JiraProject:  "GUARD",
			JiraState:    "In Progress",
			JiraAssignee: "john.doe",
			SyncStatus:   "linked",
			JiraParent:   "GUARD-100",
			ContentHash:  "abc123",
		},
		Description: "Task with assignee.",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	assert.Contains(t, content, "jira-assignee: john.doe")
}

func TestWriter_Marshal_WithoutAssignee(t *testing.T) {
	task := &domain.TaskFile{
		Path: "test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-2: Unassigned Task",
			JiraProject: "GUARD",
			JiraState:   "Todo",
			SyncStatus:  "pending",
			JiraParent:  "GUARD-100",
		},
		Description: "Task without assignee.",
	}

	writer := NewWriter()
	content, err := writer.Marshal(task)

	require.NoError(t, err)
	assert.Contains(t, content, "jira-assignee: \"\"")
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
	for idx, char := range content {
		if char == '\n' {
			lines = append(lines, content[start:idx])
			start = idx + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}
