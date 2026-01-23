package filesystem

import (
	"testing"
)

// FuzzParser_Parse tests the parser with random content.
// It verifies the parser doesn't panic on any input and properly
// returns errors for malformed content.
func FuzzParser_Parse(f *testing.F) {
	parser := NewParser()

	// Add seed corpus with valid and edge-case inputs
	seeds := []string{
		// Valid minimal frontmatter
		"---\ntitle: Test\njira-parent: GUARD-100\n---\nDescription",
		// Valid with all fields
		`---
title: "KB-1: Test Task"
jira-number: "GUARD-101"
created-date: "2026-01-16"
start-date: "2026-01-16"
end-date: "2026-01-23"
jira-url: "https://company.atlassian.net/browse/GUARD-101"
sync-status: pending
jira-parent: GUARD-100
jira-dependencies: []
content-hash: "abc123"
---

Task description here.`,
		// With dependencies
		`---
title: "ERR-2: Implement Detection"
jira-parent: GUARD-100
jira-dependencies:
  - KB-3
  - ERR-1
---

Description`,
		// Empty body
		"---\ntitle: Test\njira-parent: GUARD-100\n---\n",
		// Missing title (should error)
		"---\njira-parent: GUARD-100\n---\nDescription",
		// Missing parent (should error)
		"---\ntitle: Test\n---\nDescription",
		// Missing end delimiter
		"---\ntitle: Test\njira-parent: GUARD-100",
		// No frontmatter at all
		"Just plain text",
		// Empty string
		"",
		// Only delimiters
		"---\n---",
		// Malformed YAML
		"---\ntitle: [invalid\njira-parent: GUARD-100\n---\n",
		// Unicode content
		"---\ntitle: \"Test 日本語 émoji 🎉\"\njira-parent: GUARD-100\n---\nDescription with émojis 🚀",
		// Very long title
		"---\ntitle: \"" + string(make([]byte, 1000)) + "\"\njira-parent: GUARD-100\n---\n",
		// Special YAML characters
		"---\ntitle: \"Test: with: colons\"\njira-parent: \"GUARD-100\"\n---\n",
		// Newlines in values
		"---\ntitle: |\n  Multi\n  Line\njira-parent: GUARD-100\n---\n",
		// Tabs and spaces
		"---\n\ttitle: Test\n  jira-parent: GUARD-100\n---\n",
		// Windows line endings
		"---\r\ntitle: Test\r\njira-parent: GUARD-100\r\n---\r\nDescription",
		// Multiple --- in body
		"---\ntitle: Test\njira-parent: GUARD-100\n---\nBody with --- dashes",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content string) {
		// The parser should never panic, regardless of input
		task, err := parser.Parse("fuzz.md", content)

		// If no error, task should be valid
		if err == nil {
			if task == nil {
				t.Error("Parse returned nil task without error")
				return
			}
			if task.Path != "fuzz.md" {
				t.Errorf("Expected path 'fuzz.md', got %q", task.Path)
			}
			// Title and parent are required
			if task.Frontmatter.Title == "" {
				t.Error("Parse returned empty title without error")
			}
			if task.Frontmatter.JiraParent == "" {
				t.Error("Parse returned empty parent without error")
			}
		}
	})
}

// FuzzParser_RoundTrip tests that valid content can be parsed,
// marshaled, and parsed again to produce the same result.
func FuzzParser_RoundTrip(f *testing.F) {
	parser := NewParser()
	writer := NewWriter()

	// Add seeds that are known to produce valid TaskFiles
	validSeeds := []string{
		`---
title: "KB-1: Test Task"
jira-number: ""
created-date: "2026-01-16"
start-date: ""
end-date: ""
jira-url: ""
sync-status: pending
jira-parent: GUARD-100
jira-dependencies: []
content-hash: ""
---

Task description here.`,
		`---
title: "ERR-2: Implement Detection"
jira-number: "GUARD-102"
created-date: "2026-01-16"
start-date: "2026-01-16"
end-date: "2026-01-23"
jira-url: "https://company.atlassian.net/browse/GUARD-102"
sync-status: linked
jira-parent: GUARD-100
jira-dependencies:
  - KB-3
  - ERR-1
content-hash: "abc123"
---

Implement detection.`,
	}

	for _, seed := range validSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content string) {
		// First parse
		task1, err := parser.Parse("test.md", content)
		if err != nil {
			return // Skip invalid input
		}

		// Marshal back to content
		marshaled, err := writer.Marshal(task1)
		if err != nil {
			t.Fatalf("Marshal failed for valid task: %v", err)
		}

		// Parse again
		task2, err := parser.Parse("test.md", marshaled)
		if err != nil {
			t.Fatalf("Parse of marshaled content failed: %v", err)
		}

		// Compare key fields (some fields may have minor formatting differences)
		if task1.Frontmatter.Title != task2.Frontmatter.Title {
			t.Errorf("Title mismatch: %q vs %q", task1.Frontmatter.Title, task2.Frontmatter.Title)
		}
		if task1.Frontmatter.JiraParent != task2.Frontmatter.JiraParent {
			t.Errorf("Parent mismatch: %q vs %q", task1.Frontmatter.JiraParent, task2.Frontmatter.JiraParent)
		}
		if task1.Frontmatter.JiraNumber != task2.Frontmatter.JiraNumber {
			t.Errorf("JiraNumber mismatch: %q vs %q", task1.Frontmatter.JiraNumber, task2.Frontmatter.JiraNumber)
		}
		if task1.Frontmatter.SyncStatus != task2.Frontmatter.SyncStatus {
			t.Errorf("SyncStatus mismatch: %q vs %q", task1.Frontmatter.SyncStatus, task2.Frontmatter.SyncStatus)
		}

		// Check dependencies length match
		if len(task1.Frontmatter.JiraDependencies) != len(task2.Frontmatter.JiraDependencies) {
			t.Errorf("Dependencies length mismatch: %d vs %d",
				len(task1.Frontmatter.JiraDependencies), len(task2.Frontmatter.JiraDependencies))
		}
	})
}
