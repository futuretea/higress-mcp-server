package higress_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/futuretea/higress-mcp-server/pkg/console"
	"github.com/futuretea/higress-mcp-server/pkg/toolset"
	"github.com/futuretea/higress-mcp-server/pkg/toolset/higress"
)

func TestToolsetExposesConsoleTools(t *testing.T) {
	client, err := console.NewClient(console.Config{BaseURL: "https://console.example", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	handlers := handlersByName(set.GetTools())
	for _, tool := range set.GetTools() {
		if _, err := json.Marshal(tool.Tool); err != nil {
			t.Fatalf("marshal tool %q: %v", tool.Tool.Name, err)
		}
	}
	for _, name := range []string{
		"higress_console_capabilities", "higress_console_openapi", "higress_console_request",
		"higress_list_resource", "higress_get_resource", "higress_create_resource",
		"higress_update_resource", "higress_delete_resource", "higress_manage_plugin_instance",
	} {
		if handlers[name] == nil {
			t.Fatalf("missing tool %q", name)
		}
	}
	capabilities, err := handlers["higress_console_capabilities"](context.Background(), nil)
	if err != nil || !strings.Contains(capabilities, "service_sources") || !strings.Contains(capabilities, "higress_console_request") {
		t.Fatalf("capabilities = %q, %v", capabilities, err)
	}
}

func TestResourceAndPluginToolsUseConsolePaths(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := console.NewClient(console.Config{BaseURL: server.URL, TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handlers := handlersByName(set.GetTools())

	if _, err := handlers["higress_get_resource"](context.Background(), map[string]any{"resource": "routes", "name": "main"}); err != nil {
		t.Fatalf("get resource: %v", err)
	}
	if _, err := handlers["higress_manage_plugin_instance"](context.Background(), map[string]any{
		"operation": "upsert", "scope": "routes", "target": "main", "plugin_name": "cors", "body": map[string]any{"enabled": true},
	}); err != nil {
		t.Fatalf("upsert plugin: %v", err)
	}
	want := []string{"GET /v1/routes/main", "PUT /v1/routes/main/plugin-instances/cors"}
	if strings.Join(requests, ",") != strings.Join(want, ",") {
		t.Fatalf("requests = %#v, want %#v", requests, want)
	}
}

func TestGenericToolRejectsExternalPath(t *testing.T) {
	client, err := console.NewClient(console.Config{BaseURL: "https://console.example", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = handlersByName(set.GetTools())["higress_console_request"](context.Background(), map[string]any{
		"method": "GET", "path": "https://outside.example/v1/routes",
	})
	if err == nil || !strings.Contains(err.Error(), "console API path") {
		t.Fatalf("generic tool error = %v", err)
	}
}

func TestPluginInstanceToolRejectsEncodedPathDelimiters(t *testing.T) {
	client, err := console.NewClient(console.Config{BaseURL: "https://console.example", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler := handlersByName(set.GetTools())["higress_manage_plugin_instance"]

	for _, params := range []map[string]any{
		{"operation": "list", "scope": "routes", "target": "main%2Fother"},
		{"operation": "get", "scope": "global", "plugin_name": "cors%3Fother"},
	} {
		if _, err := handler(context.Background(), params); err == nil || !strings.Contains(err.Error(), "one path segment") {
			t.Fatalf("plugin instance params %#v error = %v, want path segment validation", params, err)
		}
	}
}

func handlersByName(tools []toolset.ServerTool) map[string]toolset.ToolHandler {
	handlers := make(map[string]toolset.ToolHandler, len(tools))
	for _, tool := range tools {
		handlers[tool.Tool.Name] = tool.Handler
	}
	return handlers
}
