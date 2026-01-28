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
	"github.com/curtbushko/jira-sync/internal/application/push"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
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
	svc := push.NewService(repo, mockJira, hasher)

	// Step 1: Create task files
	tasks := []*domain.TaskFile{
		{
			Path: filepath.Join(tmpDir, "20260116-100000.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-1: Initialize Project",
				SyncStatus:   domain.SyncStatusPending,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{},
			},
			Description: "Initialize the project repository.",
		},
		{
			Path: filepath.Join(tmpDir, "20260116-100001.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-2: Create Types",
				SyncStatus:   domain.SyncStatusPending,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{"KB-1"},
			},
			Description: "Create shared type definitions.",
		},
		{
			Path: filepath.Join(tmpDir, "20260116-100002.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "ERR-1: Detector Stub",
				SyncStatus:   domain.SyncStatusPending,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{"KB-1"},
			},
			Description: "Create detector stub implementation.",
		},
		{
			Path: filepath.Join(tmpDir, "20260116-100003.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "CTRL-1: Controller Scaffold",
				SyncStatus:   domain.SyncStatusPending,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{"KB-2", "ERR-1"},
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
	err = svc.LinkDependencies(ctx, categorized.Pending, nil, "Blocking")
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
	svc := push.NewService(repo, mockJira, hasher)

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
				JiraIsBlockedBy: []string{},
			},
		},
		{
			Path: filepath.Join(tmpDir, "kb2.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-2: Second",
				JiraNumber:   "GUARD-102",
				SyncStatus:   domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{"KB-1"},
			},
		},
		{
			Path: filepath.Join(tmpDir, "kb3.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-3: Third",
				JiraNumber:   "GUARD-103",
				SyncStatus:   domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{"KB-1"},
			},
		},
		{
			Path: filepath.Join(tmpDir, "kb4.md"),
			Frontmatter: domain.Frontmatter{
				Title:        "KB-4: Fourth",
				JiraNumber:   "GUARD-104",
				SyncStatus:   domain.SyncStatusCreated,
				JiraParent:       "GUARD-100",
				JiraIsBlockedBy: []string{"KB-2", "KB-3"},
			},
		},
	}

	for _, task := range tasks {
		err := repo.WriteTask(task)
		require.NoError(t, err)
	}

	// Link dependencies
	ctx := context.Background()
	err := svc.LinkDependencies(ctx, tasks, nil, "Blocking")
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
			JiraIsBlockedBy: []string{"KB-1", "KB-2", "ERR-1"},
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
	assert.Equal(t, original.Frontmatter.JiraIsBlockedBy, loaded.Frontmatter.JiraIsBlockedBy)
	assert.Equal(t, original.Frontmatter.ContentHash, loaded.Frontmatter.ContentHash)
	assert.Equal(t, original.Description, loaded.Description)
}

func TestE2E_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	repo := filesystem.NewFileTaskRepository()
	hasher := hashing.NewSHA256HashComputer()
	svc := push.NewService(repo, nil, hasher)

	tasks, err := repo.ListTasks(tmpDir)
	require.NoError(t, err)
	assert.Len(t, tasks, 0)

	categorized := svc.CategorizeTasks(tasks)
	assert.Len(t, categorized.Pending, 0)
	assert.Len(t, categorized.Created, 0)
	assert.Len(t, categorized.Linked, 0)
	assert.Len(t, categorized.NeedsUpdate, 0)
}

