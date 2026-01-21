package fullsync

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
			Title:       "KB-1: Test",
			JiraNumber:  "GUARD-123",
			JiraParent:  "GUARD-100",
			LastSynced:  "2026-01-15T10:00:00Z",
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
			Title:       "KB-1: Test",
			JiraNumber:  "GUARD-123",
			JiraParent:  "GUARD-100",
			LastSynced:  "2026-01-15T10:00:00Z",
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
			ContentHash: "",    // Never synced
			LastSynced:  "",    // Never synced
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
			Title:       "KB-1: Test",
			JiraNumber:  "GUARD-123",
			JiraParent:  "GUARD-100",
			JiraState:   "Todo",
			LastSynced:  "2026-01-15T10:00:00Z",
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
