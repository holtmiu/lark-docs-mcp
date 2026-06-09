# Lark Docs MCP

用于通过 MCP 客户端操作飞书 / Lark Docs 的 Remote MCP Server、stdio MCP Server 和本地 CLI。

这个项目可以把 ChatGPT、Claude Desktop、Cursor、Hermes 以及其他 MCP Host 接到飞书 / Lark 文档上，重点能力是文档身份解析、安全读取、受控写入、评论操作，以及以 HTTPS Remote MCP 服务形式部署。

## 当前产品能力

### MCP 连接方式

仓库包含：

- 远程 HTTP MCP 服务端：`cmd/feishu-doc-mcp-http-server`
- 本地 stdio MCP 服务端：`cmd/feishu-doc-mcp-server`
- 本地调试 CLI：`cmd/feishu-doc-cli`
- 飞书 / Lark API 适配层：`internal/feishu`
- MCP JSON-RPC transport：`internal/mcp`

HTTP 端点：

| 端点 | 方法 | 说明 |
| --- | --- | --- |
| `/healthz` | `GET` | 健康检查。 |
| `/mcp` | `POST` | JSON-RPC MCP 调用入口。 |
| `/mcp` | `OPTIONS` | CORS 预检。 |

HTTP 服务支持 Bearer token 保护、CORS allowlist、请求体大小限制、JSON-RPC batch 限制，并且远程未授权调用默认 fail-closed。

### 已支持的 MCP 工具

| 工具名 | 能力 |
| --- | --- |
| `feishu_oauth_auth_url` | 生成飞书 / Lark 用户 OAuth 授权 URL，不暴露 app secret 或 token。 |
| `feishu_doc_resolve` | 将飞书 / Lark URL 或 token 解析为标准化文档身份；不调用飞书 API。 |
| `feishu_doc_get_metadata` | 获取 docx 文档元信息。 |
| `feishu_doc_check_permission` | 在读、写、评论前预检文档能力。 |
| `feishu_doc_read` | 读取 docx blocks，并导出标准化 JSON / Markdown。 |
| `feishu_doc_create` | 创建 docx 文档，可选写入初始 Markdown 内容。 |
| `feishu_doc_append` | 向已有 docx 文档追加 Markdown 内容。 |
| `feishu_doc_list_comments` | 分页列出文档评论。 |
| `feishu_doc_create_comment` | 创建全文评论或支持的局部评论。 |
| `feishu_doc_reply_comment` | 在目标评论线程允许回复时追加回复。 |
| `feishu_doc_resolve_comment` | 解决或重新打开评论。 |

### 内置技能

仓库在 `./skills` 提供内置技能，MCP 客户端可以直接发现并运行可复用的文档工作流，无需自行设计 manifest 结构。

启用技能发现时，将服务端指向一个技能目录。配置的技能目录内，文件名为 `skill.yaml`、`skill.yml`、`*.yaml` 或 `*.yml` 的文件都会被视为 skill manifest；请将无关 YAML 文件放在这些目录之外。写入类技能加载开关默认是 `FEISHU_SKILLS_ENABLE_WRITE=false`；由于 `./skills` 包含写入类内置技能，只应在可信部署中加载完整内置集合：

```bash
export FEISHU_SKILLS_DIRS=./skills
export FEISHU_SKILLS_ENABLE_WRITE=true
```

配置技能 registry 后，MCP 服务会暴露：

| 工具名 | 能力 |
| --- | --- |
| `feishu_skill_list` | 列出已配置的 skill manifests。 |
| `feishu_skill_get` | 按名称返回一个 skill manifest。 |
| `feishu_skill_run` | 通过现有飞书 / Lark 工具层运行指定技能。 |

写入类技能默认不可用；只有在可信部署中显式设置 `FEISHU_SKILLS_ENABLE_WRITE=true` 后才会加载。写入类技能执行仍默认 dry-run；真实变更需要同时满足 `dryRun:false`、提供 `operationId`、服务端启用写入，并且 mutation 步骤前已经完成权限预检。

内置技能：

| 技能 | 模式 | 用途 |
| --- | --- | --- |
| `export-doc-markdown` | 只读 | 读取文档元信息，并将文档导出为 Markdown。 |
| `create-draft-doc` | 写入，默认 dry-run | 在目标文件夹中根据 Markdown 创建草稿 docx 文档。 |
| `add-review-comment` | 写入，默认 dry-run | 权限预检后向文档添加审阅评论。 |

发现与运行请求形态：

```json
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"feishu_skill_list","arguments":{}}}
```

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"feishu_skill_get","arguments":{"name":"export-doc-markdown"}}}
```

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"feishu_skill_run","arguments":{"name":"export-doc-markdown","inputs":{"input":"${FEISHU_DOC_INPUT}"}}}}
```

