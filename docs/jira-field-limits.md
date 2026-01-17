# Jira Field Size Limits

Reference documentation for Jira Cloud API field character limits.

## Field Limits Summary

| Field | Maximum Length | Configurable | Notes |
|-------|---------------|--------------|-------|
| **Summary (Title)** | 255 characters | No | Hard-coded limit, cannot be changed |
| **Description** | 32,767 characters | No (Cloud) | ~32KB limit in Jira Cloud |
| **Comments** | 32,767 characters | No (Cloud) | Same as description |
| **Environment** | 32,767 characters | No (Cloud) | Same as description |
| **Custom Field Description** | 255 characters | No | Field metadata description, not values |
| **Single-line Text Fields** | 255 characters | No | Custom single-line text fields |
| **Multi-line Text Fields** | ~32,000 characters | No (Cloud) | Custom multi-line text fields |

## Detailed Field Information

### Summary Field

The Summary field (issue title) has a **hard limit of 255 characters** that cannot be changed in any Jira deployment.

- Error message: `"Summary must be less than 255 characters"`
- This limit applies to all issue types
- Truncation recommended for longer titles

### Description Field

The Description field supports up to **32,767 characters** in Jira Cloud.

- Error message: `"The entered text is too long. It exceeds the allowed limit of 32767 characters."`
- This limit is **not configurable** in Jira Cloud
- In Jira Server/Data Center, can be configured via `jira.text.field.character.limit` (up to 2,147,483,647)
- Supports Atlassian Document Format (ADF) for rich text

### Comments

Comments follow the same limit as the Description field: **32,767 characters**.

## API Considerations

When using the Jira REST API:

1. **Validate before submission**: The API may accept content exceeding limits but cause errors
2. **Summary**: Always truncate to 255 characters
3. **Description**: Truncate to 32,767 characters or implement pagination for large content

### Example Validation (Go)

```go
const (
    JiraSummaryMaxLength     = 255
    JiraDescriptionMaxLength = 32767
)

func truncateSummary(s string) string {
    if len(s) > JiraSummaryMaxLength {
        return s[:JiraSummaryMaxLength-3] + "..."
    }
    return s
}

func truncateDescription(s string) string {
    if len(s) > JiraDescriptionMaxLength {
        return s[:JiraDescriptionMaxLength-50] + "\n\n[Content truncated due to Jira limits]"
    }
    return s
}
```

## Jira Cloud vs Server/Data Center

| Feature | Jira Cloud | Jira Server/DC |
|---------|------------|----------------|
| Summary limit | 255 (fixed) | 255 (fixed) |
| Description limit | 32,767 (fixed) | Configurable (default 32,767) |
| Config property | N/A | `jira.text.field.character.limit` |

## Sources

- [Jira custom field description can't exceed 255 characters](https://support.atlassian.com/jira/kb/jira-custom-field-description-cant-exceed-255-characters/) - Atlassian Support
- [Summary must be less than 255 characters](https://community.atlassian.com/forums/Jira-questions/Summary-must-be-less-than-255-characters/qaq-p/989632) - Atlassian Community
- [Character limit of Description field in Jira Software cloud](https://community.atlassian.com/forums/Jira-questions/Character-limit-of-Description-field-in-Jira-Software-cloud/qaq-p/2655913) - Atlassian Community
- [Description field character limit not mentioned](https://jira.atlassian.com/browse/JRACLOUD-68949) - Atlassian Jira Issue Tracker
- [Official documentation on cloud jira.text.field.character.limit](https://community.atlassian.com/forums/Jira-questions/Official-documentation-on-cloud-jira-text-field-character-limit/qaq-p/2588568) - Atlassian Community

---

*Last updated: 2026-01-17*
