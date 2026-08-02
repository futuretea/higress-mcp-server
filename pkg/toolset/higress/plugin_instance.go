package higress

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/futuretea/higress-mcp-server/pkg/console"
)

func (t *Toolset) managePluginInstance(ctx context.Context, params map[string]any) (string, error) {
	operation, err := requiredString(params, "operation")
	if err != nil {
		return "", err
	}
	scope, err := requiredString(params, "scope")
	if err != nil {
		return "", err
	}
	pluginName, _ := optionalString(params, "plugin_name")
	path, err := pluginInstancePath(scope, operation, optionalValue(params, "target"), pluginName)
	if err != nil {
		return "", err
	}

	method := map[string]string{
		"list": http.MethodGet, "get": http.MethodGet, "upsert": http.MethodPut, "delete": http.MethodDelete,
	}[operation]
	if method == "" {
		return "", fmt.Errorf("unsupported plugin instance operation %q", operation)
	}
	var body any
	if operation == "upsert" {
		body, err = requiredObject(params, "body")
		if err != nil {
			return "", err
		}
	}
	return t.call(ctx, console.Request{Method: method, Path: path, Body: body})
}

func pluginInstancePath(scope, operation, target, pluginName string) (string, error) {
	basePaths := map[string]string{
		"global":   "/v1/global/plugin-instances",
		"routes":   "/v1/routes",
		"domains":  "/v1/domains",
		"services": "/v1/services",
	}
	base, ok := basePaths[scope]
	if !ok {
		return "", fmt.Errorf("unsupported plugin instance scope %q", scope)
	}
	if scope != "global" {
		if err := validateToolPathSegment("target", target); err != nil {
			return "", fmt.Errorf("%s plugin instances: %w", scope, err)
		}
		base += "/" + target + "/plugin-instances"
	}
	if operation == "list" {
		return base, nil
	}
	if err := validateToolPathSegment("plugin_name", pluginName); err != nil {
		return "", fmt.Errorf("%s plugin instance: %w", operation, err)
	}
	return base + "/" + pluginName, nil
}

func validateToolPathSegment(label, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	decoded, err := url.PathUnescape(value)
	if err != nil || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/?#") {
		return fmt.Errorf("%s must be one path segment", label)
	}
	return nil
}