写入类技能 dry-run 请求形态：

```json
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"feishu_skill_run","arguments":{"name":"add-review-comment","inputs":{"input":"${FEISHU_DOC_INPUT}","content":"Review note from MCP skill"}}}}
```

真实写入请求需要设置顶层 `dryRun:false`，并在 `inputs` 中提供唯一 `operationId`；仅在确认目标文档或文件夹以及服务端写入策略后使用。

本阶段只验证 manifest 加载与单元级行为；真实飞书 / Lark 技能 E2E 验证单独跟踪。

### 文档类型与身份解析

当前已经支持：

- 接收 docx URL 和原始 docx token。
- 在飞书 / Lark API 暴露映射关系时，将 wiki / drive 风格输入 canonicalize 成真实 docx identity。
- 读取和写入飞书 / Lark 新版文档 docx。
- create / append 流程中的常见 Markdown block 转换。
- 可选返回 unsupported raw block，方便跟踪飞书 API 演进，同时不影响普通响应。

### 授权与安全模型

支持的凭证路径：

- App / tenant credential：通过 `FEISHU_APP_ID` 和 `FEISHU_APP_SECRET` 获取应用身份 token。
- 可选预置 `FEISHU_TENANT_ACCESS_TOKEN`：用于本地测试。
- 用户 OAuth 授权 URL 生成，以及加密 token store 的基础能力。

安全行为：

- 写入和评论类工具默认 dry-run。
- 真实写入需要显式传 `dryRun:false`，或设置 `FEISHU_DOC_WRITE_DRY_RUN_DEFAULT=false`。
- 远程部署可以禁用调用方传入的 `credentialId`，避免外部调用者任意选择存储凭证。
- MCP 工具不会返回 secret 或 token 值。
- 生产环境建议必须通过 HTTPS 暴露，并设置 `MCP_SERVER_API_KEY`。

### 已验证的真实端到端能力

已经完成真实 Feishu/Lark E2E 验证：

- Remote MCP health check 和 authenticated ping。
- 未授权 `/mcp` 请求返回 HTTP 401，默认关闭裸露访问。
- MCP `tools/list` 正常返回工具列表。
- 远程模式下拒绝 caller-supplied credential selection。
- 创建真实 docx 文档。
- 读取真实 docx 原始内容。
- 创建真实全文评论。
- 列出评论并验证目标评论存在。
- 解决评论并验证 `is_solved=true`。

脱敏验证日志见：`docs/phase8-e2e-validation-log.md`。

已观察到的限制：对某些全文评论，单独“追加回复”接口可能返回飞书 `1069302`，提示评论区不允许后续回复；但创建全文评论时附带初始 `reply_list` 已验证可成功写入和持久化。

## 适合做什么

- 给支持 MCP 的 AI 助手提供受控的飞书 / Lark Docs 访问能力。
- 将文档读取成 Markdown，用于总结、审阅、改写或结构化抽取。
- 从 AI 生成的 Markdown 创建草稿文档。
- 向已有文档追加章节、会议纪要、研究整理等内容。
- 在文档评论区创建、列出、解决评论，用于审阅工作流。
- 为个人或团队自托管一个内部 Remote MCP 文档连接器。

## 暂时还不做什么

- 它不是托管 SaaS，需要你自己部署。
- 还没有完整的多租户用户管理后台。
- 不会绕过飞书 / Lark 应用权限或文档共享规则；应用或用户 token 必须具备访问权限。

## 准备飞书 / Lark 应用

1. 在飞书开放平台或 Lark Developer Console 创建内部应用。
2. 给应用开通所需的文档、云空间、评论和 OAuth 权限。
3. 如果使用 app / tenant credential 模式，需要按需把目标文档或父文件夹共享给应用。
4. 获取 `FEISHU_APP_ID` 和 `FEISHU_APP_SECRET`。
5. 在确认权限前，保持 `FEISHU_DOC_WRITE_DRY_RUN_DEFAULT=true`。

## 本地构建与测试

```bash
go test ./...
go build ./cmd/feishu-doc-mcp-server
go build ./cmd/feishu-doc-mcp-http-server
go build ./cmd/feishu-doc-cli
```

如果环境使用项目本地 Go 工具链，可以先设置：

```bash
export PATH=/opt/data/tools/go/bin:$PATH
go test ./...
```

## 启动远程 HTTP MCP Server

```bash
export FEISHU_APP_ID="你的飞书 / Lark App ID"
export FEISHU_APP_SECRET="你的飞书 / Lark App Secret"
export MCP_SERVER_API_KEY="一个长随机字符串"
export MCP_HTTP_ADDR=":8080"
go run ./cmd/feishu-doc-mcp-http-server
```

