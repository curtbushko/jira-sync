// Package hashing provides content hashing for change detection.
package hashing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	"github.com/curtbushko/jira-sync/internal/domain"
)

// SHA256HashComputer computes SHA256 hashes for task content.
type SHA256HashComputer struct{}

// NewSHA256HashComputer creates a new SHA256HashComputer.
func NewSHA256HashComputer() *SHA256HashComputer {
	return &SHA256HashComputer{}
}

// ComputeHash returns SHA256 hash of task content.
// It hashes title + jira-parent + jira-dependencies + description.
// Fields like jira-number, jira-url, sync-status, content-hash, and sync-dependencies
// are excluded because they don't affect the Jira ticket content.
// Note: sync-dependencies only affect creation order, not Jira content.
func (h *SHA256HashComputer) ComputeHash(task *domain.TaskFile) string {
	var buf bytes.Buffer

	// Write content fields that affect Jira ticket
	buf.WriteString(task.Frontmatter.Title)
	buf.WriteString("\x00") // null separator between fields
	buf.WriteString(task.Frontmatter.JiraParent)
	buf.WriteString("\x00")

	// Write jira-dependencies in order (these create Jira links)
	// Note: sync-dependencies are NOT included since they only affect creation order
	for _, dep := range task.Frontmatter.JiraDependencies {
		buf.WriteString(dep)
		buf.WriteString("\x00")
	}

	// Write description
	buf.WriteString(task.Description)

	// Compute hash
	hash := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(hash[:])
}
