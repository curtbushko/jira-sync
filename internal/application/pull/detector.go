// Package pull provides pull-only sync from Jira to local files.
package pull

import (
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
	// Parse last synced time
	lastSynced := d.parseLastSynced(task.Frontmatter.LastSynced)

	// Check if Jira was updated after last sync
	jiraUpdatedAfterSync := jiraIssue.Updated.After(lastSynced)

	// If Jira not updated and we have a valid last sync time, no changes
	if !jiraUpdatedAfterSync && !lastSynced.IsZero() {
		return false, nil
	}

	// Check which fields differ
	changedFields := d.compareJiraFields(task, jiraIssue)
	return len(changedFields) > 0, changedFields
}

// compareJiraFields compares Jira issue fields with local task.
func (d *ChangeDetector) compareJiraFields(task *domain.TaskFile, jiraIssue *ports.Issue) []string {
	var changedFields []string

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
	// Build mapping from Jira key to task ID
	jiraKeyToTaskID := make(map[string]string)
	for _, t := range allTasks {
		taskID := t.TaskID()
		if taskID != "" && t.Frontmatter.JiraNumber != "" {
			jiraKeyToTaskID[t.Frontmatter.JiraNumber] = taskID
		}
	}

	// Get local dependency task IDs
	localDepIDs := task.JiraDependencyIDs()

	// Extract Jira "Blocks" links where this task is blocked (outward)
	// A "Blocks" link with InwardIssue=X and OutwardIssue=GUARD-123 means X blocks GUARD-123
	var jiraDepTaskIDs []string

	for _, link := range jiraLinks {
		// Only consider "Blocks" type links where this task is the blocked (outward) issue
		if link.Type == LinkTypeBlocks && link.OutwardIssue == task.Frontmatter.JiraNumber && link.InwardIssue != "" {
			// Convert Jira key to task ID
			if taskID, ok := jiraKeyToTaskID[link.InwardIssue]; ok {
				jiraDepTaskIDs = append(jiraDepTaskIDs, taskID)
			}
		}
	}

	// Check if there's a difference
	hasChanges := !stringSlicesEqual(localDepIDs, jiraDepTaskIDs)

	return DependencyPullResult{
		HasChanges: hasChanges,
		LocalDeps:  localDepIDs,
		JiraDeps:   jiraDepTaskIDs,
	}
}
