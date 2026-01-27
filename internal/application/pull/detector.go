// Package pull provides pull-only sync from Jira to local files.
package pull

import (
	"log/slog"
	"time"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// LinkTypeBlocks is the Jira link type for blocking relationships.
const LinkTypeBlocks = "Blocks"

// ChangeType indicates the direction of change detected.
type ChangeType int

const (
	// ChangeTypeNone indicates no changes detected.
	ChangeTypeNone ChangeType = iota
	// ChangeTypeLocalToJira indicates local file changed, should push to Jira.
	ChangeTypeLocalToJira
	// ChangeTypeJiraToLocal indicates Jira changed, should pull to local.
	ChangeTypeJiraToLocal
	// ChangeTypeConflict indicates both sides changed, requires resolution.
	ChangeTypeConflict
)

// ChangeResult describes the detected change between local and Jira.
type ChangeResult struct {
	Type   ChangeType
	Fields []string // Which fields changed
}

// ChangeDetector compares local task files with Jira issues.
type ChangeDetector struct {
	hasher   ports.HashComputer
	linkType string // Jira link type for dependencies (e.g., "Blocks", "Is Blocked By")
}

// NewChangeDetector creates a new change detector.
func NewChangeDetector(hasher ports.HashComputer, linkType string) *ChangeDetector {
	if linkType == "" {
		linkType = LinkTypeBlocks
	}
	slog.Debug("creating change detector", slog.String("link_type", linkType))
	return &ChangeDetector{hasher: hasher, linkType: linkType}
}

// Detect compares a local task with a Jira issue and returns the change result.
// For pull operations, Jira is the source of truth - if fields differ, pull them.
func (d *ChangeDetector) Detect(task *domain.TaskFile, jiraIssue *ports.Issue) ChangeResult {
	slog.Debug("detecting changes",
		slog.String("task", task.TaskID()),
		slog.String("jira_key", task.Frontmatter.JiraNumber),
	)

	// Compare actual field values - if Jira differs from local, we should pull
	changedFields := d.compareJiraFields(task, jiraIssue)
	hasChanges := len(changedFields) > 0

	slog.Debug("change detection results",
		slog.String("task", task.TaskID()),
		slog.Bool("has_changes", hasChanges),
		slog.Any("changed_fields", changedFields),
	)

	if hasChanges {
		slog.Debug("jira differs from local - should pull",
			slog.String("task", task.TaskID()),
			slog.Any("fields", changedFields),
		)
		return ChangeResult{Type: ChangeTypeJiraToLocal, Fields: changedFields}
	}

	// No changes needed
	slog.Debug("no changes detected - jira matches local",
		slog.String("task", task.TaskID()),
	)
	return ChangeResult{Type: ChangeTypeNone}
}

// hasLocalChanges checks if the local file has changed since last sync.
func (d *ChangeDetector) hasLocalChanges(task *domain.TaskFile) bool {
	// If never synced and has content hash, it's been modified
	if task.Frontmatter.ContentHash == "" {
		// Never synced - consider it as changed if there's content
		hasContent := task.Description != "" || task.Frontmatter.Title != ""
		slog.Debug("local changes check - never synced",
			slog.String("task", task.TaskID()),
			slog.Bool("has_content", hasContent),
		)
		return hasContent
	}

	// Compare current hash with stored hash
	currentHash := d.hasher.ComputeHash(task)
	hashChanged := currentHash != task.Frontmatter.ContentHash
	slog.Debug("local changes check - comparing hashes",
		slog.String("task", task.TaskID()),
		slog.String("stored_hash", task.Frontmatter.ContentHash),
		slog.String("current_hash", currentHash),
		slog.Bool("changed", hashChanged),
	)
	return hashChanged
}

// hasJiraChanges checks if the Jira issue differs from local.
// Always compares actual field values - doesn't rely on timestamps.
func (d *ChangeDetector) hasJiraChanges(task *domain.TaskFile, jiraIssue *ports.Issue) (bool, []string) {
	// Always compare actual field values
	changedFields := d.compareJiraFields(task, jiraIssue)
	hasChanges := len(changedFields) > 0
	slog.Debug("jira changes check - field comparison",
		slog.String("task", task.TaskID()),
		slog.Bool("has_changes", hasChanges),
		slog.Any("changed_fields", changedFields),
	)
	return hasChanges, changedFields
}

// compareJiraFields compares Jira issue fields with local task.
func (d *ChangeDetector) compareJiraFields(task *domain.TaskFile, jiraIssue *ports.Issue) []string {
	var changedFields []string

	// Check title (summary)
	if jiraIssue.Summary != task.Frontmatter.Title {
		slog.Debug("field differs: title",
			slog.String("task", task.TaskID()),
			slog.String("local", task.Frontmatter.Title),
			slog.String("jira", jiraIssue.Summary),
		)
		changedFields = append(changedFields, "title")
	}

	// Check description
	if jiraIssue.Description != task.Description {
		slog.Debug("field differs: description",
			slog.String("task", task.TaskID()),
			slog.Int("local_len", len(task.Description)),
			slog.Int("jira_len", len(jiraIssue.Description)),
		)
		changedFields = append(changedFields, "description")
	}

	// Check status
	if jiraIssue.Status != "" && jiraIssue.Status != task.Frontmatter.JiraState {
		slog.Debug("field differs: status",
			slog.String("task", task.TaskID()),
			slog.String("local", task.Frontmatter.JiraState),
			slog.String("jira", jiraIssue.Status),
		)
		changedFields = append(changedFields, "status")
	}

	return changedFields
}

// getLocalChangedFields determines which fields changed locally.
func (d *ChangeDetector) getLocalChangedFields(task *domain.TaskFile, jiraIssue *ports.Issue) []string {
	var changedFields []string

	// Check title
	if jiraIssue.Summary != task.Frontmatter.Title {
		changedFields = append(changedFields, "title")
	}

	// Check description
	if jiraIssue.Description != task.Description {
		changedFields = append(changedFields, "description")
	}

	return changedFields
}

// parseLastSynced parses the last synced timestamp.
func (d *ChangeDetector) parseLastSynced(lastSynced string) time.Time {
	if lastSynced == "" {
		return time.Time{} // Zero time
	}

	// Try RFC3339 format
	parsed, err := time.Parse(time.RFC3339, lastSynced)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// DependencyPullResult holds the result of pulling jira-dependencies.
// This is a pull-only result - it does NOT contain ToAdd/ToRemove for push operations.
type DependencyPullResult struct {
	HasChanges bool
	LocalDeps  []string // Local dependency task IDs (before pull)
	JiraDeps   []string // Jira dependency task IDs (what we pulled)
}

// DetectDependencies compares local jira-dependencies with Jira issue links.
// Returns which dependencies exist in Jira (for pulling to local).
// This is a pull-only operation - it does NOT determine what to push.
func (d *ChangeDetector) DetectDependencies(
	task *domain.TaskFile,
	jiraLinks []ports.IssueLink,
	allTasks []*domain.TaskFile,
) DependencyPullResult {
	slog.Debug("detecting dependencies",
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

	// Get local dependency task IDs
	localDepIDs := task.JiraDependencyIDs()
	slog.Debug("local dependency ids",
		slog.String("task", task.TaskID()),
		slog.Any("local_deps", localDepIDs),
	)

	// Extract dependency links - any link of the configured type with an InwardIssue
	var jiraDepTaskIDs []string

	slog.Debug("scanning jira links for dependencies",
		slog.String("task", task.TaskID()),
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

		if link.Type == d.linkType && link.InwardIssue != "" {
			// Convert Jira key to task ID, or use Jira key directly if no local task
			if taskID, ok := jiraKeyToTaskID[link.InwardIssue]; ok {
				slog.Debug("dependency link matched - mapped to task id",
					slog.String("task", task.TaskID()),
					slog.String("jira_key", link.InwardIssue),
					slog.String("mapped_task_id", taskID),
				)
				jiraDepTaskIDs = append(jiraDepTaskIDs, taskID)
			} else {
				// No local task for this Jira issue - store Jira key directly
				slog.Debug("dependency link matched - no local task, using jira key",
					slog.String("task", task.TaskID()),
					slog.String("jira_key", link.InwardIssue),
				)
				jiraDepTaskIDs = append(jiraDepTaskIDs, link.InwardIssue)
			}
		}
	}

	// Check if there's a difference
	hasChanges := !stringSlicesEqual(localDepIDs, jiraDepTaskIDs)

	slog.Debug("dependency detection complete",
		slog.String("task", task.TaskID()),
		slog.Bool("has_changes", hasChanges),
		slog.Any("local_deps", localDepIDs),
		slog.Any("jira_deps", jiraDepTaskIDs),
	)

	return DependencyPullResult{
		HasChanges: hasChanges,
		LocalDeps:  localDepIDs,
		JiraDeps:   jiraDepTaskIDs,
	}
}
