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
	Title           string   `yaml:"title"`
	JiraNumber      string   `yaml:"jira-number"`
	JiraProject     string   `yaml:"jira-project"`
	JiraType        string   `yaml:"jira-type"`
	JiraState       string   `yaml:"jira-state"`
	CreatedDate     string   `yaml:"created-date"`
	JiraURL         string   `yaml:"jira-url"`
	SyncStatus      string   `yaml:"sync-status"`
	JiraParent      string   `yaml:"jira-parent"`
	JiraBlocks      []string `yaml:"jira-blocks,flow"`
	JiraIsBlockedBy []string `yaml:"jira-is-blocked-by,flow"`
	ContentHash     string   `yaml:"content-hash"`
}

// Marshal converts a TaskFile to markdown content with YAML frontmatter.
func (w *Writer) Marshal(task *domain.TaskFile) (string, error) {
	// Ensure slices are never nil for consistent output
	jiraBlocks := task.Frontmatter.JiraBlocks
	if jiraBlocks == nil {
		jiraBlocks = []string{}
	}

	jiraIsBlockedBy := task.Frontmatter.JiraIsBlockedBy
	if jiraIsBlockedBy == nil {
		jiraIsBlockedBy = []string{}
	}

	// Create output struct with proper field ordering
	out := frontmatterOutput{
		Title:           task.Frontmatter.Title,
		JiraNumber:      task.Frontmatter.JiraNumber,
		JiraProject:     task.Frontmatter.JiraProject,
		JiraType:        task.Frontmatter.JiraType,
		JiraState:       task.Frontmatter.JiraState,
		CreatedDate:     task.Frontmatter.CreatedDate,
		JiraURL:         task.Frontmatter.JiraURL,
		SyncStatus:      task.Frontmatter.SyncStatus,
		JiraParent:      task.Frontmatter.JiraParent,
		JiraBlocks:      jiraBlocks,
		JiraIsBlockedBy: jiraIsBlockedBy,
		ContentHash:     task.Frontmatter.ContentHash,
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
