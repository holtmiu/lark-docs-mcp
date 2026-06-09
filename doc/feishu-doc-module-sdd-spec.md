# 飞书文档模块 SDD / 需求 Spec

> Repository: `holtmiu/lark-docs-mcp`  
> Module: Feishu/Lark Document Connector  
> Status: Draft  
> Version: v0.1  
> Date: 2026-05-18

## 1. 背景与目标

本模块面向 ChatGPT MCP Connector 场景，提供对飞书 / Lark 文档体系的统一接入能力，使上层 Agent 可以安全、稳定地读取、检索、解析和在授权范围内写入飞书文档内容。

飞书文档体系通常涉及云文档、知识库 Wiki、Drive 文件、Docx 文档块结构、权限与协作能力。为了避免上层业务直接耦合飞书 OpenAPI 的 URL、Token、文档类型、权限模型和限流细节，本模块需要封装一个稳定的文档能力层。

核心目标：

1. 支持通过飞书文档 URL、Wiki URL、文件 Token 等输入定位文档。
2. 支持读取文档元信息、正文结构、块级内容和附件引用。
3. 支持将飞书文档结构归一化为 Markdown / JSON Block Tree，便于 LLM 消费。
4. 支持在授权范围内创建、更新或追加文档内容。
5. 支持权限校验、Token 管理、限流重试、审计日志与错误归一化。
6. 为后续 MCP Tool 暴露提供稳定接口。

## 2. 范围

### 2.1 In Scope

- 飞书 / Lark OAuth 或应用凭证管理。
- Tenant Access Token / User Access Token 的获取、缓存与刷新。
- 文档 URL 解析与资源定位。
- Drive 文件、Wiki 节点、Docx 文档的统一身份抽象。
- 文档元信息读取。
- 文档正文块读取与分页拉取。
- 文档块结构归一化。
- 文档内容导出为 Markdown。
- 创建文档、追加块、更新块、删除块等写操作的封装。
- 权限不足、文档不存在、Token 失效、限流等错误处理。
- 操作审计与可观测性。
- MCP Tool 层接口设计。

### 2.2 Out of Scope

- 飞书即时消息、日历、审批等非文档能力。
- 完整在线协同编辑器实现。
- 飞书文档前端渲染器。
- 企业级权限审批流。
- 非飞书平台的文档源适配，除非后续抽象为通用 Document Provider。

## 3. 术语

| 术语 | 说明 |
| --- | --- |
| Feishu / Lark | 飞书国内版与 Lark 国际版。API 域名和授权配置可能不同。 |
| Drive File | 飞书云空间中的文件抽象，可能对应文档、表格、多维表格等。 |
| Wiki Node | 飞书知识库节点，通常需要解析为实际文档或文件资源。 |
| Docx Document | 飞书新版文档，通常以块结构表达正文内容。 |
| Block | 文档中的结构单元，例如标题、段落、列表、表格、图片等。 |
| Token | 飞书资源标识，例如 document token、file token、wiki node token。 |
| MCP Tool | 暴露给 ChatGPT / Agent 调用的工具接口。 |

## 4. 用户故事

### US-001：读取飞书文档

作为 Agent，我希望用户提供一个飞书文档链接后，可以读取文档标题、正文和结构化内容，以便回答用户问题或生成总结。

验收标准：

- 能识别常见飞书 / Lark 文档 URL。
- 能解析出文档类型与资源 Token。
- 能读取文档标题、更新时间、owner 信息与正文块。
- 能返回 Markdown 和结构化 JSON 两种格式。

### US-002：读取 Wiki 文档

作为 Agent，我希望用户提供 Wiki 链接后，可以定位到 Wiki 节点对应的真实文档，并读取其内容。

验收标准：

- 能从 Wiki URL 解析 Wiki Node Token。
- 能查询 Wiki Node 绑定的实体信息。
- 能继续使用实体 Token 读取实际文档内容。

### US-003：写入或追加文档内容

作为 Agent，我希望在用户授权后，可以向指定飞书文档追加总结、会议纪要或生成内容。

