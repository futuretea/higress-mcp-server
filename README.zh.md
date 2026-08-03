# Higress Console MCP Server

[English](README.md)

这是连接 Higress Console 管理 API 的 MCP（Model Context Protocol）服务。它以
Streamable HTTP 为主要使用方式，同时保留 stdio 和 SSE，方便本地客户端接入。
MCP 的 HTTP 入口按需求不做内建认证；仅应部署在可信网络中，若需要公网暴露，请由
外部反向代理负责访问控制。

## 能力范围

服务提供常用资源的易用工具：路由、域名、服务来源、消费者、代理服务器、TLS
证书、Wasm 插件与其全局/路由/域名/服务实例，以及 AI 路由和 LLM Provider。

`higress_console_request` 是完整 API 的通用入口，可访问配置的 Console 上所有
管理路径，包括 system、dashboard、user、MCP Server、AI Proxy 与 Grafana。它拒绝
带主机、查询串或路径穿越的输入，也会拒绝离开已配置 Console base URL（含路径前缀）的
重定向，因此不能被用作转发到其他主机或应用的代理。可先调用
`higress_console_capabilities` 查看常用资源名称和稳定 API 分组。

实现已对照 Higress Console `v2.1.11` 与 `v2.2.3`：两条版本线的 URL 与 HTTP
方法契约一致。Console 的 API Docs 默认关闭；若目标以 `api-docs` profile 启动，
可使用 `higress_console_openapi` 获取 `/v3/api-docs/Higress`。

## HTTP 快速开始

Higress Console 通常使用 `8080` 端口，MCP 服务请使用其他端口：

```bash
make build

MCP_HIGRESS_CONSOLE_URL=http://127.0.0.1:8080 \
./bin/higress-mcp-server mcp --port 8081 --listen 0.0.0.0
```

MCP 客户端连接 `http://127.0.0.1:8081/mcp`（或实际发布的主机地址）。该
Streamable HTTP 端点不需要认证。
`/mcp` 和 SSE 的 `/message` 端点单次请求体上限为 8 MiB。

Console 自身的管理 API 通常需要认证，这与 MCP HTTP 入口相互独立：

```bash
export MCP_HIGRESS_CONSOLE_USERNAME=admin
export MCP_HIGRESS_CONSOLE_PASSWORD='<console-password>'
export MCP_HIGRESS_CONSOLE_AUTH_MODE=session  # session（默认）或 basic
```

`session` 会登录一次并保留 Console Cookie；`basic` 则在每次请求中向已配置的
Console 地址发送 HTTP Basic 认证。仅当目标 Console 明确允许匿名操作时，才可不
设置用户名和密码。不要把真实凭据写入受版本控制的 YAML。

## 配置

优先级为：命令行参数 > 环境变量 > YAML > 默认值。

| 设置 | 参数 | 环境变量 | 默认值 |
| --- | --- | --- | --- |
| Console 地址 | `--console-url` | `MCP_HIGRESS_CONSOLE_URL` | `http://127.0.0.1:8080` |
| Console 认证方式 | `--console-auth-mode` | `MCP_HIGRESS_CONSOLE_AUTH_MODE` | `session` |
| Console 超时秒数 | `--console-timeout-seconds` | `MCP_HIGRESS_CONSOLE_TIMEOUT_SECONDS` | `30` |
| Console 用户名 | — | `MCP_HIGRESS_CONSOLE_USERNAME` | 未设置 |
| Console 密码 | — | `MCP_HIGRESS_CONSOLE_PASSWORD` | 未设置 |
| MCP HTTP 端口 | `mcp --port` | `MCP_PORT` | `0`（stdio） |
| MCP 监听地址 | `mcp --listen` | `MCP_LISTEN` | `127.0.0.1` |

非敏感的 YAML 配置见 [config.example.yaml](config.example.yaml)。

## MCP 工具

| 工具 | 用途 |
| --- | --- |
| `higress_console_capabilities` | 列出常用资源与 API Controller 分组。 |
| `higress_console_openapi` | 获取可选的 Console API Docs。 |
| `higress_console_request` | 调用已配置 Console 上的任意 API 路径。 |
| `higress_list_resource` / `higress_get_resource` | 读取常用具名资源。 |
| `higress_create_resource` / `higress_update_resource` / `higress_delete_resource` | 修改常用具名资源。 |
| `higress_manage_plugin_instance` | 管理全局、路由、域名和服务级 Wasm 实例。 |

命令行可用于查看工具 Schema 或做脚本调用：

```bash
./bin/higress-mcp-server tools list
./bin/higress-mcp-server tools describe higress_create_resource --json
./bin/higress-mcp-server tools call higress_list_resource \
  --params '{"resource":"routes"}'
```

## 其他传输方式

不传 `--port` 时仍使用 stdio：

```bash
./bin/higress-mcp-server mcp
```

同一个 HTTP 进程还提供 `/sse`、`/message` 和健康检查 `/healthz`。

## 开发

```bash
make ci
```

可选的 Docker 端到端测试会分别连接真实 Higress all-in-one 的 Console
`2.1.11` 与 `2.2.3`：

```bash
make e2e
```

测试为每个版本启动隔离的临时容器并初始化 Console，再通过无需认证的
Streamable HTTP MCP 端点创建和读取静态服务源及路由。它会验证真实网关转发到
临时后端，删除路由后确认转发停止，并删除临时服务源。需要先启动 Docker；
`make test` 不会启动容器。设置 `HIGRESS_E2E_VERSIONS=2.2.3` 可只运行一个
受支持版本。

`third-party-projects/` 用于放置本地参考源码，已被 Git 忽略。本项目以它比较
Console API 契约，不会把上游源码打进交付物。

## 发布产物

稳定发布标签使用 `vX.Y.Z`。npm 使用 `X.Y.Z`；Docker 使用 `vX.Y.Z`。

```bash
npx -y @futuretea/higress-mcp-server@X.Y.Z
docker run --rm -i ghcr.io/futuretea/higress-mcp-server:vX.Y.Z
```

两种方式都会启动 MCP server 的 `mcp` 命令。包名或镜像名之后的参数会转发给该
命令。支持 stdio 的 MCP 客户端可通过 npm 配置：

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

支持的发布平台记录在 `.github/ci/release-platforms.json`。首次发布 npm 前，设置
仓库变量 `NPM_REGISTRY_URL`，并设置对该 registry 有发布权限的 `NPM_TOKEN`。
Docker 镜像使用工作流的 `GITHUB_TOKEN` 发布到 GHCR。

## 许可证

[MIT](LICENSE)
