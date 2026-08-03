package higress_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/futuretea/higress-mcp-server/pkg/console"
	"github.com/futuretea/higress-mcp-server/pkg/toolset/higress"
)

func TestDefaultModeRedactsSensitiveConsoleResponses(t *testing.T) {
	tests := []struct {
		name             string
		tool             string
		params           map[string]any
		method           string
		path             string
		statusCode       int
		body             string
		sensitive        string
		wantError        bool
		expectedError    string
		wantRedaction    bool
		expectedContents []string
	}{
		{
			name: "specialized tls certificates key", tool: "higress_list_resource",
			params: map[string]any{"resource": "tls_certificates"}, method: http.MethodGet, path: "/v1/tls-certificates",
			statusCode: http.StatusOK, body: `{"data":[{"name":"test-cert","key":"synthetic-tls-private-key"}]}`,
			sensitive: "synthetic-tls-private-key", wantRedaction: true,
			expectedContents: []string{`"name": "test-cert"`},
		},
		{
			name: "specialized llm providers tokens", tool: "higress_list_resource",
			params: map[string]any{"resource": "llm_providers"}, method: http.MethodGet, path: "/v1/ai/providers",
			statusCode: http.StatusOK, body: `{"data":[{"name":"test-provider","tokens":["synthetic-llm-provider-token"]}]}`,
			sensitive: "synthetic-llm-provider-token", wantRedaction: true,
			expectedContents: []string{`"name": "test-provider"`},
		},
		{
			name: "generic llm providers scalar tokens success", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodGet, "path": "/v1/ai/providers"}, method: http.MethodGet, path: "/v1/ai/providers",
			statusCode: http.StatusOK, body: `{"data":[{"name":"test-provider","tokens":"synthetic-scalar-provider-token-success"}]}`,
			sensitive: "synthetic-scalar-provider-token-success", wantError: true,
		},
		{
			name: "generic llm providers scalar tokens error", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodGet, "path": "/v1/ai/providers"}, method: http.MethodGet, path: "/v1/ai/providers",
			statusCode: http.StatusInternalServerError, body: `{"data":[{"name":"test-provider","tokens":"synthetic-scalar-provider-token-error"}]}`,
			sensitive: "synthetic-scalar-provider-token-error", wantError: true,
		},
		{
			name: "generic consumers trailing slash key auth value", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodGet, "path": "/v1/consumers/"}, method: http.MethodGet, path: "/v1/consumers/",
			statusCode: http.StatusOK, body: `{"data":[{"name":"test-consumer","credentials":[{"type":"key-auth","values":["synthetic-trailing-slash-consumer-key"]}]}]}`,
			sensitive: "synthetic-trailing-slash-consumer-key", wantRedaction: true,
			expectedContents: []string{`"name": "test-consumer"`},
		},
		{
			name: "generic mcp server database password", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodGet, "path": "/v1/mcpServer"}, method: http.MethodGet, path: "/v1/mcpServer",
			statusCode: http.StatusOK, body: `{"data":[{"name":"test-mcp","dbConfig":{"password":"synthetic-mcp-db-password"}}]}`,
			sensitive: "synthetic-mcp-db-password", wantRedaction: true,
			expectedContents: []string{`"name": "test-mcp"`},
		},
		{
			name: "generic user info password", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodGet, "path": "/user/info"}, method: http.MethodGet, path: "/user/info",
			statusCode: http.StatusOK, body: `{"data":{"username":"test-user","id":9007199254740993,"password":"synthetic-user-password"}}`,
			sensitive: "synthetic-user-password", wantRedaction: true,
			expectedContents: []string{`"username": "test-user"`, `"id": 9007199254740993`},
		},
		{
			name: "generic user info unredactable JSON scalar", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodGet, "path": "/user/info"}, method: http.MethodGet, path: "/user/info",
			statusCode: http.StatusOK, body: `"synthetic-unredactable-user-info"`,
			sensitive: "synthetic-unredactable-user-info", wantError: true,
		},
		{
			name: "generic user info missing data envelope success", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodGet, "path": "/user/info"}, method: http.MethodGet, path: "/user/info",
			statusCode: http.StatusOK, body: `{"password":"synthetic-missing-envelope-password"}`,
			sensitive: "synthetic-missing-envelope-password", wantError: true,
			expectedError: "cannot safely process sensitive Higress Console response",
		},
		{
			name: "generic user info missing data envelope error", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodGet, "path": "/user/info"}, method: http.MethodGet, path: "/user/info",
			statusCode: http.StatusInternalServerError, body: `{"password":"synthetic-missing-envelope-password"}`,
			sensitive: "synthetic-missing-envelope-password", wantError: true,
			expectedError: "cannot safely process sensitive Higress Console response",
		},
		{
			name: "generic user info raw text success", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodGet, "path": "/user/info"}, method: http.MethodGet, path: "/user/info",
			statusCode: http.StatusOK, body: "password=synthetic-raw-user-info-password",
			sensitive: "synthetic-raw-user-info-password", wantError: true,
			expectedError: "cannot safely process sensitive Higress Console response",
		},
		{
			name: "generic consumer create key auth value", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodPost, "path": "/v1/consumers", "body": map[string]any{"name": "test-consumer"}}, method: http.MethodPost, path: "/v1/consumers",
			statusCode: http.StatusOK, body: `{"data":{"name":"test-consumer","credentials":[{"type":"key-auth","values":["synthetic-created-key"]}]}}`,
			sensitive: "synthetic-created-key", wantRedaction: true,
			expectedContents: []string{`"name": "test-consumer"`},
		},
		{
			name: "generic error password", tool: "higress_console_request",
			params: map[string]any{"method": http.MethodGet, "path": "/user/info"}, method: http.MethodGet, path: "/user/info",
			statusCode: http.StatusInternalServerError, body: `{"data":{"password":"synthetic-error-password"}}`,
			sensitive: "synthetic-error-password", wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != test.method || r.URL.Path != test.path {
					t.Fatalf("request = %s %s, want %s %s", r.Method, r.URL.Path, test.method, test.path)
				}
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
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
			handler := handlersByName(set.GetTools())[test.tool]

			result, err := handler(context.Background(), test.params)
			if strings.Contains(result, test.sensitive) || (err != nil && strings.Contains(err.Error(), test.sensitive)) {
				t.Fatalf("default handler result/error exposes %q: result = %q, error = %v", test.sensitive, result, err)
			}
			if (err != nil) != test.wantError {
				t.Fatalf("handler error = %v, want error %t", err, test.wantError)
			}
			if test.expectedError != "" && err.Error() != test.expectedError {
				t.Fatalf("handler error = %q, want %q", err, test.expectedError)
			}
			output := result
			if err != nil {
				output = err.Error()
			}
			if test.wantRedaction && !strings.Contains(output, "***") {
				t.Fatalf("default tool output does not contain redaction marker")
			}
			for _, expected := range test.expectedContents {
				if !strings.Contains(output, expected) {
					t.Fatalf("default tool output does not preserve %q", expected)
				}
			}
		})
	}
}
