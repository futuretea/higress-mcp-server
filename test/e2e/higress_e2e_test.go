//go:build !short

package e2e_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	e2eImageRepository = "higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/all-in-one"
	backendImage       = "hashicorp/http-echo:1.0.0"
	backendResponse    = "higress-mcp-e2e-ok"
)

func TestHigressConsoleMCPOverHTTP(t *testing.T) {
	if os.Getenv("HIGRESS_E2E") != "1" {
		t.Skip("set HIGRESS_E2E=1 to run Docker-backed Higress E2E tests")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("Docker is required for Higress E2E: %v", err)
	}

	binary := os.Getenv("HIGRESS_E2E_BINARY")
	if binary == "" {
		t.Fatal("HIGRESS_E2E_BINARY is required; use make e2e")
	}
	absBinary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatalf("resolve HIGRESS_E2E_BINARY: %v", err)
	}
	if info, err := os.Stat(absBinary); err != nil || info.IsDir() {
		t.Fatalf("HIGRESS_E2E_BINARY must be an executable file: %v", err)
	}

	versions, err := e2eVersions()
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range versions {
		version := version
		t.Run("console_"+strings.ReplaceAll(version, ".", "_"), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()
			runHigressMCPFlow(t, ctx, absBinary, version)
		})
	}
}

func runHigressMCPFlow(t *testing.T, ctx context.Context, binary, version string) {
	t.Helper()
	nameSuffix := randomSuffix(t)
	consoleURL, gatewayURL, backendIP := startHigress(t, ctx, nameSuffix, version)
	username := "e2e-admin"
	password := randomPassword(t)
	initializeConsole(t, ctx, consoleURL, username, password)

	mcpURL := startMCPServer(t, ctx, binary, consoleURL, username, password)
	mcpClient := connectMCP(t, ctx, mcpURL+"/mcp")
	t.Cleanup(func() { _ = mcpClient.Close() })
	assertToolCatalog(t, ctx, mcpClient)
	sseClient := connectSSE(t, ctx, mcpURL+"/sse")
	t.Cleanup(func() { _ = sseClient.Close() })
	assertToolCatalog(t, ctx, sseClient)
	assertConsoleVersion(t, ctx, mcpClient, version)
	assertRouteLifecycle(t, ctx, mcpClient, nameSuffix, backendIP, gatewayURL)
	assertExternalPathRejected(t, ctx, mcpClient)
}

func assertConsoleVersion(t *testing.T, ctx context.Context, mcpClient *client.Client, version string) {
	t.Helper()
	consoleInfo := callToolText(t, ctx, mcpClient, "higress_console_request", map[string]any{
		"method": "GET",
		"path":   "/system/info",
	})
	if !strings.Contains(consoleInfo, `"version": "`+version+`"`) {
		t.Fatalf("generic Console request did not return expected version %q: %s", version, consoleInfo)
	}
}

func assertRouteLifecycle(t *testing.T, ctx context.Context, mcpClient *client.Client, nameSuffix, backendIP, gatewayURL string) {
	t.Helper()
	sourceName := createStaticServiceSource(t, ctx, mcpClient, nameSuffix, backendIP)
	routeName, routePath := createRoute(t, ctx, mcpClient, nameSuffix, sourceName)

	waitForHTTP(t, ctx, gatewayURL+routePath, func(response *http.Response, body []byte) bool {
		return response.StatusCode == http.StatusOK && strings.Contains(string(body), backendResponse)
	})

	callToolText(t, ctx, mcpClient, "higress_delete_resource", map[string]any{
		"resource": "routes",
		"name":     routeName,
	})
	// Deleting a route is the destructive probe: the gateway must stop reaching the backend.
	waitForHTTP(t, ctx, gatewayURL+routePath, func(_ *http.Response, body []byte) bool {
		return !strings.Contains(string(body), backendResponse)
	})

	callToolText(t, ctx, mcpClient, "higress_delete_resource", map[string]any{
		"resource": "service_sources",
		"name":     sourceName,
	})
}

func createStaticServiceSource(t *testing.T, ctx context.Context, mcpClient *client.Client, nameSuffix, backendIP string) string {
	t.Helper()
	sourceName := "e2e-static-" + nameSuffix
	callToolText(t, ctx, mcpClient, "higress_create_resource", map[string]any{
		"resource": "service_sources",
		"body": map[string]any{
			"name":     sourceName,
			"type":     "static",
			"domain":   net.JoinHostPort(backendIP, "5678"),
			"port":     80,
			"protocol": "http",
		},
	})

	createdSource := callToolText(t, ctx, mcpClient, "higress_get_resource", map[string]any{
		"resource": "service_sources",
		"name":     sourceName,
	})
	if !strings.Contains(createdSource, `"name": "`+sourceName+`"`) {
		t.Fatalf("created service source was not returned by MCP: %s", createdSource)
	}
	return sourceName
}

func createRoute(t *testing.T, ctx context.Context, mcpClient *client.Client, nameSuffix, sourceName string) (string, string) {
	t.Helper()
	routeName := "e2e-route-" + nameSuffix
	routePath := "/e2e-" + nameSuffix
	callToolText(t, ctx, mcpClient, "higress_create_resource", map[string]any{
		"resource": "routes",
		"body": map[string]any{
			"name": routeName,
			"path": map[string]any{
				"matchType":  "PRE",
				"matchValue": routePath,
			},
			"services": []map[string]any{{"name": sourceName + ".static", "port": 80}},
		},
	})
	return routeName, routePath
}

