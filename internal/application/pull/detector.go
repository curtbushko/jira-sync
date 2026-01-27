// Package pull provides pull-only sync from Jira to local files.
package pull

import (
	"log/slog"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// LinkTypeBlocking is the default Jira link type for blocking relationships.
const LinkTypeBlocking = "Blocking"

// DependencyDetector extracts dependencies from Jira issue links.
type DependencyDetector struct {
	linkType string // Jira link type for dependencies (e.g., "Blocks", "Blocking")
}

// NewDependencyDetector creates a new dependency detector.
func NewDependencyDetector(linkType string) *DependencyDetector {
	if linkType == "" {
		linkType = LinkTypeBlocking
	}
	slog.Debug("creating dependency detector", slog.String("link_type", linkType))
	return &DependencyDetector{linkType: linkType}
}

// ExtractDependencies extracts dependencies from Jira issue links.
// Returns the list of Jira issue keys that this task depends on.
func (d *DependencyDetector) ExtractDependencies(
	task *domain.TaskFile,
	jiraLinks []ports.IssueLink,
) []string {
	slog.Debug("extracting dependencies",
		slog.String("task", task.TaskID()),
		slog.String("jira_key", task.Frontmatter.JiraNumber),
		slog.String("link_type", d.linkType),
		slog.Int("link_count", len(jiraLinks)),
	)

	var deps []string

	for i, link := range jiraLinks {
		slog.Debug("examining jira link",
			slog.String("task", task.TaskID()),
			slog.Int("link_index", i),
			slog.String("type", link.Type),
			slog.String("inward_issue", link.InwardIssue),
			slog.String("outward_issue", link.OutwardIssue),
		)

		if link.Type == d.linkType && link.InwardIssue != "" {
			slog.Debug("dependency link matched",
				slog.String("task", task.TaskID()),
				slog.String("dependency", link.InwardIssue),
			)
			deps = append(deps, link.InwardIssue)
		}
	}

	slog.Debug("dependency extraction complete",
		slog.String("task", task.TaskID()),
		slog.Any("deps", deps),
	)

	return deps
}
