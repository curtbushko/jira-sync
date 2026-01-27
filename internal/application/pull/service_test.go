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

func TestPullService_PullTask_UpdatesLocalFromJira(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with matching hash (no local changes)
	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Old Title",
			JiraNumber: "GUARD-123",
			JiraState:  "Todo",
			LastSynced: "2026-01-15T10:00:00Z",
		},
		Description: "Old description",
	}
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)

	// Mock GetIssue to return updated values from Jira
	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-123",
			Summary:     "KB-1: Updated Title",
			Description: "Updated description",
			Status:      "In Progress",
			Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), // After last sync
		}, nil
	}

	svc := NewService(mockJira, hasher)
	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)
	assert.Equal(t, ActionUpdated, result.Action)
	assert.Contains(t, result.Fields, "title")
	assert.Contains(t, result.Fields, "description")
	assert.Contains(t, result.Fields, "status")

	// Task should be updated with Jira values
	assert.Equal(t, "KB-1: Updated Title", task.Frontmatter.Title)
	assert.Equal(t, "Updated description", task.Description)
	assert.Equal(t, "In Progress", task.Frontmatter.JiraState)
}

func TestPullService_PullTask_SkipsNoChanges(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Same Title",
			JiraNumber: "GUARD-123",
			JiraState:  "Todo",
			LastSynced: "2026-01-15T10:00:00Z",
		},
		Description: "Same description",
	}
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)

	// Mock GetIssue to return same values (no changes in Jira)
	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-123",
			Summary:     "KB-1: Same Title",
			Description: "Same description",
			Status:      "Todo",
			Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), // Before last sync
		}, nil
	}

	svc := NewService(mockJira, hasher)
	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)
	assert.Equal(t, ActionSkipped, result.Action)
}

func TestPullService_PullTask_DetectsConflict(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with local changes (hash doesn't match)
	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Local Changes",
			JiraNumber:  "GUARD-123",
			ContentHash: "oldhash", // Different from current
			LastSynced:  "2026-01-15T10:00:00Z",
		},
		Description: "Local description",
	}

	// Mock GetIssue to return different version (Jira also changed)
	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-123",
			Summary:     "KB-1: Jira Changes",
			Description: "Jira description",
			Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), // After last sync
		}, nil
	}

	svc := NewService(mockJira, hasher)
	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)
	assert.Equal(t, ActionConflict, result.Action)
	// Should not update local on conflict
	assert.Equal(t, "KB-1: Local Changes", task.Frontmatter.Title)
}

func TestPullService_PullTask_ForcePull(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with local changes (would normally conflict)
	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Local Changes",
			JiraNumber:  "GUARD-123",
			ContentHash: "oldhash",
			LastSynced:  "2026-01-15T10:00:00Z",
		},
		Description: "Local description",
	}

	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-123",
			Summary:     "KB-1: Jira Version",
			Description: "Jira description",
			Status:      "Done",
			Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC),
		}, nil
	}

	svc := NewService(mockJira, hasher)
	result := svc.PullTask(context.Background(), task, WithForce(true))

	require.NoError(t, result.Error)
	assert.Equal(t, ActionUpdated, result.Action)
	// Should overwrite local with Jira values
	assert.Equal(t, "KB-1: Jira Version", task.Frontmatter.Title)
	assert.Equal(t, "Jira description", task.Description)
	assert.Equal(t, "Done", task.Frontmatter.JiraState)
}

func TestPullService_PullTask_SkipsNoJiraNumber(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Pending Task",
			JiraNumber: "", // No Jira number
		},
		Description: "Description",
	}

	svc := NewService(mockJira, hasher)
	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)
	assert.Equal(t, ActionSkipped, result.Action)
}

func TestPullService_PullAll(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	tasks := []*domain.TaskFile{
		{
			Path: "/tasks/task1.md",
			Frontmatter: domain.Frontmatter{
				Title:      "KB-1: Task 1",
				JiraNumber: "GUARD-101",
				JiraState:  "Todo",
				LastSynced: "2026-01-15T10:00:00Z",
			},
			Description: "Description 1",
		},
		{
			Path: "/tasks/task2.md",
			Frontmatter: domain.Frontmatter{
				Title:      "KB-2: Task 2",
				JiraNumber: "GUARD-102",
				JiraState:  "Todo",
				LastSynced: "2026-01-15T10:00:00Z",
			},
			Description: "Description 2",
		},
		{
			Path: "/tasks/task3.md",
			Frontmatter: domain.Frontmatter{
				Title:      "KB-3: Pending",
				JiraNumber: "", // No Jira number - should be skipped
			},
		},
	}
	// Set hashes to match (no local changes)
	tasks[0].Frontmatter.ContentHash = hasher.ComputeHash(tasks[0])
	tasks[1].Frontmatter.ContentHash = hasher.ComputeHash(tasks[1])

	mockJira.GetIssueFunc = func(_ context.Context, key string) (*ports.Issue, error) {
		if key == "GUARD-101" {
			return &ports.Issue{
				Key:         "GUARD-101",
				Summary:     "KB-1: Updated Task 1",
				Description: "Updated description",
				Status:      "In Progress",
				Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC),
			}, nil
		}
		return &ports.Issue{
			Key:         "GUARD-102",
			Summary:     "KB-2: Task 2",
			Description: "Description 2",
			Status:      "Todo",
			Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), // Before sync
		}, nil
	}

	svc := NewService(mockJira, hasher)
	results := svc.PullAll(context.Background(), tasks)

	require.Len(t, results, 3)
	assert.Equal(t, ActionUpdated, results[0].Action)
	assert.Equal(t, ActionSkipped, results[1].Action)
	assert.Equal(t, ActionSkipped, results[2].Action)

	// First task should be updated
	assert.Equal(t, "KB-1: Updated Task 1", tasks[0].Frontmatter.Title)
	assert.Equal(t, "In Progress", tasks[0].Frontmatter.JiraState)
}

