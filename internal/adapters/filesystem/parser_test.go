package filesystem

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_Parse_ValidFile(t *testing.T) {
	content := `---
title: "KB-1: Test Task"
jira-number: ""
created-date: "2026-01-16"
start-date: ""
end-date: ""
jira-url: ""
sync-status: pending
parent: GUARD-100
dependencies: []
content-hash: ""
---

Task description here.`

	parser := NewParser()
	task, err := parser.Parse("test.md", content)

	require.NoError(t, err)
	assert.Equal(t, "test.md", task.Path)
	assert.Equal(t, "KB-1: Test Task", task.Frontmatter.Title)
	assert.Equal(t, "pending", task.Frontmatter.SyncStatus)
	assert.Equal(t, "GUARD-100", task.Frontmatter.Parent)
	assert.Equal(t, "Task description here.", task.Description)
}

func TestParser_Parse_WithDependencies(t *testing.T) {
	content := `---
title: "ERR-2: Implement Detection"
jira-number: "GUARD-102"
created-date: "2026-01-16"
start-date: "2026-01-16"
end-date: "2026-01-23"
jira-url: "https://company.atlassian.net/browse/GUARD-102"
sync-status: linked
parent: GUARD-100
dependencies:
  - KB-3
  - ERR-1
content-hash: "abc123"
---

Implement detection of replica failures.

## Acceptance Criteria

- Check if ReplicaFailure detection is enabled
- Read deployment.Status.UnavailableReplicas`

	parser := NewParser()
	task, err := parser.Parse("test.md", content)

	require.NoError(t, err)
	assert.Equal(t, "ERR-2: Implement Detection", task.Frontmatter.Title)
	assert.Equal(t, "GUARD-102", task.Frontmatter.JiraNumber)
	assert.Equal(t, "linked", task.Frontmatter.SyncStatus)
	assert.Equal(t, []string{"KB-3", "ERR-1"}, task.Frontmatter.Dependencies)
	assert.Contains(t, task.Description, "Implement detection of replica failures.")
	assert.Contains(t, task.Description, "## Acceptance Criteria")
}

func TestParser_Parse_InvalidFrontmatter(t *testing.T) {
	content := `not valid frontmatter`

	parser := NewParser()
	_, err := parser.Parse("test.md", content)

	assert.Error(t, err)
}

func TestParser_Parse_MissingEndDelimiter(t *testing.T) {
	content := `---
title: "Test"
sync-status: pending
parent: GUARD-100`

	parser := NewParser()
	_, err := parser.Parse("test.md", content)

	assert.Error(t, err)
}

func TestParser_Parse_MissingTitle(t *testing.T) {
	content := `---
title: ""
sync-status: pending
parent: GUARD-100
dependencies: []
content-hash: ""
---

Description`

	parser := NewParser()
	_, err := parser.Parse("test.md", content)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title")
}

func TestParser_Parse_MissingParent(t *testing.T) {
	content := `---
title: "KB-1: Test"
sync-status: pending
parent: ""
dependencies: []
content-hash: ""
---

Description`

	parser := NewParser()
	_, err := parser.Parse("test.md", content)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parent")
}

func TestParser_Parse_EmptyDescription(t *testing.T) {
	content := `---
title: "KB-1: Test"
sync-status: pending
parent: GUARD-100
dependencies: []
content-hash: ""
---
`

	parser := NewParser()
	task, err := parser.Parse("test.md", content)

	require.NoError(t, err)
	assert.Equal(t, "", task.Description)
}

func TestParser_Parse_PreservesWhitespace(t *testing.T) {
	content := `---
title: "KB-1: Test"
sync-status: pending
parent: GUARD-100
dependencies: []
content-hash: ""
---

First paragraph.

Second paragraph.

- List item 1
- List item 2`

	parser := NewParser()
	task, err := parser.Parse("test.md", content)

	require.NoError(t, err)
	assert.Contains(t, task.Description, "First paragraph.")
	assert.Contains(t, task.Description, "Second paragraph.")
	assert.Contains(t, task.Description, "- List item 1")
}
