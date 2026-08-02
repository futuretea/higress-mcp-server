package console_test

import (
	"testing"

	"github.com/futuretea/higress-mcp-server/pkg/console"
)

func TestResourcePath(t *testing.T) {
	tests := []struct {
		resource  string
		operation string
		name      string
		want      string
	}{
		{resource: "routes", operation: "list", want: "/v1/routes"},
		{resource: "routes", operation: "create", want: "/v1/routes"},
		{resource: "ai_routes", operation: "update", name: "chat", want: "/v1/ai/routes/chat"},
	}
	for _, tt := range tests {
		t.Run(tt.resource+tt.operation, func(t *testing.T) {
			got, err := console.ResourcePath(tt.resource, tt.operation, tt.name)
			if err != nil || got != tt.want {
				t.Fatalf("ResourcePath() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestResourcePathRejectsIncompleteInput(t *testing.T) {
	if _, err := console.ResourcePath("routes", "get", ""); err == nil {
		t.Fatal("expected missing name error")
	}
	if _, err := console.ResourcePath("missing", "list", ""); err == nil {
		t.Fatal("expected unknown resource error")
	}
	if _, err := console.ResourcePath("routes", "get", "route/other"); err == nil {
		t.Fatal("expected path segment error")
	}
	for _, name := range []string{"route%2Fother", "route%3Fother", "route%23other", "%"} {
		if _, err := console.ResourcePath("routes", "get", name); err == nil {
			t.Fatalf("expected encoded path segment error for %q", name)
		}
	}
}
