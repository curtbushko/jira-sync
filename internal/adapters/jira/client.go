package jira

import (
	"context"
	"fmt"
	"io"
	"time"

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

// GetTransitions returns available transitions for an issue.
func (c *Client) GetTransitions(ctx context.Context, key string) ([]ports.Transition, error) {
	transitions, resp, err := c.client.Issue.GetTransitionsWithContext(ctx, key)
	if err != nil {
		if resp != nil {
			body := readResponseBody(resp)
			return nil, fmt.Errorf("get transitions for %s (status: %d): %s: %w", key, resp.StatusCode, body, err)
		}
		return nil, fmt.Errorf("get transitions for %s: %w", key, err)
	}

	result := make([]ports.Transition, len(transitions))
	for i, t := range transitions {
		result[i] = ports.Transition{
			ID:   t.ID,
			Name: t.Name,
		}
	}

	return result, nil
}

// DoTransition performs a workflow transition on an issue.
func (c *Client) DoTransition(ctx context.Context, key, transitionID string) error {
	resp, err := c.client.Issue.DoTransitionWithContext(ctx, key, transitionID)
	if err != nil {
		if resp != nil {
			body := readResponseBody(resp)
			return fmt.Errorf("%w for %s (status: %d): %s: %w", domain.ErrJiraTransitionFailed, key, resp.StatusCode, body, err)
		}
		return fmt.Errorf("%w for %s: %w", domain.ErrJiraTransitionFailed, key, err)
	}

	return nil
}

// GetIssueLinks returns all links for an issue.
func (c *Client) GetIssueLinks(ctx context.Context, key string) ([]ports.IssueLink, error) {
	// Fetch issue with expanded links
	opts := &jira.GetQueryOptions{
		Expand: "issuelinks",
	}
	issue, resp, err := c.client.Issue.GetWithContext(ctx, key, opts)
	if err != nil {
		if resp != nil {
			body := readResponseBody(resp)
			return nil, fmt.Errorf("get issue links for %s (status: %d): %s: %w", key, resp.StatusCode, body, err)
		}
		return nil, fmt.Errorf("get issue links for %s: %w", key, err)
	}

	if issue.Fields == nil || issue.Fields.IssueLinks == nil {
		return []ports.IssueLink{}, nil
	}

	var links []ports.IssueLink
	for _, link := range issue.Fields.IssueLinks {
		issueLink := ports.IssueLink{
			ID:   link.ID,
			Type: link.Type.Name,
		}

		// Set inward/outward issue keys
		if link.InwardIssue != nil {
			issueLink.InwardIssue = link.InwardIssue.Key
		}
		if link.OutwardIssue != nil {
			issueLink.OutwardIssue = link.OutwardIssue.Key
		}

		links = append(links, issueLink)
	}

	return links, nil
}

// DeleteLink removes an issue link by ID.
func (c *Client) DeleteLink(ctx context.Context, linkID string) error {
	resp, err := c.client.Issue.DeleteLinkWithContext(ctx, linkID)
	if err != nil {
		if resp != nil {
			body := readResponseBody(resp)
			return fmt.Errorf("delete link %s (status: %d): %s: %w", linkID, resp.StatusCode, body, err)
		}
		return fmt.Errorf("delete link %s: %w", linkID, err)
	}

	return nil
}

// GetIssueWithLinks fetches an issue with expanded links for export.
func (c *Client) GetIssueWithLinks(ctx context.Context, key string) (*ports.IssueWithLinks, error) {
	// Fetch issue with expanded links and parent
	opts := &jira.GetQueryOptions{
		Expand: "issuelinks",
	}
	issue, resp, err := c.client.Issue.GetWithContext(ctx, key, opts)
	if err != nil {
		if resp != nil {
			body := readResponseBody(resp)
			return nil, fmt.Errorf("get issue %s (status: %d): %s: %w", key, resp.StatusCode, body, err)
		}
		return nil, fmt.Errorf("get issue %s: %w", key, err)
	}

	result := &ports.IssueWithLinks{
		Key: issue.Key,
		URL: fmt.Sprintf("%s/browse/%s", c.baseURL, issue.Key),
	}

	if issue.Fields != nil {
		result.Summary = issue.Fields.Summary
		result.Description = issue.Fields.Description

		if issue.Fields.Status != nil {
			result.Status = issue.Fields.Status.Name
		}

		if issue.Fields.Project.Key != "" {
			result.Project = issue.Fields.Project.Key
		}

		if issue.Fields.Parent != nil {
			result.Parent = issue.Fields.Parent.Key
		}

		// Extract creation date from the Created field
		// jira.Time is a wrapper around time.Time, format it back to Jira string format
		createdTime := time.Time(issue.Fields.Created)
		if !createdTime.IsZero() {
			result.Created = createdTime.Format("2006-01-02T15:04:05.000-0700")
		}

		// Extract issue links
		if issue.Fields.IssueLinks != nil {
			for _, link := range issue.Fields.IssueLinks {
				issueLink := ports.IssueLink{
					ID:   link.ID,
					Type: link.Type.Name,
				}
				if link.InwardIssue != nil {
					issueLink.InwardIssue = link.InwardIssue.Key
				}
				if link.OutwardIssue != nil {
					issueLink.OutwardIssue = link.OutwardIssue.Key
				}
				result.Links = append(result.Links, issueLink)
			}
		}
	}

	return result, nil
}