验收标准：

- 支持追加到文档末尾。
- 支持按 Block ID 插入到指定位置。
- 写入操作具备幂等键或防重复策略。
- 写入结果返回变更位置、Block ID 和文档 URL。

### US-004：权限与错误提示

作为用户，我希望当文档无权限、链接错误或 Token 失效时，系统能给出可执行的错误提示。

验收标准：

- 权限不足时明确提示需要授权或共享文档。
- Token 失效时自动刷新；刷新失败时返回重新授权指引。
- 限流时自动退避重试；超过重试次数后返回标准错误码。

## 5. 功能需求

### FR-001：认证与授权

系统应支持以下认证模式：

1. Tenant App 模式：适合机器人或企业内部应用，以应用身份访问授权范围内资源。
2. User OAuth 模式：适合代表用户访问其有权限的文档。
3. 混合模式：优先使用 User Token，缺失时回退到 Tenant Token。

要求：

- Access Token 必须加密存储。
- Token 缓存需要带过期时间。
- Token 刷新需要并发安全，避免并发刷新风暴。
- Scope 配置必须最小化，按读写能力拆分。

### FR-002：URL 与 Token 解析

模块需要提供 `resolveDocumentIdentity(input)` 能力。

输入可以是：

- 飞书文档 URL。
- Lark 文档 URL。
- Wiki URL。
- Drive File Token。
- Docx Document ID / Token。

输出统一为：

```ts
interface DocumentIdentity {
  provider: 'feishu' | 'lark';
  resourceType: 'docx' | 'wiki' | 'drive_file' | 'unknown';
  token: string;
  originalUrl?: string;
  normalizedUrl?: string;
  tenantKey?: string;
}
```

### FR-003：文档元信息读取

模块需要读取并归一化以下信息：

```ts
interface DocumentMetadata {
  documentId: string;
  title: string;
  url?: string;
  ownerId?: string;
  createdTime?: string;
  updatedTime?: string;
  revisionId?: string;
  resourceType: string;
  permissions?: PermissionSnapshot;
}
```

### FR-004：正文块读取

模块需要支持分页读取文档块，并保留父子层级关系。

要求：

- 支持拉取根块。
- 支持递归拉取子块。
- 支持分页游标。
- 支持最大深度与最大块数量限制，防止超大文档拖垮调用。
- 支持跳过或降级处理暂不支持的块类型。

### FR-005：块结构归一化

所有飞书原始块需要转为统一 Block Tree。

```ts
interface NormalizedBlock {
  id: string;
  type: string;
  text?: string;
  attrs?: Record<string, unknown>;
  children?: NormalizedBlock[];
  source?: {
    provider: 'feishu' | 'lark';
    rawType: string;
    raw?: unknown;
  };
}
```

最小支持块类型：

- Heading 1-9。
- Paragraph。
- Bullet List。
- Ordered List。
- Todo List。
- Code Block。
- Quote。
- Divider。
- Table。
- Image / File 引用。
- Mention / Link。
- Unsupported Block 占位。

### FR-006：Markdown 导出

模块需要提供 `exportToMarkdown(document)`。

要求：

- 标题块转换为 Markdown Heading。
- 列表保留嵌套层级。
- 表格尽量转换为 Markdown Table；复杂表格降级为 HTML 或 JSON fenced block。
- 图片和附件转换为链接占位。
- 不支持块需输出可读占位，避免静默丢失内容。

### FR-007：文档写入

模块需要支持以下写操作：

- 创建空文档。
- 通过 Markdown 生成文档块。
- 追加内容到文档末尾。
- 插入内容到指定 Block 后。
- 更新指定 Block。
- 删除指定 Block。

写操作要求：

- 默认 dry-run，可预览即将写入的块。
- 每次写入需要 operation id。
- 支持幂等写入，避免 Agent 重试导致重复插入。
- 返回写入结果与变更摘要。

### FR-008：权限检查

模块需要提供权限检查能力：

