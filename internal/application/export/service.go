// Package export provides functionality for exporting Jira issues to local task files.
package export

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// Options holds export configuration.
type Options struct {
	ParentOverride string // Override jira-parent value
}

// Result holds the export result.
type Result struct {
	Task     *domain.TaskFile
	Filename string
}

// Service handles exporting Jira issues to task files.
type Service struct {
	jira          ports.JiraClient
	hasher        ports.HashComputer
	existingTasks []*domain.TaskFile
	taskByJiraKey map[string]*domain.TaskFile // Lookup map for O(1) access
}

// NewService creates a new export service.
func NewService(jira ports.JiraClient, hasher ports.HashComputer, existingTasks []*domain.TaskFile) *Service {
	// Build lookup map for existing tasks
	taskByJiraKey := make(map[string]*domain.TaskFile)
	for _, task := range existingTasks {
		if task.Frontmatter.JiraNumber != "" {
			taskByJiraKey[task.Frontmatter.JiraNumber] = task
		}
	}

	return &Service{
		jira:          jira,
		hasher:        hasher,
		existingTasks: existingTasks,
		taskByJiraKey: taskByJiraKey,
	}
}

// Export fetches a Jira issue and converts it to a TaskFile.
func (s *Service) Export(ctx context.Context, issueKey string, opts Options) (*Result, error) {
	// Fetch issue with links
	issue, err := s.jira.GetIssueWithLinks(ctx, issueKey)
	if err != nil {
		return nil, fmt.Errorf("fetch issue: %w", err)
	}

	// Parse creation date for filename
	createdTime, err := parseJiraDatetime(issue.Created)
	if err != nil {
		return nil, fmt.Errorf("parse creation date: %w", err)
	}

	// Generate filename from creation date
	filename := createdTime.Format("20060102-150405") + ".md"

	// Extract dependencies from issue links
	deps := s.extractDependencies(issue.Links)

	// Determine parent
	parent := issue.Parent
	if opts.ParentOverride != "" {
		parent = opts.ParentOverride
	}

	// Build task file
	task := &domain.TaskFile{
		Frontmatter: domain.Frontmatter{
			Title:            issue.Summary,
			JiraNumber:       issue.Key,
			JiraProject:      issue.Project,
			JiraState:        issue.Status,
			CreatedDate:      createdTime.Format("2006-01-02"),
			StartDate:        issue.StartDate,
			EndDate:          issue.EndDate,
			JiraURL:          issue.URL,
			SyncStatus:       domain.SyncStatusLinked,
			JiraParent:       parent,
			SyncDependencies: []string{},
			JiraDependencies: deps,
			LastSynced:       time.Now().UTC().Format(time.RFC3339),
		},
		Description: issue.Description,
	}

	// Compute content hash
	task.Frontmatter.ContentHash = s.hasher.ComputeHash(task)

	return &Result{
		Task:     task,
		Filename: filename,
	}, nil
}

// parseJiraDatetime parses Jira's datetime format.
func parseJiraDatetime(datetime string) (time.Time, error) {
	if datetime == "" {
		return time.Time{}, errors.New("empty datetime string")
	}

	// Jira formats to try in order of likelihood
	formats := []string{
		"2006-01-02T15:04:05.000-0700", // Standard Jira format
		"2006-01-02T15:04:05.000Z",     // UTC with Z suffix
		time.RFC3339,                   // Standard RFC3339
		time.RFC3339Nano,               // RFC3339 with nanoseconds
	}

	for _, format := range formats {
		if t, err := time.Parse(format, datetime); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse datetime: %s", datetime)
}

// extractDependencies extracts blocking dependencies from issue links.
// Only considers "Blocks" links where this issue is blocked by another.
func (s *Service) extractDependencies(links []ports.IssueLink) []string {
	if links == nil {
		return []string{}
	}

	var deps []string
	for _, link := range links {
		// Only consider "Blocks" links where this issue is blocked (InwardIssue is set)
		if link.Type == "Blocks" && link.InwardIssue != "" {
			// Try to map to wiki link format
			wikiLink := s.mapToWikiLink(link.InwardIssue)
			deps = append(deps, wikiLink)
		}
	}

	if deps == nil {
		return []string{}
	}

	return deps
}

// mapToWikiLink maps a Jira key to wiki link format if task exists locally.
func (s *Service) mapToWikiLink(jiraKey string) string {
	if task, ok := s.taskByJiraKey[jiraKey]; ok {
		// Found matching task, create wiki link
		return fmt.Sprintf("[%s](%s)", task.Frontmatter.Title, filepath.Base(task.Path))
	}

	// Not found locally, return plain Jira key
	return jiraKey
}
