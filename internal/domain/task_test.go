package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskFile_TaskID(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		expected string
	}{
		{
			name:     "extracts task ID before colon",
			title:    "KB-1: Initialize Project",
			expected: "KB-1",
		},
		{
			name:     "handles complex title with hyphens",
			title:    "ERR-2: Handle Pod Failures - Container State",
			expected: "ERR-2",
		},
		{
			name:     "returns full title when no colon",
			title:    "KB-1",
			expected: "KB-1",
		},
		{
			name:     "handles empty title",
			title:    "",
			expected: "",
		},
		{
			name:     "handles title with only colon",
			title:    ":",
			expected: "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			task := &TaskFile{
				Frontmatter: Frontmatter{
					Title: testCase.title,
				},
			}
			assert.Equal(t, testCase.expected, task.TaskID())
		})
	}
}

// Phase 10: Frontmatter Migration Tests

func TestMigrateFrontmatter_MissingFields(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:      "KB-1: Test Task",
			JiraParent: "GUARD-100",
			// All other fields missing
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.True(t, migrated, "should return true when fields were added")
	assert.Equal(t, DefaultJiraState, task.Frontmatter.JiraState)
	assert.Equal(t, SyncStatusPending, task.Frontmatter.SyncStatus)
	assert.NotNil(t, task.Frontmatter.JiraBlocks)
	assert.Empty(t, task.Frontmatter.JiraBlocks)
	assert.NotNil(t, task.Frontmatter.JiraIsBlockedBy)
	assert.Empty(t, task.Frontmatter.JiraIsBlockedBy)
}

func TestMigrateFrontmatter_AllFieldsPresent(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:           "KB-1: Test Task",
			JiraNumber:      "GUARD-123",
			JiraProject:     "GUARD",
			JiraType:        "Task",
			JiraState:       "In Progress",
			SyncStatus:      SyncStatusLinked,
			JiraParent:      "GUARD-100",
			JiraBlocks:      []string{"GUARD-101"},
			JiraIsBlockedBy: []string{"GUARD-200"},
			ContentHash:     "abc123",
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.False(t, migrated, "should return false when no migration needed")
	assert.Equal(t, "In Progress", task.Frontmatter.JiraState) // unchanged
	assert.Equal(t, SyncStatusLinked, task.Frontmatter.SyncStatus) // unchanged
	assert.Equal(t, "Task", task.Frontmatter.JiraType) // unchanged
}

func TestMigrateFrontmatter_PartialFields(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:      "KB-1: Test",
			JiraState:  "Done", // already set
			SyncStatus: "",     // missing
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.True(t, migrated)
	assert.Equal(t, "Done", task.Frontmatter.JiraState)       // preserved
	assert.Equal(t, SyncStatusPending, task.Frontmatter.SyncStatus) // set default
	assert.NotNil(t, task.Frontmatter.JiraBlocks)
	assert.NotNil(t, task.Frontmatter.JiraIsBlockedBy)
}

func TestMigrateFrontmatter_PreservesExistingValues(t *testing.T) {
	existingBlocks := []string{"GUARD-101"}
	existingBlockedBy := []string{"GUARD-200"}
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:           "KB-1: Test",
			JiraNumber:      "GUARD-999",
			JiraProject:     "PROJ",
			JiraType:        "Story",
			JiraState:       "In Review",
			SyncStatus:      SyncStatusCreated,
			JiraParent:      "GUARD-100",
			JiraBlocks:      existingBlocks,
			JiraIsBlockedBy: existingBlockedBy,
			ContentHash:     "existinghash",
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.False(t, migrated, "should not migrate when all fields present")
	assert.Equal(t, "GUARD-999", task.Frontmatter.JiraNumber)
	assert.Equal(t, "PROJ", task.Frontmatter.JiraProject)
	assert.Equal(t, "Story", task.Frontmatter.JiraType)
	assert.Equal(t, "In Review", task.Frontmatter.JiraState)
	assert.Equal(t, SyncStatusCreated, task.Frontmatter.SyncStatus)
	assert.Equal(t, existingBlocks, task.Frontmatter.JiraBlocks)
	assert.Equal(t, existingBlockedBy, task.Frontmatter.JiraIsBlockedBy)
	assert.Equal(t, "existinghash", task.Frontmatter.ContentHash)
}

