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
	Title        string   `yaml:"title"`
	JiraNumber   string   `yaml:"jira-number"`
	CreatedDate  string   `yaml:"created-date"`
	StartDate    string   `yaml:"start-date"`
	EndDate      string   `yaml:"end-date"`
	JiraURL      string   `yaml:"jira-url"`
	SyncStatus   string   `yaml:"sync-status"`
	Parent       string   `yaml:"parent"`
	Dependencies []string `yaml:"dependencies"`
	ContentHash  string   `yaml:"content-hash"`
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
