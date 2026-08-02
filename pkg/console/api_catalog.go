package console

// APIGroup documents the controller routes that are stable across Console 2.1.x and 2.2.x.
type APIGroup struct {
	Path        string   `json:"path"`
	Methods     []string `json:"methods"`
	Description string   `json:"description"`
}

// APIGroups returns Console route groups available through higress_console_request.
func APIGroups() []APIGroup {
	return []APIGroup{
		{Path: "/v1/routes", Methods: []string{"GET", "POST", "GET|PUT|DELETE /{name}"}, Description: "Gateway routes"},
		{Path: "/v1/domains", Methods: []string{"GET", "POST", "GET|PUT|DELETE /{name}", "GET /{name}/routes"}, Description: "Domains"},
		{Path: "/v1/service-sources", Methods: []string{"GET", "POST", "GET|PUT|DELETE /{name}"}, Description: "Service sources"},
		{Path: "/v1/services", Methods: []string{"GET"}, Description: "Discovered services"},
		{Path: "/v1/consumers", Methods: []string{"GET", "POST", "GET|PUT|DELETE /{name}"}, Description: "Consumers"},
		{Path: "/v1/proxy-servers", Methods: []string{"GET", "POST", "GET|PUT|DELETE /{name}"}, Description: "Proxy servers"},
		{Path: "/v1/tls-certificates", Methods: []string{"GET", "POST", "GET|PUT|DELETE /{name}"}, Description: "TLS certificates"},
		{Path: "/v1/wasm-plugins", Methods: []string{"GET", "POST", "GET|PUT|DELETE /{name}", "GET /{name}/config", "GET /{name}/readme"}, Description: "Wasm plugins"},
		{Path: "/v1/{global|domains|routes|services}/.../plugin-instances", Methods: []string{"GET", "GET|PUT|DELETE /{name}"}, Description: "Wasm plugin instances"},
		{Path: "/v1/ai/routes", Methods: []string{"GET", "POST", "GET|PUT|DELETE /{name}"}, Description: "AI routes"},
		{Path: "/v1/ai/providers", Methods: []string{"GET", "POST", "GET|PUT|DELETE /{name}"}, Description: "LLM providers"},
		{Path: "/v1/mcpServer", Methods: []string{"GET", "PUT", "GET|DELETE /{name}", "POST /swaggerToMcpConfig", "GET|PUT|DELETE /consumers"}, Description: "Managed MCP servers"},
		{Path: "/system", Methods: []string{"POST /init", "GET /info", "GET /config", "GET|PUT /higress-config"}, Description: "System configuration"},
		{Path: "/dashboard", Methods: []string{"GET /init", "GET|PUT /info", "GET /configData"}, Description: "Dashboard configuration"},
		{Path: "/user", Methods: []string{"GET /info", "POST /changePassword"}, Description: "Current user"},
		{Path: "/session", Methods: []string{"POST /login", "GET /logout"}, Description: "Session management"},
		{Path: "/healthz/ready", Methods: []string{"GET"}, Description: "Readiness"},
		{Path: "/aiproxy/**", Methods: []string{"ANY"}, Description: "AI backend proxy"},
		{Path: "/grafana/**", Methods: []string{"ANY"}, Description: "Grafana proxy"},
	}
}
