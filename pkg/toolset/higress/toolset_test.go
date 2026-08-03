package higress_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/futuretea/higress-mcp-server/pkg/console"
	"github.com/futuretea/higress-mcp-server/pkg/toolset"
	"github.com/futuretea/higress-mcp-server/pkg/toolset/higress"
)

func TestToolsetWriteModeExposesConsoleTools(t *testing.T) {
	client, err := console.NewClient(console.Config{BaseURL: "https://console.example", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tools := set.GetTools()
	requireToolNames(t, tools, allHigressToolNames)
	handlers := handlersByName(tools)
	for _, tool := range tools {
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

func TestToolsetReadOnlyExposesOnlyReadTools(t *testing.T) {
	client, err := console.NewClient(console.Config{BaseURL: "https://console.example", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tools := set.GetTools()
	requireToolNames(t, tools, readOnlyToolNames)
	handlers := handlersByName(tools)
	capabilities, err := handlers["higress_console_capabilities"](context.Background(), nil)
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if strings.Contains(capabilities, "higress_console_request") {
		t.Fatalf("read-only capabilities advertise the generic request tool: %q", capabilities)
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
	set, err := higress.New(client, false)
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
	set, err := higress.New(client, false)
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

func TestConsumerReadCredentialsAreMaskedUnlessExplicitlyRequested(t *testing.T) {
	handlers := newConsumerTestHandlers(t)
	tests := []consumerReadTest{
		{
			name: "specialized list", tool: "higress_list_resource",
			params: map[string]any{"resource": "consumers"}, secret: "list-secret", preserved: "jwt-secret", preservedNumber: "9007199254740993",
		},
		{
			name: "specialized get", tool: "higress_get_resource",
			params: map[string]any{"resource": "consumers", "name": "named-consumer"}, secret: "named-secret",
		},
		{
			name: "generic list", tool: "higress_console_request",
			params: map[string]any{"method": "GET", "path": "/v1/consumers"}, secret: "list-secret", preserved: "jwt-secret", preservedNumber: "9007199254740993",
		},
		{
			name: "encoded generic list", tool: "higress_console_request",
			params: map[string]any{"method": "GET", "path": "/v1%2fconsumers"}, secret: "list-secret", preserved: "jwt-secret", preservedNumber: "9007199254740993",
		},
		{
			name: "generic get", tool: "higress_console_request",
			params: map[string]any{"method": "GET", "path": "/v1/consumers/named-consumer"}, secret: "named-secret",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertConsumerReadResponse(t, handlers, test, false)
		})
	}
}

func TestConsumerReadCredentialsCanBeExplicitlyRequested(t *testing.T) {
	handlers := newConsumerTestHandlers(t)
	tests := []consumerReadTest{
		{
			name: "specialized list", tool: "higress_list_resource",
			params: map[string]any{"resource": "consumers"}, secret: "list-secret", preserved: "jwt-secret", preservedNumber: "9007199254740993",
		},
		{
			name: "specialized get", tool: "higress_get_resource",
			params: map[string]any{"resource": "consumers", "name": "named-consumer"}, secret: "named-secret",
		},
		{
			name: "generic get", tool: "higress_console_request",
			params: map[string]any{"method": "GET", "path": "/v1/consumers/named-consumer"}, secret: "named-secret",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertConsumerReadResponse(t, handlers, test, true)
		})
	}
}

func TestConsumerReadErrorsAreMaskedUnlessExplicitlyRequested(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"data":{"credentials":[{"type":"key-auth","values":["error-secret"]}]}}`))
	}))
	defer server.Close()

	client, err := console.NewClient(console.Config{BaseURL: server.URL, TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler := handlersByName(set.GetTools())["higress_console_request"]

	for _, showSensitiveData := range []bool{false, true} {
		t.Run(fmt.Sprintf("show sensitive data %t", showSensitiveData), func(t *testing.T) {
			_, err := handler(context.Background(), map[string]any{
				"method": "GET", "path": "/v1/consumers", "show_sensitive_data": showSensitiveData,
			})
			if err == nil {
				t.Fatal("handler error = nil, want Console error")
			}
			if strings.Contains(err.Error(), "error-secret") != showSensitiveData {
				t.Fatalf("error contains sensitive value unexpectedly")
			}
			if !showSensitiveData && !strings.Contains(err.Error(), "***") {
				t.Fatalf("error does not contain masked value")
			}
		})
	}
}

type consumerReadTest struct {
	name            string
	tool            string
	params          map[string]any
	secret          string
	preserved       string
	preservedNumber string
}

func newConsumerTestHandlers(t *testing.T) map[string]toolset.ToolHandler {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		switch r.URL.Path {
		case "/v1/consumers":
			_, _ = w.Write([]byte(`{"total":9007199254740993,"data":[{"name":"list-consumer","credentials":[{"type":"key-auth","key":"x-api-key","values":["list-secret","list-secret-2"]},{"type":"jwt","values":["jwt-secret"]}]}]}`))
		case "/v1/consumers/named-consumer":
			_, _ = w.Write([]byte(`{"data":{"name":"named-consumer","credentials":[{"type":"key-auth","key":"x-api-key","values":["named-secret"]}]}}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := console.NewClient(console.Config{BaseURL: server.URL, TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return handlersByName(set.GetTools())
}

func assertConsumerReadResponse(t *testing.T, handlers map[string]toolset.ToolHandler, test consumerReadTest, showSensitiveData bool) {
	t.Helper()
	params := make(map[string]any, len(test.params)+1)
	for key, value := range test.params {
		params[key] = value
	}
	params["show_sensitive_data"] = showSensitiveData

	response, err := handlers[test.tool](context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if strings.Contains(response, test.secret) != showSensitiveData {
		t.Fatalf("response contains sensitive value unexpectedly")
	}
	if !showSensitiveData && !strings.Contains(response, "***") {
		t.Fatalf("response does not contain masked values")
	}
	if test.preserved != "" && !strings.Contains(response, test.preserved) {
		t.Fatalf("response does not preserve %q", test.preserved)
	}
	if test.preservedNumber != "" && !strings.Contains(response, test.preservedNumber) {
		t.Fatalf("response does not preserve numeric value %q", test.preservedNumber)
	}
}

func TestConsumerReadToolsAdvertiseSensitiveDataOption(t *testing.T) {
	client, err := console.NewClient(console.Config{BaseURL: "https://console.example", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tools := set.GetTools()
	for _, name := range []string{"higress_console_request", "higress_list_resource", "higress_get_resource"} {
		tool, ok := findTool(tools, name)
		if !ok {
			t.Fatalf("tool %q is missing", name)
		}
		var definition map[string]any
		encoded, err := json.Marshal(tool.Tool)
		if err != nil {
			t.Fatalf("marshal tool %q: %v", name, err)
		}
		if err := json.Unmarshal(encoded, &definition); err != nil {
			t.Fatalf("unmarshal tool %q: %v", name, err)
		}
		inputSchema, ok := definition["inputSchema"].(map[string]any)
		if !ok {
			t.Fatalf("tool %q input schema = %#v", name, definition["inputSchema"])
		}
		properties, ok := inputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("tool %q properties = %#v", name, inputSchema["properties"])
		}
		option, ok := properties["show_sensitive_data"].(map[string]any)
		if !ok || option["type"] != "boolean" || option["default"] != false {
			t.Fatalf("tool %q show_sensitive_data = %#v", name, properties["show_sensitive_data"])
		}
	}
}

func TestGenericConsumerWriteDoesNotExposeCredentialsUnlessExplicitlyRequested(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/consumers" {
			t.Fatalf("request = %s %s, want POST /v1/consumers", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"credentials":[{"type":"key-auth","values":["created-secret"]}]}}`))
	}))
	defer server.Close()

	client, err := console.NewClient(console.Config{BaseURL: server.URL, TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler := handlersByName(set.GetTools())["higress_console_request"]
	for _, test := range []struct {
		name              string
		showSensitiveData bool
		wantSensitiveData bool
	}{
		{name: "default", wantSensitiveData: false},
		{name: "explicit reveal", showSensitiveData: true, wantSensitiveData: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			params := map[string]any{
				"method": "POST",
				"path":   "/v1/consumers",
				"body":   map[string]any{"name": "created-consumer"},
			}
			if test.showSensitiveData {
				params["show_sensitive_data"] = true
			}
			response, err := handler(context.Background(), params)
			if err != nil {
				t.Fatalf("handler: %v", err)
			}
			if strings.Contains(response, "created-secret") != test.wantSensitiveData {
				t.Fatalf("response contains sensitive value unexpectedly: %s", response)
			}
		})
	}
}

func TestPluginInstanceToolRejectsEncodedPathDelimiters(t *testing.T) {
	client, err := console.NewClient(console.Config{BaseURL: "https://console.example", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	set, err := higress.New(client, false)
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

func findTool(tools []toolset.ServerTool, name string) (toolset.ServerTool, bool) {
	for _, tool := range tools {
		if tool.Tool.Name == name {
			return tool, true
		}
	}
	return toolset.ServerTool{}, false
}

var readOnlyToolNames = []string{
	"higress_console_capabilities",
	"higress_console_openapi",
	"higress_list_resource",
	"higress_get_resource",
}

var allHigressToolNames = []string{
	"higress_console_capabilities",
	"higress_console_openapi",
	"higress_console_request",
	"higress_list_resource",
	"higress_get_resource",
	"higress_create_resource",
	"higress_update_resource",
	"higress_delete_resource",
	"higress_manage_plugin_instance",
}

func requireToolNames(t *testing.T, tools []toolset.ServerTool, want []string) {
	t.Helper()
	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		got = append(got, tool.Tool.Name)
	}
	want = append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tool names = %#v, want %#v", got, want)
	}
}
