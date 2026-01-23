package jira

import (
	"context"
	"errors"
	"testing"

	"github.com/curtbushko/jira-sync/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockJiraClient_CreateIssue_DefaultBehavior(t *testing.T) {
	mock := NewMockJiraClient()

	issue, err := mock.CreateIssue(context.Background(), ports.CreateIssueRequest{
		Project:     "TEST",
		Summary:     "Test Issue",
		Description: "Test Description",
		IssueType:   "Task",
	})

	require.NoError(t, err)
	assert.Equal(t, "TEST-1", issue.Key)
	assert.Equal(t, "Test Issue", issue.Summary)
	assert.Len(t, mock.CreateIssueCalls, 1)
}

func TestMockJiraClient_CreateIssue_CustomBehavior(t *testing.T) {
	mock := NewMockJiraClient()
	mock.CreateIssueFunc = func(_ context.Context, req ports.CreateIssueRequest) (*ports.Issue, error) {
		return &ports.Issue{
			Key:     "CUSTOM-999",
			Self:    "https://custom.url",
			Summary: req.Summary,
		}, nil
	}

	issue, err := mock.CreateIssue(context.Background(), ports.CreateIssueRequest{
		Project: "TEST",
		Summary: "Test",
	})

	require.NoError(t, err)
	assert.Equal(t, "CUSTOM-999", issue.Key)
}

