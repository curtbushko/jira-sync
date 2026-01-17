package hashing

import (
	"testing"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestHashComputer_ComputeHash(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:        "KB-1: Test",
			Parent:       "GUARD-100",
			Dependencies: []string{"KB-0"},
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
			Title:        "Test",
			Parent:       "P",
			Dependencies: []string{"A", "B"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:        "Test",
			Parent:       "P",
			Dependencies: []string{"A", "B"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	assert.Equal(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DifferentTitleDifferentHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test1", Parent: "P"},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test2", Parent: "P"},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DifferentParentDifferentHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test", Parent: "P1"},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test", Parent: "P2"},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DifferentDependenciesDifferentHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:        "Test",
			Parent:       "P",
			Dependencies: []string{"A"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:        "Test",
			Parent:       "P",
			Dependencies: []string{"B"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DifferentDescriptionDifferentHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test", Parent: "P"},
		Description: "Desc1",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{Title: "Test", Parent: "P"},
		Description: "Desc2",
	}

	hasher := NewSHA256HashComputer()

	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_DependencyOrderMatters(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:        "Test",
			Parent:       "P",
			Dependencies: []string{"A", "B"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:        "Test",
			Parent:       "P",
			Dependencies: []string{"B", "A"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// Order matters - different order = different hash
	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_IgnoresJiraFields(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:       "Test",
			Parent:      "P",
			JiraNumber:  "",
			JiraURL:     "",
			SyncStatus:  "pending",
			ContentHash: "",
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:       "Test",
			Parent:      "P",
			JiraNumber:  "GUARD-101",
			JiraURL:     "https://jira.com/GUARD-101",
			SyncStatus:  "linked",
			ContentHash: "abc123",
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// These fields should be ignored - hash only content that would change Jira description
	assert.Equal(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_EmptyDependencies(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:        "Test",
			Parent:       "P",
			Dependencies: nil,
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:        "Test",
			Parent:       "P",
			Dependencies: []string{},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// nil and empty slice should produce same hash
	assert.Equal(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}
