// Package jira provides Jira API client adapters.
package jira

import (
	"context"
	"fmt"
	"sync"

	"github.com/curtbushko/jira-sync/internal/ports"
)

// MockJiraClient is a mock implementation of ports.JiraClient for testing.
type MockJiraClient struct {
	mu sync.Mutex

	// Function hooks for customizing behavior
	CreateIssueFunc    func(ctx context.Context, req ports.CreateIssueRequest) (*ports.Issue, error)
	UpdateIssueFunc    func(ctx context.Context, key string, req ports.UpdateIssueRequest) error
	CreateLinkFunc     func(ctx context.Context, inward, outward, linkType string) error
	GetIssueFunc       func(ctx context.Context, key string) (*ports.Issue, error)
	GetTransitionsFunc func(ctx context.Context, key string) ([]ports.Transition, error)
	DoTransitionFunc   func(ctx context.Context, key, transitionID string) error

	// Call tracking
	CreateIssueCalls    []ports.CreateIssueRequest
	UpdateIssueCalls    []UpdateIssueCall
	CreateLinkCalls     []CreateLinkCall
	GetIssueCalls       []string
	GetTransitionsCalls []string
	DoTransitionCalls   []DoTransitionCall

	// Auto-increment for issue keys
	issueCounter int
	baseURL      string
}

// UpdateIssueCall records a call to UpdateIssue.
type UpdateIssueCall struct {
	Key string
	Req ports.UpdateIssueRequest
}

// CreateLinkCall records a call to CreateLink.
type CreateLinkCall struct {
	Inward   string
	Outward  string
	LinkType string
}

// DoTransitionCall records a call to DoTransition.
type DoTransitionCall struct {
	Key          string
	TransitionID string
}

// NewMockJiraClient creates a new MockJiraClient.
func NewMockJiraClient() *MockJiraClient {
	return &MockJiraClient{
		baseURL: "https://mock.atlassian.net",
	}
}

// CreateIssue creates a new issue and returns the created issue with key.
func (m *MockJiraClient) CreateIssue(ctx context.Context, req ports.CreateIssueRequest) (*ports.Issue, error) {
	m.mu.Lock()
	m.CreateIssueCalls = append(m.CreateIssueCalls, req)
	m.issueCounter++
	counter := m.issueCounter
	m.mu.Unlock()

	if m.CreateIssueFunc != nil {
		return m.CreateIssueFunc(ctx, req)
	}

	// Default behavior: return a mock issue
	key := fmt.Sprintf("%s-%d", req.Project, counter)
	return &ports.Issue{
		Key:         key,
		Self:        fmt.Sprintf("%s/browse/%s", m.baseURL, key),
		Summary:     req.Summary,
		Description: req.Description,
	}, nil
}

// UpdateIssue updates an existing issue.
func (m *MockJiraClient) UpdateIssue(ctx context.Context, key string, req ports.UpdateIssueRequest) error {
	m.mu.Lock()
	m.UpdateIssueCalls = append(m.UpdateIssueCalls, UpdateIssueCall{Key: key, Req: req})
	m.mu.Unlock()

	if m.UpdateIssueFunc != nil {
		return m.UpdateIssueFunc(ctx, key, req)
	}

	return nil
}

// CreateLink creates a dependency link between two issues.
func (m *MockJiraClient) CreateLink(ctx context.Context, inward, outward, linkType string) error {
	m.mu.Lock()
	m.CreateLinkCalls = append(m.CreateLinkCalls, CreateLinkCall{
		Inward:   inward,
		Outward:  outward,
		LinkType: linkType,
	})
	m.mu.Unlock()

	if m.CreateLinkFunc != nil {
		return m.CreateLinkFunc(ctx, inward, outward, linkType)
	}

	return nil
}

// GetIssue fetches an issue by key.
func (m *MockJiraClient) GetIssue(ctx context.Context, key string) (*ports.Issue, error) {
	m.mu.Lock()
	m.GetIssueCalls = append(m.GetIssueCalls, key)
	m.mu.Unlock()

	if m.GetIssueFunc != nil {
		return m.GetIssueFunc(ctx, key)
	}

	return &ports.Issue{
		Key:  key,
		Self: fmt.Sprintf("%s/browse/%s", m.baseURL, key),
	}, nil
}

// BaseURL returns the Jira instance base URL.
func (m *MockJiraClient) BaseURL() string {
	return m.baseURL
}

// GetTransitions returns available transitions for an issue.
func (m *MockJiraClient) GetTransitions(ctx context.Context, key string) ([]ports.Transition, error) {
	m.mu.Lock()
	m.GetTransitionsCalls = append(m.GetTransitionsCalls, key)
	m.mu.Unlock()

	if m.GetTransitionsFunc != nil {
		return m.GetTransitionsFunc(ctx, key)
	}

	// Default: return common transitions
	return []ports.Transition{
		{ID: "11", Name: "To Do"},
		{ID: "21", Name: "In Progress"},
		{ID: "31", Name: "Done"},
	}, nil
}

// DoTransition performs a workflow transition on an issue.
func (m *MockJiraClient) DoTransition(ctx context.Context, key, transitionID string) error {
	m.mu.Lock()
	m.DoTransitionCalls = append(m.DoTransitionCalls, DoTransitionCall{
		Key:          key,
		TransitionID: transitionID,
	})
	m.mu.Unlock()

	if m.DoTransitionFunc != nil {
		return m.DoTransitionFunc(ctx, key, transitionID)
	}

	return nil
}

// SetBaseURL sets the base URL for the mock client.
func (m *MockJiraClient) SetBaseURL(url string) {
	m.baseURL = url
}

// Reset clears all recorded calls.
func (m *MockJiraClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreateIssueCalls = nil
	m.UpdateIssueCalls = nil
	m.CreateLinkCalls = nil
	m.GetIssueCalls = nil
	m.GetTransitionsCalls = nil
	m.DoTransitionCalls = nil
	m.issueCounter = 0
}
