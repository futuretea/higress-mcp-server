package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/futuretea/higress-mcp-server/internal/toolcatalog"
	"github.com/futuretea/higress-mcp-server/pkg/console"
	"github.com/futuretea/higress-mcp-server/pkg/core/config"
	"github.com/futuretea/higress-mcp-server/pkg/core/version"
	"github.com/futuretea/higress-mcp-server/pkg/toolset"
	"github.com/futuretea/higress-mcp-server/pkg/toolset/higress"
)

// IOStreams groups the streams used by the CLI.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

// NewRootCommand creates the CLI root. The MCP server is started via the `mcp` subcommand.
func NewRootCommand(streams IOStreams) *cobra.Command {
	var cfgFile string
	v := viper.New()

	command := &cobra.Command{
		Use:           version.BinaryName,
		Short:         "Higress Console MCP server and command-line client",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	command.SetIn(streams.In)
	command.SetOut(streams.Out)
	command.SetErr(streams.ErrOut)

	addCommonFlags(command, &cfgFile)
	command.AddCommand(newMCPCommand(streams, &cfgFile, v))
	command.AddCommand(newToolsCommand(streams, &cfgFile, v))
	command.AddCommand(newVersionCommand(streams))
	command.AddCommand(newCompletionCommand(streams))
	return command
}

func addCommonFlags(command *cobra.Command, cfgFile *string) {
	flags := command.PersistentFlags()
	flags.StringVar(cfgFile, "config", "", "config file path (YAML)")
	flags.String("console-url", "http://127.0.0.1:8080", "Higress Console base URL")
	flags.String("console-auth-mode", "session", "Console authentication mode: session or basic")
	flags.Int("console-timeout-seconds", 30, "Console request timeout in seconds")
	flags.StringSlice("enabled-tools", []string{}, "comma-separated list of tool names to enable")
	flags.StringSlice("disabled-tools", []string{}, "comma-separated list of tool names to disable")
	flags.StringSlice("enable-domains", []string{}, "comma-separated list of tool domains to enable")
	flags.StringSlice("disable-domains", []string{}, "comma-separated list of tool domains to disable")
	flags.Bool("read-only", true, "expose only read-only Higress Console tools")
}

func bindCommonFlags(v *viper.Viper, cmd *cobra.Command) error {
	flags := cmd.Root().PersistentFlags()
	bindings := map[string]string{
		"console_url":             "console-url",
		"console_auth_mode":       "console-auth-mode",
		"console_timeout_seconds": "console-timeout-seconds",
		"enabled_tools":           "enabled-tools",
		"disabled_tools":          "disabled-tools",
		"enabled_domains":         "enable-domains",
		"disabled_domains":        "disable-domains",
		"read_only":               "read-only",
	}
	for key, flag := range bindings {
		if err := v.BindPFlag(key, flags.Lookup(flag)); err != nil {
			return fmt.Errorf("bind flag %s: %w", flag, err)
		}
	}
	return nil
}

func loadCLIConfig(cmd *cobra.Command, cfgFile string, v *viper.Viper) (*config.StaticConfig, error) {
	if err := bindCommonFlags(v, cmd); err != nil {
		return nil, err
	}
	cfg, err := config.LoadConfig(cfgFile, v)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func defaultToolsets(cfg *config.StaticConfig) ([]toolset.Toolset, error) {
	if cfg == nil {
		return nil, fmt.Errorf("static config is required")
	}
	client, err := console.NewClient(console.Config{
		BaseURL:        cfg.ConsoleURL,
		Username:       cfg.ConsoleUsername,
		Password:       cfg.ConsolePassword,
		AuthMode:       cfg.ConsoleAuthMode,
		TimeoutSeconds: cfg.ConsoleTimeoutSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("create Console client: %w", err)
	}
	consoleToolset, err := higress.New(client, cfg.ReadOnly)
	if err != nil {
		return nil, fmt.Errorf("create Higress toolset: %w", err)
	}
	return []toolset.Toolset{consoleToolset}, nil
}

func buildToolCatalog(cfg *config.StaticConfig) ([]toolset.ServerTool, error) {
	toolsets, err := defaultToolsets(cfg)
	if err != nil {
		return nil, err
	}
	return toolcatalog.Build(toolsets, toolset.FilterOptions{
		EnabledTools:    cfg.EnabledTools,
		DisabledTools:   cfg.DisabledTools,
		EnabledDomains:  cfg.EnabledDomains,
		DisabledDomains: cfg.DisabledDomains,
	})
}

func newVersionCommand(streams IOStreams) *cobra.Command {
	command := &cobra.Command{
		Use:   "version",
		Short: "print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(streams.Out, version.GetVersionInfo())
			return err
		},
	}
	command.SetOut(streams.Out)
	command.SetErr(streams.ErrOut)
	return command
}
