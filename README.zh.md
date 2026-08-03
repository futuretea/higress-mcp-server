# Higress Console MCP Server

[English](README.md)

这是连接 Higress Console 管理 API 的 MCP（Model Context Protocol）服务。它以
Streamable HTTP 为主要使用方式，同时保留 stdio 和 SSE，方便本地客户端接入。
MCP 的 HTTP 入口按需求不做内建认证；仅应部署在可信网络中，若需要公网暴露，请由
外部反向代理负责访问控制。

## 能力范围

服务提供常用资源的易用工具：路由、域名、服务来源、消费者、代理服务器、TLS
证书、Wasm 插件与其全局/路由/域名/服务实例，以及 AI 路由和 LLM Provider。

`higress_console_request` 是完整 API 的通用入口，仅在 `read_only=false` 时可用，
可访问配置的 Console 上所有管理路径，包括 system、dashboard、user、MCP Server、AI
Proxy 与 Grafana。它拒绝带主机、查询串或路径穿越的输入，也会拒绝离开已配置 Console base URL（含路径前缀）的
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
| 只读工具目录 | `--read-only` | `MCP_READ_ONLY` | `true` |

非敏感的 YAML 配置见 [config.example.yaml](config.example.yaml)。

服务默认只读，仅暴露 `higress_console_capabilities`、
`higress_console_openapi`、`higress_list_resource` 和
`higress_get_resource`；`enabled_tools` 不能恢复被排除的写能力工具。需要管理
Console 资源时，显式设置 `read_only: false`、`MCP_READ_ONLY=false` 或
`--read-only=false`，然后重启服务。该目录策略不为公开 MCP HTTP 端点新增认证或
授权；公开部署应放在已认证的反向代理之后。

## MCP 工具

| 工具 | 用途 |
| --- | --- |
| `higress_console_capabilities` | 列出常用资源与 API Controller 分组。 |
| `higress_console_openapi` | 获取可选的 Console API Docs。 |
| `higress_console_request` | 调用已配置 Console 上的任意 API 路径；仅在 `read_only=false` 时可用。 |
| `higress_list_resource` / `higress_get_resource` | 读取常用具名资源。 |
| `higress_create_resource` / `higress_update_resource` / `higress_delete_resource` | 修改常用具名资源；仅在 `read_only=false` 时可用。 |
| `higress_manage_plugin_instance` | 管理全局、路由、域名和服务级 Wasm 实例；仅在 `read_only=false` 时可用。 |

命令行可用于查看工具 Schema 或做脚本调用：

```bash
./bin/higress-mcp-server tools list
./bin/higress-mcp-server --read-only=false tools describe higress_create_resource --json
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

## 使用已发布版本

GitHub 发布标签使用 `vX.Y.Z`。对应的 npm 包版本为 `X.Y.Z`，Docker 镜像标签为
`vX.Y.Z`。以下命令适用于 npm 和 Docker 发布工作流均已成功的标签。最初的
`v0.0.1` GitHub release 早于这些工作流，因此不提供该 scoped npm 包或可公开访问的
Docker 镜像。

### npm

通过 `npx` 以 stdio 方式运行 MCP server：

```bash
MCP_HIGRESS_CONSOLE_URL=http://higress-console:8080 \
npx -y @futuretea/higress-mcp-server@X.Y.Z
```

MCP 客户端可通过环境变量传入 Console 地址：

```json
{
  "mcpServers": {
    "higress-console": {
      "command": "npx",
      "args": ["-y", "@futuretea/higress-mcp-server@X.Y.Z"],
      "env": {
        "MCP_HIGRESS_CONSOLE_URL": "http://higress-console:8080"
      }
    }
  }
}
```

npm 命令是 MCP 入口，始终启动服务的 `mcp` 命令。包名之后的参数是 MCP 参数，例如
`--port 8081`；不能借此执行服务的其他 CLI 子命令。请固定为需要运行的包版本。
Console 凭据应通过运行环境的 secret 机制提供，不要写入受版本控制的客户端配置。
若发布使用 npmjs.org 以外的 registry，请先将 `@futuretea` scope 配置为从该 registry
解析，再运行 `npx`。

### Docker

直接运行已发布镜像，以 stdio 方式提供 MCP：

```bash
MCP_HIGRESS_CONSOLE_URL=http://higress-console:8080 \
docker run --rm -i \
  --env MCP_HIGRESS_CONSOLE_URL \
  ghcr.io/futuretea/higress-mcp-server:vX.Y.Z
```

如果 MCP 客户端通过 Docker 启动服务，需要把 Console 地址继续传入容器：

```json
{
  "mcpServers": {
    "higress-console": {
      "command": "docker",
      "args": [
        "run",
        "--rm",
        "-i",
        "--env",
        "MCP_HIGRESS_CONSOLE_URL",
        "ghcr.io/futuretea/higress-mcp-server:vX.Y.Z"
      ],
      "env": {
        "MCP_HIGRESS_CONSOLE_URL": "http://higress-console:8080"
      }
    }
  }
}
```

配置的 Console 主机名必须能从容器访问。容器中的 `127.0.0.1` 指向容器自身，而不是
Docker 宿主机；请使用 Docker 网络中的 Console 主机名，或在适用时使用平台对应的
宿主机网关。若 Console 需要认证，请通过运行环境的 secret 机制传递
`MCP_HIGRESS_CONSOLE_USERNAME` 与 `MCP_HIGRESS_CONSOLE_PASSWORD`。

### HTTP 模式

在 npm 包名后传入 MCP 参数即可启动 Streamable HTTP，而非 stdio。默认监听
loopback：

```bash
MCP_HIGRESS_CONSOLE_URL=http://127.0.0.1:8080 \
npx -y @futuretea/higress-mcp-server@X.Y.Z --port 8081
```

使用 Docker 时，服务必须监听容器的所有接口，但向宿主机发布的端口仍可只绑定
loopback：

```bash
MCP_HIGRESS_CONSOLE_URL=http://higress-console:8080 \
docker run --rm \
  --env MCP_HIGRESS_CONSOLE_URL \
  --publish 127.0.0.1:8081:8081 \
  ghcr.io/futuretea/higress-mcp-server:vX.Y.Z \
  --port 8081 --listen 0.0.0.0
```

Streamable HTTP MCP 客户端连接 `http://127.0.0.1:8081/mcp`。HTTP server 没有
内建认证或 TLS；仅应部署在可信网络中，或置于提供所需保护的基础设施之后。

支持的发布平台记录在 `.github/ci/release-platforms.json`。首次发布 npm 前，设置
仓库变量 `NPM_REGISTRY_URL`，并设置对该 registry 有发布权限的 `NPM_TOKEN`。
Docker 镜像使用工作流的 `GITHUB_TOKEN` 发布到 GHCR。

## 许可证

[MIT](LICENSE)
