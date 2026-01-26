// Package config provides configuration management for jira-sync.
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

var (
	errJiraURLRequired   = errors.New("jira.url is required (set JIRA_URL or use config file)")
	errJiraUserRequired  = errors.New("jira.user is required (set JIRA_USER or use config file)")
	errJiraTokenRequired = errors.New("JIRA_TOKEN environment variable is required")
)

// Config holds all configuration for jira-sync.
type Config struct {
	Jira      JiraConfig
	Defaults  DefaultsConfig
	LinkTypes LinkTypesConfig
}

// JiraConfig holds Jira connection settings.
type JiraConfig struct {
	URL   string // From JIRA_URL env var or config file
	User  string // From JIRA_USER env var or config file
	Token string // From JIRA_TOKEN env var ONLY (never in config file)
}

// DefaultsConfig holds default values for ticket creation.
type DefaultsConfig struct {
	Project       string
	IssueType     string
	EndDateOffset int // days from start
}

// LinkTypesConfig holds Jira link type names.
type LinkTypesConfig struct {
	Dependency string // e.g., "Blocks"
}

// LoadFromFile loads configuration from a specific file path.
func LoadFromFile(path string) (*Config, error) {
	viperInstance := viper.New()

	viperInstance.SetConfigFile(path)

	// Set up environment variables
	setupEnvVars(viperInstance)

	// Read config file
	if err := viperInstance.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	return buildConfig(viperInstance)
}

// LoadFromEnv loads configuration from environment variables only.
func LoadFromEnv() (*Config, error) {
	viperInstance := viper.New()

	// Set up environment variables
	setupEnvVars(viperInstance)

	return buildConfig(viperInstance)
}

// setupEnvVars configures Viper to read from environment variables.
func setupEnvVars(viperInstance *viper.Viper) {
	viperInstance.SetEnvPrefix("JIRA")
	viperInstance.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viperInstance.AutomaticEnv()

	// Bind specific keys to ensure proper mapping - errors ignored as these are static bindings
	_ = viperInstance.BindEnv("url", "JIRA_URL")
	_ = viperInstance.BindEnv("user", "JIRA_USER")
	_ = viperInstance.BindEnv("token", "JIRA_TOKEN")
	_ = viperInstance.BindEnv("jira.url", "JIRA_URL")
	_ = viperInstance.BindEnv("jira.user", "JIRA_USER")
	_ = viperInstance.BindEnv("defaults.project", "JIRA_DEFAULTS_PROJECT")
	_ = viperInstance.BindEnv("defaults.issue_type", "JIRA_DEFAULTS_ISSUE_TYPE")
	_ = viperInstance.BindEnv("defaults.end_date_offset", "JIRA_DEFAULTS_END_DATE_OFFSET")
	_ = viperInstance.BindEnv("link_types.dependency", "JIRA_LINK_TYPES_DEPENDENCY")
}

// buildConfig creates a Config from Viper values.
func buildConfig(viperInstance *viper.Viper) (*Config, error) {
	cfg := &Config{
		Jira: JiraConfig{
			URL:   getStringWithFallback(viperInstance, "jira.url", "url"),
			User:  getStringWithFallback(viperInstance, "jira.user", "user"),
			Token: viperInstance.GetString("token"),
		},
		Defaults: DefaultsConfig{
			Project:       viperInstance.GetString("defaults.project"),
			IssueType:     viperInstance.GetString("defaults.issue_type"),
			EndDateOffset: viperInstance.GetInt("defaults.end_date_offset"),
		},
		LinkTypes: LinkTypesConfig{
			Dependency: viperInstance.GetString("link_types.dependency"),
		},
	}

	// Apply defaults
	if cfg.Defaults.IssueType == "" {
		cfg.Defaults.IssueType = "Story"
	}
	if cfg.Defaults.EndDateOffset == 0 {
		cfg.Defaults.EndDateOffset = 7
	}
	if cfg.LinkTypes.Dependency == "" {
		cfg.LinkTypes.Dependency = "Blocks"
	}

	// Validate required fields
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// getStringWithFallback tries primary key, then fallback key.
func getStringWithFallback(viperInstance *viper.Viper, primary, fallback string) string {
	if val := viperInstance.GetString(primary); val != "" {
		return val
	}
	return viperInstance.GetString(fallback)
}

// Validate checks that required fields are present.
func (c *Config) Validate() error {
	if c.Jira.URL == "" {
		return errJiraURLRequired
	}
	if c.Jira.User == "" {
		return errJiraUserRequired
	}
	if c.Jira.Token == "" {
		return errJiraTokenRequired
	}
	return nil
}