func e2eVersions() ([]string, error) {
	rawVersions, configured := os.LookupEnv("HIGRESS_E2E_VERSIONS")
	return parseE2EVersions(rawVersions, configured)
}

func parseE2EVersions(rawVersions string, configured bool) ([]string, error) {
	if !configured {
		return []string{"2.1.11", "2.2.3"}, nil
	}

	versions := strings.Split(rawVersions, ",")
	result := make([]string, 0, len(versions))
	for _, version := range versions {
		if version = strings.TrimSpace(version); version != "" {
			result = append(result, version)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("HIGRESS_E2E_VERSIONS must contain at least one version")
	}
	return result, nil
}

func TestParseE2EVersions(t *testing.T) {
	testCases := []struct {
		name       string
		raw        string
		configured bool
		want       []string
		wantErr    string
	}{
		{name: "defaults", want: []string{"2.1.11", "2.2.3"}},
		{name: "one version", raw: "2.2.3", configured: true, want: []string{"2.2.3"}},
		{name: "trimmed versions", raw: " 2.1.11 , 2.2.3 ", configured: true, want: []string{"2.1.11", "2.2.3"}},
		{name: "empty override", raw: " ", configured: true, wantErr: "must contain at least one version"},
		{name: "commas only", raw: ",,", configured: true, wantErr: "must contain at least one version"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseE2EVersions(testCase.raw, testCase.configured)
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("parseE2EVersions() error = %v, want %q", err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseE2EVersions() error = %v", err)
			}
			if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
				t.Fatalf("parseE2EVersions() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func initializeConsole(t *testing.T, ctx context.Context, consoleURL, username, password string) {
	t.Helper()
	payload := map[string]any{
		"adminUser": map[string]string{
			"name":        username,
			"displayName": "Higress MCP E2E",
			"password":    password,
		},
		"configs": map[string]any{},
	}
	postJSON(t, ctx, consoleURL+"/system/init", payload)
}

func connectMCP(t *testing.T, ctx context.Context, endpoint string) *client.Client {
	t.Helper()
	mcpClient, err := client.NewStreamableHttpClient(endpoint)
	if err != nil {
		t.Fatalf("create MCP HTTP client: %v", err)
	}
	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("start MCP HTTP client: %v", err)
	}
	_, err = mcpClient.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "higress-e2e", Version: "1.0"},
	}})
	if err != nil {
		_ = mcpClient.Close()
		t.Fatalf("initialize unauthenticated MCP HTTP client: %v", err)
	}
	return mcpClient
}

func connectSSE(t *testing.T, ctx context.Context, endpoint string) *client.Client {
	t.Helper()
	mcpClient, err := client.NewSSEMCPClient(endpoint)
	if err != nil {
		t.Fatalf("create SSE MCP client: %v", err)
	}
	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("start SSE MCP client: %v", err)
	}
	_, err = mcpClient.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "higress-e2e-sse", Version: "1.0"},
	}})
	if err != nil {
		_ = mcpClient.Close()
		t.Fatalf("initialize unauthenticated SSE MCP client: %v", err)
	}
	return mcpClient
}

func assertToolCatalog(t *testing.T, ctx context.Context, mcpClient *client.Client) {
	t.Helper()
	result, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list MCP tools: %v", err)
	}
	available := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		available[tool.Name] = true
	}
	for _, name := range []string{
		"higress_console_capabilities", "higress_console_openapi", "higress_console_request",
		"higress_list_resource", "higress_get_resource", "higress_create_resource",
		"higress_update_resource", "higress_delete_resource", "higress_manage_plugin_instance",
	} {
		if !available[name] {
			t.Fatalf("MCP tool catalog is missing %q: %#v", name, available)
		}
	}

	capabilities := callToolText(t, ctx, mcpClient, "higress_console_capabilities", nil)
	if !strings.Contains(capabilities, "service_sources") {
		t.Fatalf("capabilities response does not list service_sources: %s", capabilities)
	}
}

func callToolText(t *testing.T, ctx context.Context, mcpClient *client.Client, name string, arguments map[string]any) string {
	t.Helper()
	result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: arguments}})
	if err != nil {
		t.Fatalf("call MCP tool %q: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("MCP tool %q returned an error: %s", name, toolResultText(result))
	}
	return toolResultText(result)
}

func assertExternalPathRejected(t *testing.T, ctx context.Context, mcpClient *client.Client) {
	t.Helper()
	result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name: "higress_console_request",
		Arguments: map[string]any{
			"method": "GET",
			"path":   "https://outside.invalid/v1/routes",
		},
	}})
	if err != nil {
		t.Fatalf("call invalid-path MCP tool request: %v", err)
	}
	if !result.IsError || !strings.Contains(toolResultText(result), "console API path") {
		t.Fatalf("external Console path was not rejected: %#v", result)
	}
}

func toolResultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	texts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func randomPassword(t *testing.T) string {
	t.Helper()
	return "e2e-" + randomSuffix(t) + "-Password"
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	data := make([]byte, 6)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("generate E2E resource suffix: %v", err)
	}
	return hex.EncodeToString(data)
}