// TestE2E_AddDependencyToLinkedTask tests adding a new dependency to an already-linked task.
// This ensures that new dependencies are linked during the update phase.
func TestE2E_AddDependencyToLinkedTask(t *testing.T) {
	tmpDir := t.TempDir()

	repo := filesystem.NewFileTaskRepository()
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")
	hasher := hashing.NewSHA256HashComputer()
	svc := push.NewService(repo, mockJira, hasher)

	// Create two tasks that are already linked (simulating previous sync)
	task1 := &domain.TaskFile{
		Path: filepath.Join(tmpDir, "task1.md"),
		Frontmatter: domain.Frontmatter{
			Title:           "KB-1: First Task",
			JiraNumber:      "GUARD-101",
			JiraURL:         "https://test.atlassian.net/browse/GUARD-101",
			SyncStatus:      domain.SyncStatusLinked,
			JiraState:       domain.DefaultJiraState,
			JiraParent:      "GUARD-100",
			JiraBlocks:      []string{},
			JiraIsBlockedBy: []string{},
		},
		Description: "First task description.",
	}
	task1.Frontmatter.ContentHash = hasher.ComputeHash(task1)

	task2 := &domain.TaskFile{
		Path: filepath.Join(tmpDir, "task2.md"),
		Frontmatter: domain.Frontmatter{
			Title:           "KB-2: Second Task",
			JiraNumber:      "GUARD-102",
			JiraURL:         "https://test.atlassian.net/browse/GUARD-102",
			SyncStatus:      domain.SyncStatusLinked,
			JiraState:       domain.DefaultJiraState,
			JiraParent:      "GUARD-100",
			JiraBlocks:      []string{},
			JiraIsBlockedBy: []string{},
		},
		Description: "Second task description.",
	}
	task2.Frontmatter.ContentHash = hasher.ComputeHash(task2)

	// Write both tasks
	require.NoError(t, repo.WriteTask(task1))
	require.NoError(t, repo.WriteTask(task2))

	// Reload and verify both are in Linked state
	tasks, err := repo.ListTasks(tmpDir)
	require.NoError(t, err)
	categorized := svc.CategorizeTasks(tasks)
	assert.Len(t, categorized.Linked, 2)
	assert.Len(t, categorized.NeedsUpdate, 0)

	// Now add a dependency: KB-2 is blocked by KB-1
	for _, task := range tasks {
		if task.TaskID() == "KB-2" {
			task.Frontmatter.JiraIsBlockedBy = []string{"KB-1"}
			require.NoError(t, repo.WriteTask(task))
			break
		}
	}

	// Reload and verify KB-2 is now in NeedsUpdate
	tasks, err = repo.ListTasks(tmpDir)
	require.NoError(t, err)
	categorized = svc.CategorizeTasks(tasks)
	assert.Len(t, categorized.Linked, 1)
	assert.Len(t, categorized.NeedsUpdate, 1)
	assert.Equal(t, "KB-2", categorized.NeedsUpdate[0].TaskID())

	// Update the modified task and link dependencies
	ctx := context.Background()
	err = svc.UpdateModified(ctx, categorized.NeedsUpdate)
	require.NoError(t, err)

	// Link dependencies for the updated task
	err = svc.LinkDependencies(ctx, categorized.NeedsUpdate, tasks, "Blocking")
	require.NoError(t, err)

	// Verify the link was created: GUARD-102 is blocked by GUARD-101
	assert.Len(t, mockJira.CreateLinkCalls, 1)
	link := mockJira.CreateLinkCalls[0]
	assert.Equal(t, "GUARD-102", link.Inward)  // blocked issue
	assert.Equal(t, "GUARD-101", link.Outward) // blocker issue
}

// TestE2E_AddExternalDependency tests adding a dependency to an external Jira issue.
func TestE2E_AddExternalDependency(t *testing.T) {
	tmpDir := t.TempDir()

	repo := filesystem.NewFileTaskRepository()
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")
	hasher := hashing.NewSHA256HashComputer()
	svc := push.NewService(repo, mockJira, hasher)

	// Create a linked task
	task := &domain.TaskFile{
		Path: filepath.Join(tmpDir, "task.md"),
		Frontmatter: domain.Frontmatter{
			Title:           "KB-1: My Task",
			JiraNumber:      "GUARD-101",
			JiraURL:         "https://test.atlassian.net/browse/GUARD-101",
			SyncStatus:      domain.SyncStatusLinked,
			JiraState:       domain.DefaultJiraState,
			JiraParent:      "GUARD-100",
			JiraBlocks:      []string{},
			JiraIsBlockedBy: []string{},
		},
		Description: "Task description.",
	}
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)
	require.NoError(t, repo.WriteTask(task))

	// Verify it's in Linked state
	tasks, err := repo.ListTasks(tmpDir)
	require.NoError(t, err)
	categorized := svc.CategorizeTasks(tasks)
	assert.Len(t, categorized.Linked, 1)

	// Add a dependency to an external Jira issue (not a local task)
	tasks[0].Frontmatter.JiraIsBlockedBy = []string{"EXTERNAL-999"}
	require.NoError(t, repo.WriteTask(tasks[0]))

	// Reload and verify it's in NeedsUpdate
	tasks, err = repo.ListTasks(tmpDir)
	require.NoError(t, err)
	categorized = svc.CategorizeTasks(tasks)
	assert.Len(t, categorized.NeedsUpdate, 1)

	// Update and link
	ctx := context.Background()
	err = svc.UpdateModified(ctx, categorized.NeedsUpdate)
	require.NoError(t, err)

	err = svc.LinkDependencies(ctx, categorized.NeedsUpdate, tasks, "Blocking")
	require.NoError(t, err)

	// Verify the link was created to the external issue
	assert.Len(t, mockJira.CreateLinkCalls, 1)
	link := mockJira.CreateLinkCalls[0]
	assert.Equal(t, "GUARD-101", link.Inward)    // blocked issue (our task)
	assert.Equal(t, "EXTERNAL-999", link.Outward) // blocker issue (external)
}

