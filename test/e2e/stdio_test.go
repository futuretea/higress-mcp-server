//go:build !short

package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPServerStdio(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	t.Setenv("MCP_DISABLED_TOOLS", "higress_console_capabilities")
	t.Setenv("MCP_ENABLED_TOOLS", "higress_list_resource")

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get test working directory: %v", err)
	}
	repositoryRoot := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))

	server, err := client.NewStdioMCPClientWithOptions(
		"go",
		withEnvironment(withoutMCPEnvironment(os.Environ()), map[string]string{
			"MCP_PORT":      "0",
			"MCP_LOG_LEVEL": "disabled",
		}),
		[]string{"run", "./cmd/mcp-server", "mcp"},
		transport.WithCommandFunc(func(ctx context.Context, command string, environment []string, arguments []string) (*exec.Cmd, error) {
			process := exec.CommandContext(ctx, command, arguments...)
			process.Dir = repositoryRoot
			process.Env = environment
			return process, nil
		}),
	)
	if err != nil {
		t.Fatalf("start stdio MCP server: %v", err)
	}
	defer func() {
		if err := server.Close(); err != nil {
			t.Errorf("stop stdio MCP server: %v", err)
		}
	}()

	initialized, err := server.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "stdio-e2e", Version: "1.0"},
		},
	})
	if err != nil {
		t.Fatalf("initialize stdio MCP server: %v", err)
	}
	if !initialized.Capabilities.Tools.ListChanged {
		t.Fatal("stdio MCP server did not advertise tool capabilities")
	}

	tools, err := server.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list stdio MCP tools: %v", err)
	}
	toolNames := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	if !slices.Contains(toolNames, "higress_console_capabilities") {
		t.Fatalf("stdio MCP tools did not include higress_console_capabilities: %v", toolNames)
	}
}

func withoutMCPEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, item := range environment {
		key, _, found := strings.Cut(item, "=")
		if !found || !strings.HasPrefix(key, "MCP_") {
			result = append(result, item)
		}
	}
	return result
}
