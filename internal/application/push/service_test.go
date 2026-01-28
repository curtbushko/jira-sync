package push

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

func TestPushService_CategorizeTasks(t *testing.T) {
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

func TestPushService_CreateTickets(t *testing.T) {
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

func TestPushService_CreateTickets_UsesTaskProject(t *testing.T) {
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

func TestPushService_CreateTickets_JiraError(t *testing.T) {
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

func TestPushService_LinkDependencies(t *testing.T) {
	tests := []struct {
		name       string
		dependency string
	}{
		{
			name:       "plain_id",
			dependency: "KB-1",
		},
		{
			name:       "wiki_link_format",
			dependency: "[KB-1: First](20260116-100000.md)",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			mockJira := jira.NewMockJiraClient()

			tasks := []*domain.TaskFile{
				{
					Frontmatter: domain.Frontmatter{
						Title:            "KB-1: First",
						JiraNumber:       "GUARD-101",
						SyncStatus:       domain.SyncStatusCreated,
						JiraParent:       "GUARD-100",
						JiraIsBlockedBy: []string{},
					},
				},
				{
					Frontmatter: domain.Frontmatter{
						Title:            "KB-2: Second",
						JiraNumber:       "GUARD-102",
						SyncStatus:       domain.SyncStatusCreated,
						JiraParent:       "GUARD-100",
						JiraIsBlockedBy: []string{testCase.dependency},
					},
				},
			}

			svc := NewService(nil, mockJira, nil)
			err := svc.LinkDependencies(context.Background(), tasks, nil, "Blocking")

			require.NoError(t, err)

			// Verify link was created: GUARD-101 blocks GUARD-102
			assert.Len(t, mockJira.CreateLinkCalls, 1)
			assert.Equal(t, "GUARD-101", mockJira.CreateLinkCalls[0].Inward)
			assert.Equal(t, "GUARD-102", mockJira.CreateLinkCalls[0].Outward)
			assert.Equal(t, "Blocking", mockJira.CreateLinkCalls[0].LinkType)

			// Verify tasks updated to linked
			assert.Equal(t, domain.SyncStatusLinked, tasks[0].Frontmatter.SyncStatus)
			assert.Equal(t, domain.SyncStatusLinked, tasks[1].Frontmatter.SyncStatus)
		})
	}
}

func TestPushService_LinkDependencies_MultipleDeps(t *testing.T) {
	mockJira := jira.NewMockJiraClient()

	tasks := []*domain.TaskFile{
		{
			Frontmatter: domain.Frontmatter{
				Title:            "KB-1: First",
				JiraNumber:       "GUARD-101",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{},
			},
		},
		{
			Frontmatter: domain.Frontmatter{
				Title:            "ERR-1: Second",
				JiraNumber:       "GUARD-102",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{},
			},
		},
		{
			Frontmatter: domain.Frontmatter{
				Title:            "CTRL-1: Third",
				JiraNumber:       "GUARD-103",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{"KB-1", "ERR-1"},
			},
		},
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.LinkDependencies(context.Background(), tasks, nil, "Blocking")

	require.NoError(t, err)

	// Verify two links were created
	assert.Len(t, mockJira.CreateLinkCalls, 2)
}

func TestPushService_LinkDependencies_MissingDep(t *testing.T) {
	mockJira := jira.NewMockJiraClient()

	tasks := []*domain.TaskFile{
		{
			Frontmatter: domain.Frontmatter{
				Title:            "KB-2: Second",
				JiraNumber:       "GUARD-102",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{"invalid-dep"}, // lowercase, not a valid Jira key
			},
		},
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.LinkDependencies(context.Background(), tasks, nil, "Blocking")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid-dep")
}

func TestPushService_LinkDependencies_ExternalJiraKey(t *testing.T) {
	// Test that external Jira keys (not in local tasks) are used directly
	mockJira := jira.NewMockJiraClient()

	tasks := []*domain.TaskFile{
		{
			Frontmatter: domain.Frontmatter{
				Title:            "KB-1: First",
				JiraNumber:       "GUARD-101",
				SyncStatus:       domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{"GUARD-999"}, // External Jira key, not in local tasks
			},
		},
	}

	svc := NewService(nil, mockJira, nil)
	err := svc.LinkDependencies(context.Background(), tasks, nil, "Blocking")

	require.NoError(t, err)
	assert.Len(t, mockJira.CreateLinkCalls, 1)
	// Verify the link was created: GUARD-999 blocks GUARD-101
	assert.Equal(t, "GUARD-999", mockJira.CreateLinkCalls[0].Inward)
	assert.Equal(t, "GUARD-101", mockJira.CreateLinkCalls[0].Outward)
}

func TestPushService_UpdateModified(t *testing.T) {
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

func TestPushService_BuildTaskIDMap(t *testing.T) {
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

func TestPushService_ExtractTaskID(t *testing.T) {
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

func TestPushService_CreateTickets_TruncatesLongSummary(t *testing.T) {
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

func TestPushService_CreateTickets_TruncatesLongDescription(t *testing.T) {
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

func TestPushService_CreateTickets_WithLogger(t *testing.T) {
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

func TestPushService_UpdateModified_TruncatesFields(t *testing.T) {
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

func TestPushService_CreateTickets_NoTruncationNeeded(t *testing.T) {
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

func TestTransitionIssues_TransitionsToTargetState(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Mock GetIssue to return current state
	mockJira.GetIssueFunc = func(_ context.Context, key string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:    key,
			Status: "To Do", // Current state
		}, nil
	}

	// Mock GetTransitions to return available transitions
	mockJira.GetTransitionsFunc = func(_ context.Context, _ string) ([]ports.Transition, error) {
		return []ports.Transition{
			{ID: "11", Name: "To Do"},
			{ID: "21", Name: "In Progress"},
			{ID: "31", Name: "Done"},
		}, nil
	}

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-101",
			JiraState:  "In Progress", // Target state
		},
	}

	svc := NewService(nil, mockJira, hasher)
	transitioned, err := svc.TransitionIssues(context.Background(), []*domain.TaskFile{task})

	require.NoError(t, err)
	assert.Equal(t, 1, transitioned)
	assert.Len(t, mockJira.DoTransitionCalls, 1)
	assert.Equal(t, "GUARD-101", mockJira.DoTransitionCalls[0].Key)
	assert.Equal(t, "21", mockJira.DoTransitionCalls[0].TransitionID)
}

func TestTransitionIssues_SkipsWhenAlreadyInTargetState(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Mock GetIssue to return same state as target
	mockJira.GetIssueFunc = func(_ context.Context, key string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:    key,
			Status: "Done",
		}, nil
	}

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-101",
			JiraState:  "Done", // Already in this state
		},
	}

	svc := NewService(nil, mockJira, hasher)
	transitioned, err := svc.TransitionIssues(context.Background(), []*domain.TaskFile{task})

	require.NoError(t, err)
	assert.Equal(t, 0, transitioned)
	assert.Len(t, mockJira.DoTransitionCalls, 0)
}

func TestTransitionIssues_SkipsTasksWithoutJiraState(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-101",
			JiraState:  "", // No target state
		},
	}

	svc := NewService(nil, mockJira, hasher)
	transitioned, err := svc.TransitionIssues(context.Background(), []*domain.TaskFile{task})

	require.NoError(t, err)
	assert.Equal(t, 0, transitioned)
	assert.Len(t, mockJira.GetIssueCalls, 0) // Should not even fetch issue
}

func TestTransitionIssues_ReturnsErrorForUnavailableTransition(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	mockJira.GetIssueFunc = func(_ context.Context, key string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:    key,
			Status: "To Do",
		}, nil
	}

	// Only "In Progress" is available, not "Done"
	mockJira.GetTransitionsFunc = func(_ context.Context, _ string) ([]ports.Transition, error) {
		return []ports.Transition{
			{ID: "21", Name: "In Progress"},
		}, nil
	}

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-101",
			JiraState:  "Done", // Not available
		},
	}

	svc := NewService(nil, mockJira, hasher)
	_, err := svc.TransitionIssues(context.Background(), []*domain.TaskFile{task})

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrTransitionNotAvailable)
	assert.Contains(t, err.Error(), "Done")
	assert.Contains(t, err.Error(), "In Progress") // Available transition listed
}

func TestUpdateModified_IncludesParent(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-101",
			JiraParent: "GUARD-200", // New parent
		},
		Description: "Test description",
	}

	svc := NewService(nil, mockJira, hasher)
	err := svc.UpdateModified(context.Background(), []*domain.TaskFile{task})

	require.NoError(t, err)
	assert.Len(t, mockJira.UpdateIssueCalls, 1)
	assert.Equal(t, "GUARD-200", mockJira.UpdateIssueCalls[0].Req.Parent)
}
