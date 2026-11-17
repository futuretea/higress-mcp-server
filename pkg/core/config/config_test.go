package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/futuretea/higress-mcp-server/pkg/core/config"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := config.LoadConfig("", viper.New())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Port != 0 {
		t.Fatalf("expected port 0, got %d", cfg.Port)
	}
	if cfg.Listen != "127.0.0.1" {
		t.Fatalf("expected listen 127.0.0.1, got %q", cfg.Listen)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected log_level info, got %q", cfg.LogLevel)
	}
	if cfg.ConsoleURL != "http://127.0.0.1:8080" {
		t.Fatalf("expected default console_url, got %q", cfg.ConsoleURL)
	}
	if !cfg.ReadOnly {
		t.Fatal("expected read_only to default to true")
	}
}

func TestLoadConfigReadOnlyFalseOverrides(t *testing.T) {
	t.Run("YAML", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(configPath, []byte("read_only: false\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		assertReadOnly(t, configPath, viper.New(), false)
	})

	t.Run("environment", func(t *testing.T) {
		t.Setenv("MCP_READ_ONLY", "false")
		assertReadOnly(t, "", viper.New(), false)
	})

	t.Run("flag", func(t *testing.T) {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flags.Bool("read-only", true, "")
		if err := flags.Parse([]string{"--read-only=false"}); err != nil {
			t.Fatalf("parse flag: %v", err)
		}

		v := viper.New()
		if err := v.BindPFlag("read_only", flags.Lookup("read-only")); err != nil {
			t.Fatalf("bind flag: %v", err)
		}
		assertReadOnly(t, "", v, false)
	})
}

func assertReadOnly(t *testing.T, configPath string, v *viper.Viper, want bool) {
	t.Helper()
	cfg, err := config.LoadConfig(configPath, v)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ReadOnly != want {
		t.Fatalf("ReadOnly = %t, want %t", cfg.ReadOnly, want)
	}
}

func TestLoadConfigRejectsBlankOnlyNameLists(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "enabled tools",
			yaml: "enabled_tools:\n  - \" \"\n  - \"\"\n",
			want: "enabled_tools contains no names",
		},
		{
			name: "disabled tools",
			yaml: "disabled_tools:\n  - \" \"\n",
			want: "disabled_tools contains no names",
		},
		{
			name: "enabled domains",
			yaml: "enabled_domains:\n  - \"\t\"\n",
			want: "enabled_domains contains no names",
		},
		{
			name: "disabled domains",
			yaml: "disabled_domains:\n  - \" \"\n  - \"  \"\n",
			want: "disabled_domains contains no names",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.LoadConfig(writeConfigFile(t, tt.yaml), viper.New())
			if err == nil || err.Error() != tt.want {
				t.Fatalf("LoadConfig error = %v, want %q", err, tt.want)
			}
		})
	}

	t.Run("blank environment overrides allow list", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", " , ")
		_, err := config.LoadConfig(writeConfigFile(t, "enabled_tools:\n  - higress_list_resource\n"), viper.New())
		if err == nil || err.Error() != "enabled_tools contains no names" {
			t.Fatalf("LoadConfig error = %v, want blank enabled_tools error", err)
		}
	})

	t.Run("blank flag overrides allow list", func(t *testing.T) {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flags.StringSlice("enabled-tools", []string{}, "")
		if err := flags.Parse([]string{"--enabled-tools", " , "}); err != nil {
			t.Fatalf("parse flag: %v", err)
		}
		v := viper.New()
		if err := v.BindPFlag("enabled_tools", flags.Lookup("enabled-tools")); err != nil {
			t.Fatalf("bind flag: %v", err)
		}
		_, err := config.LoadConfig(writeConfigFile(t, "enabled_tools:\n  - higress_list_resource\n"), v)
		if err == nil || err.Error() != "enabled_tools contains no names" {
			t.Fatalf("LoadConfig error = %v, want blank enabled_tools error", err)
		}
	})
}

