package hashing

import (
	"testing"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestHashComputer_ComputeHash(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "KB-1: Test",
			JiraParent:       "GUARD-100",
			JiraDependencies: []string{"KB-0"},
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
			Title:            "Test",
			JiraParent:       "P",
			JiraDependencies: []string{"A", "B"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraDependencies: []string{"A", "B"},
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

func TestHashComputer_DifferentJiraDependenciesDifferentHash(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraDependencies: []string{"A"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraDependencies: []string{"B"},
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

func TestHashComputer_JiraDependencyOrderMatters(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraDependencies: []string{"A", "B"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraDependencies: []string{"B", "A"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// Order matters - different order = different hash
	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_IgnoresJiraFieldsAndJiraDependencies(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraNumber:       "",
			JiraURL:          "",
			SyncStatus:       "pending",
			ContentHash:      "",
			JiraDependencies: []string{"A"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraNumber:       "GUARD-101",
			JiraURL:          "https://jira.com/GUARD-101",
			SyncStatus:       "linked",
			ContentHash:      "abc123",
			JiraDependencies: []string{"A"},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// These fields should be ignored - hash only content that would change Jira description
	assert.Equal(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_EmptyJiraDependencies(t *testing.T) {
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraDependencies: nil,
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraDependencies: []string{},
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// nil and empty slice should produce same hash
	assert.Equal(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

func TestHashComputer_JiraDependenciesIncluded(t *testing.T) {
	// Test that changing jira-dependencies changes the hash
	task1 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraDependencies: []string{"A", "B"},
		},
		Description: "Desc",
	}
	task2 := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            "Test",
			JiraParent:       "P",
			JiraDependencies: []string{"C", "D", "E"}, // Different jira deps
		},
		Description: "Desc",
	}

	hasher := NewSHA256HashComputer()

	// Hash should be different since jira-dependencies are included
	assert.NotEqual(t, hasher.ComputeHash(task1), hasher.ComputeHash(task2))
}

