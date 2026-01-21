// Package transition provides Jira workflow transition handling.
package transition

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// Service handles Jira workflow transitions.
type Service struct {
	jira ports.JiraClient
}

// NewService creates a new transition service.
func NewService(jira ports.JiraClient) *Service {
	return &Service{jira: jira}
}

// TransitionTo transitions an issue to the target state.
// Returns nil if already in target state.
// Returns ErrTransitionNotAvailable if the transition is not available.
func (s *Service) TransitionTo(ctx context.Context, issueKey, targetState string) error {
	if targetState == "" {
		return errors.New("target state cannot be empty")
	}

	// Check if already in target state
	issue, err := s.jira.GetIssue(ctx, issueKey)
	if err != nil {
		return fmt.Errorf("get issue: %w", err)
	}

	if strings.EqualFold(issue.Status, targetState) {
		// Already in target state, nothing to do
		return nil
	}

	// Get available transitions
	transitions, err := s.jira.GetTransitions(ctx, issueKey)
	if err != nil {
		return fmt.Errorf("get transitions: %w", err)
	}

	// Find the transition with matching name (case-insensitive)
	var transitionID string
	var availableNames []string
	for _, t := range transitions {
		availableNames = append(availableNames, t.Name)
		if strings.EqualFold(t.Name, targetState) {
			transitionID = t.ID
			break
		}
	}

	if transitionID == "" {
		return fmt.Errorf("%w: '%s' (available: %s)",
			domain.ErrTransitionNotAvailable,
			targetState,
			strings.Join(availableNames, ", "))
	}

	// Perform the transition
	if err := s.jira.DoTransition(ctx, issueKey, transitionID); err != nil {
		return fmt.Errorf("do transition: %w", err)
	}

	return nil
}

// GetAvailableTransitions returns available transitions for an issue.
func (s *Service) GetAvailableTransitions(ctx context.Context, issueKey string) ([]ports.Transition, error) {
	return s.jira.GetTransitions(ctx, issueKey)
}
