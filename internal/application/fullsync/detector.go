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

// DependencyChangeResult holds the result of comparing jira-dependencies.
type DependencyChangeResult struct {
	HasChanges bool
	ToAdd      []string // Jira issue keys to create "Blocks" links for
	ToRemove   []string // Link IDs to delete
	LocalDeps  []string // Local dependency task IDs (for reporting)
	JiraDeps   []string // Jira dependency task IDs (for reporting)
}

// DetectDependencyChanges compares local jira-dependencies with Jira issue links.
// Returns which links need to be added or removed to sync.
func (d *ChangeDetector) DetectDependencyChanges(
	task *domain.TaskFile,
	jiraLinks []ports.IssueLink,
	allTasks []*domain.TaskFile,
) DependencyChangeResult {
	// Build mapping from task ID to Jira key and vice versa
	taskIDToJiraKey := make(map[string]string)
	jiraKeyToTaskID := make(map[string]string)
	for _, t := range allTasks {
		taskID := t.TaskID()
		if taskID != "" && t.Frontmatter.JiraNumber != "" {
			taskIDToJiraKey[taskID] = t.Frontmatter.JiraNumber
			jiraKeyToTaskID[t.Frontmatter.JiraNumber] = taskID
		}
	}

	// Get local dependency task IDs
	localDepIDs := task.GetJiraDependencyIDs()

	// Convert local deps to Jira keys
	var localJiraKeys []string
	for _, depID := range localDepIDs {
		if jiraKey, ok := taskIDToJiraKey[depID]; ok {
			localJiraKeys = append(localJiraKeys, jiraKey)
		}
	}

	// Extract Jira "Blocks" links where this task is blocked (outward)
	// A "Blocks" link with InwardIssue=X and OutwardIssue=GUARD-123 means X blocks GUARD-123
	var jiraBlockerKeys []string
	linkIDByBlocker := make(map[string]string) // Jira key -> link ID

	for _, link := range jiraLinks {
		// Only consider "Blocks" type links where this task is the blocked (outward) issue
		if link.Type == LinkTypeBlocks && link.OutwardIssue == task.Frontmatter.JiraNumber && link.InwardIssue != "" {
			jiraBlockerKeys = append(jiraBlockerKeys, link.InwardIssue)
			linkIDByBlocker[link.InwardIssue] = link.ID
		}
	}

	// Compare local vs Jira
	if stringSlicesEqual(localJiraKeys, jiraBlockerKeys) {
		// Convert Jira keys back to task IDs for reporting
		var jiraDepTaskIDs []string
		for _, key := range jiraBlockerKeys {
			if taskID, ok := jiraKeyToTaskID[key]; ok {
				jiraDepTaskIDs = append(jiraDepTaskIDs, taskID)
			}
		}
		return DependencyChangeResult{
			HasChanges: false,
			LocalDeps:  localDepIDs,
			JiraDeps:   jiraDepTaskIDs,
		}
	}

	// Calculate what needs to be added/removed
	toAddKeys := difference(localJiraKeys, jiraBlockerKeys)
	toRemoveKeys := difference(jiraBlockerKeys, localJiraKeys)

	// Convert toRemove keys to link IDs
	var toRemoveLinkIDs []string
	for _, key := range toRemoveKeys {
		if linkID, ok := linkIDByBlocker[key]; ok {
			toRemoveLinkIDs = append(toRemoveLinkIDs, linkID)
		}
	}

	// Convert Jira keys back to task IDs for reporting
	var jiraDepTaskIDs []string
	for _, key := range jiraBlockerKeys {
		if taskID, ok := jiraKeyToTaskID[key]; ok {
			jiraDepTaskIDs = append(jiraDepTaskIDs, taskID)
		}
	}

	return DependencyChangeResult{
		HasChanges: true,
		ToAdd:      toAddKeys,
		ToRemove:   toRemoveLinkIDs,
		LocalDeps:  localDepIDs,
		JiraDeps:   jiraDepTaskIDs,
	}
}
