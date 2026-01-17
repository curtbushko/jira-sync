package hashing

import (
	"testing"

	"github.com/curtbushko/jira-sync/internal/domain"
)

// FuzzSHA256HashComputer_ComputeHash tests the hash computation with fuzzed input.
// It verifies the function doesn't panic and produces consistent, valid hashes.
func FuzzSHA256HashComputer_ComputeHash(f *testing.F) {
	// Add seed values for various task content
	seeds := []struct {
		title, parent, description string
	}{
		// Normal case
		{"KB-1: Test Task", "GUARD-100", "Description here"},
		// Empty fields
		{"", "", ""},
		// Unicode content
		{"KB-1: Test 日本語 émoji 🎉", "親タスク", "Description with émojis 🚀"},
		// Long content
		{string(make([]byte, 1000)), "GUARD-100", string(make([]byte, 10000))},
		// Special characters
		{"KB-1: Test\x00with\x00nulls", "GUARD-100", "Desc\x00ription"},
		// Newlines and tabs
		{"KB-1: Test\nwith\nnewlines", "GUARD\t100", "Desc\r\nription"},
		// Whitespace
		{"  KB-1: Test  ", "  GUARD-100  ", "  Description  "},
		// YAML special chars
		{"KB-1: Title: with: colons", "GUARD-100", "key: value\n- item"},
	}

	for _, s := range seeds {
		f.Add(s.title, s.parent, s.description)
	}

	hasher := NewSHA256HashComputer()

	f.Fuzz(func(t *testing.T, title, parent, description string) {
		task := &domain.TaskFile{
			Path: "test.md",
			Frontmatter: domain.Frontmatter{
				Title:        title,
				Parent:       parent,
				Dependencies: nil,
			},
			Description: description,
		}

		// ComputeHash should never panic
		hash1 := hasher.ComputeHash(task)

		// Hash should always be 64 characters (SHA256 hex)
		if len(hash1) != 64 {
			t.Errorf("Hash length = %d, expected 64", len(hash1))
		}

		// Hash should be valid hex
		for _, c := range hash1 {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("Invalid hex character in hash: %c", c)
				break
			}
		}

		// Hash should be deterministic - same input = same output
		hash2 := hasher.ComputeHash(task)
		if hash1 != hash2 {
			t.Errorf("Hash not deterministic: %q != %q", hash1, hash2)
		}

		// Different content should (almost always) produce different hash
		// This is a weak test but catches obvious issues
		differentTask := &domain.TaskFile{
			Path: "test.md",
			Frontmatter: domain.Frontmatter{
				Title:        title + "X",
				Parent:       parent,
				Dependencies: nil,
			},
			Description: description,
		}
		hash3 := hasher.ComputeHash(differentTask)
		if hash1 == hash3 && title != "" {
			// Only report if titles were different but hashes match
			// (empty title + "X" == "X" could theoretically collide)
			t.Logf("Possible hash collision: %q and %q both hash to %s",
				title, title+"X", hash1)
		}
	})
}

// FuzzSHA256HashComputer_WithDependencies tests hash computation with various dependency lists.
func FuzzSHA256HashComputer_WithDependencies(f *testing.F) {
	// Add seeds
	f.Add("KB-1: Test", "GUARD-100", "Desc", "DEP-1", "DEP-2")
	f.Add("KB-1: Test", "GUARD-100", "Desc", "", "")
	f.Add("", "", "", "A", "B")
	f.Add("KB-1: Test", "GUARD-100", "Desc", "タスク-1", "タスク-2")
	f.Add("KB-1: Test", "GUARD-100", "Desc", "DEP-1", "DEP-1") // Duplicate deps

	hasher := NewSHA256HashComputer()

	f.Fuzz(func(t *testing.T, title, parent, description, dep1, dep2 string) {
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
				Title:        title,
				Parent:       parent,
				Dependencies: deps,
			},
			Description: description,
		}

		// Should never panic
		hash := hasher.ComputeHash(task)

		// Hash should always be 64 characters
		if len(hash) != 64 {
			t.Errorf("Hash length = %d, expected 64", len(hash))
		}

		// Verify determinism
		hash2 := hasher.ComputeHash(task)
		if hash != hash2 {
			t.Errorf("Hash not deterministic")
		}

		// Verify dependency order matters
		if len(deps) >= 2 && dep1 != dep2 {
			reversedDeps := make([]string, len(deps))
			for i, d := range deps {
				reversedDeps[len(deps)-1-i] = d
			}

			reversedTask := &domain.TaskFile{
				Path: "test.md",
				Frontmatter: domain.Frontmatter{
					Title:        title,
					Parent:       parent,
					Dependencies: reversedDeps,
				},
				Description: description,
			}

			reversedHash := hasher.ComputeHash(reversedTask)
			if hash == reversedHash {
				t.Log("Hash same for different dependency order - this may be intentional")
			}
		}
	})
}

// FuzzSHA256HashComputer_NilDependencies tests handling of nil vs empty dependencies.
func FuzzSHA256HashComputer_NilDependencies(f *testing.F) {
	f.Add("KB-1: Test", "GUARD-100", "Description")

	hasher := NewSHA256HashComputer()

	f.Fuzz(func(t *testing.T, title, parent, description string) {
		// Task with nil dependencies
		taskNil := &domain.TaskFile{
			Path: "test.md",
			Frontmatter: domain.Frontmatter{
				Title:        title,
				Parent:       parent,
				Dependencies: nil,
			},
			Description: description,
		}

		// Task with empty slice dependencies
		taskEmpty := &domain.TaskFile{
			Path: "test.md",
			Frontmatter: domain.Frontmatter{
				Title:        title,
				Parent:       parent,
				Dependencies: []string{},
			},
			Description: description,
		}

		hashNil := hasher.ComputeHash(taskNil)
		hashEmpty := hasher.ComputeHash(taskEmpty)

		// Nil and empty should produce the same hash
		if hashNil != hashEmpty {
			t.Errorf("Nil vs empty deps produce different hashes: %q vs %q", hashNil, hashEmpty)
		}
	})
}