```ts
interface PermissionSnapshot {
  canRead: boolean;
  canWrite: boolean;
  canComment?: boolean;
  visibility?: 'private' | 'tenant' | 'public' | 'unknown';
  reason?: string;
}
```

权限不足时不得尝试写操作。

### FR-009：搜索与列表能力

后续可选支持：

- 按关键字搜索用户可访问的文档。
- 列出指定 Wiki 空间下的文档。
- 列出最近访问或最近更新文档。

初版可作为 P1 / P2 能力，不阻塞文档读取和写入 MVP。

### FR-010：MCP Tool 暴露

建议暴露以下 MCP Tools：

| Tool | 说明 |
| --- | --- |
| `feishu_doc_resolve` | 解析 URL / Token 为统一文档身份。 |
| `feishu_doc_get_metadata` | 获取文档元信息。 |
| `feishu_doc_read` | 读取文档并返回 Markdown / JSON。 |
| `feishu_doc_append` | 向文档追加内容。 |
| `feishu_doc_update_block` | 更新指定块。 |
| `feishu_doc_create` | 创建新文档。 |
| `feishu_doc_check_permission` | 检查读写权限。 |

## 6. 非功能需求

### NFR-001：安全

- 不在日志中输出 access token、refresh token、app secret。
- 文档内容日志默认脱敏或关闭。
- 写操作必须显式区分 dry-run 与 execute。
- 最小权限 Scope 原则。
- 对外错误信息不能泄露内部凭证或堆栈。

### NFR-002：可靠性

- 所有 API 调用需要超时控制。
- 5xx 和限流错误需要指数退避重试。
- 读文档需要支持部分成功；单个 unsupported block 不应导致整体失败。
- 写操作需要幂等保护。

### NFR-003：性能

- 小型文档读取 P95 < 3s。
- 中型文档读取 P95 < 10s。
- 超大文档需要分页和截断策略。
- 对 metadata 和 block tree 可设置短 TTL 缓存。

### NFR-004：可观测性

每次调用记录：

- operation id。
- provider。
- resource type。
- API endpoint category。
- latency。
- retry count。
- error code。
- token source，但不记录 token 值。

### NFR-005：兼容性

- 支持飞书国内版与 Lark 国际版域名差异。
- 支持 API 版本演进，适配层不得向上泄露原始 API 结构。
- 不支持的块类型应保留原始摘要，避免信息丢失。

## 7. 系统设计

### 7.1 分层架构

```text
MCP Tool Layer
  -> Document Service
    -> Provider Adapter: Feishu / Lark
      -> API Client
        -> Auth Client
        -> HTTP Client
    -> Normalizer
    -> Markdown Exporter
    -> Permission Service
    -> Audit Logger
```

### 7.2 核心组件

#### AuthClient

职责：

- 获取 tenant access token。
- 获取 / 刷新 user access token。
- 管理 token 缓存。
- 对调用方隐藏飞书授权细节。

#### DocumentResolver

职责：

- 解析 URL。
- 判断资源类型。
- 将 Wiki Node 解析为实际文档实体。
- 返回 `DocumentIdentity`。

#### FeishuDocumentClient

职责：

- 调用飞书 / Lark 原始 API。
- 处理分页、重试、限流。
- 将原始错误映射为标准错误。

#### DocumentService

职责：

- 对上提供业务级读写接口。
- 编排 resolver、client、normalizer、exporter。
- 管理读取深度、格式、权限策略。

#### BlockNormalizer

职责：

- 将飞书原始块转为 `NormalizedBlock`。
- 保留 unsupported block 的原始类型和关键字段。
- 支持从 NormalizedBlock 反向生成写入请求。

#### MarkdownExporter

职责：

- 将 NormalizedBlock Tree 转为 Markdown。
- 处理列表、表格、图片、附件、代码块等格式。

#### AuditLogger

职责：

- 记录读写操作摘要。
- 不记录敏感凭证。
- 为问题排查提供 operation id。

## 8. 核心流程

### 8.1 读取流程

