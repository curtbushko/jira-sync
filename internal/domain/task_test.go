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

func TestTaskFile_GetSyncDependencyIDs(t *testing.T) {
	tests := []struct {
		name     string
		deps     []string
		expected []string
	}{
		{
			name:     "extracts IDs from wiki links",
			deps:     []string{"[KB-1: Initialize Project](20260116-103001.md)"},
			expected: []string{"KB-1"},
		},
		{
			name:     "extracts IDs from legacy format",
			deps:     []string{"KB-1", "ERR-2"},
			expected: []string{"KB-1", "ERR-2"},
		},
		{
			name:     "handles mixed formats",
			deps:     []string{"[KB-1: Init](file.md)", "ERR-2"},
			expected: []string{"KB-1", "ERR-2"},
		},
		{
			name:     "handles empty list",
			deps:     []string{},
			expected: nil,
		},
		{
			name:     "handles nil list",
			deps:     nil,
			expected: nil,
		},
		{
			name:     "handles whitespace in legacy format",
			deps:     []string{"  KB-1  ", "  ERR-2  "},
			expected: []string{"KB-1", "ERR-2"},
		},
		{
			name:     "filters empty strings",
			deps:     []string{"", "KB-1", ""},
			expected: []string{"KB-1"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			task := &TaskFile{
				Frontmatter: Frontmatter{
					SyncDependencies: testCase.deps,
				},
			}
			assert.Equal(t, testCase.expected, task.GetSyncDependencyIDs())
		})
	}
}

func TestTaskFile_GetJiraDependencyIDs(t *testing.T) {
	tests := []struct {
		name     string
		deps     []string
		expected []string
	}{
		{
			name:     "extracts IDs from wiki links",
			deps:     []string{"[KB-1: Initialize Project](20260116-103001.md)"},
			expected: []string{"KB-1"},
		},
		{
			name:     "extracts IDs from legacy format",
			deps:     []string{"KB-1", "ERR-2"},
			expected: []string{"KB-1", "ERR-2"},
		},
		{
			name:     "handles mixed formats",
			deps:     []string{"[KB-1: Init](file.md)", "ERR-2"},
			expected: []string{"KB-1", "ERR-2"},
		},
		{
			name:     "handles empty list",
			deps:     []string{},
			expected: nil,
		},
		{
			name:     "handles nil list",
			deps:     nil,
			expected: nil,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			task := &TaskFile{
				Frontmatter: Frontmatter{
					JiraDependencies: testCase.deps,
				},
			}
			assert.Equal(t, testCase.expected, task.GetJiraDependencyIDs())
		})
	}
}

func TestExtractDependencyIDs(t *testing.T) {
	tests := []struct {
		name     string
		deps     []string
		expected []string
	}{
		{
			name:     "wiki link with task ID and title",
			deps:     []string{"[KB-1: Title](file.md)"},
			expected: []string{"KB-1"},
		},
		{
			name:     "wiki link without colon in title",
			deps:     []string{"[TASK-1](file.md)"},
			expected: []string{"TASK-1"},
		},
		{
			name:     "multiple dependencies",
			deps:     []string{"[KB-1: A](a.md)", "[KB-2: B](b.md)", "KB-3"},
			expected: []string{"KB-1", "KB-2", "KB-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDependencyIDs(tt.deps)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseWikiLinkTaskID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "wiki link format",
			input:    "[KB-1: Initialize Project](20260116-103001.md)",
			expected: "KB-1",
		},
		{
			name:     "wiki link without colon",
			input:    "[TASK-1](file.md)",
			expected: "TASK-1",
		},
		{
			name:     "legacy format",
			input:    "KB-1",
			expected: "KB-1",
		},
		{
			name:     "legacy with whitespace",
			input:    "  KB-1  ",
			expected: "KB-1",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseWikiLinkTaskID(tt.input)
			assert.Equal(t, tt.expected, result)
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
	assert.NotNil(t, task.Frontmatter.SyncDependencies)
	assert.NotNil(t, task.Frontmatter.JiraDependencies)
	assert.Empty(t, task.Frontmatter.SyncDependencies)
	assert.Empty(t, task.Frontmatter.JiraDependencies)
}

func TestMigrateFrontmatter_AllFieldsPresent(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:            "KB-1: Test Task",
			JiraNumber:       "GUARD-123",
			JiraProject:      "GUARD",
			JiraType:         "Task",
			JiraState:        "In Progress",
			SyncStatus:       SyncStatusLinked,
			JiraParent:       "GUARD-100",
			SyncDependencies: []string{"KB-0"},
			JiraDependencies: []string{"KB-0"},
			ContentHash:      "abc123",
			LastSynced:       "2026-01-20T10:00:00Z",
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
	assert.NotNil(t, task.Frontmatter.SyncDependencies)
	assert.NotNil(t, task.Frontmatter.JiraDependencies)
}

func TestMigrateFrontmatter_PreservesExistingValues(t *testing.T) {
	existingDeps := []string{"[KB-0: Prereq](prereq.md)"}
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:            "KB-1: Test",
			JiraNumber:       "GUARD-999",
			JiraProject:      "PROJ",
			JiraType:         "Story",
			JiraState:        "In Review",
			SyncStatus:       SyncStatusCreated,
			JiraParent:       "GUARD-100",
			SyncDependencies: existingDeps,
			JiraDependencies: existingDeps,
			ContentHash:      "existinghash",
			LastSynced:       "2026-01-15T08:00:00Z",
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.False(t, migrated, "should not migrate when all fields present")
	assert.Equal(t, "GUARD-999", task.Frontmatter.JiraNumber)
	assert.Equal(t, "PROJ", task.Frontmatter.JiraProject)
	assert.Equal(t, "Story", task.Frontmatter.JiraType)
	assert.Equal(t, "In Review", task.Frontmatter.JiraState)
	assert.Equal(t, SyncStatusCreated, task.Frontmatter.SyncStatus)
	assert.Equal(t, existingDeps, task.Frontmatter.SyncDependencies)
	assert.Equal(t, existingDeps, task.Frontmatter.JiraDependencies)
	assert.Equal(t, "existinghash", task.Frontmatter.ContentHash)
	assert.Equal(t, "2026-01-15T08:00:00Z", task.Frontmatter.LastSynced)
}

func TestMigrateFrontmatter_InitializesEmptySlices(t *testing.T) {
	task := &TaskFile{
		Frontmatter: Frontmatter{
			Title:            "KB-1: Test",
			SyncDependencies: nil, // nil slice
			JiraDependencies: nil, // nil slice
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.True(t, migrated)
	// Should be non-nil empty slices after migration
	assert.NotNil(t, task.Frontmatter.SyncDependencies)
	assert.NotNil(t, task.Frontmatter.JiraDependencies)
	assert.Len(t, task.Frontmatter.SyncDependencies, 0)
	assert.Len(t, task.Frontmatter.JiraDependencies, 0)
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
			Title:            "KB-1: Test",
			JiraType:         "Epic",
			JiraState:        "Todo",
			SyncStatus:       SyncStatusPending,
			SyncDependencies: []string{},
			JiraDependencies: []string{},
		},
	}

	migrated := task.MigrateFrontmatter()

	assert.False(t, migrated)
	assert.Equal(t, "Epic", task.Frontmatter.JiraType)
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
