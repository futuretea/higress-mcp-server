package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/futuretea/higress-mcp-server/internal/cmd"
)

func TestRootHelpListsSubcommands(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	text := out.String()
	for _, want := range []string{"mcp", "tools", "version", "completion"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help missing %q:\n%s", want, text)
		}
	}
}

func TestRootWithoutArgsShowsHelp(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{})
	if err := root.Execute(); err != nil {
		t.Fatalf("root with no args: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Available Commands") {
		t.Fatalf("expected help output, got:\n%s", text)
	}
	if strings.Contains(text, "registered MCP tools") || strings.Contains(text, "starting HTTP MCP server") {
		t.Fatalf("root must not start the MCP server:\n%s", text)
	}
}

func TestToolsDefaultReadOnlyCatalog(t *testing.T) {
	requireCLIToolNames(t, nil, []string{
		"higress_console_capabilities",
		"higress_console_openapi",
		"higress_list_resource",
		"higress_get_resource",
	})

	for _, name := range writeCapableToolNames {
		t.Run(name+" is unknown", func(t *testing.T) {
			for _, args := range [][]string{
				{"tools", "describe", name},
				{"tools", "call", name, "--params", "{}"},
			} {
				var out bytes.Buffer
				root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
				root.SetArgs(args)
				err := root.Execute()
				if err == nil || !strings.Contains(err.Error(), "unknown tool") {
					t.Fatalf("%v error = %v, want unknown tool", args, err)
				}
			}
		})
	}

	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "call", "higress_console_capabilities", "--params", `{}`})
	if err := root.Execute(); err != nil {
		t.Fatalf("tools call: %v", err)
	}
	if !strings.Contains(out.String(), "common_resources") {
		t.Fatalf("capabilities result = %q", out.String())
	}
	if strings.Contains(out.String(), "higress_console_request") {
		t.Fatalf("read-only capabilities advertise the generic request tool: %q", out.String())
	}
}

func TestToolsReadOnlyFalseRestoresFullCatalog(t *testing.T) {
	requireCLIToolNames(t, []string{"--read-only=false"}, []string{
		"higress_console_capabilities",
		"higress_console_openapi",
		"higress_console_request",
		"higress_list_resource",
		"higress_get_resource",
		"higress_create_resource",
		"higress_update_resource",
		"higress_delete_resource",
		"higress_manage_plugin_instance",
	})
}

func TestToolsDescribeHelpUsesWriteModeForWriteToolExamples(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "describe", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("describe help: %v", err)
	}
	if !strings.Contains(out.String(), "--read-only=false tools describe higress_console_request") {
		t.Fatalf("describe help does not opt into write mode:\n%s", out.String())
	}
}

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out.String(), "higress-mcp-server") {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}

func TestToolsUnknownTool(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "describe", "missing-tool"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %v", err)
	}

	root = cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "call", "missing-tool", "--params", "{}"})
	err = root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error on call, got %v", err)
	}
}

func TestToolsListRejectsBlankNameLists(t *testing.T) {
	allowPath := filepath.Join(t.TempDir(), "allow.yaml")
	if err := os.WriteFile(allowPath, []byte("read_only: false\nenabled_tools:\n  - higress_list_resource\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	domainPath := filepath.Join(t.TempDir(), "domain.yaml")
	if err := os.WriteFile(domainPath, []byte("disabled_domains:\n  - higress\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Run("environment", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", " , ")
		assertCLIConfigError(t, []string{"--config", allowPath, "tools", "list", "--json"}, "load config: enabled_tools contains no names")
	})
	t.Run("flag", func(t *testing.T) {
		assertCLIConfigError(t, []string{"--config", allowPath, "--enabled-tools", " , ", "tools", "list", "--json"}, "load config: enabled_tools contains no names")
	})
	t.Run("disabled domain environment", func(t *testing.T) {
		t.Setenv("MCP_DISABLED_DOMAINS", " ")
		assertCLIConfigError(t, []string{"--config", domainPath, "tools", "list", "--json"}, "load config: disabled_domains contains no names")
	})
}

func TestEmptyEnabledToolsFlagOverridesAllowList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allow.yaml")
	if err := os.WriteFile(path, []byte("enabled_tools:\n  - higress_list_resource\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	requireCLIToolNames(t, []string{"--config", path, "--enabled-tools="}, []string{
		"higress_console_capabilities",
		"higress_console_openapi",
		"higress_list_resource",
		"higress_get_resource",
	})
}

func TestEmptyEnabledToolsEnvKeepsAllowList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allow.yaml")
	if err := os.WriteFile(path, []byte("enabled_tools:\n  - higress_list_resource\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("MCP_ENABLED_TOOLS", "")
	requireCLIToolNames(t, []string{"--config", path}, []string{"higress_list_resource"})
}

func assertCLIConfigError(t *testing.T, args []string, want string) {
	t.Helper()
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs(args)
	err := root.Execute()
	if err == nil || err.Error() != want {
		t.Fatalf("Execute() error = %v, want %q\n%s", err, want, out.String())
	}
	if strings.Contains(out.String(), "higress_") {
		t.Fatalf("tools were listed before config failed:\n%s", out.String())
	}
}

func TestToolsRejectsNullParams(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "call", "higress_console_capabilities", "--params", "null"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "expected an object") {
		t.Fatalf("expected JSON object error, got %v", err)
	}
}

func TestCommandsReturnOutputErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	tests := []struct {
		name string
		args []string
	}{
		{name: "completion script", args: []string{"completion", "bash"}},
		{name: "completion install", args: []string{"completion", "install"}},
		{name: "version", args: []string{"version"}},
		{name: "tools list", args: []string{"tools", "list"}},
		{name: "tools describe", args: []string{"--read-only=false", "tools", "describe", "higress_console_request"}},
		{name: "tools call", args: []string{"tools", "call", "higress_console_capabilities", "--params", `{}`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := cmd.NewRootCommand(cmd.IOStreams{
				In:     strings.NewReader(""),
				Out:    errorWriter{err: writeErr},
				ErrOut: io.Discard,
			})
			root.SetArgs(tt.args)
			if err := root.Execute(); !errors.Is(err, writeErr) {
				t.Fatalf("Execute() error = %v, want %v", err, writeErr)
			}
		})
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

var writeCapableToolNames = []string{
	"higress_console_request",
	"higress_create_resource",
	"higress_update_resource",
	"higress_delete_resource",
	"higress_manage_plugin_instance",
}

func requireCLIToolNames(t *testing.T, prefix, want []string) {
	t.Helper()
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	args := append([]string(nil), prefix...)
	args = append(args, "tools", "list", "--json")
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("tools list: %v", err)
	}

	var summaries []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out.Bytes(), &summaries); err != nil {
		t.Fatalf("decode tools list: %v\n%s", err, out.String())
	}
	got := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		got = append(got, summary.Name)
	}
	want = append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tools list = %#v, want %#v", got, want)
	}
}