```text
User Input URL / Token
  -> DocumentResolver.resolve()
  -> PermissionService.checkRead()
  -> DocumentClient.getMetadata()
  -> DocumentClient.listBlocksPaginated()
  -> BlockNormalizer.normalizeTree()
  -> MarkdownExporter.export() optional
  -> Return DocumentReadResult
```

返回结构：

```ts
interface DocumentReadResult {
  metadata: DocumentMetadata;
  blocks: NormalizedBlock[];
  markdown?: string;
  warnings?: string[];
  truncated?: boolean;
}
```

### 8.2 写入流程

```text
Write Request
  -> DocumentResolver.resolve()
  -> PermissionService.checkWrite()
  -> MarkdownParser / BlockBuilder
  -> Dry Run Preview optional
  -> Idempotency Check
  -> DocumentClient.batchWrite()
  -> AuditLogger.record()
  -> Return DocumentWriteResult
```

返回结构：

```ts
interface DocumentWriteResult {
  operationId: string;
  documentId: string;
  changedBlocks: string[];
  url?: string;
  dryRun: boolean;
  warnings?: string[];
}
```

## 9. 错误模型

统一错误码：

| Code | 场景 | 用户提示 |
| --- | --- | --- |
| `AUTH_REQUIRED` | 未授权或 token 缺失 | 请先完成飞书授权。 |
| `AUTH_EXPIRED` | token 过期且刷新失败 | 授权已失效，请重新授权。 |
| `PERMISSION_DENIED` | 无读写权限 | 请确认文档已共享给当前应用或用户。 |
| `DOCUMENT_NOT_FOUND` | 文档不存在或 token 错误 | 请检查链接是否正确。 |
| `UNSUPPORTED_DOCUMENT_TYPE` | 暂不支持的资源类型 | 当前仅支持文档 / Wiki 文档。 |
| `RATE_LIMITED` | API 限流 | 请求过于频繁，请稍后重试。 |
| `PARTIAL_CONTENT` | 部分块读取失败 | 文档部分内容未能读取，结果可能不完整。 |
| `WRITE_CONFLICT` | 写入版本冲突 | 文档已被修改，请重新读取后再写入。 |

## 10. 数据模型

### 10.1 CredentialBinding

```ts
interface CredentialBinding {
  id: string;
  provider: 'feishu' | 'lark';
  authType: 'tenant' | 'user';
  tenantKey?: string;
  userId?: string;
  encryptedAccessToken: string;
  encryptedRefreshToken?: string;
  expiresAt: string;
  scopes: string[];
  createdAt: string;
  updatedAt: string;
}
```

### 10.2 SyncRecord

```ts
interface SyncRecord {
  id: string;
  documentId: string;
  provider: 'feishu' | 'lark';
  lastRevisionId?: string;
  lastSyncedAt: string;
  status: 'success' | 'partial' | 'failed';
  errorCode?: string;
}
```

### 10.3 OperationLog

```ts
interface OperationLog {
  operationId: string;
  action: 'read' | 'create' | 'append' | 'update' | 'delete';
  documentId?: string;
  provider: 'feishu' | 'lark';
  actorType: 'tenant_app' | 'user';
  status: 'success' | 'failed';
  latencyMs: number;
  retryCount: number;
  errorCode?: string;
  createdAt: string;
}
```

## 11. 配置项

```env
FEISHU_APP_ID=
FEISHU_APP_SECRET=
FEISHU_BASE_URL=https://open.feishu.cn
LARK_BASE_URL=https://open.larksuite.com
FEISHU_TOKEN_CACHE_TTL_SECONDS=5400
FEISHU_API_TIMEOUT_MS=15000
FEISHU_API_MAX_RETRIES=3
FEISHU_DOC_MAX_BLOCKS=3000
FEISHU_DOC_MAX_DEPTH=20
FEISHU_DOC_WRITE_DRY_RUN_DEFAULT=true
```

## 12. 测试计划

### 12.1 单元测试

