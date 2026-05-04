package pull

import (
	"context"
	"testing"
	"time"

	"github.com/curtbushko/jira-sync/internal/adapters/hashing"
	"github.com/curtbushko/jira-sync/internal/adapters/jira"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullTask_SyncsFromJira(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Old Title",
			JiraNumber: "GUARD-123",
			JiraState:  "Todo",
		},
		Description: "Old description",
	}

	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-123",
			Summary:     "KB-1: New Title",
			Description: "New description",
			Status:      "In Progress",
			Updated:     time.Now(),
		}, nil
	}

	svc := NewService(mockJira, hasher, "Blocking")
	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)
	assert.Equal(t, "KB-1: New Title", task.Frontmatter.Title)
	assert.Equal(t, "New description", task.Description)
	assert.Equal(t, "In Progress", task.Frontmatter.JiraState)
	assert.NotEmpty(t, task.Frontmatter.ContentHash)
}

func TestPullTask_SyncsResolutionDate(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Task",
			JiraNumber: "GUARD-123",
			JiraState:  "Todo",
		},
	}

	resolutionTime := time.Date(2026, 1, 20, 14, 30, 0, 0, time.FixedZone("MST", -7*3600))

	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:            "GUARD-123",
			Summary:        "KB-1: Resolved Task",
			Description:    "Resolved description",
			Status:         "Done",
			ResolutionDate: resolutionTime,
			Updated:        time.Now(),
		}, nil
	}

	svc := NewService(mockJira, hasher, "Blocking")
	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)
	assert.Equal(t, "Done", task.Frontmatter.JiraState)
	assert.Equal(t, "2026-01-20T14:30:00.000-0700", task.Frontmatter.JiraResolutionDate)
}

func TestPullTask_EmptyResolutionDateWhenUnresolved(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:              "KB-1: Task",
			JiraNumber:         "GUARD-123",
			JiraState:          "Todo",
			JiraResolutionDate: "2026-01-15T10:00:00.000-0700", // Old resolution date
		},
	}

	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:            "GUARD-123",
			Summary:        "KB-1: Open Task",
			Description:    "Open description",
			Status:         "In Progress",
			ResolutionDate: time.Time{}, // Zero time (not resolved)
			Updated:        time.Now(),
		}, nil
	}

	svc := NewService(mockJira, hasher, "Blocking")
	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)
	assert.Equal(t, "In Progress", task.Frontmatter.JiraState)
	assert.Equal(t, "", task.Frontmatter.JiraResolutionDate)
}

func TestPullTask_SkipsWithoutJiraNumber(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Task",
			JiraNumber: "", // No Jira number
		},
	}

	svc := NewService(mockJira, hasher, "Blocking")
	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)
	// Task should be unchanged
	assert.Equal(t, "KB-1: Task", task.Frontmatter.Title)
}

func TestPullTask_SyncsDependencies(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/task1.md",
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Task 1",
			JiraNumber:       "GUARD-101",
			JiraIsBlockedBy: nil,
		},
	}

	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-101",
			Summary:     "KB-1: Task 1",
			Description: "Description",
			Updated:     time.Now(),
		}, nil
	}

	mockJira.GetIssueLinksFunc = func(_ context.Context, _ string) ([]ports.IssueLink, error) {
		return []ports.IssueLink{
			// InwardIssue = issues that block this task (goes to JiraIsBlockedBy)
			{Type: "Blocking", InwardIssue: "GUARD-102", OutwardIssue: ""},
			{Type: "Blocking", InwardIssue: "GUARD-103", OutwardIssue: ""},
		}, nil
	}

	svc := NewService(mockJira, hasher, "Blocking")

	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)
	assert.ElementsMatch(t, []string{"GUARD-102", "GUARD-103"}, task.Frontmatter.JiraIsBlockedBy)
	assert.ElementsMatch(t, []string{"GUARD-102", "GUARD-103"}, result.JiraIsBlockedBy)
	assert.True(t, result.UpdatedLinks)
}

func TestPullTask_HandlesError(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Task",
			JiraNumber: "GUARD-123",
		},
	}

	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return nil, assert.AnError
	}

	svc := NewService(mockJira, hasher, "Blocking")
	result := svc.PullTask(context.Background(), task)

	assert.Error(t, result.Error)
}

func TestPullAll_PullsMultipleTasks(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	tasks := []*domain.TaskFile{
		{
			Path: "/tasks/task1.md",
			Frontmatter: domain.Frontmatter{
				Title:      "KB-1: Task 1",
				JiraNumber: "GUARD-101",
			},
		},
		{
			Path: "/tasks/task2.md",
			Frontmatter: domain.Frontmatter{
				Title:      "KB-2: Task 2",
				JiraNumber: "GUARD-102",
			},
		},
	}

	mockJira.GetIssueFunc = func(_ context.Context, key string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         key,
			Summary:     "Updated " + key,
			Description: "Description for " + key,
			Status:      "Done",
			Updated:     time.Now(),
		}, nil
	}

	svc := NewService(mockJira, hasher, "Blocking")
	results := svc.PullAll(context.Background(), tasks)

	require.Len(t, results, 2)
	for _, result := range results {
		require.NoError(t, result.Error)
	}
}
