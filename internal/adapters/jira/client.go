package jira

import (
	"context"
	"fmt"
	"io"

	jira "github.com/andygrunwald/go-jira"
	"github.com/curtbushko/jira-sync/internal/domain"
	"github.com/curtbushko/jira-sync/internal/ports"
)

// readResponseBody reads and returns the response body for error reporting.
func readResponseBody(resp *jira.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("(failed to read body: %v)", err)
	}
	return string(body)
}

// Client wraps the go-jira client with our configuration.
type Client struct {
	client  *jira.Client
	baseURL string
}

// NewClient creates a new Jira client using Basic Auth with API token.
// The token should come from JIRA_TOKEN environment variable.
func NewClient(url, user, token string) (*Client, error) {
	tp := jira.BasicAuthTransport{
		Username: user,
		Password: token, // API token, not password
	}

	client, err := jira.NewClient(tp.Client(), url)
	if err != nil {
		return nil, fmt.Errorf("create jira client: %w", err)
	}

	// Verify connection
	_, _, err = client.User.GetSelf()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrJiraAuthentication, err)
	}

	return &Client{
		client:  client,
		baseURL: url,
	}, nil
}

// CreateIssue creates a new issue and returns the created issue with key.
func (c *Client) CreateIssue(ctx context.Context, req ports.CreateIssueRequest) (*ports.Issue, error) {
	fields := &jira.IssueFields{
		Project:     jira.Project{Key: req.Project},
		Summary:     req.Summary,
		Description: req.Description,
		Type:        jira.IssueType{Name: req.IssueType},
	}

	// Set parent if provided
	if req.Parent != "" {
		fields.Parent = &jira.Parent{Key: req.Parent}
	}

	issue := &jira.Issue{
		Fields: fields,
	}

	created, resp, err := c.client.Issue.CreateWithContext(ctx, issue)
	if err != nil {
		if resp != nil {
			body := readResponseBody(resp)
			return nil, fmt.Errorf("%w (status: %d): %s: %w", domain.ErrJiraCreateFailed, resp.StatusCode, body, err)
		}
		return nil, fmt.Errorf("%w: %w", domain.ErrJiraCreateFailed, err)
	}

	return &ports.Issue{
		Key:         created.Key,
		Self:        created.Self,
		Summary:     req.Summary,
		Description: req.Description,
	}, nil
}

// UpdateIssue updates an existing issue.
func (c *Client) UpdateIssue(ctx context.Context, key string, req ports.UpdateIssueRequest) error {
	issue := &jira.Issue{
		Key: key,
		Fields: &jira.IssueFields{
			Summary:     req.Summary,
			Description: req.Description,
		},
	}

	_, resp, err := c.client.Issue.UpdateWithContext(ctx, issue)
	if err != nil {
		if resp != nil {
			body := readResponseBody(resp)
			return fmt.Errorf("%w (status: %d): %s: %w", domain.ErrJiraUpdateFailed, resp.StatusCode, body, err)
		}
		return fmt.Errorf("%w: %w", domain.ErrJiraUpdateFailed, err)
	}

	return nil
}

// CreateLink creates a dependency link between two issues.
// inward is the blocked issue, outward is the blocker.
func (c *Client) CreateLink(ctx context.Context, inward, outward, linkType string) error {
	link := &jira.IssueLink{
		Type: jira.IssueLinkType{
			Name: linkType,
		},
		InwardIssue: &jira.Issue{
			Key: inward,
		},
		OutwardIssue: &jira.Issue{
			Key: outward,
		},
	}

	resp, err := c.client.Issue.AddLinkWithContext(ctx, link)
	if err != nil {
		if resp != nil {
			body := readResponseBody(resp)
			return fmt.Errorf("%w (status: %d): %s: %w", domain.ErrJiraLinkFailed, resp.StatusCode, body, err)
		}
		return fmt.Errorf("%w: %w", domain.ErrJiraLinkFailed, err)
	}

	return nil
}

// GetIssue fetches an issue by key.
func (c *Client) GetIssue(ctx context.Context, key string) (*ports.Issue, error) {
	issue, resp, err := c.client.Issue.GetWithContext(ctx, key, nil)
	if err != nil {
		if resp != nil {
			body := readResponseBody(resp)
			return nil, fmt.Errorf("get issue %s (status: %d): %s: %w", key, resp.StatusCode, body, err)
		}
		return nil, fmt.Errorf("get issue %s: %w", key, err)
	}

	var status string
	if issue.Fields != nil && issue.Fields.Status != nil {
		status = issue.Fields.Status.Name
	}

	return &ports.Issue{
		Key:         issue.Key,
		Self:        issue.Self,
		Summary:     issue.Fields.Summary,
		Description: issue.Fields.Description,
		Status:      status,
	}, nil
}

// BaseURL returns the Jira instance base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}
