package sync

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/curtbushko/jira-sync/internal/adapters/hashing"
	"github.com/curtbushko/jira-sync/internal/adapters/jira"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncService_CategorizeTasks(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	// Create tasks with different states
	pendingTask := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Test",
			SyncStatus:  domain.SyncStatusPending,
			JiraParent:  "GUARD-100",
			ContentHash: "",
		},
		Description: "Pending task",
	}

	createdTask := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:       "KB-2: Test",
			SyncStatus:  domain.SyncStatusCreated,
			JiraNumber:  "GUARD-101",
			JiraParent:  "GUARD-100",
			ContentHash: "",
		},
		Description: "Created task",
	}

	linkedTask := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:       "KB-3: Test",
			SyncStatus:  domain.SyncStatusLinked,
			JiraNumber:  "GUARD-102",
			JiraParent:  "GUARD-100",
			ContentHash: "", // Will be set to match
		},
		Description: "Linked task",
	}
	// Set hash to match so it appears up-to-date
	linkedTask.Frontmatter.ContentHash = hasher.ComputeHash(linkedTask)

	modifiedTask := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:       "KB-4: Test",
			SyncStatus:  domain.SyncStatusLinked,
			JiraNumber:  "GUARD-103",
			JiraParent:  "GUARD-100",
			ContentHash: "oldhash", // Different from actual
		},
		Description: "Modified task",
	}

	tasks := []*domain.TaskFile{pendingTask, createdTask, linkedTask, modifiedTask}

	svc := NewService(nil, nil, hasher)
	result := svc.CategorizeTasks(tasks)

	assert.Len(t, result.Pending, 1)
	assert.Equal(t, pendingTask, result.Pending[0])

	assert.Len(t, result.Created, 1)
	assert.Equal(t, createdTask, result.Created[0])

	assert.Len(t, result.Linked, 1)
	assert.Equal(t, linkedTask, result.Linked[0])

	assert.Len(t, result.NeedsUpdate, 1)
	assert.Equal(t, modifiedTask, result.NeedsUpdate[0])
}

func TestSyncService_CreateTickets(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")
	hasher := hashing.NewSHA256HashComputer()

	pendingTask := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Test Task",
			SyncStatus:  domain.SyncStatusPending,
			JiraParent:  "GUARD-100",
			JiraProject: "", // Will use default project
		},
		Description: "Test description",
	}

	svc := NewService(nil, mockJira, hasher)
	err := svc.CreateTickets(context.Background(), []*domain.TaskFile{pendingTask}, "GUARD", "Task")

	require.NoError(t, err)

	// Verify Jira was called
	assert.Len(t, mockJira.CreateIssueCalls, 1)
	assert.Equal(t, "KB-1: Test Task", mockJira.CreateIssueCalls[0].Summary)
	assert.Equal(t, "Test description", mockJira.CreateIssueCalls[0].Description)
	assert.Equal(t, "GUARD-100", mockJira.CreateIssueCalls[0].Parent)

	// Verify task was updated
	assert.Equal(t, "GUARD-1", pendingTask.Frontmatter.JiraNumber)
	assert.Equal(t, domain.SyncStatusCreated, pendingTask.Frontmatter.SyncStatus)
	assert.Contains(t, pendingTask.Frontmatter.JiraURL, "https://test.atlassian.net")
}

func TestSyncService_CreateTickets_UsesTaskProject(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")

	pendingTask := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Test Task",
			SyncStatus:  domain.SyncStatusPending,
			JiraParent:  "MYPROJ-100",
			JiraProject: "MYPROJ", // Task-specific project
		},
		Description: "Test description",
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.CreateTickets(context.Background(), []*domain.TaskFile{pendingTask}, "DEFAULT", "Task")

	require.NoError(t, err)

	// Verify task's project was used, not the default
	assert.Len(t, mockJira.CreateIssueCalls, 1)
	assert.Equal(t, "MYPROJ", mockJira.CreateIssueCalls[0].Project)
}

