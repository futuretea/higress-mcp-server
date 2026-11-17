package mcp

import (
	"context"
	"errors"
	"testing"

	mcpprotocol "github.com/mark3labs/mcp-go/mcp"

	"github.com/futuretea/higress-mcp-server/pkg/toolset"
)

func TestNewToolHandlerRejectsNonObjectArguments(t *testing.T) {
	tests := []struct {
		name      string
		arguments any
		wantCalls int
		wantError bool
		wantText  string
	}{
		{name: "omitted", arguments: nil, wantCalls: 1, wantError: true, wantText: "resource is required"},
		{name: "object", arguments: map[string]any{"resource": "routes"}, wantCalls: 1, wantText: "ok"},
		{name: "empty object", arguments: map[string]any{}, wantCalls: 1, wantError: true, wantText: "resource is required"},
		{name: "array", arguments: []any{}, wantError: true, wantText: "expected an object"},
		{name: "string", arguments: "nope", wantError: true, wantText: "expected an object"},
		{name: "number", arguments: 1.0, wantError: true, wantText: "expected an object"},
		{name: "bool", arguments: true, wantError: true, wantText: "expected an object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			handler := newToolHandler(toolset.ServerTool{
				Handler: func(_ context.Context, params map[string]any) (string, error) {
					calls++
					if params == nil {
						t.Fatal("handler params must be a non-nil map")
					}
					resource, _ := params["resource"].(string)
					if resource == "" {
						return "", errors.New("resource is required")
					}
					return "ok", nil
				},
			})

			result, err := handler(t.Context(), mcpprotocol.CallToolRequest{
				Params: mcpprotocol.CallToolParams{Arguments: tt.arguments},
			})
			if err != nil {
				t.Fatalf("handler transport error = %v, want nil", err)
			}
			if calls != tt.wantCalls {
				t.Fatalf("handler calls = %d, want %d", calls, tt.wantCalls)
			}
			if result.IsError != tt.wantError {
				t.Fatalf("IsError = %t, want %t", result.IsError, tt.wantError)
			}
			text, ok := result.Content[0].(mcpprotocol.TextContent)
			if !ok || text.Text != tt.wantText {
				t.Fatalf("text = %#v, want %q", result.Content[0], tt.wantText)
			}
		})
	}
}

func TestNewToolHandlerReturnsErrorResult(t *testing.T) {
	wantErr := errors.New("boom")
	handler := newToolHandler(toolset.ServerTool{
		Handler: func(_ context.Context, params map[string]any) (string, error) {
			if params == nil {
				t.Fatal("handler params must be a non-nil map")
			}
			return "", wantErr
		},
	})

	result, err := handler(t.Context(), mcpprotocol.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler transport error = %v, want nil", err)
	}
	if !result.IsError {
		t.Fatal("handler result IsError = false, want true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("handler content count = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(mcpprotocol.TextContent)
	if !ok {
		t.Fatalf("handler content type = %T, want mcp.TextContent", result.Content[0])
	}
	if text.Type != "text" || text.Text != wantErr.Error() {
		t.Fatalf("handler text = {Type:%q Text:%q}, want {Type:%q Text:%q}", text.Type, text.Text, "text", wantErr)
	}
}
