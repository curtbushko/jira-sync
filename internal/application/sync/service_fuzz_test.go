package sync

import (
	"testing"

	"github.com/curtbushko/jira-sync/internal/domain"
)

// FuzzExtractTaskID tests the extractTaskID function with random input.
// It verifies the function doesn't panic and returns valid results.
func FuzzExtractTaskID(f *testing.F) {
	// Add seed corpus with various title formats
	seeds := []string{
		"KB-1: Initialize Project",
		"ERR-10: Complex Detection",
		"CTRL-1: Controller Scaffold",
		"MET-12: Logging Integration",
		"HELM-14: Documentation",
		"Simple Task Without Prefix",
		"", // Empty string
		":", // Just colon
		"KB-1:", // ID with colon but no title
		":Description", // Colon at start
		"Multiple: Colons: Here",
		"TASK-123456789: Very Long ID",
		"task-1: lowercase",
		"TASK_1: With underscore",
		"タスク-1: Japanese",
		"ЗАДАЧА-1: Russian",
		"🎉-1: Emoji prefix",
		"A: Single char prefix",
		"   KB-1   : Spaces around",
		"KB-1:No space",
		"KB-1  :  Extra spaces",
		"\tKB-1:\tTabs",
		"KB-1\n: Newline",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, title string) {
		// extractTaskID should never panic
		result := extractTaskID(title)

		// Result should always be non-empty if title is non-empty
		// The function returns the full title if:
		// - no colon is found
		// - the part before the colon is empty/whitespace only
		if title != "" && result == "" {
			t.Errorf("extractTaskID(%q) returned empty string unexpectedly", title)
		}

		// Result should never be longer than input
		if len(result) > len(title) {
			t.Errorf("extractTaskID(%q) returned longer string %q", title, result)
		}
	})
}

// FuzzCategorizeTasks tests the CategorizeTasks function with fuzzed TaskFiles.
func FuzzCategorizeTasks(f *testing.F) {
	// Add seed status values
	f.Add("pending", "")
	f.Add("created", "GUARD-101")
	f.Add("linked", "GUARD-102")
	f.Add("unknown", "")
	f.Add("", "")
	f.Add("PENDING", "")
	f.Add("Linked", "GUARD-103")

	f.Fuzz(func(t *testing.T, status, jiraNumber string) {
		svc := NewService(nil, nil, nil)

		task := &domain.TaskFile{
			Frontmatter: domain.Frontmatter{
				Title:       "KB-1: Test",
				SyncStatus:  status,
				JiraNumber:  jiraNumber,
				Parent:      "GUARD-100",
				ContentHash: "",
			},
			Description: "Test description",
		}

		tasks := []*domain.TaskFile{task}

		// Should never panic
		result := svc.CategorizeTasks(tasks)

		// Result should always be non-nil
		if result == nil {
			t.Fatal("CategorizeTasks returned nil")
		}

		// Task should be categorized exactly once
		total := len(result.Pending) + len(result.Created) + len(result.Linked) + len(result.NeedsUpdate)
		if total != 1 {
			t.Errorf("Task was categorized %d times, expected 1", total)
		}
	})
}

// FuzzBuildTaskIDMap tests the BuildTaskIDMap function with various titles.
func FuzzBuildTaskIDMap(f *testing.F) {
	// Add seed titles
	f.Add("KB-1: First Task", "GUARD-101")
	f.Add("Simple Title", "GUARD-102")
	f.Add("", "GUARD-103")
	f.Add("Multiple: Colons: Here", "GUARD-104")
	f.Add("タスク-1: Japanese Title", "GUARD-105")

	f.Fuzz(func(t *testing.T, title, jiraNumber string) {
		svc := NewService(nil, nil, nil)

		tasks := []*domain.TaskFile{
			{
				Frontmatter: domain.Frontmatter{
					Title:      title,
					JiraNumber: jiraNumber,
				},
			},
		}

		// Should never panic
		idMap := svc.BuildTaskIDMap(tasks)

		// Map should have exactly one entry
		if len(idMap) != 1 {
			t.Errorf("BuildTaskIDMap returned %d entries, expected 1", len(idMap))
		}

		// The extracted ID should map to the jira number
		extractedID := extractTaskID(title)
		if idMap[extractedID] != jiraNumber {
			t.Errorf("idMap[%q] = %q, expected %q", extractedID, idMap[extractedID], jiraNumber)
		}
	})
}
