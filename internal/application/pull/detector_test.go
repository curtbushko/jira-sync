package pull

import (
	"testing"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
	"github.com/stretchr/testify/assert"
)

func TestExtractBlockingRelationships_BothDirections(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocking", InwardIssue: "GUARD-101", OutwardIssue: ""},  // We block GUARD-101
		{Type: "Blocking", InwardIssue: "", OutwardIssue: "GUARD-200"}, // We are blocked by GUARD-200
	}

	detector := NewDependencyDetector("Blocking")
	blocks, blockedBy := detector.ExtractBlockingRelationships(task, jiraLinks)

	assert.Len(t, blocks, 1)
	assert.Contains(t, blocks, "GUARD-101")
	assert.Len(t, blockedBy, 1)
	assert.Contains(t, blockedBy, "GUARD-200")
}

func TestExtractBlockingRelationships_OnlyBlocks(t *testing.T) {
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
	blocks, blockedBy := detector.ExtractBlockingRelationships(task, jiraLinks)

	assert.Len(t, blocks, 2)
	assert.Contains(t, blocks, "GUARD-101")
	assert.Contains(t, blocks, "GUARD-102")
	assert.Empty(t, blockedBy)
}

func TestExtractBlockingRelationships_OnlyBlockedBy(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocking", InwardIssue: "", OutwardIssue: "GUARD-200"},
		{Type: "Blocking", InwardIssue: "", OutwardIssue: "GUARD-201"},
	}

	detector := NewDependencyDetector("Blocking")
	blocks, blockedBy := detector.ExtractBlockingRelationships(task, jiraLinks)

	assert.Empty(t, blocks)
	assert.Len(t, blockedBy, 2)
	assert.Contains(t, blockedBy, "GUARD-200")
	assert.Contains(t, blockedBy, "GUARD-201")
}

func TestExtractBlockingRelationships_NoLinks(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	jiraLinks := []ports.IssueLink{}

	detector := NewDependencyDetector("Blocking")
	blocks, blockedBy := detector.ExtractBlockingRelationships(task, jiraLinks)

	assert.Empty(t, blocks)
	assert.Empty(t, blockedBy)
}

func TestExtractBlockingRelationships_IgnoresOtherLinkTypes(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocking", InwardIssue: "GUARD-101", OutwardIssue: ""},
		{Type: "Relates", InwardIssue: "GUARD-102", OutwardIssue: ""},   // Different type - ignored
		{Type: "Blocking", InwardIssue: "", OutwardIssue: "GUARD-200"},
		{Type: "Clones", InwardIssue: "", OutwardIssue: "GUARD-201"},   // Different type - ignored
	}

	detector := NewDependencyDetector("Blocking")
	blocks, blockedBy := detector.ExtractBlockingRelationships(task, jiraLinks)

	assert.Len(t, blocks, 1)
	assert.Contains(t, blocks, "GUARD-101")
	assert.Len(t, blockedBy, 1)
	assert.Contains(t, blockedBy, "GUARD-200")
}

func TestExtractBlockingRelationships_CustomLinkType(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "CustomBlocks", InwardIssue: "GUARD-101", OutwardIssue: ""},
		{Type: "Blocking", InwardIssue: "GUARD-102", OutwardIssue: ""},       // Wrong type
		{Type: "CustomBlocks", InwardIssue: "", OutwardIssue: "GUARD-200"},
	}

	detector := NewDependencyDetector("CustomBlocks")
	blocks, blockedBy := detector.ExtractBlockingRelationships(task, jiraLinks)

	assert.Len(t, blocks, 1)
	assert.Contains(t, blocks, "GUARD-101")
	assert.Len(t, blockedBy, 1)
	assert.Contains(t, blockedBy, "GUARD-200")
}

func TestExtractBlockingRelationships_DefaultLinkType(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocking", InwardIssue: "GUARD-101", OutwardIssue: ""},
	}

	// Empty string should use default "Blocking"
	detector := NewDependencyDetector("")
	blocks, blockedBy := detector.ExtractBlockingRelationships(task, jiraLinks)

	assert.Len(t, blocks, 1)
	assert.Contains(t, blocks, "GUARD-101")
	assert.Empty(t, blockedBy)
}
