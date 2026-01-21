package filesystem

import (
	"bytes"
	"fmt"

	"github.com/curtbushko/jira-sync/internal/domain"
	"gopkg.in/yaml.v3"
)

// Writer marshals TaskFile to markdown with YAML frontmatter.
type Writer struct{}

// NewWriter creates a new Writer.
func NewWriter() *Writer {
	return &Writer{}
}

// frontmatterOutput is the struct used for YAML output with explicit field ordering.
type frontmatterOutput struct {
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
	SyncDependencies []string `yaml:"sync-dependencies,flow"`
	JiraDependencies []string `yaml:"jira-dependencies,flow"`
	ContentHash      string   `yaml:"content-hash"`
	LastSynced       string   `yaml:"last-synced"`
}

// Marshal converts a TaskFile to markdown content with YAML frontmatter.
func (w *Writer) Marshal(task *domain.TaskFile) (string, error) {
	// Ensure dependencies are never nil for consistent output
	syncDeps := task.Frontmatter.SyncDependencies
	if syncDeps == nil {
		syncDeps = []string{}
	}
	jiraDeps := task.Frontmatter.JiraDependencies
	if jiraDeps == nil {
		jiraDeps = []string{}
	}

	// Create output struct with proper field ordering
	out := frontmatterOutput{
		Title:            task.Frontmatter.Title,
		JiraNumber:       task.Frontmatter.JiraNumber,
		JiraProject:      task.Frontmatter.JiraProject,
		JiraState:        task.Frontmatter.JiraState,
		CreatedDate:      task.Frontmatter.CreatedDate,
		StartDate:        task.Frontmatter.StartDate,
		EndDate:          task.Frontmatter.EndDate,
		JiraURL:          task.Frontmatter.JiraURL,
		SyncStatus:       task.Frontmatter.SyncStatus,
		JiraParent:       task.Frontmatter.JiraParent,
		SyncDependencies: syncDeps,
		JiraDependencies: jiraDeps,
		ContentHash:      task.Frontmatter.ContentHash,
		LastSynced:       task.Frontmatter.LastSynced,
	}

	var buf bytes.Buffer

	// Write opening delimiter
	buf.WriteString("---\n")

	// Marshal frontmatter
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&out); err != nil {
		return "", fmt.Errorf("marshal frontmatter: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("close encoder: %w", err)
	}

	// Write closing delimiter
	buf.WriteString("---\n")

	// Write description if present
	if task.Description != "" {
		buf.WriteString("\n")
		buf.WriteString(task.Description)
		buf.WriteString("\n")
	}

	return buf.String(), nil
}