func TestSyncService_CreateTickets_JiraError(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.CreateIssueFunc = func(_ context.Context, _ ports.CreateIssueRequest) (*ports.Issue, error) {
		return nil, errors.New("jira connection failed")
	}

	pendingTask := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test",
			SyncStatus: domain.SyncStatusPending,
			JiraParent: "GUARD-100",
		},
		Description: "Test",
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.CreateTickets(context.Background(), []*domain.TaskFile{pendingTask}, "GUARD", "Task")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jira connection failed")
}

func TestSyncService_LinkDependencies(t *testing.T) {
	mockJira := jira.NewMockJiraClient()

	tasks := []*domain.TaskFile{
		{
			Frontmatter: domain.Frontmatter{
				Title:            "KB-1: First",
				JiraNumber:       "GUARD-101",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{},
			},
		},
		{
			Frontmatter: domain.Frontmatter{
				Title:            "KB-2: Second",
				JiraNumber:       "GUARD-102",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{"KB-1"},
			},
		},
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.LinkDependencies(context.Background(), tasks, "Blocks")

	require.NoError(t, err)

	// Verify link was created: GUARD-102 blocked by GUARD-101
	assert.Len(t, mockJira.CreateLinkCalls, 1)
	assert.Equal(t, "GUARD-102", mockJira.CreateLinkCalls[0].Inward)
	assert.Equal(t, "GUARD-101", mockJira.CreateLinkCalls[0].Outward)
	assert.Equal(t, "Blocks", mockJira.CreateLinkCalls[0].LinkType)

	// Verify tasks updated to linked
	assert.Equal(t, domain.SyncStatusLinked, tasks[0].Frontmatter.SyncStatus)
	assert.Equal(t, domain.SyncStatusLinked, tasks[1].Frontmatter.SyncStatus)
}

func TestSyncService_LinkDependencies_MultipleDeps(t *testing.T) {
	mockJira := jira.NewMockJiraClient()

	tasks := []*domain.TaskFile{
		{
			Frontmatter: domain.Frontmatter{
				Title:            "KB-1: First",
				JiraNumber:       "GUARD-101",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{},
			},
		},
		{
			Frontmatter: domain.Frontmatter{
				Title:            "ERR-1: Second",
				JiraNumber:       "GUARD-102",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{},
			},
		},
		{
			Frontmatter: domain.Frontmatter{
				Title:            "CTRL-1: Third",
				JiraNumber:       "GUARD-103",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{"KB-1", "ERR-1"},
			},
		},
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.LinkDependencies(context.Background(), tasks, "Blocks")

	require.NoError(t, err)

	// Verify two links were created
	assert.Len(t, mockJira.CreateLinkCalls, 2)
}

func TestSyncService_LinkDependencies_MissingDep(t *testing.T) {
	mockJira := jira.NewMockJiraClient()

	tasks := []*domain.TaskFile{
		{
			Frontmatter: domain.Frontmatter{
				Title:            "KB-2: Second",
				JiraNumber:       "GUARD-102",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{"KB-1"}, // KB-1 doesn't exist in tasks
			},
		},
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.LinkDependencies(context.Background(), tasks, "Blocks")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "KB-1")
}

func TestSyncService_LinkDependencies_WikiLinkFormat(t *testing.T) {
	mockJira := jira.NewMockJiraClient()

	tasks := []*domain.TaskFile{
		{
			Frontmatter: domain.Frontmatter{
				Title:            "KB-1: First",
				JiraNumber:       "GUARD-101",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{},
			},
		},
		{
			Frontmatter: domain.Frontmatter{
				Title:            "KB-2: Second",
				JiraNumber:       "GUARD-102",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{"[KB-1: First](20260116-100000.md)"},
			},
		},
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.LinkDependencies(context.Background(), tasks, "Blocks")

	require.NoError(t, err)

	// Verify link was created: GUARD-102 blocked by GUARD-101
	assert.Len(t, mockJira.CreateLinkCalls, 1)
	assert.Equal(t, "GUARD-102", mockJira.CreateLinkCalls[0].Inward)
	assert.Equal(t, "GUARD-101", mockJira.CreateLinkCalls[0].Outward)
	assert.Equal(t, "Blocks", mockJira.CreateLinkCalls[0].LinkType)

	// Verify tasks updated to linked
	assert.Equal(t, domain.SyncStatusLinked, tasks[0].Frontmatter.SyncStatus)
	assert.Equal(t, domain.SyncStatusLinked, tasks[1].Frontmatter.SyncStatus)
}

func TestSyncService_UpdateModified(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	modifiedTask := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Updated Title",
			JiraNumber:  "GUARD-101",
			SyncStatus:  domain.SyncStatusLinked,
			JiraParent:  "GUARD-100",
			ContentHash: "oldhash",
		},
		Description: "Updated description",
	}

	svc := NewService(nil, mockJira, hasher)
	err := svc.UpdateModified(context.Background(), []*domain.TaskFile{modifiedTask})

	require.NoError(t, err)

	// Verify Jira update was called
	assert.Len(t, mockJira.UpdateIssueCalls, 1)
	assert.Equal(t, "GUARD-101", mockJira.UpdateIssueCalls[0].Key)
	assert.Equal(t, "KB-1: Updated Title", mockJira.UpdateIssueCalls[0].Req.Summary)
	assert.Equal(t, "Updated description", mockJira.UpdateIssueCalls[0].Req.Description)

	// Verify hash was updated
	expectedHash := hasher.ComputeHash(modifiedTask)
	assert.Equal(t, expectedHash, modifiedTask.Frontmatter.ContentHash)
}

func TestSyncService_BuildTaskIDMap(t *testing.T) {
	tasks := []*domain.TaskFile{
		{
			Frontmatter: domain.Frontmatter{
				Title:      "KB-1: First Task",
				JiraNumber: "GUARD-101",
			},
		},
		{
			Frontmatter: domain.Frontmatter{
				Title:      "ERR-2: Second Task",
				JiraNumber: "GUARD-102",
			},
		},
		{
			Frontmatter: domain.Frontmatter{
				Title:      "CTRL-10: Third Task",
				JiraNumber: "GUARD-103",
			},
		},
	}

	svc := NewService(nil, nil, nil)
	idMap := svc.BuildTaskIDMap(tasks)

	assert.Equal(t, "GUARD-101", idMap["KB-1"])
	assert.Equal(t, "GUARD-102", idMap["ERR-2"])
	assert.Equal(t, "GUARD-103", idMap["CTRL-10"])
}

func TestSyncService_ExtractTaskID(t *testing.T) {
	tests := []struct {
		title    string
		expected string
	}{
		{"KB-1: Initialize Project", "KB-1"},
		{"ERR-10: Complex Detection", "ERR-10"},
		{"CTRL-1: Controller Scaffold", "CTRL-1"},
		{"MET-12: Logging Integration", "MET-12"},
		{"HELM-14: Documentation", "HELM-14"},
		{"Simple Task Without Prefix", "Simple Task Without Prefix"},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			result := extractTaskID(tt.title)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSyncService_CreateTickets_TruncatesLongSummary(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")

	// Create a title that exceeds the 255 character limit
	longTitle := "KB-1: " + strings.Repeat("x", 300)
	pendingTask := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      longTitle,
			SyncStatus: domain.SyncStatusPending,
			JiraParent: "GUARD-100",
		},
		Description: "Test description",
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.CreateTickets(context.Background(), []*domain.TaskFile{pendingTask}, "GUARD", "Task")

	require.NoError(t, err)

	// Verify the summary sent to Jira was truncated
	require.Len(t, mockJira.CreateIssueCalls, 1)
	sentSummary := mockJira.CreateIssueCalls[0].Summary
	assert.LessOrEqual(t, len(sentSummary), domain.JiraSummaryMaxLength)
	assert.True(t, strings.HasSuffix(sentSummary, "..."))

	// Verify task was also updated with truncated title
	assert.LessOrEqual(t, len(pendingTask.Frontmatter.Title), domain.JiraSummaryMaxLength)
}

func TestSyncService_CreateTickets_TruncatesLongDescription(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")

	// Create a description that exceeds the 32767 character limit
	longDescription := strings.Repeat("This is a long description. ", 2000)
	pendingTask := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			SyncStatus: domain.SyncStatusPending,
			JiraParent: "GUARD-100",
		},
		Description: longDescription,
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.CreateTickets(context.Background(), []*domain.TaskFile{pendingTask}, "GUARD", "Task")

	require.NoError(t, err)

	// Verify the description sent to Jira was truncated
	require.Len(t, mockJira.CreateIssueCalls, 1)
	sentDescription := mockJira.CreateIssueCalls[0].Description
	assert.LessOrEqual(t, len(sentDescription), domain.JiraDescriptionMaxLength)
	assert.True(t, strings.HasSuffix(sentDescription, "[Content truncated: exceeded Jira limit]"))
}

func TestSyncService_CreateTickets_WithLogger(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")

	var logBuf bytes.Buffer

	// Create a title that exceeds the limit
	longTitle := "KB-1: " + strings.Repeat("x", 300)
	pendingTask := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      longTitle,
			SyncStatus: domain.SyncStatusPending,
			JiraParent: "GUARD-100",
		},
		Description: "Test description",
	}

	svc := NewServiceWithLogger(nil, mockJira, nil, &logBuf)
	err := svc.CreateTickets(context.Background(), []*domain.TaskFile{pendingTask}, "GUARD", "Task")

	require.NoError(t, err)

	// Verify warning was logged
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "WARNING")
	assert.Contains(t, logOutput, "Summary truncated")
	assert.Contains(t, logOutput, "/tasks/test.md")
}

