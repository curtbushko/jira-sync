// Package filesystem provides file system adapters for task files.
package filesystem

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/curtbushko/jira-sync/internal/domain"
)

// Parser parses markdown files with YAML frontmatter.
type Parser struct{}

// NewParser creates a new Parser.
func NewParser() *Parser {
	return &Parser{}
}

// frontmatterWithLegacy includes legacy fields for backward compatibility parsing.
type frontmatterWithLegacy struct {
	domain.Frontmatter `yaml:",inline"`
	// Legacy field - maps to JiraIsBlockedBy
	JiraDependencies []string `yaml:"jira-dependencies"`
}

// Parse parses a markdown file content and returns a TaskFile.
func (p *Parser) Parse(path, content string) (*domain.TaskFile, error) {
	frontmatter, body, err := p.splitFrontmatter(content)
	if err != nil {
		return nil, domain.NewParseError(path, err)
	}

	var legacyData frontmatterWithLegacy
	if err := yaml.Unmarshal([]byte(frontmatter), &legacyData); err != nil {
		return nil, domain.NewParseError(path, fmt.Errorf("parse yaml: %w", err))
	}

	// Migrate legacy jira-dependencies to jira-is-blocked-by
	if len(legacyData.JiraDependencies) > 0 && len(legacyData.JiraIsBlockedBy) == 0 {
		legacyData.JiraIsBlockedBy = legacyData.JiraDependencies
	}

	if err := p.validate(&legacyData.Frontmatter); err != nil {
		return nil, domain.NewParseError(path, err)
	}

	task := &domain.TaskFile{
		Path:        path,
		Frontmatter: legacyData.Frontmatter,
		Description: strings.TrimSpace(body),
	}

	// Migrate frontmatter to add any missing fields with defaults
	task.MigrateFrontmatter()

	return task, nil
}

// splitFrontmatter splits content into frontmatter and body.
// Frontmatter is delimited by "---" at the start and end.
func (p *Parser) splitFrontmatter(content string) (string, string, error) {
	content = strings.TrimSpace(content)

	if !strings.HasPrefix(content, "---") {
		return "", "", domain.ErrInvalidFrontmatter
	}

	// Find the second "---" delimiter
	rest := content[3:] // Skip first "---"
	rest = strings.TrimPrefix(rest, "\n")

	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return "", "", domain.ErrInvalidFrontmatter
	}

	frontmatter := rest[:idx]
	body := strings.TrimPrefix(rest[idx+4:], "\n") // Skip "\n---"

	return frontmatter, body, nil
}

// validate checks that required fields are present.
func (p *Parser) validate(fm *domain.Frontmatter) error {
	if fm.Title == "" {
		return domain.NewValidationError("title", "is required")
	}
	// jira-parent is required for all types except Epic
	if fm.JiraParent == "" && !strings.EqualFold(fm.JiraType, "Epic") {
		return domain.NewValidationError("jira-parent", "is required")
	}
	return nil
}
