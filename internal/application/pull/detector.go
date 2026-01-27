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
// Returns the list of dependency task IDs (or Jira keys if no local task exists).
func (d *DependencyDetector) ExtractDependencies(
	task *domain.TaskFile,
	jiraLinks []ports.IssueLink,
	allTasks []*domain.TaskFile,
) []string {
	slog.Debug("extracting dependencies",
		slog.String("task", task.TaskID()),
		slog.String("jira_key", task.Frontmatter.JiraNumber),
		slog.String("link_type", d.linkType),
		slog.Int("link_count", len(jiraLinks)),
	)

	// Build mapping from Jira key to task ID
	jiraKeyToTaskID := make(map[string]string)
	for _, t := range allTasks {
		taskID := t.TaskID()
		if taskID != "" && t.Frontmatter.JiraNumber != "" {
			jiraKeyToTaskID[t.Frontmatter.JiraNumber] = taskID
		}
	}
	slog.Debug("built jira key to task id mapping", slog.Int("mapping_count", len(jiraKeyToTaskID)))

	// Extract dependency links - any link of the configured type with an InwardIssue
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
			// Convert Jira key to task ID, or use Jira key directly if no local task
			if taskID, ok := jiraKeyToTaskID[link.InwardIssue]; ok {
				slog.Debug("dependency link matched - mapped to task id",
					slog.String("task", task.TaskID()),
					slog.String("jira_key", link.InwardIssue),
					slog.String("mapped_task_id", taskID),
				)
				deps = append(deps, taskID)
			} else {
				// No local task for this Jira issue - store Jira key directly
				slog.Debug("dependency link matched - no local task, using jira key",
					slog.String("task", task.TaskID()),
					slog.String("jira_key", link.InwardIssue),
				)
				deps = append(deps, link.InwardIssue)
			}
		}
	}

	slog.Debug("dependency extraction complete",
		slog.String("task", task.TaskID()),
		slog.Any("deps", deps),
	)

	return deps
}
