# Higress Console MCP Server

[中文文档](README.zh.md)

An MCP (Model Context Protocol) server for the Higress Console management API.
It is designed for Streamable HTTP first, while retaining stdio and SSE for
local clients. The MCP HTTP endpoint has no built-in authentication, as
required; keep it on a trusted network or put your own reverse proxy in front
when exposing it.

## What it manages

The server provides friendly tools for the resources most agents manage often:

- routes, domains, service sources, consumers, proxy servers, TLS certificates;
- Wasm plugins and scoped plugin instances;
- AI routes and LLM providers.

`higress_console_request` is the complete API escape hatch when
`read_only=false`. It can invoke any path on the configured Console origin,
including system, dashboard, user, MCP server, AI proxy, and Grafana controller
paths. It rejects a host, query string,
or traversal sequence in the supplied path, and it rejects redirects that leave
the configured Console base URL (including any path prefix), so it cannot be
used as a proxy to another origin or application. Run
`higress_console_capabilities` to discover the common resource names and stable
API groups.

The URL and HTTP-method surface is verified against Higress Console `v2.1.11`
and `v2.2.3`; these are the latest available `2.1.x` and `2.2.x` tags when the
implementation was produced. Console API Docs are optional and disabled by
default. If the target starts its `api-docs` profile, use
`higress_console_openapi` to retrieve `/v3/api-docs/Higress`.

## Quick start: HTTP

The Higress Console commonly listens on port `8080`. Use a different port for
this MCP server:

```bash
make build

MCP_HIGRESS_CONSOLE_URL=http://127.0.0.1:8080 \
./bin/higress-mcp-server mcp --port 8081 --listen 0.0.0.0
```

Connect an MCP client to `http://127.0.0.1:8081/mcp` (or the host address you
published). The Streamable HTTP endpoint does not require authentication.
`/mcp` and the SSE `/message` endpoint accept request bodies up to 8 MiB.

The target Console normally authenticates its management APIs. Configure this
separately from the MCP endpoint:

```bash
export MCP_HIGRESS_CONSOLE_USERNAME=admin
export MCP_HIGRESS_CONSOLE_PASSWORD='<console-password>'
export MCP_HIGRESS_CONSOLE_AUTH_MODE=session  # session (default) or basic
```

`session` logs in once and retains the Console cookie. `basic` sends HTTP Basic
authentication to the configured Console origin on every request. Leave both
credential variables unset only when the target Console endpoint permits the
operation anonymously. Do not put credentials in version-controlled YAML.

## Client configuration

Configuration precedence is flags, environment variables, YAML, then defaults.

| Setting | Flag | Environment variable | Default |
| --- | --- | --- | --- |
| Console base URL | `--console-url` | `MCP_HIGRESS_CONSOLE_URL` | `http://127.0.0.1:8080` |
| Console auth mode | `--console-auth-mode` | `MCP_HIGRESS_CONSOLE_AUTH_MODE` | `session` |
| Console timeout | `--console-timeout-seconds` | `MCP_HIGRESS_CONSOLE_TIMEOUT_SECONDS` | `30` |
| Console username | — | `MCP_HIGRESS_CONSOLE_USERNAME` | unset |
| Console password | — | `MCP_HIGRESS_CONSOLE_PASSWORD` | unset |
| MCP HTTP port | `mcp --port` | `MCP_PORT` | `0` (stdio) |
| MCP listen host | `mcp --listen` | `MCP_LISTEN` | `127.0.0.1` |
| Read-only tool catalog | `--read-only` | `MCP_READ_ONLY` | `true` |

See [config.example.yaml](config.example.yaml) for non-secret YAML settings.

The server is read-only by default. It exposes only `higress_console_capabilities`,
`higress_console_openapi`, `higress_list_resource`, and
`higress_get_resource`; `enabled_tools` cannot restore excluded write-capable
tools. To manage Console resources, explicitly set `read_only: false`,
`MCP_READ_ONLY=false`, or `--read-only=false` and restart the server. This
catalog policy does not add authentication or authorization for a public MCP
HTTP endpoint; place public deployments behind an authenticated reverse proxy.

## Tools

| Tool | Purpose |
| --- | --- |
| `higress_console_capabilities` | List common resource names and API controller groups. |
| `higress_console_openapi` | Fetch optional Console API Docs. |
| `higress_console_request` | Call any Console API path on the configured origin; available only with `read_only=false`. |
| `higress_list_resource` / `higress_get_resource` | Read common named resources. |
| `higress_create_resource` / `higress_update_resource` / `higress_delete_resource` | Change common named resources; available only with `read_only=false`. |
| `higress_manage_plugin_instance` | Manage global, route, domain, and service Wasm instances; available only with `read_only=false`. |

The CLI is useful for checking schemas and making scripted calls:

```bash
./bin/higress-mcp-server tools list
./bin/higress-mcp-server --read-only=false tools describe higress_create_resource --json
./bin/higress-mcp-server tools call higress_list_resource \
  --params '{"resource":"routes"}'
```

## Other transports

Stdio remains the default when `--port` is omitted:

```bash
./bin/higress-mcp-server mcp
```

The same HTTP process also serves SSE at `/sse`, with `/message` as its message
endpoint. Health is available at `/healthz`.

## Development

```bash
make ci
```

Run the opt-in Docker E2E suite against real Higress all-in-one instances for
Console `2.1.11` and `2.2.3`:

```bash
make e2e
```

The suite starts isolated temporary containers, initializes each Console, then
uses the unauthenticated Streamable HTTP MCP endpoint to create and read a
static service source and route. It verifies actual gateway forwarding to a
temporary backend, deletes the route and verifies forwarding stops, and removes
the temporary source. Docker must be running; `make test` does not start
containers. Set `HIGRESS_E2E_VERSIONS=2.2.3` to run one supported version.

Local reference source checkouts belong in `third-party-projects/`; the
directory is ignored by Git. This project uses it to compare Console API
contracts without shipping the upstream source.

## Release artifacts

Release tags use `vX.Y.Z`. Use `X.Y.Z` for npm and `vX.Y.Z` for Docker.

```bash
npx -y @futuretea/higress-mcp-server@X.Y.Z
docker run --rm -i ghcr.io/futuretea/higress-mcp-server:vX.Y.Z
```

Both launch the MCP server's `mcp` command. Arguments after the package or
image are forwarded to that command. Configure an MCP client that supports
stdio with npm:

```json
{
  "mcpServers": {
    "higress-console": {
      "command": "npx",
      "args": ["-y", "@futuretea/higress-mcp-server@X.Y.Z"]
    }
  }
}
```

Supported release platforms are recorded in `.github/ci/release-platforms.json`.
Before the first npm release, set the repository variable `NPM_REGISTRY_URL`
and set `NPM_TOKEN` with publish permission for that registry. Docker images
publish to GHCR with the workflow's `GITHUB_TOKEN`.

## License

[MIT](LICENSE)
