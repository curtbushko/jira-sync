package filesystem

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseWikiLink_ValidLink(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTaskID string
		wantFile   string
	}{
		{
			name:       "full wiki link",
			input:      "[KB-1: Initialize Project](20260116-103001.md)",
			wantTaskID: "KB-1",
			wantFile:   "20260116-103001.md",
		},
		{
			name:       "wiki link with complex title",
			input:      "[ERR-2: Handle Pod Failures - Container State](20260116-103002.md)",
			wantTaskID: "ERR-2",
			wantFile:   "20260116-103002.md",
		},
		{
			name:       "legacy plain task ID",
			input:      "KB-1",
			wantTaskID: "KB-1",
			wantFile:   "",
		},
		{
			name:       "legacy with whitespace",
			input:      "  KB-1  ",
			wantTaskID: "KB-1",
			wantFile:   "",
		},
		{
			name:       "wiki link title without colon",
			input:      "[TASK-1](20260116-103001.md)",
			wantTaskID: "TASK-1",
			wantFile:   "20260116-103001.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskID, filename := ParseWikiLink(tt.input)
			assert.Equal(t, tt.wantTaskID, taskID)
			assert.Equal(t, tt.wantFile, filename)
		})
	}
}

func TestParseWikiLink_InvalidLinks(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTaskID string
	}{
		{"empty string", "", ""},
		// Malformed links are treated as legacy task IDs
		{"malformed link missing close paren", "[KB-1: Title](file.md", "[KB-1: Title](file.md"},
		{"malformed link missing open bracket", "KB-1: Title](file.md)", "KB-1: Title](file.md)"},
		{"non-md extension treated as legacy", "[KB-1](file.txt)", "[KB-1](file.txt)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskID, _ := ParseWikiLink(tt.input)
			assert.Equal(t, tt.wantTaskID, taskID)
		})
	}
}

func TestFormatWikiLink(t *testing.T) {
	tests := []struct {
		taskID   string
		title    string
		filename string
		want     string
	}{
		{
			taskID:   "KB-1",
			title:    "Initialize Project",
			filename: "20260116-103001.md",
			want:     "[KB-1: Initialize Project](20260116-103001.md)",
		},
		{
			taskID:   "ERR-5",
			title:    "Pod Listing",
			filename: "20260116-103005.md",
			want:     "[ERR-5: Pod Listing](20260116-103005.md)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.taskID, func(t *testing.T) {
			got := FormatWikiLink(tt.taskID, tt.title, tt.filename)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractTaskIDFromWikiLink(t *testing.T) {
	tests := []struct {
		input    string
		wantID   string
		wantFile string
	}{
		// Wiki link format
		{"[KB-1: Init](file.md)", "KB-1", "file.md"},
		{"[ERR-2: Detection](20260116.md)", "ERR-2", "20260116.md"},
		// Legacy format
		{"KB-1", "KB-1", ""},
		{"  ERR-2  ", "ERR-2", ""},
		// Empty
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			id, file := ParseWikiLink(tt.input)
			assert.Equal(t, tt.wantID, id)
			assert.Equal(t, tt.wantFile, file)
		})
	}
}
