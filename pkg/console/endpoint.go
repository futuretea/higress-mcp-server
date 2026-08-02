package console

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func parseBaseURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("parse console URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("console URL must use http or https, got %q", parsed.Scheme)
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("console URL must contain only an http(s) origin and optional path prefix")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, nil
}

func normalizeMethod(method string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(method))
	switch normalized {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported Console HTTP method %q", method)
	}
}

func (c *Client) resolvePath(rawPath string, query map[string]string) (*url.URL, error) {
	parsed, err := url.Parse(rawPath)
	if err != nil {
		return nil, fmt.Errorf("parse Console API path: %w", err)
	}
	if parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || !strings.HasPrefix(parsed.Path, "/") {
		return nil, errors.New("console API path must be an absolute path without host or query string")
	}
	if hasTraversalSegment(parsed.Path) {
		return nil, errors.New("console API path must not contain traversal segments")
	}

	endpoint := *c.baseURL
	endpoint.Path += parsed.Path
	endpoint.RawPath = ""
	values := make(url.Values, len(query))
	for key, value := range query {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errors.New("console query parameter name must not be empty")
		}
		values.Set(key, value)
	}
	endpoint.RawQuery = values.Encode()
	return &endpoint, nil
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		portForOrigin(left) == portForOrigin(right)
}

func portForOrigin(endpoint *url.URL) string {
	if port := endpoint.Port(); port != "" {
		return port
	}
	if strings.EqualFold(endpoint.Scheme, "https") {
		return "443"
	}
	return "80"
}

func isWithinBasePath(targetPath, basePath string) bool {
	basePath = strings.TrimRight(basePath, "/")
	return basePath == "" || targetPath == basePath || strings.HasPrefix(targetPath, basePath+"/")
}

func hasTraversalSegment(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}
