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

	jiraLinks := []ports.IssueLink{
		{Type: "Blocking", InwardIssue: "GUARD-101", OutwardIssue: ""},
		{Type: "Blocking", InwardIssue: "GUARD-102", OutwardIssue: ""},
	}

	detector := NewDependencyDetector("Blocking")
	deps := detector.ExtractDependencies(task, jiraLinks)

	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "GUARD-101")
	assert.Contains(t, deps, "GUARD-102")
}

func TestExtractDependencies_NoMatches(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	jiraLinks := []ports.IssueLink{}

	detector := NewDependencyDetector("Blocking")
	deps := detector.ExtractDependencies(task, jiraLinks)

	assert.Empty(t, deps)
}

func TestExtractDependencies_IgnoresOtherLinkTypes(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocking", InwardIssue: "GUARD-101", OutwardIssue: ""},
		{Type: "Relates", InwardIssue: "GUARD-102", OutwardIssue: ""}, // Different type - ignored
	}

	detector := NewDependencyDetector("Blocking")
	deps := detector.ExtractDependencies(task, jiraLinks)

	assert.Len(t, deps, 1)
	assert.Contains(t, deps, "GUARD-101")
}

func TestExtractDependencies_UsesJiraKey(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocking", InwardIssue: "GUARD-999", OutwardIssue: ""},
	}

	detector := NewDependencyDetector("Blocking")
	deps := detector.ExtractDependencies(task, jiraLinks)

	// Should always use Jira key
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

	jiraLinks := []ports.IssueLink{
		{Type: "CustomBlocks", InwardIssue: "GUARD-101", OutwardIssue: ""},
		{Type: "Blocking", InwardIssue: "GUARD-102", OutwardIssue: ""}, // Wrong type
	}

	detector := NewDependencyDetector("CustomBlocks")
	deps := detector.ExtractDependencies(task, jiraLinks)

	assert.Len(t, deps, 1)
	assert.Contains(t, deps, "GUARD-101")
}
