package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

// StaticConfig contains configuration that is fixed when the server starts.
type StaticConfig struct {
	Port                  int      `mapstructure:"port"`
	Listen                string   `mapstructure:"listen"`
	SSEBaseURL            string   `mapstructure:"sse_base_url"`
	LogLevel              string   `mapstructure:"log_level"`
	ConsoleURL            string   `mapstructure:"console_url"`
	ConsoleUsername       string   `mapstructure:"console_username"`
	ConsolePassword       string   `mapstructure:"console_password"`
	ConsoleAuthMode       string   `mapstructure:"console_auth_mode"`
	ConsoleTimeoutSeconds int      `mapstructure:"console_timeout_seconds"`
	EnabledTools          []string `mapstructure:"enabled_tools"`
	DisabledTools         []string `mapstructure:"disabled_tools"`
	EnabledDomains        []string `mapstructure:"enabled_domains"`
	DisabledDomains       []string `mapstructure:"disabled_domains"`
}

// Validate checks whether the configuration can be used to start the server.
func (c *StaticConfig) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535, got %d", c.Port)
	}
	if c.Port != 0 && strings.TrimSpace(c.Listen) == "" {
		return fmt.Errorf("listen must be set when port is non-zero")
	}
	if _, err := zerolog.ParseLevel(c.LogLevel); err != nil {
		return fmt.Errorf("invalid log_level %q: %w", c.LogLevel, err)
	}
	consoleURL, err := url.Parse(c.ConsoleURL)
	if err != nil || (consoleURL.Scheme != "http" && consoleURL.Scheme != "https") || consoleURL.Host == "" {
		return fmt.Errorf("console_url must be an absolute http(s) URL, got %q", c.ConsoleURL)
	}
	if (c.ConsoleUsername == "") != (c.ConsolePassword == "") {
		return fmt.Errorf("console_username and console_password must be set together")
	}
	if c.ConsoleAuthMode != "session" && c.ConsoleAuthMode != "basic" {
		return fmt.Errorf("console_auth_mode must be session or basic, got %q", c.ConsoleAuthMode)
	}
	if c.ConsoleTimeoutSeconds < 1 || c.ConsoleTimeoutSeconds > 300 {
		return fmt.Errorf("console_timeout_seconds must be between 1 and 300, got %d", c.ConsoleTimeoutSeconds)
	}
	return nil
}

// GetListenAddress returns the configured HTTP listen address (host:port).
func (c *StaticConfig) GetListenAddress() string {
	if c.Port == 0 {
		return ""
	}
	return net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))
}

// LoadConfig loads configuration from defaults, optional YAML, environment, and flags.
func LoadConfig(configPath string, v *viper.Viper) (*StaticConfig, error) {
	if v == nil {
		v = viper.New()
	}

	defaults := map[string]any{
		"port":                    0,
		"listen":                  "127.0.0.1",
		"sse_base_url":            "",
		"log_level":               "info",
		"console_url":             "http://127.0.0.1:8080",
		"console_username":        "",
		"console_password":        "",
		"console_auth_mode":       "session",
		"console_timeout_seconds": 30,
		"enabled_tools":           []string{},
		"disabled_tools":          []string{},
		"enabled_domains":         []string{},
		"disabled_domains":        []string{},
	}
	for key, value := range defaults {
		v.SetDefault(key, value)
	}

	if configPath != "" {
		v.SetConfigFile(configPath)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
	}

	envBindings := map[string]string{
		"port":                    "MCP_PORT",
		"listen":                  "MCP_LISTEN",
		"sse_base_url":            "MCP_SSE_BASE_URL",
		"log_level":               "MCP_LOG_LEVEL",
		"console_url":             "MCP_HIGRESS_CONSOLE_URL",
		"console_username":        "MCP_HIGRESS_CONSOLE_USERNAME",
		"console_password":        "MCP_HIGRESS_CONSOLE_PASSWORD",
		"console_auth_mode":       "MCP_HIGRESS_CONSOLE_AUTH_MODE",
		"console_timeout_seconds": "MCP_HIGRESS_CONSOLE_TIMEOUT_SECONDS",
		"enabled_tools":           "MCP_ENABLED_TOOLS",
		"disabled_tools":          "MCP_DISABLED_TOOLS",
		"enabled_domains":         "MCP_ENABLED_DOMAINS",
		"disabled_domains":        "MCP_DISABLED_DOMAINS",
	}
	for key, variable := range envBindings {
		if err := v.BindEnv(key, variable); err != nil {
			return nil, fmt.Errorf("bind environment variable %s: %w", variable, err)
		}
	}

	cfg := &StaticConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	cfg.ConsoleURL = strings.TrimRight(strings.TrimSpace(cfg.ConsoleURL), "/")
	cfg.ConsoleUsername = strings.TrimSpace(cfg.ConsoleUsername)
	cfg.ConsoleAuthMode = strings.ToLower(strings.TrimSpace(cfg.ConsoleAuthMode))
	cfg.EnabledTools = normalizeStringSlice(cfg.EnabledTools)
	cfg.DisabledTools = normalizeStringSlice(cfg.DisabledTools)
	cfg.EnabledDomains = normalizeStringSlice(cfg.EnabledDomains)
	cfg.DisabledDomains = normalizeStringSlice(cfg.DisabledDomains)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func normalizeStringSlice(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}
