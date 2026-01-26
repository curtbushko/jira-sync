package fullsync

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

func TestFullSyncService_SyncTask_LocalToJira(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with local changes (hash doesn't match)
	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Updated Title",
			JiraNumber:  "GUARD-123",
			JiraParent:  "GUARD-100",
			ContentHash: "oldhash",
			LastSynced:  "2026-01-15T10:00:00Z",
		},
		Description: "Updated description",
	}

	// Mock GetIssue to return old version
	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-123",
			Summary:     "KB-1: Old Title",
			Description: "Old description",
			Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), // Before last sync
		}, nil
	}

	svc := NewService(mockJira, hasher)
	result, err := svc.SyncTask(context.Background(), task)

	require.NoError(t, err)
	assert.Equal(t, ChangeTypeLocalToJira, result.Type)
	assert.Len(t, mockJira.UpdateIssueCalls, 1)
	assert.Equal(t, "GUARD-123", mockJira.UpdateIssueCalls[0].Key)
}

func TestFullSyncService_SyncTask_JiraToLocal(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Create task with matching hash (no local changes)
	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Test",
			JiraNumber:  "GUARD-123",
			JiraParent:  "GUARD-100",
			LastSynced:  "2026-01-15T10:00:00Z",
		},
		Description: "Original description",
	}
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)

	// Mock GetIssue to return updated version
	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-123",
			Summary:     "KB-1: Updated in Jira",
			Description: "Updated in Jira description",
			Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), // After last sync
		}, nil
	}

	svc := NewService(mockJira, hasher)
	result, err := svc.SyncTask(context.Background(), task)

	require.NoError(t, err)
	assert.Equal(t, ChangeTypeJiraToLocal, result.Type)
	// Task should be updated with Jira values
	assert.Equal(t, "KB-1: Updated in Jira", task.Frontmatter.Title)
	assert.Equal(t, "Updated in Jira description", task.Description)
}

func TestFullSyncService_SyncTask_Conflict(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with local changes
	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Local Changes",
			JiraNumber:  "GUARD-123",
			JiraParent:  "GUARD-100",
			ContentHash: "oldhash", // Different from current
			LastSynced:  "2026-01-15T10:00:00Z",
		},
		Description: "Local description",
	}

	// Mock GetIssue to return different version (both changed)
	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-123",
			Summary:     "KB-1: Jira Changes",
			Description: "Jira description",
			Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), // After last sync
		}, nil
	}

	svc := NewService(mockJira, hasher)
	result, err := svc.SyncTask(context.Background(), task)

	require.NoError(t, err)
	assert.Equal(t, ChangeTypeConflict, result.Type)
	// Should not update either side on conflict
	assert.Len(t, mockJira.UpdateIssueCalls, 0)
}

func TestFullSyncService_SyncTask_NoChanges(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with no changes
	task := &domain.TaskFile{
		Path: "/tasks/test.md",
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Same",
			JiraNumber:  "GUARD-123",
			JiraParent:  "GUARD-100",
			LastSynced:  "2026-01-15T10:00:00Z",
		},
		Description: "Same description",
	}
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)

	// Mock GetIssue to return same version
	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-123",
			Summary:     "KB-1: Same",
			Description: "Same description",
			Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), // Before last sync
		}, nil
	}

	svc := NewService(mockJira, hasher)
	result, err := svc.SyncTask(context.Background(), task)

	require.NoError(t, err)
	assert.Equal(t, ChangeTypeNone, result.Type)
	assert.Len(t, mockJira.UpdateIssueCalls, 0)
}

