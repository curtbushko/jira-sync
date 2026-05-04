package domain

import (
	"errors"
	"fmt"
)

// Common domain errors.
var (
	ErrInvalidFrontmatter     = errors.New("invalid frontmatter format")
	ErrMissingTitle           = errors.New("title is required")
	ErrMissingParent          = errors.New("parent is required")
	ErrMissingDescription     = errors.New("description is required")
	ErrDependencyNotFound     = errors.New("dependency not found")
	ErrCircularDependency     = errors.New("circular dependency detected")
	ErrJiraAuthentication     = errors.New("jira authentication failed")
	ErrJiraCreateFailed       = errors.New("failed to create jira issue")
	ErrJiraUpdateFailed       = errors.New("failed to update jira issue")
	ErrJiraLinkFailed         = errors.New("failed to create jira link")
	ErrJiraTransitionFailed   = errors.New("failed to transition jira issue")
	ErrTransitionNotAvailable = errors.New("transition not available")
)

// ValidationError wraps a validation error with field context.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewValidationError creates a new validation error.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

// ParseError wraps a parse error with file context.
type ParseError struct {
	Path string
	Err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse %s: %v", e.Path, e.Err)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// NewParseError creates a new parse error.
func NewParseError(path string, err error) *ParseError {
	return &ParseError{Path: path, Err: err}
}
