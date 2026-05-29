---
name: futrixdata
description: Query, manage, and inspect databases via FutrixData CLI — supports MySQL, PostgreSQL, MongoDB, Redis, Elasticsearch, DynamoDB, Cloudflare D1 / 通过 FutrixData CLI 查询、管理和检查数据库
user-invocable: true
---

# FutrixData CLI

English:
A unified command-line interface for managing and querying databases. Supports MySQL, PostgreSQL, MongoDB, Redis, Elasticsearch, DynamoDB, and Cloudflare D1.

中文：
统一的数据库管理与查询命令行工具，支持 MySQL、PostgreSQL、MongoDB、Redis、Elasticsearch、DynamoDB 和 Cloudflare D1。

## Prerequisites / 前置条件

English:
The `futrixdata-cli` binary must be in your PATH. If you installed the FutrixData desktop app, it is available automatically.

中文：
`futrixdata-cli` 需要在 PATH 中可用。如果已安装 FutrixData 桌面应用，CLI 会自动注入。

## Recommended Workflow / 推荐工作流

English:
1. Run `futrixdata-cli datasource list` to discover available datasource IDs
2. Use `console databases` and `console entities` to explore schema
3. Use `console describe` to inspect column types before writing queries
4. Use `console execute` to run queries — write operations require `--approve`
5. Always prefer `--json` flag for structured output when parsing results programmatically
6. If a datasource connection fails with auth errors, use the appropriate auth command to re-authenticate

中文：
1. 先执行 `futrixdata-cli datasource list` 获取可用数据源 ID
2. 用 `console databases` 和 `console entities` 浏览数据库结构
3. 用 `console describe` 了解列类型后再编写查询
4. 用 `console execute` 执行查询 — 写操作需要 `--approve`
5. 需要程序化处理结果时始终加 `--json`
6. 如果数据源连接因授权失败，使用对应的授权命令重新认证

## Commands / 命令

### Auth — Session Management / 认证管理

```bash
futrixdata-cli auth status                    # Check login status / 查看登录状态
futrixdata-cli auth login                     # Login (opens browser) / 登录（打开浏览器）
futrixdata-cli auth login --no-browser        # Login without opening browser / 登录（不打开浏览器）
futrixdata-cli auth login --code <code>       # Complete login with manual code / 使用手动代码完成登录
futrixdata-cli auth logout                    # Logout / 登出
futrixdata-cli auth devices list              # List authorized devices / 列出授权设备
futrixdata-cli auth devices remove --device-id <id>  # Remove a device / 移除设备
```

English:
The CLI shares authentication with the FutrixData desktop app. If you get auth errors, run `auth login` first.

中文：
CLI 与 FutrixData 桌面应用共享认证。如果遇到认证错误，先执行 `auth login`。

### Datasource — Connection Management / 数据源管理

```bash
futrixdata-cli datasource list                        # List all datasources / 列出所有数据源
futrixdata-cli datasource test --id <datasourceId>    # Test connectivity / 测试连接
```

### Console — Query & Exploration / 查询与探索

```bash
futrixdata-cli console databases --datasource <datasourceId>
futrixdata-cli console entities --datasource <datasourceId> [--database <db>]
futrixdata-cli console describe --datasource <datasourceId> --name <entity> [--database <db>]
futrixdata-cli console execute --datasource <datasourceId> --statement "<SQL>" [--database <db>] [--approve]
futrixdata-cli console explain --datasource <datasourceId> --statement "<SQL>" [--database <db>]
futrixdata-cli console scan-redis --datasource <datasourceId> [--pattern "user:*"]
```

English:
- `databases` — list databases/schemas in a datasource
- `entities` — list tables, collections, or indices
- `describe` — show column types, keys, and defaults
- `execute` — run a query; write operations require `--approve`
- `explain` — show query execution plan
- `scan-redis` — scan Redis keys by pattern

中文：
- `databases` — 列出数据源中的数据库/schema
- `entities` — 列出表、集合或索引
- `describe` — 显示列类型、键和默认值
- `execute` — 执行查询；写操作需要 `--approve`
- `explain` — 显示查询执行计划
- `scan-redis` — 按模式扫描 Redis 键

### D1 — Cloudflare D1 Management / Cloudflare D1 管理

```bash
futrixdata-cli d1 oauth-login                         # Login to Cloudflare via OAuth (opens browser) / 通过 OAuth 登录 Cloudflare（打开浏览器）
futrixdata-cli d1 oauth-relogin                       # Re-login with existing Cloudflare OAuth session / 使用已有会话重新登录
futrixdata-cli d1 wrangler-installed                  # Check if wrangler CLI is available / 检查 wrangler 是否可用
futrixdata-cli d1 list-cloud-databases --account-id <id> --token <token>   # List D1 databases / 列出 D1 数据库
futrixdata-cli d1 create-cloud-database --account-id <id> --token <token> --name <name> --approve  # Create D1 database / 创建 D1 数据库
futrixdata-cli d1 deploy-migrations --datasource <datasourceId> --approve  # Deploy pending migrations / 部署待执行的迁移
```

English:
- If a D1 datasource returns 401 Unauthorized, run `d1 oauth-login` to re-authenticate with Cloudflare
- `oauth-login` opens a browser for Cloudflare OAuth; after authorizing, the token is stored for subsequent requests
- `oauth-relogin` refreshes an existing session without a full re-auth

