package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/curtbushko/jira-sync/internal/adapters/jira"
)

var debugCmd = &cobra.Command{
	Use:   "debug <jira-key>",
	Short: "Debug: show raw Jira link data for an issue",
	Long:  `Fetches and displays the raw link data from Jira for debugging purposes.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDebug,
}

func init() {
	rootCmd.AddCommand(debugCmd)
}

func runDebug(_ *cobra.Command, args []string) error {
	issueKey := args[0]

	slog.Debug("debug command started", slog.String("issue_key", issueKey))

	jiraURL := viper.GetString("jira.url")
	jiraUser := viper.GetString("jira.user")
	jiraToken := viper.GetString("token")

	slog.Debug("jira config",
		slog.String("jira_url", jiraURL),
		slog.String("jira_user", jiraUser),
		slog.Bool("has_token", jiraToken != ""),
	)

	if jiraURL == "" || jiraUser == "" || jiraToken == "" {
		slog.Debug("missing jira configuration")
		return errors.New("jira.url, jira.user, and token are required")
	}

	client, err := jira.NewClient(jiraURL, jiraUser, jiraToken)
	if err != nil {
		slog.Debug("failed to create jira client", slog.String("error", err.Error()))
		return fmt.Errorf("create jira client: %w", err)
	}

	slog.Debug("fetching issue links", slog.String("issue_key", issueKey))
	color.Cyan("Fetching links for %s...\n\n", issueKey)

	links, err := client.GetIssueLinks(context.Background(), issueKey)
	if err != nil {
		slog.Debug("failed to get issue links", slog.String("issue_key", issueKey), slog.String("error", err.Error()))
		return fmt.Errorf("get issue links: %w", err)
	}

	slog.Debug("fetched issue links", slog.String("issue_key", issueKey), slog.Int("count", len(links)))

	if len(links) == 0 {
		color.Yellow("No links found for %s\n", issueKey)
		return nil
	}

	fmt.Printf("Found %d links:\n\n", len(links))
	for index, link := range links {
		slog.Debug("link details",
			slog.Int("index", index),
			slog.String("id", link.ID),
			slog.String("type", link.Type),
			slog.String("inward_issue", link.InwardIssue),
			slog.String("outward_issue", link.OutwardIssue),
		)
		fmt.Printf("[%d] ID=%q\n", index, link.ID)
		fmt.Printf("    Type=%q\n", link.Type)
		fmt.Printf("    InwardIssue=%q\n", link.InwardIssue)
		fmt.Printf("    OutwardIssue=%q\n", link.OutwardIssue)
		fmt.Println()
	}

	// Also show what the config says
	linkType := viper.GetString("link_types.dependency")
	if linkType == "" {
		linkType = "Blocking"
	}
	slog.Debug("configured link type", slog.String("link_type", linkType))
	color.Cyan("Configured link_types.dependency: %q\n", linkType)

	return nil
}
