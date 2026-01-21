package domain

// SyncStatus constants track the sync state between local files and Jira.
const (
	SyncStatusPending = "pending" // Not yet created in Jira
	SyncStatusCreated = "created" // Created in Jira, dependencies not linked
	SyncStatusLinked  = "linked"  // Created and dependencies linked
)

// DefaultLinkType is the default Jira link type for dependencies.
const DefaultLinkType = "Blocks"

// DefaultIssueType is the default Jira issue type.
const DefaultIssueType = "Task"

// DefaultEndDateOffset is the default number of days to add for end date calculation.
const DefaultEndDateOffset = 7

// DefaultJiraState is the default Jira ticket state for new tasks.
const DefaultJiraState = "Todo"

// Jira field size limits (Jira Cloud).
// See docs/jira-field-limits.md for details and sources.
const (
	// JiraSummaryMaxLength is the maximum length for issue summary/title (hard limit).
	JiraSummaryMaxLength = 255

	// JiraDescriptionMaxLength is the maximum length for issue description.
	JiraDescriptionMaxLength = 32767

	// JiraCommentMaxLength is the maximum length for comments (same as description).
	JiraCommentMaxLength = 32767
)
