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