// TestE2E_ChangeJiraState tests transitioning a task to a new state.
func TestE2E_ChangeJiraState(t *testing.T) {
	tmpDir := t.TempDir()

	repo := filesystem.NewFileTaskRepository()
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")
	hasher := hashing.NewSHA256HashComputer()
	svc := push.NewService(repo, mockJira, hasher)

	// Mock GetIssue to return current state as "To Do"
	mockJira.GetIssueFunc = func(_ context.Context, key string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:    key,
			Status: "To Do",
		}, nil
	}

	// Create a linked task with state "To Do"
	task := &domain.TaskFile{
		Path: filepath.Join(tmpDir, "task.md"),
		Frontmatter: domain.Frontmatter{
			Title:           "KB-1: My Task",
			JiraNumber:      "GUARD-101",
			JiraURL:         "https://test.atlassian.net/browse/GUARD-101",
			SyncStatus:      domain.SyncStatusLinked,
			JiraState:       "To Do",
			JiraParent:      "GUARD-100",
			JiraBlocks:      []string{},
			JiraIsBlockedBy: []string{},
		},
		Description: "Task description.",
	}
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)
	require.NoError(t, repo.WriteTask(task))

	// Verify it's in Linked state (no changes needed)
	tasks, err := repo.ListTasks(tmpDir)
	require.NoError(t, err)
	categorized := svc.CategorizeTasks(tasks)
	assert.Len(t, categorized.Linked, 1)
	assert.Len(t, categorized.NeedsUpdate, 0)

	// Change state to "In Progress"
	tasks[0].Frontmatter.JiraState = "In Progress"
	require.NoError(t, repo.WriteTask(tasks[0]))

	// Reload and verify it's in NeedsUpdate
	tasks, err = repo.ListTasks(tmpDir)
	require.NoError(t, err)
	categorized = svc.CategorizeTasks(tasks)
	assert.Len(t, categorized.NeedsUpdate, 1)

	// Transition the task
	ctx := context.Background()
	transitioned, err := svc.TransitionIssues(ctx, categorized.NeedsUpdate)
	require.NoError(t, err)
	assert.Equal(t, 1, transitioned)

	// Verify DoTransition was called
	assert.Len(t, mockJira.DoTransitionCalls, 1)
	assert.Equal(t, "GUARD-101", mockJira.DoTransitionCalls[0].Key)
	assert.Equal(t, "21", mockJira.DoTransitionCalls[0].TransitionID) // "In Progress" ID
}

// TestE2E_UpdateParent tests updating a task's parent.
func TestE2E_UpdateParent(t *testing.T) {
	tmpDir := t.TempDir()

	repo := filesystem.NewFileTaskRepository()
	mockJira := jira.NewMockJiraClient()
	mockJira.SetBaseURL("https://test.atlassian.net")
	hasher := hashing.NewSHA256HashComputer()
	svc := push.NewService(repo, mockJira, hasher)

	// Create a linked task with parent GUARD-100
	task := &domain.TaskFile{
		Path: filepath.Join(tmpDir, "task.md"),
		Frontmatter: domain.Frontmatter{
			Title:           "KB-1: My Task",
			JiraNumber:      "GUARD-101",
			JiraURL:         "https://test.atlassian.net/browse/GUARD-101",
			SyncStatus:      domain.SyncStatusLinked,
			JiraState:       domain.DefaultJiraState,
			JiraParent:      "GUARD-100",
			JiraBlocks:      []string{},
			JiraIsBlockedBy: []string{},
		},
		Description: "Task description.",
	}
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)
	require.NoError(t, repo.WriteTask(task))

	// Verify it's in Linked state
	tasks, err := repo.ListTasks(tmpDir)
	require.NoError(t, err)
	categorized := svc.CategorizeTasks(tasks)
	assert.Len(t, categorized.Linked, 1)

	// Change parent to GUARD-200
	tasks[0].Frontmatter.JiraParent = "GUARD-200"
	require.NoError(t, repo.WriteTask(tasks[0]))

	// Reload and verify it's in NeedsUpdate
	tasks, err = repo.ListTasks(tmpDir)
	require.NoError(t, err)
	categorized = svc.CategorizeTasks(tasks)
	assert.Len(t, categorized.NeedsUpdate, 1)

	// Update the task
	ctx := context.Background()
	err = svc.UpdateModified(ctx, categorized.NeedsUpdate)
	require.NoError(t, err)

	// Verify UpdateIssue was called with new parent
	assert.Len(t, mockJira.UpdateIssueCalls, 1)
	assert.Equal(t, "GUARD-200", mockJira.UpdateIssueCalls[0].Req.Parent)
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
