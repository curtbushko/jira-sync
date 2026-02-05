package export

import (
	"context"
	"testing"
	"time"

	"github.com/curtbushko/jira-sync/internal/adapters/jira"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHashComputer implements ports.HashComputer for testing.
type mockHashComputer struct {
	hash string
}

func (m *mockHashComputer) ComputeHash(_ *domain.TaskFile) string {
	if m.hash != "" {
		return m.hash
	}
	return "mock-hash-12345"
}

func TestExport_BasicIssue(t *testing.T) {
	// Arrange
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:         "TEST-123",
		URL:         "https://jira.example.com/browse/TEST-123",
		Project:     "TEST",
		Summary:     "Test Issue Title",
		Description: "This is a test description",
		Status:      "To Do",
		IssueType:   "Story",
		Parent:      "TEST-100",
		Created:     "2026-01-15T14:30:45.000+0000",
	})

	hasher := &mockHashComputer{}
	svc := NewService(mockJira, hasher, nil)

	// Act
	result, err := svc.Export(context.Background(), "TEST-123", Options{})

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Task)

	assert.Equal(t, "TEST-123", result.Task.Frontmatter.JiraNumber)
	assert.Equal(t, "TEST", result.Task.Frontmatter.JiraProject)
	assert.Equal(t, "Test Issue Title", result.Task.Frontmatter.Title)
	assert.Equal(t, "This is a test description", result.Task.Description)
	assert.Equal(t, "To Do", result.Task.Frontmatter.JiraState)
	assert.Equal(t, "Story", result.Task.Frontmatter.JiraType)
	assert.Equal(t, "TEST-100", result.Task.Frontmatter.JiraParent)
	assert.Equal(t, "https://jira.example.com/browse/TEST-123", result.Task.Frontmatter.JiraURL)
	assert.Equal(t, domain.SyncStatusLinked, result.Task.Frontmatter.SyncStatus)
}

func TestExport_EpicIssueType(t *testing.T) {
	// Arrange
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:       "TEST-1",
		URL:       "https://jira.example.com/browse/TEST-1",
		Project:   "TEST",
		Summary:   "My Epic",
		IssueType: "Epic",
		Created:   "2026-01-15T14:30:45.000+0000",
	})

	svc := NewService(mockJira, &mockHashComputer{}, nil)

	// Act
	result, err := svc.Export(context.Background(), "TEST-1", Options{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "Epic", result.Task.Frontmatter.JiraType)
	assert.Empty(t, result.Task.Frontmatter.JiraParent) // Epics have no parent
}

func TestExport_FilenameFromCreationDate(t *testing.T) {
	tests := []struct {
		name             string
		created          string
		expectedFilename string
	}{
		{
			name:             "standard Jira format with positive offset",
			created:          "2026-01-15T14:30:45.000+0000",
			expectedFilename: "20260115-143045.md",
		},
		{
			name:             "Jira format with timezone offset",
			created:          "2026-03-20T09:15:30.000-0500",
			expectedFilename: "20260320-091530.md",
		},
		{
			name:             "Z suffix (UTC)",
			created:          "2026-06-01T00:00:00.000Z",
			expectedFilename: "20260601-000000.md",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			mockJira := jira.NewMockJiraClient()
			mockJira.AddStoredIssue(&ports.IssueWithLinks{
				Key:     "TEST-1",
				Project: "TEST",
				Summary: "Test",
				Created: testCase.created,
			})

			svc := NewService(mockJira, &mockHashComputer{}, nil)

			result, err := svc.Export(context.Background(), "TEST-1", Options{})

			require.NoError(t, err)
			assert.Equal(t, testCase.expectedFilename, result.Filename)
		})
	}
}

func TestExport_ExtractDependencies_BlocksLinks(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:     "TEST-3",
		Project: "TEST",
		Summary: "Test with Dependencies",
		Created: "2026-01-15T14:30:45.000+0000",
		Links: []ports.IssueLink{
			// InwardIssue = issues this task blocks
			{ID: "link-1", Type: "Blocking", InwardIssue: "TEST-1"},
			{ID: "link-2", Type: "Blocking", InwardIssue: "TEST-2"},
		},
	})

	svc := NewService(mockJira, &mockHashComputer{}, nil)

	result, err := svc.Export(context.Background(), "TEST-3", Options{})

	require.NoError(t, err)
	// InwardIssue goes to JiraBlocks (issues this task blocks)
	assert.Len(t, result.Task.Frontmatter.JiraBlocks, 2)
	assert.Contains(t, result.Task.Frontmatter.JiraBlocks, "TEST-1")
	assert.Contains(t, result.Task.Frontmatter.JiraBlocks, "TEST-2")
}