func TestLoadConfigKeepsEmptyMixedAndEmptyEnvironmentNameLists(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		cfg, err := config.LoadConfig(writeConfigFile(t, "enabled_tools: []\n"), viper.New())
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if len(cfg.EnabledTools) != 0 {
			t.Fatalf("EnabledTools = %#v, want empty", cfg.EnabledTools)
		}
	})

	t.Run("mixed blanks and names", func(t *testing.T) {
		cfg, err := config.LoadConfig(writeConfigFile(t, "enabled_tools:\n  - \" \"\n  - higress_get_resource\n  - \"\"\n  - higress_list_resource\n"), viper.New())
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		want := []string{"higress_get_resource", "higress_list_resource"}
		if !slices.Equal(cfg.EnabledTools, want) {
			t.Fatalf("EnabledTools = %#v, want %#v", cfg.EnabledTools, want)
		}
	})

	t.Run("unknown name stays", func(t *testing.T) {
		cfg, err := config.LoadConfig(writeConfigFile(t, "enabled_tools:\n  - not-a-real-tool\n"), viper.New())
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if !slices.Equal(cfg.EnabledTools, []string{"not-a-real-tool"}) {
			t.Fatalf("EnabledTools = %#v", cfg.EnabledTools)
		}
	})

	t.Run("empty environment keeps YAML allow list", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", "")
		cfg, err := config.LoadConfig(writeConfigFile(t, "enabled_tools:\n  - higress_list_resource\n"), viper.New())
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if !slices.Equal(cfg.EnabledTools, []string{"higress_list_resource"}) {
			t.Fatalf("EnabledTools = %#v, want YAML allow list", cfg.EnabledTools)
		}
	})

	t.Run("unset flag keeps YAML allow list", func(t *testing.T) {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flags.StringSlice("enabled-tools", []string{}, "")
		if err := flags.Parse([]string{}); err != nil {
			t.Fatalf("parse flag: %v", err)
		}
		v := viper.New()
		if err := v.BindPFlag("enabled_tools", flags.Lookup("enabled-tools")); err != nil {
			t.Fatalf("bind flag: %v", err)
		}
		cfg, err := config.LoadConfig(writeConfigFile(t, "enabled_tools:\n  - higress_list_resource\n"), v)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if !slices.Equal(cfg.EnabledTools, []string{"higress_list_resource"}) {
			t.Fatalf("EnabledTools = %#v, want YAML allow list", cfg.EnabledTools)
		}
	})
}

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfigUsesHigressConsoleEnvironmentVariables(t *testing.T) {
	t.Setenv("MCP_CONSOLE_URL", "https://old.example")
	t.Setenv("MCP_HIGRESS_CONSOLE_URL", "https://higress.example")

	cfg, err := config.LoadConfig("", viper.New())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ConsoleURL != "https://higress.example" {
		t.Fatalf("ConsoleURL = %q, want %q", cfg.ConsoleURL, "https://higress.example")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.StaticConfig
		wantSub string
	}{
		{
			name:    "bad port",
			cfg:     config.StaticConfig{Port: 70000, LogLevel: "info", Listen: "127.0.0.1"},
			wantSub: "port must be between",
		},
		{
			name:    "empty listen",
			cfg:     config.StaticConfig{Port: 8080, LogLevel: "info", Listen: " "},
			wantSub: "listen must be set",
		},
		{
			name:    "bad log level",
			cfg:     config.StaticConfig{Port: 0, LogLevel: "nope", Listen: "127.0.0.1"},
			wantSub: "invalid log_level",
		},
		{
			name:    "bad console URL",
			cfg:     config.StaticConfig{Port: 0, LogLevel: "info", Listen: "127.0.0.1", ConsoleURL: "console.example", ConsoleAuthMode: "session", ConsoleTimeoutSeconds: 30},
			wantSub: "console_url",
		},
		{
			name:    "partial Console credentials",
			cfg:     config.StaticConfig{Port: 0, LogLevel: "info", Listen: "127.0.0.1", ConsoleURL: "https://console.example", ConsoleUsername: "admin", ConsoleAuthMode: "session", ConsoleTimeoutSeconds: 30},
			wantSub: "console_username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestGetListenAddress(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.StaticConfig
		want string
	}{
		{name: "stdio", cfg: config.StaticConfig{Port: 0, Listen: "127.0.0.1"}, want: ""},
		{name: "IPv4", cfg: config.StaticConfig{Port: 8080, Listen: "127.0.0.1"}, want: "127.0.0.1:8080"},
		{name: "hostname", cfg: config.StaticConfig{Port: 8080, Listen: "localhost"}, want: "localhost:8080"},
		{name: "IPv6", cfg: config.StaticConfig{Port: 8080, Listen: "::1"}, want: "[::1]:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.GetListenAddress(); got != tt.want {
				t.Fatalf("GetListenAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}
