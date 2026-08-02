package console

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Resource describes a common Higress Console resource with conventional CRUD paths.
type Resource struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Collection  string   `json:"collectionPath"`
	Operations  []string `json:"operations"`
}

var resources = map[string]Resource{
	"ai_routes": {
		Name: "ai_routes", Description: "AI routes", Collection: "/v1/ai/routes",
		Operations: []string{"list", "get", "create", "update", "delete"},
	},
	"consumers": {
		Name: "consumers", Description: "Consumers", Collection: "/v1/consumers",
		Operations: []string{"list", "get", "create", "update", "delete"},
	},
	"domains": {
		Name: "domains", Description: "Domains", Collection: "/v1/domains",
		Operations: []string{"list", "get", "create", "update", "delete"},
	},
	"llm_providers": {
		Name: "llm_providers", Description: "LLM providers", Collection: "/v1/ai/providers",
		Operations: []string{"list", "get", "create", "update", "delete"},
	},
	"proxy_servers": {
		Name: "proxy_servers", Description: "Proxy servers", Collection: "/v1/proxy-servers",
		Operations: []string{"list", "get", "create", "update", "delete"},
	},
	"routes": {
		Name: "routes", Description: "Gateway routes", Collection: "/v1/routes",
		Operations: []string{"list", "get", "create", "update", "delete"},
	},
	"service_sources": {
		Name: "service_sources", Description: "Service sources", Collection: "/v1/service-sources",
		Operations: []string{"list", "get", "create", "update", "delete"},
	},
	"tls_certificates": {
		Name: "tls_certificates", Description: "TLS certificates", Collection: "/v1/tls-certificates",
		Operations: []string{"list", "get", "create", "update", "delete"},
	},
	"wasm_plugins": {
		Name: "wasm_plugins", Description: "Wasm plugins", Collection: "/v1/wasm-plugins",
		Operations: []string{"list", "get", "create", "update", "delete"},
	},
}

// Resources returns common resource metadata in stable display order.
func Resources() []Resource {
	items := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		resource.Operations = append([]string(nil), resource.Operations...)
		items = append(items, resource)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

// ResourcePath resolves a common resource operation to a Console API path.
func ResourcePath(resourceName, operation, name string) (string, error) {
	resource, ok := resources[resourceName]
	if !ok {
		return "", fmt.Errorf("unknown resource %q; use higress_console_capabilities to list supported resources", resourceName)
	}
	for _, supported := range resource.Operations {
		if operation == supported {
			if operation == "list" || operation == "create" {
				return resource.Collection, nil
			}
			if err := validatePathSegment("name", name); err != nil {
				return "", fmt.Errorf("%s %s", operation, err)
			}
			return resource.Collection + "/" + name, nil
		}
	}
	return "", fmt.Errorf("resource %q does not support %s", resourceName, operation)
}

func validatePathSegment(label, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	decoded, err := url.PathUnescape(value)
	if err != nil || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/?#") {
		return fmt.Errorf("%s must be one path segment", label)
	}
	return nil
}