func TestSyncService_UpdateModified_TruncatesFields(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Create a task with long fields
	longTitle := "KB-1: " + strings.Repeat("y", 300)
	modifiedTask := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:       longTitle,
			JiraNumber:  "GUARD-101",
			SyncStatus:  domain.SyncStatusLinked,
			JiraParent:  "GUARD-100",
			ContentHash: "oldhash",
		},
		Description: "Updated description",
	}

	svc := NewService(nil, mockJira, hasher)
	err := svc.UpdateModified(context.Background(), []*domain.TaskFile{modifiedTask})

	require.NoError(t, err)

	// Verify the summary sent to Jira was truncated
	require.Len(t, mockJira.UpdateIssueCalls, 1)
	sentSummary := mockJira.UpdateIssueCalls[0].Req.Summary
	assert.LessOrEqual(t, len(sentSummary), domain.JiraSummaryMaxLength)
}

func TestSyncService_CreateTickets_NoTruncationNeeded(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")

	var logBuf bytes.Buffer

	// Create a task with normal-length fields
	pendingTask := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Normal Task",
			SyncStatus: domain.SyncStatusPending,
			JiraParent: "GUARD-100",
		},
		Description: "Normal description",
	}

	svc := NewServiceWithLogger(nil, mockJira, nil, &logBuf)
	err := svc.CreateTickets(context.Background(), []*domain.TaskFile{pendingTask}, "GUARD", "Task")

	require.NoError(t, err)

	// Verify no warning was logged
	logOutput := logBuf.String()
	assert.Empty(t, logOutput)

	// Verify fields were not modified
	assert.Equal(t, "KB-1: Normal Task", mockJira.CreateIssueCalls[0].Summary)
	assert.Equal(t, "Normal description", mockJira.CreateIssueCalls[0].Description)
}
