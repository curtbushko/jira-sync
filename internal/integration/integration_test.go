// Package integration provides end-to-end integration tests.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/curtbushko/jira-sync/internal/adapters/filesystem"
	"github.com/curtbushko/jira-sync/internal/adapters/hashing"
	"github.com/curtbushko/jira-sync/internal/adapters/jira"
	"github.com/curtbushko/jira-sync/internal/application/sync"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_CreateAndSyncWorkflow tests the full workflow:
// 1. Create task files
// 2. Sync to Jira (create tickets)
// 3. Link dependencies
// 4. Modify a task
// 5. Resync to update Jira
func TestE2E_CreateAndSyncWorkflow(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize components
	repo := filesystem.NewFileTaskRepository()
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")
	hasher := hashing.NewSHA256HashComputer()
	svc := sync.NewService(repo, mockJira, hasher)

	// Step 1: Create task files
	tasks := []*domain.TaskFile{
		{
			Path: filepath.Join(tmpDir, "20260116-100000.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-1: Initialize Project",
				SyncStatus:   domain.SyncStatusPending,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{},
			},
			Description: "Initialize the project repository.",
		},
		{
			Path: filepath.Join(tmpDir, "20260116-100001.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-2: Create Types",
				SyncStatus:   domain.SyncStatusPending,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{"KB-1"},
			},
			Description: "Create shared type definitions.",
		},
		{
			Path: filepath.Join(tmpDir, "20260116-100002.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "ERR-1: Detector Stub",
				SyncStatus:   domain.SyncStatusPending,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{"KB-1"},
			},
			Description: "Create detector stub implementation.",
		},
		{
			Path: filepath.Join(tmpDir, "20260116-100003.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "CTRL-1: Controller Scaffold",
				SyncStatus:   domain.SyncStatusPending,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{"KB-2", "ERR-1"},
			},
			Description: "Create controller scaffold.",
		},
	}

	// Write task files
	for _, task := range tasks {
		err := repo.WriteTask(task)
		require.NoError(t, err)
	}

	// Step 2: Read tasks back and categorize
	loadedTasks, err := repo.ListTasks(tmpDir)
	require.NoError(t, err)
	assert.Len(t, loadedTasks, 4)

	categorized := svc.CategorizeTasks(loadedTasks)
	assert.Len(t, categorized.Pending, 4)
	assert.Len(t, categorized.Created, 0)
	assert.Len(t, categorized.Linked, 0)

	// Step 3: Create tickets
	ctx := context.Background()
	err = svc.CreateTickets(ctx, categorized.Pending, "GUARD", "Task")
	require.NoError(t, err)

	// Verify all tasks have Jira numbers
	for _, task := range categorized.Pending {
		assert.NotEmpty(t, task.Frontmatter.JiraNumber)
		assert.Equal(t, domain.SyncStatusCreated, task.Frontmatter.SyncStatus)
		assert.Contains(t, task.Frontmatter.JiraURL, "https://test.atlassian.net")
	}

	// Verify Jira was called 4 times
	assert.Len(t, mockJira.CreateIssueCalls, 4)

	// Save updated tasks
	for _, task := range categorized.Pending {
		err := repo.WriteTask(task)
		require.NoError(t, err)
	}

	// Step 4: Link dependencies
	err = svc.LinkDependencies(ctx, categorized.Pending, "Blocks")
	require.NoError(t, err)

	// Verify links were created (KB-2 -> KB-1, ERR-1 -> KB-1, CTRL-1 -> KB-2, CTRL-1 -> ERR-1)
	assert.Len(t, mockJira.CreateLinkCalls, 4)

	// Verify all tasks are now linked
	for _, task := range categorized.Pending {
		assert.Equal(t, domain.SyncStatusLinked, task.Frontmatter.SyncStatus)
	}

	// Save updated tasks with content hash
	for _, task := range categorized.Pending {
		task.Frontmatter.ContentHash = hasher.ComputeHash(task)
		err := repo.WriteTask(task)
		require.NoError(t, err)
	}

	// Step 5: Reload and verify no updates needed
	loadedTasks, err = repo.ListTasks(tmpDir)
	require.NoError(t, err)

	categorized = svc.CategorizeTasks(loadedTasks)
	assert.Len(t, categorized.Pending, 0)
	assert.Len(t, categorized.Created, 0)
	assert.Len(t, categorized.Linked, 4)
	assert.Len(t, categorized.NeedsUpdate, 0)

	// Step 6: Modify a task and verify it needs update
	for _, task := range loadedTasks {
		if task.TaskID() == "KB-1" {
			task.Description = "UPDATED: Initialize the project repository with new requirements."
			err := repo.WriteTask(task)
			require.NoError(t, err)
			break
		}
	}

	// Reload and categorize
	loadedTasks, err = repo.ListTasks(tmpDir)
	require.NoError(t, err)

	categorized = svc.CategorizeTasks(loadedTasks)
	assert.Len(t, categorized.NeedsUpdate, 1)
	assert.Equal(t, "KB-1", categorized.NeedsUpdate[0].TaskID())

	// Step 7: Update the modified task
	err = svc.UpdateModified(ctx, categorized.NeedsUpdate)
	require.NoError(t, err)

	// Verify update was called
	assert.Len(t, mockJira.UpdateIssueCalls, 1)
	assert.Contains(t, mockJira.UpdateIssueCalls[0].Req.Description, "UPDATED")
}