中文：
- 如果 D1 数据源返回 401 未授权，执行 `d1 oauth-login` 重新认证 Cloudflare
- `oauth-login` 会打开浏览器进行 Cloudflare OAuth 认证；授权后 token 会自动保存
- `oauth-relogin` 使用已有会话刷新，无需重新授权

### DynamoDB SSO — AWS SSO Authentication / AWS SSO 认证

```bash
futrixdata-cli dynamodb-sso list-profiles [--config-path <path>]     # List AWS SSO profiles / 列出 SSO 配置
futrixdata-cli dynamodb-sso login --profile <name> [--config-path <path>]  # SSO login (opens browser) / SSO 登录（打开浏览器）
futrixdata-cli dynamodb-sso authorize --profile <name> --region <region> [--config-path <path>]  # OAuth authorize / OAuth 授权
futrixdata-cli dynamodb-sso list-accounts --access-token <token> --region <region>  # List SSO accounts / 列出 SSO 账户
futrixdata-cli dynamodb-sso list-account-roles --access-token <token> --account-id <id> --region <region>  # List roles / 列出角色
futrixdata-cli dynamodb-sso get-role-credentials --access-token <token> --account-id <id> --role-name <name> --region <region>  # Get temp credentials / 获取临时凭证
```

English:
- DynamoDB datasources using AWS SSO require periodic re-authentication
- Run `dynamodb-sso login` when you get credential expiration errors

中文：
- 使用 AWS SSO 的 DynamoDB 数据源需要定期重新认证
- 遇到凭证过期错误时执行 `dynamodb-sso login`

### Skill — AI Agent Skill Management / AI 技能管理

```bash
futrixdata-cli skill status                            # Show agent detection and install status / 显示检测和安装状态
futrixdata-cli skill install [--agent claude,cursor]   # Install skill for agents / 为 agent 安装技能
futrixdata-cli skill uninstall [--agent claude,cursor] # Uninstall skill from agents / 卸载技能
```

### Tool — Agent Tool Interface / Agent 工具接口

```bash
futrixdata-cli tool list                    # List all available tools / 列出所有可用工具
futrixdata-cli tool call <tool> --stdin     # Call tool with JSON payload from stdin / 从 stdin 传入 JSON 调用工具
futrixdata-cli tool call <tool> --file payload.json --approve
```

English:
The `tool` interface provides a structured JSON-in/JSON-out surface designed for AI agent integration. Use `tool list` to discover available tools and their parameter schemas.

中文：
`tool` 接口提供结构化的 JSON 输入/输出，专为 AI agent 集成设计。使用 `tool list` 获取可用工具及其参数 schema。

### Data Classification — Local Agent Workflow / 数据分级 — 本地 Agent 工作流

English:
- Start with `get_sensitivity_config` to read current mode, saved rules, and level definitions
- If the user gives new classification rules, persist them with `set_sensitivity_custom_rules`
- Reuse schema tools such as `list_entities`, `describe_entity`, and `get_schema_knowledge` to inspect tables and fields
- Classify locally, then write the full result back with `save_sensitivity_report`
- Use `get_sensitivity_report` to verify the stored result
- Prefer schema-only classification; do not read live row data unless the user explicitly asks for it

中文：
- 先调用 `get_sensitivity_config` 读取当前模式、已保存规则和分级定义
- 如果用户给了新的分级规则，用 `set_sensitivity_custom_rules` 保存
- 复用 `list_entities`、`describe_entity`、`get_schema_knowledge` 这些 schema 工具查看表和字段
- 在本地完成分级后，用 `save_sensitivity_report` 整体回写结果
- 用 `get_sensitivity_report` 验证最终保存内容
- 默认只基于 schema 做分级，除非用户明确要求，否则不要读取真实行数据

## JSON output / JSON 输出

Add `--json` to any command for structured JSON output:
```bash
futrixdata-cli --json datasource list
futrixdata-cli --json console execute --datasource <id> --statement "..."
futrixdata-cli --json auth status
```

## Troubleshooting / 故障排查

English:
- **401 / auth error on any datasource**: Run `futrixdata-cli auth status` to check login, then `auth login` if needed
- **401 on D1 datasource**: Run `futrixdata-cli d1 oauth-login` to re-authenticate Cloudflare
- **Credential expired on DynamoDB**: Run `futrixdata-cli dynamodb-sso login --profile <name>` to refresh AWS SSO
- **Connection refused**: Check that the database host/port is reachable from your machine

中文：
- **任何数据源 401 / 认证错误**：先 `futrixdata-cli auth status` 查看登录状态，再 `auth login`
- **D1 数据源 401**：执行 `futrixdata-cli d1 oauth-login` 重新认证 Cloudflare
- **DynamoDB 凭证过期**：执行 `futrixdata-cli dynamodb-sso login --profile <name>` 刷新 AWS SSO
- **连接被拒**：检查数据库的 host/port 是否可达

## Safety / 安全机制

English:
- All write operations are gated behind `--approve` — never auto-approved
- The CLI inherits the authenticated session from the FutrixData desktop app
- Queries are executed against real datasources; always verify the datasource ID and database before running destructive statements

中文：
- 所有写操作必须显式 `--approve` — 不会自动批准
- CLI 复用 FutrixData 桌面应用的认证会话
- 查询直接在真实数据源上执行，执行危险语句前务必确认数据源 ID 和数据库
