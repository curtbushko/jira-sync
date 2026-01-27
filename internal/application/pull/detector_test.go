package pull

import (
	"testing"
	"time"

	"github.com/curtbushko/jira-sync/internal/adapters/hashing"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
	"github.com/stretchr/testify/assert"
)

func TestDetectChanges_LocalOnlyChanged(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	// Create task with old hash (content has changed locally)
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Test",
			JiraNumber:  "GUARD-123",
			JiraParent:  "GUARD-100",
			ContentHash: "oldhash", // Different from actual content
			LastSynced:  "2026-01-15T10:00:00Z",
		},
		Description: "Updated description", // Changed locally
	}

	jiraIssue := &ports.Issue{
		Key:         "GUARD-123",
		Summary:     "KB-1: Test",
		Description: "Original description",
		Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), // Before last sync
	}

	detector := NewChangeDetector(hasher)
	result := detector.Detect(task, jiraIssue)

	assert.Equal(t, ChangeTypeLocalToJira, result.Type)
	assert.Contains(t, result.Fields, "description")
}

func TestDetectChanges_JiraOnlyChanged(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	// Create task and compute its actual hash
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test",
			JiraNumber: "GUARD-123",
			JiraParent: "GUARD-100",
			LastSynced: "2026-01-15T10:00:00Z",
		},
		Description: "Original description",
	}
	// Set hash to match current content (no local changes)
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)

	jiraIssue := &ports.Issue{
		Key:         "GUARD-123",
		Summary:     "KB-1: Updated Title", // Changed in Jira
		Description: "Original description",
		Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), // After last sync
	}

	detector := NewChangeDetector(hasher)
	result := detector.Detect(task, jiraIssue)

	assert.Equal(t, ChangeTypeJiraToLocal, result.Type)
	assert.Contains(t, result.Fields, "title")
}

func TestDetectChanges_BothChanged_Conflict(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Local Title",
			JiraNumber:  "GUARD-123",
			JiraParent:  "GUARD-100",
			ContentHash: "oldhash", // Different from current (local changed)
			LastSynced:  "2026-01-15T10:00:00Z",
		},
		Description: "Local description",
	}

	jiraIssue := &ports.Issue{
		Key:         "GUARD-123",
		Summary:     "KB-1: Jira Title",
		Description: "Jira description",
		Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC), // After last sync
	}

	detector := NewChangeDetector(hasher)
	result := detector.Detect(task, jiraIssue)

	assert.Equal(t, ChangeTypeConflict, result.Type)
}

func TestDetectChanges_NoChanges(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test",
			JiraNumber: "GUARD-123",
			JiraParent: "GUARD-100",
			LastSynced: "2026-01-15T10:00:00Z",
		},
		Description: "Same description",
	}
	// Set hash to match current content
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)

	jiraIssue := &ports.Issue{
		Key:         "GUARD-123",
		Summary:     "KB-1: Test",
		Description: "Same description",
		Updated:     time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), // Before last sync
	}

	detector := NewChangeDetector(hasher)
	result := detector.Detect(task, jiraIssue)

	assert.Equal(t, ChangeTypeNone, result.Type)
	assert.Empty(t, result.Fields)
}

func TestDetectChanges_NeverSynced(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:       "KB-1: Test",
			JiraNumber:  "GUARD-123",
			JiraParent:  "GUARD-100",
			ContentHash: "", // Never synced
			LastSynced:  "", // Never synced
		},
		Description: "New description",
	}

	jiraIssue := &ports.Issue{
		Key:         "GUARD-123",
		Summary:     "KB-1: Test",
		Description: "Jira description",
		Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC),
	}

	detector := NewChangeDetector(hasher)
	result := detector.Detect(task, jiraIssue)

	// When never synced and content differs, should be conflict
	// (user needs to decide which to keep)
	assert.Equal(t, ChangeTypeConflict, result.Type)
}

func TestDetectChanges_StatusChanged(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test",
			JiraNumber: "GUARD-123",
			JiraParent: "GUARD-100",
			JiraState:  "Todo",
			LastSynced: "2026-01-15T10:00:00Z",
		},
		Description: "Same description",
	}
	task.Frontmatter.ContentHash = hasher.ComputeHash(task)

	jiraIssue := &ports.Issue{
		Key:         "GUARD-123",
		Summary:     "KB-1: Test",
		Description: "Same description",
		Status:      "In Progress", // Status changed in Jira
		Updated:     time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC),
	}

	detector := NewChangeDetector(hasher)
	result := detector.Detect(task, jiraIssue)

	assert.Equal(t, ChangeTypeJiraToLocal, result.Type)
	assert.Contains(t, result.Fields, "status")
}

// Tests for jira-dependencies detection

