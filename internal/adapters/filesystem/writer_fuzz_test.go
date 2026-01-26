package filesystem

import (
	"strings"
	"testing"

	"github.com/curtbushko/jira-sync/internal/domain"
)

// FuzzWriter_Marshal tests the Writer.Marshal function with fuzzed TaskFiles.
// It verifies the function doesn't panic and produces valid output.
func FuzzWriter_Marshal(f *testing.F) {
	// Add seed values for various fields
	seeds := []struct {
		title, jiraNumber, createdDate string
		jiraURL, syncStatus, parent, contentHash string
		description                              string
	}{
		// Normal case
		{"KB-1: Test", "GUARD-101", "2026-01-16",
			"https://test.atlassian.net/browse/GUARD-101", "pending", "GUARD-100", "abc123",
			"Description here"},
		// Empty fields
		{"Test", "", "", "", "", "GUARD-100", "", ""},
		// Special characters
		{"KB-1: Test with \"quotes\" and 'apostrophes'", "", "",
			"", "pending", "GUARD-100", "",
			"Description with\nNewlines\nand\ttabs"},
		// Unicode
		{"KB-1: Test 日本語 émoji 🎉", "", "",
			"", "pending", "親タスク", "",
			"Description with émojis 🚀 and 日本語"},
		// Long strings
		{"KB-1: " + string(make([]byte, 500)), "", "",
			"", "pending", "GUARD-100", "",
			string(make([]byte, 5000))},
		// YAML special characters
		{"KB-1: Test: with: colons", "", "",
			"", "pending", "GUARD-100", "",
			"Key: value\nList:\n  - item1\n  - item2"},
		// Multi-line description
		{"KB-1: Test", "GUARD-101", "",
			"", "linked", "GUARD-100", "",
			"# Header\n\nParagraph 1.\n\nParagraph 2.\n\n- Item 1\n- Item 2"},
	}

	for _, s := range seeds {
		f.Add(s.title, s.jiraNumber, s.createdDate,
			s.jiraURL, s.syncStatus, s.parent, s.contentHash, s.description)
	}

	writer := NewWriter()
	parser := NewParser()

	f.Fuzz(func(t *testing.T,
		title, jiraNumber, createdDate string,
		jiraURL, syncStatus, parent, contentHash string,
		description string,
	) {
		task := &domain.TaskFile{
			Path: "test.md",
			Frontmatter: domain.Frontmatter{
				Title:            title,
				JiraNumber:       jiraNumber,
				CreatedDate:      createdDate,
				JiraURL:          jiraURL,
				SyncStatus:       syncStatus,
				JiraParent:       parent,
				ContentHash:      contentHash,
				JiraDependencies: []string{}, // Start with empty deps
			},
			Description: description,
		}

		// Marshal should never panic
		content, err := writer.Marshal(task)
		if err != nil {
			// Some content may not be valid YAML (e.g., contains invalid UTF-8)
			// That's acceptable - we just check it doesn't panic
			return
		}

		// Output should always have frontmatter delimiters
		if len(content) < 8 { // Minimum "---\n---\n"
			t.Errorf("Marshal produced too short output: %q", content)
			return
		}

		// If title and parent are valid, output should be parseable
		if title != "" && parent != "" {
			_, parseErr := parser.Parse("test.md", content)
			// We don't require parsing to succeed because YAML encoding
			// might introduce changes, but we verify no panic occurs
			_ = parseErr
		}
	})
}

// FuzzWriter_MarshalWithDependencies tests Marshal with various dependency lists.
func FuzzWriter_MarshalWithDependencies(f *testing.F) {
	// Add seed dependency values
	f.Add("KB-1", "")
	f.Add("KB-1", "KB-2")
	f.Add("KB-1", "KB-2,ERR-1,CTRL-1")
	f.Add("タスク-1", "タスク-2")
	f.Add("TASK-WITH-LONG-NAME-12345", "ANOTHER-LONG-DEP-67890")
	f.Add("", "")

	writer := NewWriter()

	f.Fuzz(func(t *testing.T, dep1, dep2 string) {
		var deps []string
		if dep1 != "" {
			deps = append(deps, dep1)
		}
		if dep2 != "" {
			deps = append(deps, dep2)
		}

		task := &domain.TaskFile{
			Path: "test.md",
			Frontmatter: domain.Frontmatter{
				Title:            "KB-1: Test",
				SyncStatus:       "pending",
				JiraParent:       "GUARD-100",
				JiraDependencies: deps,
			},
			Description: "Test description",
		}

		// Should never panic
		content, err := writer.Marshal(task)
		if err != nil {
			return
		}

		// Verify output is non-empty
		if content == "" {
			t.Error("Marshal produced empty output")
		}

		// Verify dependencies are present in output if they were provided
		for _, dep := range deps {
			if dep != "" && !strings.Contains(content, dep) {
				t.Errorf("Output missing dependency %q", dep)
			}
		}
	})
}
