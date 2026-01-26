// Package cmd provides CLI commands for jira-sync.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "jira-sync",
	Short: "Sync markdown task files with Jira",
	Long: `jira-sync manages Jira tickets from local markdown files.

It supports batch creation, dependency linking, and bidirectional sync
between local task files and Jira issues.

Commands:
  create    Create a new task file
  push      Push local changes to Jira
  pull      Pull Jira changes to local files`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default is $HOME/.jira-sync.yaml)")

	// Jira connection flags (can be overridden by env vars)
	rootCmd.PersistentFlags().String("jira-url", "", "Jira instance URL")
	rootCmd.PersistentFlags().String("jira-user", "", "Jira username/email")

	// Bind flags to viper - errors ignored as flags are defined above
	_ = viper.BindPFlag("jira.url", rootCmd.PersistentFlags().Lookup("jira-url"))
	_ = viper.BindPFlag("jira.user", rootCmd.PersistentFlags().Lookup("jira-user"))
}

func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config in home directory and current directory
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
		}
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".jira-sync")
	}

	// Environment variable support
	viper.SetEnvPrefix("JIRA")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	// Bind specific keys to ensure proper mapping - errors ignored as these are static bindings
	_ = viper.BindEnv("url", "JIRA_URL")
	_ = viper.BindEnv("user", "JIRA_USER")
	_ = viper.BindEnv("token", "JIRA_TOKEN")
	_ = viper.BindEnv("jira.url", "JIRA_URL")
	_ = viper.BindEnv("jira.user", "JIRA_USER")
	_ = viper.BindEnv("defaults.project", "JIRA_DEFAULTS_PROJECT")
	_ = viper.BindEnv("defaults.issue_type", "JIRA_DEFAULTS_ISSUE_TYPE")
	_ = viper.BindEnv("defaults.end_date_offset", "JIRA_DEFAULTS_END_DATE_OFFSET")
	_ = viper.BindEnv("link_types.dependency", "JIRA_LINK_TYPES_DEPENDENCY")

	// Read config file (ignore if not found)
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
