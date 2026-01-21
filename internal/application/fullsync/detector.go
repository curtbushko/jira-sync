// Package fullsync provides bidirectional sync between local files and Jira.
package fullsync

import (
	"time"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

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
	hasher ports.HashComputer
}

// NewChangeDetector creates a new change detector.
func NewChangeDetector(hasher ports.HashComputer) *ChangeDetector {
	return &ChangeDetector{hasher: hasher}
}

// Detect compares a local task with a Jira issue and returns the change result.
func (d *ChangeDetector) Detect(task *domain.TaskFile, jiraIssue *ports.Issue) ChangeResult {
	localChanged := d.hasLocalChanges(task)
	jiraChanged, changedFields := d.hasJiraChanges(task, jiraIssue)

	// Both changed = conflict
	if localChanged && jiraChanged {
		return ChangeResult{Type: ChangeTypeConflict, Fields: changedFields}
	}

	// Only local changed = push to Jira
	if localChanged {
		return ChangeResult{Type: ChangeTypeLocalToJira, Fields: d.getLocalChangedFields(task, jiraIssue)}
	}

	// Only Jira changed = pull to local
	if jiraChanged {
		return ChangeResult{Type: ChangeTypeJiraToLocal, Fields: changedFields}
	}

	// No changes
	return ChangeResult{Type: ChangeTypeNone}
}

// hasLocalChanges checks if the local file has changed since last sync.
func (d *ChangeDetector) hasLocalChanges(task *domain.TaskFile) bool {
	// If never synced and has content hash, it's been modified
	if task.Frontmatter.ContentHash == "" {
		// Never synced - consider it as changed if there's content
		return task.Description != "" || task.Frontmatter.Title != ""
	}

	// Compare current hash with stored hash
	currentHash := d.hasher.ComputeHash(task)
	return currentHash != task.Frontmatter.ContentHash
}

// hasJiraChanges checks if the Jira issue has changed since last sync.
func (d *ChangeDetector) hasJiraChanges(task *domain.TaskFile, jiraIssue *ports.Issue) (bool, []string) {
	var changedFields []string

	// Parse last synced time
	lastSynced := d.parseLastSynced(task.Frontmatter.LastSynced)

	// Check if Jira was updated after last sync
	jiraUpdatedAfterSync := jiraIssue.Updated.After(lastSynced)

	// If Jira updated after last sync, check which fields differ
	if jiraUpdatedAfterSync || lastSynced.IsZero() {
		// Check title (summary)
		if jiraIssue.Summary != task.Frontmatter.Title {
			changedFields = append(changedFields, "title")
		}

		// Check description
		if jiraIssue.Description != task.Description {
			changedFields = append(changedFields, "description")
		}

		// Check status
		if jiraIssue.Status != "" && jiraIssue.Status != task.Frontmatter.JiraState {
			changedFields = append(changedFields, "status")
		}
	}

	return len(changedFields) > 0, changedFields
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