部署时应放在 HTTPS 后面，然后在 MCP 客户端中配置：

```text
MCP URL: https://your-domain.example/mcp
Authorization: Bearer <MCP_SERVER_API_KEY>
```

## 本地 stdio MCP Server

```bash
go run ./cmd/feishu-doc-mcp-server
```

## 本地 CLI

```bash
go run ./cmd/feishu-doc-cli resolve "https://..."
go run ./cmd/feishu-doc-cli metadata "https://..."
go run ./cmd/feishu-doc-cli read "https://..."
go run ./cmd/feishu-doc-cli create "New title" "# Hello"
go run ./cmd/feishu-doc-cli append "https://..." "## Added from CLI"
```

## 示例：读取文档

```json
{
  "name": "feishu_doc_read",
  "arguments": {
    "input": "https://example.feishu.cn/docx/xxxx",
    "format": "both"
  }
}
```

## 示例：创建文档

```json
{
  "name": "feishu_doc_create",
  "arguments": {
    "title": "AI 生成的文档",
    "markdown": "# 标题\n\n这是通过 MCP 创建的飞书文档。",
    "dryRun": false
  }
}
```

## 示例：追加内容

```json
{
  "name": "feishu_doc_append",
  "arguments": {
    "input": "https://example.feishu.cn/docx/xxxx",
    "markdown": "## 追加内容\n\n这是一段通过 MCP 写入的内容。",
    "dryRun": false
  }
}
```

## 示例：创建评论

```json
{
  "name": "feishu_doc_create_comment",
  "arguments": {
    "input": "https://example.feishu.cn/docx/xxxx",
    "content": "这里需要补充依据。",
    "dryRun": false
  }
}
```

## 常用环境变量

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `MCP_HTTP_ADDR` | `:8080` | 远程 MCP HTTP server 监听地址。 |
| `MCP_SERVER_API_KEY` | 空 | `/mcp` 的 Bearer token；远程部署建议必须设置。 |
| `MCP_ALLOW_UNAUTHENTICATED` | `false` | 是否允许无鉴权调用 `/mcp`；仅建议本地开发使用。 |
| `MCP_ALLOWED_ORIGINS` | 空 | CORS origins，逗号分隔。 |
| `MCP_MAX_BODY_BYTES` | `16777216` | 最大 HTTP 请求体大小。 |
| `MCP_MAX_BATCH_REQUESTS` | `50` | 最大 JSON-RPC batch 数量。 |
| `FEISHU_PROVIDER` | `feishu` | `feishu` 或 `lark`。 |
| `FEISHU_BASE_URL` | provider 默认值 | 飞书 / Lark OpenAPI 基础地址。 |
| `FEISHU_APP_ID` | 空 | 飞书 / Lark 应用 ID。 |
| `FEISHU_APP_SECRET` | 空 | 飞书 / Lark 应用密钥。 |
| `FEISHU_TENANT_ACCESS_TOKEN` | 空 | 可选：本地测试用的预置 tenant access token。 |
| `FEISHU_DOC_WRITE_DRY_RUN_DEFAULT` | `true` | 写入/评论工具是否默认 dry-run。 |
| `FEISHU_DOC_MAX_BLOCKS` | `3000` | 单次读取文档的最大 blocks 数。 |
| `FEISHU_DOC_MAX_DEPTH` | `20` | 文档 block 递归读取最大深度。 |
| `FEISHU_OAUTH_REDIRECT_URI` | 空 | 用户 OAuth 授权 URL 使用的 redirect URI。 |
| `FEISHU_OAUTH_SCOPES` | 文档相关 scopes | 默认 OAuth scopes。 |
| `FEISHU_TOKEN_STORE_PATH` | `.data/feishu_tokens.json` | 本地 token store 路径。 |
| `FEISHU_TOKEN_ENCRYPT_KEY` | 空 | token store AES-GCM 加密 key。 |

完整 endpoint 配置见 `.env.example`。

## 安全建议

- 生产环境必须通过 HTTPS 暴露 `/mcp`。
- 远程部署必须设置 `MCP_SERVER_API_KEY`。
- 不要把 `FEISHU_APP_SECRET`、OAuth token store 或真实 access token 提交到仓库。
- 权限未验证前，保持写入工具 dry-run。
- 面向不可信远程客户端时，禁用调用方传入 credentialId。
- 飞书 / Lark scopes 按最小权限配置。
- 公开日志中应脱敏真实 document token、comment ID、app ID 和 user ID。

## 仓库

```bash
git clone git@github.com:holtmiu/lark-docs-mcp.git
```

## License

MIT
