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
