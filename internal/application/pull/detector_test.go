package pull

import (
	"testing"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
	"github.com/stretchr/testify/assert"
)

func TestExtractDependencies_WithMatches(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	allTasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Task 2", JiraNumber: "GUARD-101"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Task 3", JiraNumber: "GUARD-102"}},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocks", InwardIssue: "GUARD-101", OutwardIssue: ""},
		{Type: "Blocks", InwardIssue: "GUARD-102", OutwardIssue: ""},
	}

	detector := NewDependencyDetector("Blocks")
	deps := detector.ExtractDependencies(task, jiraLinks, allTasks)

	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "KB-2")
	assert.Contains(t, deps, "KB-3")
}

func TestExtractDependencies_NoMatches(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	allTasks := []*domain.TaskFile{}
	jiraLinks := []ports.IssueLink{}

	detector := NewDependencyDetector("Blocks")
	deps := detector.ExtractDependencies(task, jiraLinks, allTasks)

	assert.Empty(t, deps)
}

func TestExtractDependencies_IgnoresOtherLinkTypes(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	allTasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Task 2", JiraNumber: "GUARD-101"}},
		{Frontmatter: domain.Frontmatter{Title: "KB-3: Task 3", JiraNumber: "GUARD-102"}},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocks", InwardIssue: "GUARD-101", OutwardIssue: ""},
		{Type: "Relates", InwardIssue: "GUARD-102", OutwardIssue: ""}, // Different type - ignored
	}

	detector := NewDependencyDetector("Blocks")
	deps := detector.ExtractDependencies(task, jiraLinks, allTasks)

	assert.Len(t, deps, 1)
	assert.Contains(t, deps, "KB-2")
}

func TestExtractDependencies_UsesJiraKeyWhenNoLocalTask(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	// No local tasks that match
	allTasks := []*domain.TaskFile{}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocks", InwardIssue: "GUARD-999", OutwardIssue: ""},
	}

	detector := NewDependencyDetector("Blocks")
	deps := detector.ExtractDependencies(task, jiraLinks, allTasks)

	// Should use Jira key directly since no local task
	assert.Len(t, deps, 1)
	assert.Contains(t, deps, "GUARD-999")
}

func TestExtractDependencies_CustomLinkType(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	allTasks := []*domain.TaskFile{
		{Frontmatter: domain.Frontmatter{Title: "KB-2: Task 2", JiraNumber: "GUARD-101"}},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocking", InwardIssue: "GUARD-101", OutwardIssue: ""},
		{Type: "Blocks", InwardIssue: "GUARD-102", OutwardIssue: ""}, // Wrong type
	}

	detector := NewDependencyDetector("Blocking")
	deps := detector.ExtractDependencies(task, jiraLinks, allTasks)

	assert.Len(t, deps, 1)
	assert.Contains(t, deps, "KB-2")
}
