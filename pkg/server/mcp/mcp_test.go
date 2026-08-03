package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/futuretea/higress-mcp-server/pkg/console"
	"github.com/futuretea/higress-mcp-server/pkg/core/config"
	mcpserver "github.com/futuretea/higress-mcp-server/pkg/server/mcp"
	"github.com/futuretea/higress-mcp-server/pkg/toolset"
	"github.com/futuretea/higress-mcp-server/pkg/toolset/example"
	"github.com/futuretea/higress-mcp-server/pkg/toolset/higress"
)

func TestNewServerRegistersExampleTools(t *testing.T) {
	server, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{&example.Toolset{}},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Close()

	if !server.IsHealthy() {
		t.Fatal("expected healthy server")
	}
	tools := server.GetEnabledTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %v", tools)
	}
	names := map[string]bool{}
	for _, name := range tools {
		names[name] = true
	}
	if !names["echo"] || !names["ping"] {
		t.Fatalf("expected echo and ping, got %v", tools)
	}
}

func TestNewServerRegistersReadOnlyHigressTools(t *testing.T) {
	server, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info", ReadOnly: true},
		Toolsets:     []toolset.Toolset{newHigressToolset(t, true)},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Close()

	requireRegisteredTools(t, server.GetEnabledTools(), []string{
		"higress_console_capabilities",
		"higress_console_openapi",
		"higress_list_resource",
		"higress_get_resource",
	})
}

func TestNewServerReadOnlyHigressToolsCannotBeRestoredByEnabledTools(t *testing.T) {
	server, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{
			LogLevel: "info",
			ReadOnly: true,
			EnabledTools: []string{
				"higress_list_resource",
				"higress_console_request",
				"higress_create_resource",
				"higress_update_resource",
				"higress_delete_resource",
				"higress_manage_plugin_instance",
			},
		},
		Toolsets: []toolset.Toolset{newHigressToolset(t, true)},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Close()

	requireRegisteredTools(t, server.GetEnabledTools(), []string{"higress_list_resource"})
}

func TestNewServerRegistersAllHigressToolsWhenReadOnlyDisabled(t *testing.T) {
	server, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info", ReadOnly: false},
		Toolsets:     []toolset.Toolset{newHigressToolset(t, false)},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Close()

	requireRegisteredTools(t, server.GetEnabledTools(), []string{
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

func TestNewServerRequiresStaticConfig(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{})
	if err == nil || !strings.Contains(err.Error(), "static config is required") {
		t.Fatalf("expected static config error, got %v", err)
	}
}

func TestNewServerRejectsEmptyToolFilter(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{
			LogLevel:     "info",
			EnabledTools: []string{"missing"},
		},
		Toolsets: []toolset.Toolset{&example.Toolset{}},
	})
	if err == nil || !strings.Contains(err.Error(), "no tools registered") {
		t.Fatalf("expected no tools registered error, got %v", err)
	}
}

func TestNewServerRejectsDuplicateToolNames(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets: []toolset.Toolset{
			&example.Toolset{},
			&example.Toolset{},
		},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicate tool name "echo"`) {
		t.Fatalf("expected duplicate tool name error, got %v", err)
	}
}

func TestNewServerValidatesRawInputSchemaRootType(t *testing.T) {
	testCases := []struct {
		name    string
		schema  string
		wantErr string
	}{
		{name: "object root", schema: `{"type":"object","properties":{}}`},
		{name: "missing type", schema: `{"oneOf":[]}`, wantErr: `raw input schema must declare root type "object"`},
		{name: "non-object type", schema: `{"type":"array"}`, wantErr: `raw input schema must declare root type "object"`},
		{name: "duplicate type", schema: `{"type":"array","type":"object"}`, wantErr: `raw input schema must declare root type "object" only once`},
		{name: "invalid JSON", schema: `{`, wantErr: "has an invalid raw input schema"},
		{name: "trailing JSON", schema: `{"type":"object"} null`, wantErr: "has an invalid raw input schema"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server, err := mcpserver.NewServer(mcpserver.Configuration{
				StaticConfig: &config.StaticConfig{LogLevel: "info"},
				Toolsets:     []toolset.Toolset{staticToolset{tools: []toolset.ServerTool{rawSchemaTool(testCase.schema)}}},
			})
			if testCase.wantErr == "" {
				if err != nil {
					t.Fatalf("NewServer: %v", err)
				}
				defer server.Close()
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("NewServer error = %v, want %q", err, testCase.wantErr)
			}
		})
	}
}

func TestNewTextResult(t *testing.T) {
	ok := mcpserver.NewTextResult("hello", nil)
	if ok.IsError || len(ok.Content) != 1 {
		t.Fatalf("unexpected ok result: %+v", ok)
	}

	fail := mcpserver.NewTextResult("", errString("boom"))
	if !fail.IsError {
		t.Fatal("expected error result")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

type staticToolset struct {
	tools []toolset.ServerTool
}

func (t staticToolset) GetName() string        { return "test" }
func (t staticToolset) GetDescription() string { return "test toolset" }
func (t staticToolset) GetTools() []toolset.ServerTool {
	return t.tools
}

func rawSchemaTool(schema string) toolset.ServerTool {
	return toolset.ServerTool{
		Tool: mcp.NewToolWithRawSchema("raw", "raw schema test tool", []byte(schema)),
		Handler: func(context.Context, map[string]any) (string, error) {
			return "", nil
		},
		Domain: "test",
	}
}

func newHigressToolset(t *testing.T, readOnly bool) *higress.Toolset {
	t.Helper()
	client, err := console.NewClient(console.Config{BaseURL: "https://console.example", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client, readOnly)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return set
}

func requireRegisteredTools(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("registered tools = %#v, want %#v", got, want)
	}
	seen := make(map[string]bool, len(got))
	for _, name := range got {
		seen[name] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Fatalf("registered tools = %#v, missing %q", got, name)
		}
	}
}