func TestMockJiraClient_CreateIssue_Error(t *testing.T) {
	mock := NewMockJiraClient()
	mock.CreateIssueFunc = func(_ context.Context, _ ports.CreateIssueRequest) (*ports.Issue, error) {
		return nil, errors.New("jira error")
	}

	_, err := mock.CreateIssue(context.Background(), ports.CreateIssueRequest{
		Project: "TEST",
		Summary: "Test",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jira error")
}

func TestMockJiraClient_UpdateIssue(t *testing.T) {
	mock := NewMockJiraClient()

	err := mock.UpdateIssue(context.Background(), "TEST-1", ports.UpdateIssueRequest{
		Summary:     "Updated Summary",
		Description: "Updated Description",
	})

	require.NoError(t, err)
	assert.Len(t, mock.UpdateIssueCalls, 1)
	assert.Equal(t, "TEST-1", mock.UpdateIssueCalls[0].Key)
	assert.Equal(t, "Updated Summary", mock.UpdateIssueCalls[0].Req.Summary)
}

func TestMockJiraClient_CreateLink(t *testing.T) {
	mock := NewMockJiraClient()

	err := mock.CreateLink(context.Background(), "TEST-2", "TEST-1", "Blocks")

	require.NoError(t, err)
	assert.Len(t, mock.CreateLinkCalls, 1)
	assert.Equal(t, "TEST-2", mock.CreateLinkCalls[0].Inward)
	assert.Equal(t, "TEST-1", mock.CreateLinkCalls[0].Outward)
	assert.Equal(t, "Blocks", mock.CreateLinkCalls[0].LinkType)
}

func TestMockJiraClient_GetIssue(t *testing.T) {
	mock := NewMockJiraClient()

	issue, err := mock.GetIssue(context.Background(), "TEST-1")

	require.NoError(t, err)
	assert.Equal(t, "TEST-1", issue.Key)
	assert.Len(t, mock.GetIssueCalls, 1)
}

func TestMockJiraClient_BaseURL(t *testing.T) {
	mock := NewMockJiraClient()
	assert.Equal(t, "https://mock.atlassian.net", mock.BaseURL())

	mock.SetBaseURL("https://custom.atlassian.net")
	assert.Equal(t, "https://custom.atlassian.net", mock.BaseURL())
}

func TestMockJiraClient_Reset(t *testing.T) {
	mock := NewMockJiraClient()

	// Make some calls - errors intentionally ignored as we're testing call tracking
	_, _ = mock.CreateIssue(context.Background(), ports.CreateIssueRequest{Project: "TEST", Summary: "Test"})
	_ = mock.UpdateIssue(context.Background(), "TEST-1", ports.UpdateIssueRequest{Summary: "Updated"})
	_ = mock.CreateLink(context.Background(), "TEST-2", "TEST-1", "Blocks")
	_, _ = mock.GetIssue(context.Background(), "TEST-1")

	assert.Len(t, mock.CreateIssueCalls, 1)
	assert.Len(t, mock.UpdateIssueCalls, 1)
	assert.Len(t, mock.CreateLinkCalls, 1)
	assert.Len(t, mock.GetIssueCalls, 1)

	// Reset
	mock.Reset()

	assert.Len(t, mock.CreateIssueCalls, 0)
	assert.Len(t, mock.UpdateIssueCalls, 0)
	assert.Len(t, mock.CreateLinkCalls, 0)
	assert.Len(t, mock.GetIssueCalls, 0)
}

func TestMockJiraClient_AutoIncrementKeys(t *testing.T) {
	mock := NewMockJiraClient()

	issue1, _ := mock.CreateIssue(context.Background(), ports.CreateIssueRequest{Project: "TEST", Summary: "First"})
	issue2, _ := mock.CreateIssue(context.Background(), ports.CreateIssueRequest{Project: "TEST", Summary: "Second"})
	issue3, _ := mock.CreateIssue(context.Background(), ports.CreateIssueRequest{Project: "OTHER", Summary: "Third"})

	assert.Equal(t, "TEST-1", issue1.Key)
	assert.Equal(t, "TEST-2", issue2.Key)
	assert.Equal(t, "OTHER-3", issue3.Key)
}

func TestMockJiraClient_GetIssueWithLinks_DefaultBehavior(t *testing.T) {
	mock := NewMockJiraClient()

	issue, err := mock.GetIssueWithLinks(context.Background(), "TEST-1")

	require.NoError(t, err)
	assert.Equal(t, "TEST-1", issue.Key)
	assert.Equal(t, "https://mock.atlassian.net/browse/TEST-1", issue.URL)
	assert.Equal(t, "MOCK", issue.Project)
	assert.Equal(t, "Mock Issue", issue.Summary)
	assert.Equal(t, "2026-01-15T14:30:45.000+0000", issue.Created)
	assert.Len(t, mock.GetIssueWithLinksCalls, 1)
}

func TestMockJiraClient_GetIssueWithLinks_CustomBehavior(t *testing.T) {
	mock := NewMockJiraClient()
	mock.GetIssueWithLinksFunc = func(_ context.Context, key string) (*ports.IssueWithLinks, error) {
		return &ports.IssueWithLinks{
			Key:         key,
			URL:         "https://custom.url/browse/" + key,
			Project:     "CUSTOM",
			Summary:     "Custom Summary",
			Description: "Custom Description",
			Status:      "In Progress",
			Parent:      "CUSTOM-100",
			Created:     "2026-01-20T10:00:00.000+0000",
			Links: []ports.IssueLink{
				{ID: "link-1", Type: "Blocks", InwardIssue: "CUSTOM-50"},
			},
		}, nil
	}

	issue, err := mock.GetIssueWithLinks(context.Background(), "CUSTOM-123")

	require.NoError(t, err)
	assert.Equal(t, "CUSTOM-123", issue.Key)
	assert.Equal(t, "CUSTOM", issue.Project)
	assert.Equal(t, "CUSTOM-100", issue.Parent)
	assert.Len(t, issue.Links, 1)
	assert.Equal(t, "Blocks", issue.Links[0].Type)
}

func TestMockJiraClient_GetIssueWithLinks_StoredIssue(t *testing.T) {
	mock := NewMockJiraClient()
	mock.AddStoredIssue(&ports.IssueWithLinks{
		Key:         "STORED-1",
		URL:         "https://stored.url/browse/STORED-1",
		Project:     "STORED",
		Summary:     "Stored Issue",
		Description: "Stored Description",
		Status:      "Done",
		Parent:      "STORED-100",
		Created:     "2026-01-10T08:00:00.000+0000",
	})

	issue, err := mock.GetIssueWithLinks(context.Background(), "STORED-1")

	require.NoError(t, err)
	assert.Equal(t, "STORED-1", issue.Key)
	assert.Equal(t, "STORED", issue.Project)
	assert.Equal(t, "Stored Issue", issue.Summary)
	assert.Equal(t, "STORED-100", issue.Parent)
}

func TestMockJiraClient_GetIssueWithLinks_Error(t *testing.T) {
	mock := NewMockJiraClient()
	mock.GetIssueWithLinksFunc = func(_ context.Context, _ string) (*ports.IssueWithLinks, error) {
		return nil, errors.New("issue not found")
	}

	_, err := mock.GetIssueWithLinks(context.Background(), "NOTFOUND-1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue not found")
}
