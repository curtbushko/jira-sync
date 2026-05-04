package pull

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

func TestExtractBlockingRelationships_BothDirections(t *testing.T) {
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:      "KB-1: Test Task",
			JiraNumber: "GUARD-123",
		},
	}

	jiraLinks := []ports.IssueLink{
		{Type: "Blocking", InwardIssue: "", OutwardIssue: "GUARD-101"}, // We block GUARD-101
		{Type: "Blocking", InwardIssue: "GUARD-200", OutwardIssue: ""}, // We are blocked by GUARD-200
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
		{Type: "Blocking", InwardIssue: "", OutwardIssue: "GUARD-101"},
		{Type: "Blocking", InwardIssue: "", OutwardIssue: "GUARD-102"},
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
		{Type: "Blocking", InwardIssue: "GUARD-200", OutwardIssue: ""},
		{Type: "Blocking", InwardIssue: "GUARD-201", OutwardIssue: ""},
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
		{Type: "Blocking", InwardIssue: "", OutwardIssue: "GUARD-101"},
		{Type: "Relates", InwardIssue: "", OutwardIssue: "GUARD-102"}, // Different type - ignored
		{Type: "Blocking", InwardIssue: "GUARD-200", OutwardIssue: ""},
		{Type: "Clones", InwardIssue: "GUARD-201", OutwardIssue: ""}, // Different type - ignored
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
		{Type: "CustomBlocks", InwardIssue: "", OutwardIssue: "GUARD-101"},
		{Type: "Blocking", InwardIssue: "", OutwardIssue: "GUARD-102"}, // Wrong type
		{Type: "CustomBlocks", InwardIssue: "GUARD-200", OutwardIssue: ""},
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
		{Type: "Blocking", InwardIssue: "", OutwardIssue: "GUARD-101"},
	}

	// Empty string should use default "Blocking"
	detector := NewDependencyDetector("")
	blocks, blockedBy := detector.ExtractBlockingRelationships(task, jiraLinks)

	assert.Len(t, blocks, 1)
	assert.Contains(t, blocks, "GUARD-101")
	assert.Empty(t, blockedBy)
}
