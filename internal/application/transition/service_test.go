package transition

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/curtbushko/jira-sync/internal/adapters/jira"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

func TestTransitionService_TransitionTo_ValidTransition(t *testing.T) {
	mockClient := jira.NewMockJiraClient()
	mockClient.GetTransitionsFunc = func(_ context.Context, _ string) ([]ports.Transition, error) {
		return []ports.Transition{
			{ID: "21", Name: "In Progress"},
			{ID: "31", Name: "Done"},
		}, nil
	}

	svc := NewService(mockClient)
	err := svc.TransitionTo(context.Background(), "GUARD-123", "In Progress")

	require.NoError(t, err)
	require.Len(t, mockClient.DoTransitionCalls, 1)
	assert.Equal(t, "GUARD-123", mockClient.DoTransitionCalls[0].Key)
	assert.Equal(t, "21", mockClient.DoTransitionCalls[0].TransitionID)
}

func TestTransitionService_TransitionTo_InvalidTransition(t *testing.T) {
	mockClient := jira.NewMockJiraClient()
	mockClient.GetTransitionsFunc = func(_ context.Context, _ string) ([]ports.Transition, error) {
		return []ports.Transition{
			{ID: "21", Name: "In Progress"},
		}, nil
	}

	svc := NewService(mockClient)
	err := svc.TransitionTo(context.Background(), "GUARD-123", "Done")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrTransitionNotAvailable)
	assert.Contains(t, err.Error(), "Done")
	assert.Contains(t, err.Error(), "In Progress") // Should list available transitions
	assert.Len(t, mockClient.DoTransitionCalls, 0) // No transition attempted
}

func TestTransitionService_TransitionTo_AlreadyInState(t *testing.T) {
	mockClient := jira.NewMockJiraClient()
	mockClient.GetIssueFunc = func(_ context.Context, _ string) (*ports.Issue, error) {
		return &ports.Issue{
			Key:    "GUARD-123",
			Status: "Done",
		}, nil
	}

	svc := NewService(mockClient)
	err := svc.TransitionTo(context.Background(), "GUARD-123", "Done")

	require.NoError(t, err)
	// No transition should be attempted if already in target state
	assert.Len(t, mockClient.DoTransitionCalls, 0)
}

func TestTransitionService_TransitionTo_CaseInsensitive(t *testing.T) {
	mockClient := jira.NewMockJiraClient()
	mockClient.GetTransitionsFunc = func(_ context.Context, _ string) ([]ports.Transition, error) {
		return []ports.Transition{
			{ID: "21", Name: "In Progress"},
			{ID: "31", Name: "Done"},
		}, nil
	}

	svc := NewService(mockClient)

	// Use lowercase when transition is "In Progress"
	err := svc.TransitionTo(context.Background(), "GUARD-123", "in progress")

	require.NoError(t, err)
	require.Len(t, mockClient.DoTransitionCalls, 1)
	assert.Equal(t, "21", mockClient.DoTransitionCalls[0].TransitionID)
}

func TestTransitionService_GetAvailableTransitions(t *testing.T) {
	mockClient := jira.NewMockJiraClient()
	mockClient.GetTransitionsFunc = func(_ context.Context, _ string) ([]ports.Transition, error) {
		return []ports.Transition{
			{ID: "21", Name: "In Progress"},
			{ID: "31", Name: "Done"},
			{ID: "41", Name: "Blocked"},
		}, nil
	}

	svc := NewService(mockClient)
	transitions, err := svc.GetAvailableTransitions(context.Background(), "GUARD-123")

	require.NoError(t, err)
	assert.Len(t, transitions, 3)
	assert.Equal(t, "In Progress", transitions[0].Name)
	assert.Equal(t, "Done", transitions[1].Name)
	assert.Equal(t, "Blocked", transitions[2].Name)
}

func TestTransitionService_TransitionTo_EmptyTarget(t *testing.T) {
	mockClient := jira.NewMockJiraClient()

	svc := NewService(mockClient)
	err := svc.TransitionTo(context.Background(), "GUARD-123", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "target state")
}
