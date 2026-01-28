package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/curtbushko/jira-sync/internal/adapters/jira"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

	jiraURL := viper.GetString("jira.url")
	jiraUser := viper.GetString("jira.user")
	jiraToken := viper.GetString("token")

	if jiraURL == "" || jiraUser == "" || jiraToken == "" {
		return errors.New("jira.url, jira.user, and token are required")
	}

	client, err := jira.NewClient(jiraURL, jiraUser, jiraToken)
	if err != nil {
		return fmt.Errorf("create jira client: %w", err)
	}

	color.Cyan("Fetching links for %s...\n\n", issueKey)

	links, err := client.GetIssueLinks(context.Background(), issueKey)
	if err != nil {
		return fmt.Errorf("get issue links: %w", err)
	}

	if len(links) == 0 {
		color.Yellow("No links found for %s\n", issueKey)
		return nil
	}

	fmt.Printf("Found %d links:\n\n", len(links))
	for i, link := range links {
		fmt.Printf("[%d] ID=%q\n", i, link.ID)
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
	color.Cyan("Configured link_types.dependency: %q\n", linkType)

	return nil
}
