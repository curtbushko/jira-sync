package filesystem

import (
	"fmt"
	"regexp"
	"strings"
)

// wikiLinkRegex matches wiki-style links: [Title](filename.md)
var wikiLinkRegex = regexp.MustCompile(`^\[([^\]]+)\]\(([^)]+\.md)\)$`)

// ParseWikiLink extracts task ID and filename from a wiki-style link.
// Supports both formats:
//   - Wiki link: "[KB-1: Initialize Project](20260116-103001.md)" -> ("KB-1", "20260116-103001.md")
//   - Legacy: "KB-1" -> ("KB-1", "")
func ParseWikiLink(link string) (taskID string, filename string) {
	link = strings.TrimSpace(link)
	if link == "" {
		return "", ""
	}

	matches := wikiLinkRegex.FindStringSubmatch(link)
	if len(matches) == 3 {
		// Wiki link format: extract task ID from title (before colon)
		title := matches[1]
		filename = matches[2]

		// Extract task ID from title (e.g., "KB-1: Init" -> "KB-1")
		if idx := strings.Index(title, ":"); idx != -1 {
			taskID = strings.TrimSpace(title[:idx])
		} else {
			// No colon, use whole title as task ID
			taskID = strings.TrimSpace(title)
		}
		return taskID, filename
	}

	// Legacy format: plain task ID
	return strings.TrimSpace(link), ""
}

// FormatWikiLink creates a wiki-style link from task info.
// Format: "[TaskID: Title](filename.md)"
func FormatWikiLink(taskID, title, filename string) string {
	return fmt.Sprintf("[%s: %s](%s)", taskID, title, filename)
}