func TestPullService_PullTask_PullsDependenciesFromJira(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with NO local dependencies
	task := &domain.TaskFile{
		Path: "/tasks/task1.md",
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Task 1",
			JiraNumber:       "GUARD-101",
			JiraDependencies: nil, // No local dependencies
			LastSynced:       "2026-01-15T10:00:00Z",
		},
		Description: "Description",
	}
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)

	// All tasks for mapping
	allTasks := []*domain.TaskFile{
		task,
		{
			Path: "/tasks/task2.md",
			Frontmatter: domain.Frontmatter{
				Title:      "KB-2: Task 2",
				JiraNumber: "GUARD-102",
			},
		},
		{
			Path: "/tasks/task3.md",
			Frontmatter: domain.Frontmatter{
				Title:      "KB-3: Task 3",
				JiraNumber: "GUARD-103",
			},
		},
	}

	// Mock GetIssue - no content changes
	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-101",
			Summary:     "KB-1: Task 1",
			Description: "Description",
			Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC),
		}, nil
	}

	// Mock GetIssueLinks - Jira HAS dependencies that local doesn't have
	// GUARD-102 and GUARD-103 block GUARD-101
	mockJira.GetIssueLinksFunc = func(_ context.Context, _ string) ([]ports.IssueLink, error) {
		return []ports.IssueLink{
			{
				ID:           "link-1",
				Type:         "Blocks",
				InwardIssue:  "GUARD-102", // GUARD-102 blocks GUARD-101
				OutwardIssue: "GUARD-101",
			},
			{
				ID:           "link-2",
				Type:         "Blocks",
				InwardIssue:  "GUARD-103", // GUARD-103 blocks GUARD-101
				OutwardIssue: "GUARD-101",
			},
		}, nil
	}

	svc := NewService(mockJira, hasher)
	svc.SetAllTasks(allTasks)

	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)

	// The task's jira-dependencies should be updated with Jira's dependencies
	// converted to local task IDs (KB-2, KB-3)
	assert.ElementsMatch(t, []string{"KB-2", "KB-3"}, task.Frontmatter.JiraDependencies,
		"Jira dependencies should be pulled to local task file")

	// DependencyResult should reflect the changes
	require.NotNil(t, result.DependencyResult)
	assert.ElementsMatch(t, []string{"KB-2", "KB-3"}, result.DependencyResult.JiraDeps)

	// IMPORTANT: Pull should NOT create/delete any links in Jira
	assert.Len(t, mockJira.CreateLinkCalls, 0, "Pull should not create links in Jira")
	assert.Len(t, mockJira.DeleteLinkCalls, 0, "Pull should not delete links in Jira")
}

func TestPullService_PullTask_DoesNotPushDependenciesToJira(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with a local dependency that Jira doesn't have
	task := &domain.TaskFile{
		Path: "/tasks/task1.md",
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Task 1",
			JiraNumber:       "GUARD-101",
			JiraDependencies: []string{"KB-2"}, // Local has this dependency
			LastSynced:       "2026-01-15T10:00:00Z",
		},
		Description: "Description",
	}
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)

	allTasks := []*domain.TaskFile{
		task,
		{
			Path: "/tasks/task2.md",
			Frontmatter: domain.Frontmatter{
				Title:      "KB-2: Task 2",
				JiraNumber: "GUARD-102",
			},
		},
	}

	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-101",
			Summary:     "KB-1: Task 1",
			Description: "Description",
			Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC),
		}, nil
	}

	// Jira has NO links - but local has KB-2
	mockJira.GetIssueLinksFunc = func(_ context.Context, _ string) ([]ports.IssueLink, error) {
		return []ports.IssueLink{}, nil
	}

	svc := NewService(mockJira, hasher)
	svc.SetAllTasks(allTasks)

	result := svc.PullTask(context.Background(), task)

	require.NoError(t, result.Error)

	// Pull operation should update local to match Jira (which has no deps)
	// Local should be cleared because Jira is authoritative
	assert.Nil(t, task.Frontmatter.JiraDependencies,
		"Local dependencies should be cleared to match Jira")

	// CRITICAL: Pull should NEVER create links in Jira
	assert.Len(t, mockJira.CreateLinkCalls, 0, "Pull must not create links in Jira")
	assert.Len(t, mockJira.DeleteLinkCalls, 0, "Pull must not delete links in Jira")
}
