// Package pull provides pull-only sync from Jira to local files.
package pull

import (
	"log/slog"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// LinkTypeBlocking is the default Jira link type for blocking relationships.
const LinkTypeBlocking = "Blocking"

// DependencyDetector extracts blocking relationships from Jira issue links.
type DependencyDetector struct {
	linkType string // Jira link type for blocking relationships (e.g., "Blocks", "Blocking")
}

// NewDependencyDetector creates a new dependency detector.
func NewDependencyDetector(linkType string) *DependencyDetector {
	if linkType == "" {
		linkType = LinkTypeBlocking
	}
	slog.Debug("creating dependency detector", slog.String("link_type", linkType))
	return &DependencyDetector{linkType: linkType}
}

// ExtractBlockingRelationships extracts blocking relationships from Jira issue links.
// Returns two lists:
//   - blocks: Jira issue keys that this task blocks (OutwardIssue - they depend on us)
//   - blockedBy: Jira issue keys that block this task (InwardIssue - we depend on them)
func (d *DependencyDetector) ExtractBlockingRelationships(
	task *domain.TaskFile,
	jiraLinks []ports.IssueLink,
) (blocks []string, blockedBy []string) {
	slog.Debug("extracting blocking relationships",
		slog.String("task", task.TaskID()),
		slog.String("jira_key", task.Frontmatter.JiraNumber),
		slog.String("link_type", d.linkType),
		slog.Int("link_count", len(jiraLinks)),
	)

	for i, link := range jiraLinks {
		slog.Debug("examining jira link",
			slog.String("task", task.TaskID()),
			slog.Int("link_index", i),
			slog.String("type", link.Type),
			slog.String("inward_issue", link.InwardIssue),
			slog.String("outward_issue", link.OutwardIssue),
		)

		if link.Type != d.linkType {
			continue
		}

		// OutwardIssue = issues this task blocks (they depend on us)
		if link.OutwardIssue != "" {
			slog.Debug("found issue we block",
				slog.String("task", task.TaskID()),
				slog.String("blocks", link.OutwardIssue),
			)
			blocks = append(blocks, link.OutwardIssue)
		}

		// InwardIssue = issues that block this task (we depend on them)
		if link.InwardIssue != "" {
			slog.Debug("found issue that blocks us",
				slog.String("task", task.TaskID()),
				slog.String("blocked_by", link.InwardIssue),
			)
			blockedBy = append(blockedBy, link.InwardIssue)
		}
	}

	slog.Debug("blocking relationship extraction complete",
		slog.String("task", task.TaskID()),
		slog.Any("blocks", blocks),
		slog.Any("blocked_by", blockedBy),
	)

	return blocks, blockedBy
}