func TestFullSyncService_SyncAllTasks(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	tasks := []*domain.TaskFile{
		{
			Path: "/tasks/task1.md",
			Frontmatter: domain.Frontmatter{
				Title:       "KB-1: Task 1",
				JiraNumber:  "GUARD-101",
				JiraParent:  "GUARD-100",
				ContentHash: "oldhash",
				LastSynced:  "2026-01-15T10:00:00Z",
			},
			Description: "Updated locally",
		},
		{
			Path: "/tasks/task2.md",
			Frontmatter: domain.Frontmatter{
				Title:       "KB-2: Task 2",
				JiraNumber:  "GUARD-102",
				JiraParent:  "GUARD-100",
				LastSynced:  "2026-01-15T10:00:00Z",
			},
			Description: "Same",
		},
	}
	// Set hash for task2 to match (no local changes)
	tasks[1].Frontmatter.ContentHash = hasher.ComputeHash(tasks[1])

	// Mock GetIssue
	mockJira.GetIssueFunc = func(_ context.Context, key string) (*ports.Issue, error) {
		if key == "GUARD-101" {
			return &ports.Issue{
				Key:         "GUARD-101",
				Summary:     "KB-1: Task 1",
				Description: "Old description",
				Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC),
			}, nil
		}
		return &ports.Issue{
			Key:         "GUARD-102",
			Summary:     "KB-2: Task 2",
			Description: "Same",
			Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC),
		}, nil
	}

	svc := NewService(mockJira, hasher)
	results, err := svc.SyncAllTasks(context.Background(), tasks)

	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, ChangeTypeLocalToJira, results[0].Type)
	assert.Equal(t, ChangeTypeNone, results[1].Type)
}

// Tests for jira-dependencies syncing

func TestFullSyncService_SyncDependencies_AddLink(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with a new dependency on KB-2
	task := &domain.TaskFile{
		Path: "/tasks/task1.md",
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Task 1",
			JiraNumber:       "GUARD-101",
			JiraParent:       "GUARD-100",
			JiraDependencies: []string{"KB-2"}, // New dependency
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
	}

	// Mock GetIssue to return no changes in content
	mockJira.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:         "GUARD-101",
			Summary:     "KB-1: Task 1",
			Description: "Description",
			Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC),
		}, nil
	}

	// Mock GetIssueLinks to return empty (no existing links)
	mockJira.GetIssueLinksFunc = func(_ context.Context, _ string) ([]ports.IssueLink, error) {
		return []ports.IssueLink{}, nil
	}

	svc := NewService(mockJira, hasher)
	svc.SetAllTasks(allTasks)

	result, err := svc.SyncTask(context.Background(), task)

	require.NoError(t, err)
	assert.NotNil(t, result.DependencyResult)
	assert.True(t, result.DependencyResult.HasChanges)
	assert.Equal(t, []string{"GUARD-102"}, result.DependencyResult.ToAdd)

	// Should have created a link
	assert.Len(t, mockJira.CreateLinkCalls, 1)
	assert.Equal(t, "GUARD-101", mockJira.CreateLinkCalls[0].Inward)
	assert.Equal(t, "GUARD-102", mockJira.CreateLinkCalls[0].Outward)
	assert.Equal(t, "Blocks", mockJira.CreateLinkCalls[0].LinkType)
}

