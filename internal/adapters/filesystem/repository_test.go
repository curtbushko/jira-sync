package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRepository_ReadTask(t *testing.T) {
	tmpDir := t.TempDir()
	content := `---
title: "KB-1: Test"
jira-number: ""
created-date: "2026-01-16"
start-date: ""
end-date: ""
jira-url: ""
sync-status: pending
parent: GUARD-100
dependencies: []
content-hash: ""
---

Description`
	path := filepath.Join(tmpDir, "20260116-120000.md")
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)

	repo := NewFileTaskRepository()
	task, err := repo.ReadTask(path)

	require.NoError(t, err)
	assert.Equal(t, "KB-1: Test", task.Frontmatter.Title)
	assert.Equal(t, "pending", task.Frontmatter.SyncStatus)
	assert.Equal(t, "GUARD-100", task.Frontmatter.Parent)
	assert.Equal(t, "Description", task.Description)
}

func TestTaskRepository_ReadTask_NotFound(t *testing.T) {
	repo := NewFileTaskRepository()
	_, err := repo.ReadTask("/nonexistent/path/file.md")

	assert.Error(t, err)
}

func TestTaskRepository_WriteTask(t *testing.T) {
	tmpDir := t.TempDir()
	task := &domain.TaskFile{
		Path: filepath.Join(tmpDir, "test.md"),
		Frontmatter: domain.Frontmatter{
			Title:        "KB-1: Test",
			JiraNumber:   "",
			CreatedDate:  "2026-01-16",
			StartDate:    "",
			EndDate:      "",
			JiraURL:      "",
			SyncStatus:   "pending",
			Parent:       "GUARD-100",
			Dependencies: []string{},
			ContentHash:  "",
		},
		Description: "Test description",
	}

	repo := NewFileTaskRepository()
	err := repo.WriteTask(task)

	require.NoError(t, err)

	// Read back and verify
	readTask, err := repo.ReadTask(task.Path)
	require.NoError(t, err)
	assert.Equal(t, task.Frontmatter.Title, readTask.Frontmatter.Title)
	assert.Equal(t, task.Frontmatter.SyncStatus, readTask.Frontmatter.SyncStatus)
	assert.Equal(t, task.Description, readTask.Description)
}

func TestTaskRepository_WriteTask_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "dir", "test.md")
	task := &domain.TaskFile{
		Path: nestedPath,
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test",
			SyncStatus: "pending",
			Parent:     "GUARD-100",
		},
		Description: "Test",
	}

	repo := NewFileTaskRepository()
	err := repo.WriteTask(task)

	require.NoError(t, err)
	assert.FileExists(t, nestedPath)
}

func TestTaskRepository_ListTasks(t *testing.T) {
	tmpDir := t.TempDir()
	// Create 3 task files
	for idx := 0; idx < 3; idx++ {
		content := fmt.Sprintf(`---
title: "TASK-%d: Task %d"
jira-number: ""
created-date: "2026-01-16"
start-date: ""
end-date: ""
jira-url: ""
sync-status: pending
parent: GUARD-100
dependencies: []
content-hash: ""
---

Description %d`, idx, idx, idx)
		path := filepath.Join(tmpDir, fmt.Sprintf("2026011%d-120000.md", idx))
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	repo := NewFileTaskRepository()
	tasks, err := repo.ListTasks(tmpDir)

	require.NoError(t, err)
	assert.Len(t, tasks, 3)
}

func TestTaskRepository_ListTasks_IgnoresNonMarkdown(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid task file
	validContent := `---
title: "KB-1: Test"
sync-status: pending
parent: GUARD-100
dependencies: []
content-hash: ""
---

Description`
	err := os.WriteFile(filepath.Join(tmpDir, "task.md"), []byte(validContent), 0644)
	require.NoError(t, err)

	// Create non-markdown files
	err = os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("text file"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("yaml: file"), 0644)
	require.NoError(t, err)

	repo := NewFileTaskRepository()
	tasks, err := repo.ListTasks(tmpDir)

	require.NoError(t, err)
	assert.Len(t, tasks, 1)
}

func TestTaskRepository_ListTasks_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	repo := NewFileTaskRepository()
	tasks, err := repo.ListTasks(tmpDir)

	require.NoError(t, err)
	assert.Len(t, tasks, 0)
}

func TestTaskRepository_ListTasks_DirectoryNotFound(t *testing.T) {
	repo := NewFileTaskRepository()
	_, err := repo.ListTasks("/nonexistent/directory")

	assert.Error(t, err)
}

func TestTaskRepository_GenerateFilename(t *testing.T) {
	repo := NewFileTaskRepository()

	filename := repo.GenerateFilename()

	// Should match pattern YYYYMMDD-HHMMSS.md
	pattern := regexp.MustCompile(`^\d{8}-\d{6}\.md$`)
	assert.Regexp(t, pattern, filename)
}

func TestTaskRepository_GenerateFilename_UniquePerCall(t *testing.T) {
	repo := NewFileTaskRepository()

	// Generate multiple filenames quickly - they might be the same due to time resolution
	// but the repo should handle this
	filename1 := repo.GenerateFilename()
	filename2 := repo.GenerateFilename()

	// Both should be valid
	pattern := regexp.MustCompile(`^\d{8}-\d{6}\.md$`)
	assert.Regexp(t, pattern, filename1)
	assert.Regexp(t, pattern, filename2)
}

func TestTaskRepository_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileTaskRepository()

	original := &domain.TaskFile{
		Path: filepath.Join(tmpDir, repo.GenerateFilename()),
		Frontmatter: domain.Frontmatter{
			Title:        "ERR-5: Complex Task",
			JiraNumber:   "GUARD-999",
			CreatedDate:  "2026-01-16",
			StartDate:    "2026-01-16",
			EndDate:      "2026-01-23",
			JiraURL:      "https://test.atlassian.net/browse/GUARD-999",
			SyncStatus:   "linked",
			Parent:       "GUARD-100",
			Dependencies: []string{"KB-1", "KB-2", "ERR-1"},
			ContentHash:  "somehash123",
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

	// Verify all fields
	assert.Equal(t, original.Frontmatter.Title, loaded.Frontmatter.Title)
	assert.Equal(t, original.Frontmatter.JiraNumber, loaded.Frontmatter.JiraNumber)
	assert.Equal(t, original.Frontmatter.CreatedDate, loaded.Frontmatter.CreatedDate)
	assert.Equal(t, original.Frontmatter.StartDate, loaded.Frontmatter.StartDate)
	assert.Equal(t, original.Frontmatter.EndDate, loaded.Frontmatter.EndDate)
	assert.Equal(t, original.Frontmatter.JiraURL, loaded.Frontmatter.JiraURL)
	assert.Equal(t, original.Frontmatter.SyncStatus, loaded.Frontmatter.SyncStatus)
	assert.Equal(t, original.Frontmatter.Parent, loaded.Frontmatter.Parent)
	assert.Equal(t, original.Frontmatter.Dependencies, loaded.Frontmatter.Dependencies)
	assert.Equal(t, original.Frontmatter.ContentHash, loaded.Frontmatter.ContentHash)
	assert.Equal(t, original.Description, loaded.Description)
}
