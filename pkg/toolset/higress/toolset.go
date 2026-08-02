// Package higress exposes Higress Console management APIs as MCP tools.
package higress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mark3labs/mcp-go/mcp"
	"net/http"

	"github.com/futuretea/higress-mcp-server/pkg/console"
	"github.com/futuretea/higress-mcp-server/pkg/toolset"
)

const Domain = "higress"

// Toolset groups the Higress Console tools around a configured API client.
type Toolset struct {
	client *console.Client
}

// New creates a Higress Console toolset.
func New(client *console.Client) (*Toolset, error) {
	if client == nil {
		return nil, errors.New("console client is required")
	}
	return &Toolset{client: client}, nil
}

// GetName returns the toolset identifier.
func (t *Toolset) GetName() string { return Domain }

// GetDescription describes the Console API coverage of this toolset.
func (t *Toolset) GetDescription() string {
	return "Manage a Higress Console through common resource tools or its complete documented HTTP API"
}

// GetTools returns all Higress Console MCP tools.
func (t *Toolset) GetTools() []toolset.ServerTool {
	return []toolset.ServerTool{
		t.readOnlyTool("higress_console_capabilities", "List common resources and explain how to access the complete Console API.", capabilitiesSchema, t.capabilities),
		t.readOnlyTool("higress_console_openapi", "Fetch Higress Console's OpenAPI document. The target must enable its api-docs profile.", emptySchema, t.openapi),
		t.requestTool("higress_console_request", "Call any Higress Console API path. Use higress_console_openapi first when a specialized tool does not cover the operation.", requestSchema, t.request),
		t.readOnlyTool("higress_list_resource", "List one common resource type, such as routes, domains, service_sources, or wasm_plugins.", listSchema, t.listResource),
		t.readOnlyTool("higress_get_resource", "Get one named common resource.", namedResourceSchema, t.getResource),
		t.requestTool("higress_create_resource", "Create one common Higress resource with a JSON body.", createSchema, t.createResource),
		t.requestTool("higress_update_resource", "Update one named common Higress resource with a JSON body.", namedBodySchema, t.updateResource),
		t.requestTool("higress_delete_resource", "Delete one named common Higress resource.", namedResourceSchema, t.deleteResource),
		t.requestTool("higress_manage_plugin_instance", "List, get, upsert, or delete a Wasm plugin instance at global, route, domain, or service scope.", pluginInstanceSchema, t.managePluginInstance),
	}
}

func (t *Toolset) readOnlyTool(name, description string, schema json.RawMessage, handler toolset.ToolHandler) toolset.ServerTool {
	tool := mcp.NewToolWithRawSchema(name, description, schema)
	tool.Annotations.ReadOnlyHint = mcp.ToBoolPtr(true)

	return toolset.ServerTool{
		Tool:    tool,
		Handler: handler,
		Domain:  Domain,
	}
}

func (t *Toolset) requestTool(name, description string, schema json.RawMessage, handler toolset.ToolHandler) toolset.ServerTool {
	tool := mcp.NewToolWithRawSchema(name, description, schema)
	tool.Annotations.DestructiveHint = mcp.ToBoolPtr(true)
	tool.Annotations.OpenWorldHint = mcp.ToBoolPtr(true)

	return toolset.ServerTool{
		Tool:    tool,
		Handler: handler,
		Domain:  Domain,
	}
}

func (t *Toolset) capabilities(_ context.Context, _ map[string]any) (string, error) {
	return formatJSON(map[string]any{
		"common_resources": console.Resources(),
		"api_groups":       console.APIGroups(),
		"complete_api": map[string]string{
			"discovery_tool": "higress_console_openapi",
			"request_tool":   "higress_console_request",
			"note":           "The request tool is restricted to the configured Higress Console origin. API Docs are disabled by default in Higress Console.",
		},
	})
}

func (t *Toolset) openapi(ctx context.Context, _ map[string]any) (string, error) {
	return t.call(ctx, console.Request{Method: http.MethodGet, Path: "/v3/api-docs/Higress"})
}

func (t *Toolset) request(ctx context.Context, params map[string]any) (string, error) {
	method, err := requiredString(params, "method")
	if err != nil {
		return "", err
	}
	path, err := requiredString(params, "path")
	if err != nil {
		return "", err
	}
	query, err := stringMap(params["query"])
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	return t.call(ctx, console.Request{Method: method, Path: path, Query: query, Body: params["body"]})
}