func TestE2E_DependencyResolution(t *testing.T) {
	tmpDir := t.TempDir()

	repo := filesystem.NewFileTaskRepository()
	mockJira := jira.NewMockJiraClient()
	hasher := hashing.NewSHA256HashComputer()
	svc := sync.NewService(repo, mockJira, hasher)

	// Create a complex dependency graph
	// KB-1 (no deps)
	// KB-2 depends on KB-1
	// KB-3 depends on KB-1
	// KB-4 depends on KB-2, KB-3
	tasks := []*domain.TaskFile{
		{
			Path: filepath.Join(tmpDir, "kb1.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-1: First",
				JiraNumber:   "GUARD-101",
				SyncStatus:   domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{},
			},
		},
		{
			Path: filepath.Join(tmpDir, "kb2.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-2: Second",
				JiraNumber:   "GUARD-102",
				SyncStatus:   domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{"KB-1"},
			},
		},
		{
			Path: filepath.Join(tmpDir, "kb3.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-3: Third",
				JiraNumber:   "GUARD-103",
				SyncStatus:   domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{"KB-1"},
			},
		},
		{
			Path: filepath.Join(tmpDir, "kb4.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-4: Fourth",
				JiraNumber:   "GUARD-104",
				SyncStatus:   domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraDependencies: []string{"KB-2", "KB-3"},
			},
		},
	}

	for _, task := range tasks {
		err := repo.WriteTask(task)
		require.NoError(t, err)
	}

	// Link dependencies
	ctx := context.Background()
	err := svc.LinkDependencies(ctx, tasks, "Blocks")
	require.NoError(t, err)

	// Verify 4 links: KB-2->KB-1, KB-3->KB-1, KB-4->KB-2, KB-4->KB-3
	assert.Len(t, mockJira.CreateLinkCalls, 4)

	// Verify specific links
	links := mockJira.CreateLinkCalls
	linkMap := make(map[string]string)
	for _, l := range links {
		linkMap[l.Inward] = l.Outward
	}

	// KB-2 blocked by KB-1
	assert.Contains(t, linkMap, "GUARD-102")
}

func TestE2E_RoundTrip_PreservesAllFields(t *testing.T) {
	tmpDir := t.TempDir()
	repo := filesystem.NewFileTaskRepository()

	original := &domain.TaskFile{
		Path: filepath.Join(tmpDir, "test.md"),
		Frontmatter: domain.Frontmatter{
			Title:            "ERR-5: Complex Task",
			JiraNumber:       "GUARD-999",
			CreatedDate:      "2026-01-16",
			JiraURL:          "https://test.atlassian.net/browse/GUARD-999",
			SyncStatus:       domain.SyncStatusLinked,
			JiraParent:       "GUARD-100",
			JiraDependencies: []string{"KB-1", "KB-2", "ERR-1"},
			ContentHash:      "somehash123",
		},
		Description: `Implement pod listing logic.

## Acceptance Criteria

- List all pods for deployment
- Handle pagination
- Unit tests required`,
	}

	// Write
	err := repo.WriteTask(original)
	require.NoError(t, err)

	// Read back
	loaded, err := repo.ReadTask(original.Path)
	require.NoError(t, err)

	// Verify all fields preserved
	assert.Equal(t, original.Frontmatter.Title, loaded.Frontmatter.Title)
	assert.Equal(t, original.Frontmatter.JiraNumber, loaded.Frontmatter.JiraNumber)
	assert.Equal(t, original.Frontmatter.CreatedDate, loaded.Frontmatter.CreatedDate)
	assert.Equal(t, original.Frontmatter.JiraURL, loaded.Frontmatter.JiraURL)
	assert.Equal(t, original.Frontmatter.SyncStatus, loaded.Frontmatter.SyncStatus)
	assert.Equal(t, original.Frontmatter.JiraParent, loaded.Frontmatter.JiraParent)
	assert.Equal(t, original.Frontmatter.JiraDependencies, loaded.Frontmatter.JiraDependencies)
	assert.Equal(t, original.Frontmatter.ContentHash, loaded.Frontmatter.ContentHash)
	assert.Equal(t, original.Description, loaded.Description)
}

func TestE2E_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	repo := filesystem.NewFileTaskRepository()
	hasher := hashing.NewSHA256HashComputer()
	svc := sync.NewService(repo, nil, hasher)

	tasks, err := repo.ListTasks(tmpDir)
	require.NoError(t, err)
	assert.Len(t, tasks, 0)

	categorized := svc.CategorizeTasks(tasks)
	assert.Len(t, categorized.Pending, 0)
	assert.Len(t, categorized.Created, 0)
	assert.Len(t, categorized.Linked, 0)
	assert.Len(t, categorized.NeedsUpdate, 0)
}

// TestJiraIntegration_Real is an integration test that requires real Jira credentials.
// It's skipped unless JIRA_TOKEN is set.
func TestJiraIntegration_Real(t *testing.T) {
	if os.Getenv("JIRA_TOKEN") == "" {
		t.Skip("JIRA_TOKEN not set - skipping real Jira integration test")
	}

	// This test would connect to real Jira and create a test issue
	// It's left as a template for manual testing
	t.Log("Real Jira integration test would run here")
}