func TestFullSyncService_SyncDependencies_RemoveLink(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with no dependencies (dependency was removed locally)
	task := &domain.TaskFile{
		Path: "/tasks/task1.md",
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Task 1",
			JiraNumber:       "GUARD-101",
			JiraParent:       "GUARD-100",
			JiraDependencies: []string{}, // No dependencies
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

	// Mock GetIssueLinks to return existing link to KB-2
	mockJira.GetIssueLinksFunc = func(_ context.Context, _ string) ([]ports.IssueLink, error) {
		return []ports.IssueLink{
			{
				ID:           "link-123",
				Type:         "Blocks",
				InwardIssue:  "GUARD-102",
				OutwardIssue: "GUARD-101",
			},
		}, nil
	}

	svc := NewService(mockJira, hasher)
	svc.SetAllTasks(allTasks)

	result, err := svc.SyncTask(context.Background(), task)

	require.NoError(t, err)
	assert.NotNil(t, result.DependencyResult)
	assert.True(t, result.DependencyResult.HasChanges)
	assert.Equal(t, []string{"link-123"}, result.DependencyResult.ToRemove)

	// Should have deleted the link
	assert.Len(t, mockJira.DeleteLinkCalls, 1)
	assert.Equal(t, "link-123", mockJira.DeleteLinkCalls[0])
}

func TestFullSyncService_SyncDependencies_NoChanges(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	// Task with dependency on KB-2
	task := &domain.TaskFile{
		Path: "/tasks/task1.md",
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Task 1",
			JiraNumber:       "GUARD-101",
			JiraParent:       "GUARD-100",
			JiraDependencies: []string{"KB-2"},
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

	// Jira already has the link
	mockJira.GetIssueLinksFunc = func(_ context.Context, _ string) ([]ports.IssueLink, error) {
		return []ports.IssueLink{
			{
				ID:           "link-123",
				Type:         "Blocks",
				InwardIssue:  "GUARD-102",
				OutwardIssue: "GUARD-101",
			},
		}, nil
	}

	svc := NewService(mockJira, hasher)
	svc.SetAllTasks(allTasks)

	result, err := svc.SyncTask(context.Background(), task)

	require.NoError(t, err)
	assert.NotNil(t, result.DependencyResult)
	assert.False(t, result.DependencyResult.HasChanges)

	// Should not have created or deleted any links
	assert.Len(t, mockJira.CreateLinkCalls, 0)
	assert.Len(t, mockJira.DeleteLinkCalls, 0)
}

func TestFullSyncService_SyncDependenciesOnly(t *testing.T) {
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Path: "/tasks/task1.md",
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Task 1",
			JiraNumber:       "GUARD-101",
			JiraDependencies: []string{"KB-2", "KB-3"},
		},
	}

	allTasks := []*domain.TaskFile{
		task,
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Task 2", JiraNumber: "GUARD-102"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Task 3", JiraNumber: "GUARD-103"}},
	}

	// Only KB-2 link exists
	mockJira.GetIssueLinksFunc = func(_ context.Context, _ string) ([]ports.IssueLink, error) {
		return []ports.IssueLink{
			{ID: "link-1", Type: "Blocks", InwardIssue: "GUARD-102", OutwardIssue: "GUARD-101"},
		}, nil
	}

	svc := NewService(mockJira, hasher)
	svc.SetAllTasks(allTasks)

	result, err := svc.SyncDependenciesOnly(context.Background(), task)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.HasChanges)
	assert.Equal(t, []string{"GUARD-103"}, result.ToAdd)

	// Should have created a link for KB-3
	assert.Len(t, mockJira.CreateLinkCalls, 1)
	assert.Equal(t, "GUARD-103", mockJira.CreateLinkCalls[0].Outward)
}

// Tests for PullTask and PullAll methods

func TestFullSyncService_PullTask_UpdatesLocalFromJira(t *testing.T) {
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
	assert.Equal(t, PullActionUpdated, result.Action)
	assert.Contains(t, result.Fields, "title")
	assert.Contains(t, result.Fields, "description")
	assert.Contains(t, result.Fields, "status")

	// Task should be updated with Jira values
	assert.Equal(t, "KB-1: Updated Title", task.Frontmatter.Title)
	assert.Equal(t, "Updated description", task.Description)
	assert.Equal(t, "In Progress", task.Frontmatter.JiraState)
}

func TestFullSyncService_PullTask_SkipsNoChanges(t *testing.T) {
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
	assert.Equal(t, PullActionSkipped, result.Action)
}

func TestFullSyncService_PullTask_DetectsConflict(t *testing.T) {
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
	assert.Equal(t, PullActionConflict, result.Action)
	// Should not update local on conflict
	assert.Equal(t, "KB-1: Local Changes", task.Frontmatter.Title)
}

func TestFullSyncService_PullTask_ForcePull(t *testing.T) {
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
	assert.Equal(t, PullActionUpdated, result.Action)
	// Should overwrite local with Jira values
	assert.Equal(t, "KB-1: Jira Version", task.Frontmatter.Title)
	assert.Equal(t, "Jira description", task.Description)
	assert.Equal(t, "Done", task.Frontmatter.JiraState)
}

func TestFullSyncService_PullTask_SkipsNoJiraNumber(t *testing.T) {
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
	assert.Equal(t, PullActionSkipped, result.Action)
}

func TestFullSyncService_PullAll(t *testing.T) {
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
	assert.Equal(t, PullActionUpdated, results[0].Action)
	assert.Equal(t, PullActionSkipped, results[1].Action)
	assert.Equal(t, PullActionSkipped, results[2].Action)

	// First task should be updated
	assert.Equal(t, "KB-1: Updated Task 1", tasks[0].Frontmatter.Title)
	assert.Equal(t, "In Progress", tasks[0].Frontmatter.JiraState)
}