func (t *Toolset) listResource(ctx context.Context, params map[string]any) (string, error) {
	resource, err := requiredString(params, "resource")
	if err != nil {
		return "", err
	}
	path, err := console.ResourcePath(resource, "list", "")
	if err != nil {
		return "", err
	}
	query, err := stringMap(params["query"])
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	return t.call(ctx, console.Request{Method: http.MethodGet, Path: path, Query: query})
}

func (t *Toolset) getResource(ctx context.Context, params map[string]any) (string, error) {
	return t.namedCall(ctx, params, "get", http.MethodGet, nil)
}

func (t *Toolset) createResource(ctx context.Context, params map[string]any) (string, error) {
	resource, err := requiredString(params, "resource")
	if err != nil {
		return "", err
	}
	path, err := console.ResourcePath(resource, "create", "")
	if err != nil {
		return "", err
	}
	body, err := requiredObject(params, "body")
	if err != nil {
		return "", err
	}
	return t.call(ctx, console.Request{Method: http.MethodPost, Path: path, Body: body})
}

func (t *Toolset) updateResource(ctx context.Context, params map[string]any) (string, error) {
	body, err := requiredObject(params, "body")
	if err != nil {
		return "", err
	}
	return t.namedCall(ctx, params, "update", http.MethodPut, body)
}

func (t *Toolset) deleteResource(ctx context.Context, params map[string]any) (string, error) {
	return t.namedCall(ctx, params, "delete", http.MethodDelete, nil)
}

func (t *Toolset) namedCall(ctx context.Context, params map[string]any, operation, method string, body any) (string, error) {
	resource, err := requiredString(params, "resource")
	if err != nil {
		return "", err
	}
	name, err := requiredString(params, "name")
	if err != nil {
		return "", err
	}
	path, err := console.ResourcePath(resource, operation, name)
	if err != nil {
		return "", err
	}
	return t.call(ctx, console.Request{Method: method, Path: path, Body: body})
}

func (t *Toolset) call(ctx context.Context, request console.Request) (string, error) {
	response, err := t.client.Request(ctx, request)
	if err != nil {
		return "", err
	}
	return formatResponse(response), nil
}

var (
	emptySchema          = json.RawMessage(`{"type":"object","additionalProperties":false}`)
	capabilitiesSchema   = emptySchema
	requestSchema        = json.RawMessage(`{"type":"object","properties":{"method":{"type":"string","enum":["GET","POST","PUT","PATCH","DELETE"]},"path":{"type":"string","description":"Absolute Console API path, for example /v1/routes. It cannot contain a host or query string."},"query":{"type":"object","additionalProperties":{"type":"string"}},"body":{"type":"object","additionalProperties":true}},"required":["method","path"],"additionalProperties":false}`)
	listSchema           = json.RawMessage(`{"type":"object","properties":{"resource":{"type":"string","enum":["ai_routes","consumers","domains","llm_providers","proxy_servers","routes","service_sources","tls_certificates","wasm_plugins"]},"query":{"type":"object","additionalProperties":{"type":"string"}}},"required":["resource"],"additionalProperties":false}`)
	namedResourceSchema  = json.RawMessage(`{"type":"object","properties":{"resource":{"type":"string","enum":["ai_routes","consumers","domains","llm_providers","proxy_servers","routes","service_sources","tls_certificates","wasm_plugins"]},"name":{"type":"string"}},"required":["resource","name"],"additionalProperties":false}`)
	createSchema         = json.RawMessage(`{"type":"object","properties":{"resource":{"type":"string","enum":["ai_routes","consumers","domains","llm_providers","proxy_servers","routes","service_sources","tls_certificates","wasm_plugins"]},"body":{"type":"object","additionalProperties":true}},"required":["resource","body"],"additionalProperties":false}`)
	namedBodySchema      = json.RawMessage(`{"type":"object","properties":{"resource":{"type":"string","enum":["ai_routes","consumers","domains","llm_providers","proxy_servers","routes","service_sources","tls_certificates","wasm_plugins"]},"name":{"type":"string"},"body":{"type":"object","additionalProperties":true}},"required":["resource","name","body"],"additionalProperties":false}`)
	pluginInstanceSchema = json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["list","get","upsert","delete"]},"scope":{"type":"string","enum":["global","routes","domains","services"]},"target":{"type":"string","description":"Required for routes, domains, and services scopes."},"plugin_name":{"type":"string","description":"Required except for list."},"body":{"type":"object","description":"Required for upsert.","additionalProperties":true}},"required":["operation","scope"],"additionalProperties":false}`)
)
