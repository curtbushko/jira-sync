// Package hashing provides content hashing for change detection.
package hashing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"

	"github.com/curtbushko/jira-sync/internal/domain"
)

// SHA256HashComputer computes SHA256 hashes for task content.
type SHA256HashComputer struct{}

// NewSHA256HashComputer creates a new SHA256HashComputer.
func NewSHA256HashComputer() *SHA256HashComputer {
	return &SHA256HashComputer{}
}

// ComputeHash returns SHA256 hash of task content.
// It hashes title + jira-parent + jira-state + jira-blocks + jira-is-blocked-by + description.
// Fields like jira-number, jira-url, sync-status, and content-hash
// are excluded because they don't affect the Jira ticket content.
func (h *SHA256HashComputer) ComputeHash(task *domain.TaskFile) string {
	var buf bytes.Buffer

	// Write content fields that affect Jira ticket
	buf.WriteString(task.Frontmatter.Title)
	buf.WriteString("\x00") // null separator between fields
	buf.WriteString(task.Frontmatter.JiraParent)
	buf.WriteString("\x00")
	buf.WriteString(task.Frontmatter.JiraState)
	buf.WriteString("\x00")

	// Write jira-blocks in order (issues we block - creates Jira links)
	for _, dep := range task.Frontmatter.JiraBlocks {
		buf.WriteString(dep)
		buf.WriteString("\x00")
	}

	// Write jira-is-blocked-by in order (issues that block us - creates Jira links)
	buf.WriteString("blocked-by:")
	for _, dep := range task.Frontmatter.JiraIsBlockedBy {
		buf.WriteString(dep)
		buf.WriteString("\x00")
	}

	// Write description
	buf.WriteString(task.Description)

	// Compute hash
	hash := sha256.Sum256(buf.Bytes())
	result := hex.EncodeToString(hash[:])

	slog.Debug("computed content hash",
		slog.String("task", task.TaskID()),
		slog.String("hash", result),
		slog.Int("buf_len", buf.Len()),
		slog.Int("desc_len", len(task.Description)),
	)

	return result
}