- URL 解析测试。
- Token 类型识别测试。
- 原始 Block 到 NormalizedBlock 转换测试。
- Markdown 导出测试。
- 错误码映射测试。
- 幂等键生成测试。

### 12.2 集成测试

- 使用测试租户读取公开测试文档。
- 读取 Wiki 节点并解析真实文档。
- 分页读取大型文档。
- 追加内容到测试文档。
- 权限不足场景验证。
- Token 过期刷新验证。

### 12.3 回归测试样例

建议准备以下 fixture：

1. 简单段落文档。
2. 多级标题文档。
3. 嵌套列表文档。
4. 表格文档。
5. 图片和附件文档。
6. Wiki 文档。
7. 包含 unsupported block 的文档。
8. 超大文档。

## 13. MVP 划分

### MVP 0：只读能力

- AuthClient。
- URL Resolver。
- Metadata 读取。
- Block 分页读取。
- Markdown 导出。
- MCP Tool: `feishu_doc_read`。

### MVP 1：Wiki 与权限

- Wiki Node 解析。
- Permission Snapshot。
- 更完整错误模型。
- 读缓存与审计日志。

### MVP 2：写入能力

- Create Document。
- Append Blocks。
- Update Block。
- Dry-run。
- Idempotency。

### MVP 3：搜索与同步

- 文档搜索。
- Wiki 空间遍历。
- 增量同步。
- Webhook 事件接入。

## 14. 风险与缓解

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| 飞书 API 版本变化 | 读写失败或字段不兼容 | 通过 Provider Adapter 隔离 API 细节。 |
| 文档块类型复杂 | Markdown 导出丢失信息 | Unsupported Block 保留原始摘要与 warning。 |
| 权限模型复杂 | 用户体验差 | 提供 permission check 与明确错误提示。 |
| 限流 | 大文档读取失败 | 分页、缓存、指数退避、截断策略。 |
| Agent 重试导致重复写入 | 文档内容重复 | 幂等 operation id 与写入记录表。 |
| 凭证泄露 | 安全事故 | 加密存储、日志脱敏、最小 Scope。 |

## 15. 开放问题

1. 首期目标是仅支持飞书国内版，还是同时支持 Lark 国际版？
2. 首期认证模式优先 Tenant App，还是 User OAuth？
3. 写操作是否需要用户二次确认，还是 MCP Tool 层通过权限控制即可？
4. 是否需要支持评论、协作者、分享权限管理？
5. Markdown 到飞书 Block 的转换是否作为首期能力？
6. 文档内容是否允许缓存？如允许，缓存 TTL 和数据存放位置如何定义？
7. 是否需要支持私有化部署或多租户隔离？

## 16. 初始接口草案

```ts
export interface FeishuDocConnector {
  resolve(input: string): Promise<DocumentIdentity>;
  getMetadata(identity: DocumentIdentity): Promise<DocumentMetadata>;
  readDocument(identity: DocumentIdentity, options?: ReadOptions): Promise<DocumentReadResult>;
  createDocument(request: CreateDocumentRequest): Promise<DocumentWriteResult>;
  appendDocument(identity: DocumentIdentity, request: AppendRequest): Promise<DocumentWriteResult>;
  updateBlock(identity: DocumentIdentity, request: UpdateBlockRequest): Promise<DocumentWriteResult>;
  checkPermission(identity: DocumentIdentity): Promise<PermissionSnapshot>;
}

export interface ReadOptions {
  format?: 'json' | 'markdown' | 'both';
  maxBlocks?: number;
  maxDepth?: number;
  includeUnsupportedRaw?: boolean;
}

export interface AppendRequest {
  markdown?: string;
  blocks?: NormalizedBlock[];
  afterBlockId?: string;
  dryRun?: boolean;
  operationId?: string;
}
```

## 17. 结论

飞书文档模块应以“统一文档身份解析 + 文档块归一化 + 安全读写封装”为核心，而不是让 MCP Tool 直接依赖飞书 OpenAPI。这样可以在后续扩展 Wiki、搜索、同步、写入、权限管理和多 Provider 能力时保持稳定的上层接口。
