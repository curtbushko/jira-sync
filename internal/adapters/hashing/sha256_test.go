package hashing

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/curtbushko/jira-sync/internal/domain"
)

func TestHashComputer_ComputeHash(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "KB-1: Test",
			JiraParent:      "GUARD-100",
			JiraIsBlockedBy: []string{"KB-0"},
		},
		Description: "Test description",
	}

	hasher := NewSHA256HashComputer()
	hash := hasher.ComputeHash(task)

	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64) // SHA256 hex = 64 chars
}

func TestHashComputer_SameContentSameHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "Test",
			JiraParent:      "P",
			JiraBlocks:      []string{"A", "B"},
			JiraIsBlockedBy: []string{"C"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "Test",
			JiraParent:      "P",
			JiraBlocks:      []string{"A", "B"},
			JiraIsBlockedBy: []string{"C"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	assert.Equal(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DifferentTitleDifferentHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test1", JiraParent: "P"},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test2", JiraParent: "P"},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DifferentJiraParentDifferentHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test", JiraParent: "P1"},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test", JiraParent: "P2"},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DifferentJiraBlocksDifferentHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "Test",
			JiraParent: "P",
			JiraBlocks: []string{"A"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "Test",
			JiraParent: "P",
			JiraBlocks: []string{"B"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DifferentJiraIsBlockedByDifferentHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "Test",
			JiraParent:      "P",
			JiraIsBlockedBy: []string{"A"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "Test",
			JiraParent:      "P",
			JiraIsBlockedBy: []string{"B"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DifferentDescriptionDifferentHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test", JiraParent: "P"},
		Description: "Desc1",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test", JiraParent: "P"},
		Description: "Desc2",
	}

	hasher := NewSHA256HashComputer()

	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_BlockingOrderMatters(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "Test",
			JiraParent: "P",
			JiraBlocks: []string{"A", "B"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "Test",
			JiraParent: "P",
			JiraBlocks: []string{"B", "A"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// Order matters - different order = different hash
	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_IgnoresMetadataFields(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "Test",
			JiraParent:      "P",
			JiraNumber:      "",
			JiraURL:         "",
			SyncStatus:      "pending",
			ContentHash:     "",
			JiraIsBlockedBy: []string{"A"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "Test",
			JiraParent:      "P",
			JiraNumber:      "GUARD-101",
			JiraURL:         "https://jira.com/GUARD-101",
			SyncStatus:      "linked",
			ContentHash:     "abc123",
			JiraIsBlockedBy: []string{"A"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// These fields should be ignored - hash only content that would change Jira description
	assert.Equal(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_EmptyBlockingSlices(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "Test",
			JiraParent:      "P",
			JiraBlocks:      nil,
			JiraIsBlockedBy: nil,
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "Test",
			JiraParent:      "P",
			JiraBlocks:      []string{},
			JiraIsBlockedBy: []string{},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// nil and empty slice should produce same hash
	assert.Equal(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_BlockingFieldsIncluded(t *testing.T) {
	// Test that changing blocking fields changes the hash
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "Test",
			JiraParent:      "P",
			JiraBlocks:      []string{"A", "B"},
			JiraIsBlockedBy: []string{"C"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:           "Test",
			JiraParent:      "P",
			JiraBlocks:      []string{"C", "D", "E"},
			JiraIsBlockedBy: []string{"F"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// Hash should be different since blocking fields are included
	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}