func TestDetectDependencies_InSync(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	// Task with dependencies on KB-2 and KB-3
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			JiraNumber:       "GUARD-123",
			JiraDependencies: []string{"KB-2", "KB-3"},
		},
	}

	// All tasks for mapping
	allTasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Task 2", JiraNumber: "GUARD-101"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Task 3", JiraNumber: "GUARD-102"}},
	}

	// Jira links match local dependencies
	jiraLinks := []ports.IssueLink{
		{Type: "Blocks", InwardIssue: "GUARD-101", OutwardIssue: "GUARD-123"},
		{Type: "Blocks", InwardIssue: "GUARD-102", OutwardIssue: "GUARD-123"},
	}

	detector := NewChangeDetector(hasher)
	result := detector.DetectDependencies(task, jiraLinks, allTasks)

	assert.False(t, result.HasChanges)
}

func TestDetectDependencies_JiraHasMore(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	// Task with only KB-2 dependency locally
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			JiraNumber:       "GUARD-123",
			JiraDependencies: []string{"KB-2"},
		},
	}

	allTasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Task 2", JiraNumber: "GUARD-101"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Task 3", JiraNumber: "GUARD-102"}},
	}

	// Jira has both KB-2 and KB-3
	jiraLinks := []ports.IssueLink{
		{Type: "Blocks", InwardIssue: "GUARD-101", OutwardIssue: "GUARD-123"},
		{Type: "Blocks", InwardIssue: "GUARD-102", OutwardIssue: "GUARD-123"},
	}

	detector := NewChangeDetector(hasher)
	result := detector.DetectDependencies(task, jiraLinks, allTasks)

	assert.True(t, result.HasChanges)
	assert.ElementsMatch(t, []string{"KB-2", "KB-3"}, result.JiraDeps)
	assert.Equal(t, []string{"KB-2"}, result.LocalDeps)
}

func TestDetectDependencies_LocalHasMore(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	// Task with KB-2 and KB-3 dependencies locally
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			JiraNumber:       "GUARD-123",
			JiraDependencies: []string{"KB-2", "KB-3"},
		},
	}

	allTasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Task 2", JiraNumber: "GUARD-101"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Task 3", JiraNumber: "GUARD-102"}},
	}

	// Jira only has KB-2
	jiraLinks := []ports.IssueLink{
		{Type: "Blocks", InwardIssue: "GUARD-101", OutwardIssue: "GUARD-123"},
	}

	detector := NewChangeDetector(hasher)
	result := detector.DetectDependencies(task, jiraLinks, allTasks)

	assert.True(t, result.HasChanges)
	assert.Equal(t, []string{"KB-2"}, result.JiraDeps)
	assert.ElementsMatch(t, []string{"KB-2", "KB-3"}, result.LocalDeps)
}

func TestDetectDependencies_IgnoresOtherLinkTypes(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			JiraNumber:       "GUARD-123",
			JiraDependencies: []string{"KB-2"},
		},
	}

	allTasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Task 2", JiraNumber: "GUARD-101"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Task 3", JiraNumber: "GUARD-102"}},
	}

	// Jira has a "Blocks" link and a "Relates" link (should ignore "Relates")
	jiraLinks := []ports.IssueLink{
		{ID: "link-1", Type: "Blocks", InwardIssue: "GUARD-101", OutwardIssue: "GUARD-123"},
		{ID: "link-2", Type: "Relates", InwardIssue: "GUARD-102", OutwardIssue: "GUARD-123"},
	}

	detector := NewChangeDetector(hasher)
	result := detector.DetectDependencies(task, jiraLinks, allTasks)

	// Should be in sync (Relates link is ignored)
	assert.False(t, result.HasChanges)
}

func TestDetectDependencies_EmptyDependencies(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			JiraNumber:       "GUARD-123",
			JiraDependencies: []string{},
		},
	}

	allTasks := []*domain.TaskFile{}

	jiraLinks := []ports.IssueLink{}

	detector := NewChangeDetector(hasher)
	result := detector.DetectDependencies(task, jiraLinks, allTasks)

	assert.False(t, result.HasChanges)
}

func TestDetectDependencies_WikiLinkFormat(t *testing.T) {
	hasher := hashing.NewSHA256HashComputer()

	// Task with wiki link format dependencies
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			JiraNumber:       "GUARD-123",
			JiraDependencies: []string{"[KB-2: Task 2](20260116-103001.md)"},
		},
	}

	allTasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Task 2", JiraNumber: "GUARD-101"}},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocks", InwardIssue: "GUARD-101", OutwardIssue: "GUARD-123"},
	}

	detector := NewChangeDetector(hasher)
	result := detector.DetectDependencies(task, jiraLinks, allTasks)

	assert.False(t, result.HasChanges)
}
