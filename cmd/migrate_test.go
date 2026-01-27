package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetMigrateFlags resets the flags on the migrate command for test isolation.
func resetMigrateFlags() {
	_ = migrateCmd.Flags().Set("dry-run", "false")
	_ = migrateCmd.Flags().Set("default-project", "")
}

func TestMigrateCommand_DryRun(t *testing.T) {
	resetMigrateFlags()

	// Create temp directory with old-format task
	tmpDir := t.TempDir()
	oldTaskPath := filepath.Join(tmpDir, "old-task.md")
	oldContent := `---
title: "KB-1: Old Task"
jira-parent: GUARD-100
---

Old task description.`
	err := os.WriteFile(oldTaskPath, []byte(oldContent), 0644)
	require.NoError(t, err)

	// Execute migrate with --dry-run
	rootCmd.SetArgs([]string{"migrate", tmpDir, "--dry-run"})
	err = rootCmd.Execute()
	require.NoError(t, err)

	// File should NOT be modified in dry-run mode
	content, err := os.ReadFile(oldTaskPath)
	require.NoError(t, err)
	assert.Equal(t, oldContent, string(content), "file should not be modified in dry-run mode")
}

func TestMigrateCommand_ActualMigration(t *testing.T) {
	resetMigrateFlags()

	// Create temp directory with old-format task
	tmpDir := t.TempDir()
	oldTaskPath := filepath.Join(tmpDir, "old-task.md")
	oldContent := `---
title: "KB-1: Old Task"
jira-parent: GUARD-100
---

Old task description.`
	err := os.WriteFile(oldTaskPath, []byte(oldContent), 0644)
	require.NoError(t, err)

	// Execute migrate (no --dry-run)
	rootCmd.SetArgs([]string{"migrate", tmpDir})
	err = rootCmd.Execute()
	require.NoError(t, err)

	// File should be modified with new fields
	content, err := os.ReadFile(oldTaskPath)
	require.NoError(t, err)
	contentStr := string(content)

	// Should contain migrated fields
	assert.Contains(t, contentStr, "jira-state: Todo")
	assert.Contains(t, contentStr, "sync-status: pending")
	assert.Contains(t, contentStr, "jira-dependencies: []")
}

func TestMigrateCommand_DefaultProject(t *testing.T) {
	resetMigrateFlags()

	// Create temp directory with task missing jira-project
	tmpDir := t.TempDir()
	taskPath := filepath.Join(tmpDir, "task.md")
	content := `---
title: "KB-1: Test Task"
jira-parent: GUARD-100
---

Task description.`
	err := os.WriteFile(taskPath, []byte(content), 0644)
	require.NoError(t, err)

	// Execute migrate with --default-project
	rootCmd.SetArgs([]string{"migrate", tmpDir, "--default-project", "MYPROJ"})
	err = rootCmd.Execute()
	require.NoError(t, err)

	// File should have the default project
	updatedContent, err := os.ReadFile(taskPath)
	require.NoError(t, err)
	assert.Contains(t, string(updatedContent), "jira-project: MYPROJ")
}

func TestMigrateCommand_NoChangesNeeded(t *testing.T) {
	resetMigrateFlags()

	// Create temp directory with already-migrated task
	tmpDir := t.TempDir()
	taskPath := filepath.Join(tmpDir, "task.md")
	content := `---
title: "KB-1: Complete Task"
jira-number: GUARD-123
jira-project: GUARD
jira-state: Done
sync-status: linked
jira-parent: GUARD-100
jira-dependencies: []
content-hash: abc123
---

Task description.`
	err := os.WriteFile(taskPath, []byte(content), 0644)
	require.NoError(t, err)

	// Execute migrate
	rootCmd.SetArgs([]string{"migrate", tmpDir})
	err = rootCmd.Execute()
	require.NoError(t, err)

	// File should remain unchanged (state preserved)
	updatedContent, err := os.ReadFile(taskPath)
	require.NoError(t, err)
	assert.Contains(t, string(updatedContent), "jira-state: Done")
	assert.Contains(t, string(updatedContent), "sync-status: linked")
}