func TestMigrateFrontmatter_InitializesEmptySlices(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:           "KB-1: Test",
			JiraBlocks:      nil, // nil slice
			JiraIsBlockedBy: nil, // nil slice
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.True(t, migrated)
	// Should be non-nil empty slices after migration
	assert.NotNil(t, task.Frontmatter.JiraBlocks)
	assert.Len(t, task.Frontmatter.JiraBlocks, 0)
	assert.NotNil(t, task.Frontmatter.JiraIsBlockedBy)
	assert.Len(t, task.Frontmatter.JiraIsBlockedBy, 0)
}

func TestMigrateFrontmatter_SetsDefaultJiraType(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:    "KB-1: Test",
			JiraType: "", // missing
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.True(t, migrated)
	assert.Equal(t, DefaultIssueType, task.Frontmatter.JiraType)
}

func TestMigrateFrontmatter_PreservesExistingJiraType(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:           "KB-1: Test",
			JiraType:        "Epic",
			JiraState:       "Todo",
			SyncStatus:      SyncStatusPending,
			JiraBlocks:      []string{},
			JiraIsBlockedBy: []string{},
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.False(t, migrated)
	assert.Equal(t, "Epic", task.Frontmatter.JiraType)
}

// Tests for jira-blocks and jira-is-blocked-by fields

func TestTaskFile_JiraBlocks(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			JiraBlocks: []string{"GUARD-101", "GUARD-102"},
		},
	}

	assert.Equal(t, []string{"GUARD-101", "GUARD-102"}, task.Frontmatter.JiraBlocks)
}

func TestTaskFile_JiraIsBlockedBy(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			JiraIsBlockedBy: []string{"GUARD-200", "GUARD-201"},
		},
	}

	assert.Equal(t, []string{"GUARD-200", "GUARD-201"}, task.Frontmatter.JiraIsBlockedBy)
}

func TestMigrateFrontmatter_InitializesBlockingFields(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:           "KB-1: Test",
			JiraBlocks:      nil,
			JiraIsBlockedBy: nil,
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.True(t, migrated)
	assert.NotNil(t, task.Frontmatter.JiraBlocks)
	assert.NotNil(t, task.Frontmatter.JiraIsBlockedBy)
	assert.Empty(t, task.Frontmatter.JiraBlocks)
	assert.Empty(t, task.Frontmatter.JiraIsBlockedBy)
}

func TestMigrateFrontmatter_PreservesExistingBlockingFields(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:           "KB-1: Test",
			JiraType:        "Task",
			JiraState:       "Todo",
			SyncStatus:      SyncStatusPending,
			JiraBlocks:      []string{"GUARD-101"},
			JiraIsBlockedBy: []string{"GUARD-200"},
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.False(t, migrated)
	assert.Equal(t, []string{"GUARD-101"}, task.Frontmatter.JiraBlocks)
	assert.Equal(t, []string{"GUARD-200"}, task.Frontmatter.JiraIsBlockedBy)
}

func TestIsEpic_True(t *testing.T) {
	tests := []struct {
		name     string
		jiraType string
		expected bool
	}{
		{"lowercase epic", "epic", true},
		{"uppercase Epic", "Epic", true},
		{"uppercase EPIC", "EPIC", true},
		{"mixed case", "EpIc", true},
		{"task", "Task", false},
		{"story", "Story", false},
		{"bug", "Bug", false},
		{"empty", "", false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			task := &TaskFile{
				Frontmatter: Frontmatter{
					JiraType: testCase.jiraType,
				},
			}
			assert.Equal(t, testCase.expected, task.IsEpic())
		})
	}
}