func TestExport_IgnoresOtherLinkTypes(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:     "TEST-3",
		Project: "TEST",
		Summary: "Test with Mixed Links",
		Created: "2026-01-15T14:30:45.000+0000",
		Links: []ports.IssueLink{
			{ID: "link-1", Type: "Blocking", InwardIssue: "TEST-1"},   // Goes to JiraBlocks
			{ID: "link-2", Type: "Relates", InwardIssue: "TEST-2"},  // Ignored (not Blocks type)
			{ID: "link-3", Type: "Clones", InwardIssue: "TEST-4"},   // Ignored (not Blocks type)
			{ID: "link-4", Type: "Blocking", OutwardIssue: "TEST-5"},  // Goes to JiraIsBlockedBy
		},
	})

	svc := NewService(mockJira, &mockHashComputer{}, nil)

	result, err := svc.Export(context.Background(), "TEST-3", Options{})

	require.NoError(t, err)
	// InwardIssue goes to JiraBlocks
	assert.Len(t, result.Task.Frontmatter.JiraBlocks, 1)
	assert.Contains(t, result.Task.Frontmatter.JiraBlocks, "TEST-1")
	// OutwardIssue goes to JiraIsBlockedBy
	assert.Len(t, result.Task.Frontmatter.JiraIsBlockedBy, 1)
	assert.Contains(t, result.Task.Frontmatter.JiraIsBlockedBy, "TEST-5")
}

func TestExport_MapToWikiLink_FoundLocally(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:     "TEST-2",
		Project: "TEST",
		Summary: "Test with Local Dependency",
		Created: "2026-01-15T14:30:45.000+0000",
		Links: []ports.IssueLink{
			// InwardIssue = issues this task blocks
			{ID: "link-1", Type: "Blocking", InwardIssue: "TEST-1"},
		},
	})

	// Existing local tasks that the dependency can be mapped to
	existingTasks := []*domain.TaskFile{
		{
			Path: "/tasks/20260114-100000.md",
			Frontmatter: domain.Frontmatter{
				Title:      "KB-1: Initialize Project",
				JiraNumber: "TEST-1",
			},
		},
	}

	svc := NewService(mockJira, &mockHashComputer{}, existingTasks)

	result, err := svc.Export(context.Background(), "TEST-2", Options{})

	require.NoError(t, err)
	// InwardIssue goes to JiraBlocks (issues this task blocks)
	assert.Len(t, result.Task.Frontmatter.JiraBlocks, 1)
	assert.Equal(t, "[KB-1: Initialize Project](20260114-100000.md)", result.Task.Frontmatter.JiraBlocks[0])
}

func TestExport_MapToWikiLink_NotFoundLocally(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:     "TEST-2",
		Project: "TEST",
		Summary: "Test with External Dependency",
		Created: "2026-01-15T14:30:45.000+0000",
		Links: []ports.IssueLink{
			// InwardIssue = issues this task blocks
			{ID: "link-1", Type: "Blocking", InwardIssue: "EXTERNAL-99"},
		},
	})

	// No existing local tasks
	svc := NewService(mockJira, &mockHashComputer{}, nil)

	result, err := svc.Export(context.Background(), "TEST-2", Options{})

	require.NoError(t, err)
	// InwardIssue goes to JiraBlocks (issues this task blocks)
	assert.Len(t, result.Task.Frontmatter.JiraBlocks, 1)
	// Should fall back to plain Jira key
	assert.Equal(t, "EXTERNAL-99", result.Task.Frontmatter.JiraBlocks[0])
}

func TestExport_HandleMissingParent(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:     "TEST-1",
		Project: "TEST",
		Summary: "Test without Parent",
		Created: "2026-01-15T14:30:45.000+0000",
		Parent:  "", // No parent
	})

	svc := NewService(mockJira, &mockHashComputer{}, nil)

	result, err := svc.Export(context.Background(), "TEST-1", Options{})

	require.NoError(t, err)
	assert.Empty(t, result.Task.Frontmatter.JiraParent)
}

