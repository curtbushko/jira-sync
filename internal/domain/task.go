// Package domain contains core domain types for jira-sync.
package domain

// TaskFile represents a markdown task file with frontmatter and description.
type TaskFile struct {
	Path        string
	Frontmatter Frontmatter
	Description string // Body content (becomes Jira description)
}

// Frontmatter contains the YAML frontmatter fields of a task file.
type Frontmatter struct {
	Title            string   `yaml:"title"`
	JiraNumber       string   `yaml:"jira-number"`
	JiraProject      string   `yaml:"jira-project"`
	JiraState        string   `yaml:"jira-state"`
	CreatedDate      string   `yaml:"created-date"`
	StartDate        string   `yaml:"start-date"`
	EndDate          string   `yaml:"end-date"`
	JiraURL          string   `yaml:"jira-url"`
	SyncStatus       string   `yaml:"sync-status"`
	JiraParent       string   `yaml:"jira-parent"`
	SyncDependencies []string `yaml:"sync-dependencies"`
	JiraDependencies []string `yaml:"jira-dependencies"`
	ContentHash      string   `yaml:"content-hash"`
	LastSynced       string   `yaml:"last-synced"`
}

// TaskID extracts the task ID prefix from the title (e.g., "KB-1" from "KB-1: Title").
func (t *TaskFile) TaskID() string {
	for i, c := range t.Frontmatter.Title {
		if c == ':' {
			return t.Frontmatter.Title[:i]
		}
	}
	return t.Frontmatter.Title
}