func TestExport_ParentOverride(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:     "TEST-1",
		Project: "TEST",
		Summary: "Test with Parent",
		Created: "2026-01-15T14:30:45.000+0000",
		Parent:  "TEST-100", // Original parent
	})

	svc := NewService(mockJira, &mockHashComputer{}, nil)

	result, err := svc.Export(context.Background(), "TEST-1", Options{
		ParentOverride: "CUSTOM-200", // Override parent
	})

	require.NoError(t, err)
	assert.Equal(t, "CUSTOM-200", result.Task.Frontmatter.JiraParent)
}

func TestExport_ComputesContentHash(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:     "TEST-1",
		Project: "TEST",
		Summary: "Test",
		Created: "2026-01-15T14:30:45.000+0000",
	})

	hasher := &mockHashComputer{hash: "computed-hash-abc123"}
	svc := NewService(mockJira, hasher, nil)

	result, err := svc.Export(context.Background(), "TEST-1", Options{})

	require.NoError(t, err)
	assert.Equal(t, "computed-hash-abc123", result.Task.Frontmatter.ContentHash)
}

func TestExport_SetsSyncStatusLinked(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:     "TEST-1",
		Project: "TEST",
		Summary: "Test",
		Created: "2026-01-15T14:30:45.000+0000",
	})

	svc := NewService(mockJira, &mockHashComputer{}, nil)

	result, err := svc.Export(context.Background(), "TEST-1", Options{})

	require.NoError(t, err)
	assert.Equal(t, domain.SyncStatusLinked, result.Task.Frontmatter.SyncStatus)
}

func TestExport_SetsCreatedDate(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:     "TEST-1",
		Project: "TEST",
		Summary: "Test",
		Created: "2026-01-15T14:30:45.000+0000",
	})

	svc := NewService(mockJira, &mockHashComputer{}, nil)

	result, err := svc.Export(context.Background(), "TEST-1", Options{})

	require.NoError(t, err)
	assert.Equal(t, "2026-01-15", result.Task.Frontmatter.CreatedDate)
}

func TestExport_EmptyDescription(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:         "TEST-1",
		Project:     "TEST",
		Summary:     "Test without description",
		Description: "",
		Created:     "2026-01-15T14:30:45.000+0000",
	})

	svc := NewService(mockJira, &mockHashComputer{}, nil)

	result, err := svc.Export(context.Background(), "TEST-1", Options{})

	require.NoError(t, err)
	assert.Empty(t, result.Task.Description)
}

func TestExport_InitializesEmptySlices(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.AddStoredIssue(&ports.IssueWithLinks{
		Key:     "TEST-1",
		Project: "TEST",
		Summary: "Test",
		Created: "2026-01-15T14:30:45.000+0000",
		Links:   nil, // No links
	})

	svc := NewService(mockJira, &mockHashComputer{}, nil)

	result, err := svc.Export(context.Background(), "TEST-1", Options{})

	require.NoError(t, err)
	// Should have empty slices, not nil
	assert.NotNil(t, result.Task.Frontmatter.JiraIsBlockedBy)
	assert.NotNil(t, result.Task.Frontmatter.JiraIsBlockedBy)
	assert.Empty(t, result.Task.Frontmatter.JiraIsBlockedBy)
	assert.Empty(t, result.Task.Frontmatter.JiraIsBlockedBy)
}

func TestParseJiraDatetime(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantYear  int
		wantMonth time.Month
		wantDay   int
	}{
		{
			name:      "standard Jira format",
			input:     "2026-01-15T14:30:45.000+0000",
			wantErr:   false,
			wantYear:  2026,
			wantMonth: time.January,
			wantDay:   15,
		},
		{
			name:      "Jira format with negative offset",
			input:     "2026-03-20T09:15:30.000-0500",
			wantErr:   false,
			wantYear:  2026,
			wantMonth: time.March,
			wantDay:   20,
		},
		{
			name:      "RFC3339 format",
			input:     "2026-06-01T00:00:00Z",
			wantErr:   false,
			wantYear:  2026,
			wantMonth: time.June,
			wantDay:   1,
		},
		{
			name:    "invalid format",
			input:   "not-a-date",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := parseJiraDatetime(testCase.input)

			if testCase.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, testCase.wantYear, result.Year())
			assert.Equal(t, testCase.wantMonth, result.Month())
			assert.Equal(t, testCase.wantDay, result.Day())
		})
	}
}
