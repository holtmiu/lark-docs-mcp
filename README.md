# Lark Docs MCP

Remote MCP server, stdio MCP server, and CLI for operating Feishu/Lark Docs from MCP-compatible clients.

This project connects AI clients such as ChatGPT, Claude Desktop, Cursor, Hermes, and other MCP hosts to Feishu/Lark documents. It focuses on document identity resolution, safe reads, controlled writes, comments, and deployment as an HTTPS Remote MCP endpoint.

## Current capabilities

### MCP transports

- Remote HTTP MCP server: `cmd/feishu-doc-mcp-http-server`
- Local stdio MCP server: `cmd/feishu-doc-mcp-server`
- Local CLI for debugging and scripts: `cmd/feishu-doc-cli`

HTTP endpoints:

- `GET /healthz`
- `POST /mcp`
- `OPTIONS /mcp`

The HTTP server supports Bearer-token protection, CORS allowlists, request body limits, JSON-RPC batch limits, and fail-closed defaults for unauthenticated remote calls.

### Feishu/Lark document operations

The server currently exposes these MCP tools:

| Tool | What it does |
| --- | --- |
| `feishu_oauth_auth_url` | Builds a Feishu/Lark user OAuth authorization URL without exposing app secrets or tokens. |
| `feishu_doc_resolve` | Resolves a Feishu/Lark URL or token into normalized document identity. Does not call Feishu APIs. |
| `feishu_doc_get_metadata` | Reads docx metadata. |
| `feishu_doc_check_permission` | Preflights document capability before read/write/comment actions. |
| `feishu_doc_read` | Reads docx blocks and exports normalized JSON and/or Markdown. |
| `feishu_doc_create` | Creates a docx document and optionally writes initial Markdown content. |
| `feishu_doc_append` | Appends Markdown content to an existing docx document. |
| `feishu_doc_list_comments` | Lists document comments with pagination. |
| `feishu_doc_create_comment` | Creates a whole-document or supported local comment. |
| `feishu_doc_reply_comment` | Replies to a comment when the target comment thread allows replies. |
| `feishu_doc_resolve_comment` | Resolves or reopens a comment. |

### Built-in skills

The repository includes built-in skills in `./skills` so MCP clients can discover and run reusable document workflows without inventing the manifest structure.

Enable skill discovery by pointing the server at a skill directory. Files named `skill.yaml`, `skill.yml`, `*.yaml`, or `*.yml` inside configured skill directories are treated as skill manifests; keep unrelated YAML files outside those directories. The write-skill load gate defaults to `FEISHU_SKILLS_ENABLE_WRITE=false`; because `./skills` includes write-capable built-in skills, load the full built-in set only in a trusted deployment:

```bash
export FEISHU_SKILLS_DIRS=./skills
export FEISHU_SKILLS_ENABLE_WRITE=true
```

When a skill registry is configured, the MCP server exposes:

| Tool | What it does |
| --- | --- |
| `feishu_skill_list` | Lists configured skill manifests. |
| `feishu_skill_get` | Returns one skill manifest by name. |
| `feishu_skill_run` | Runs a configured skill through the existing Feishu/Lark tool layer. |

Write-capable skills stay disabled unless `FEISHU_SKILLS_ENABLE_WRITE=true` is set for a trusted deployment. Write skill execution remains dry-run by default; real mutations require `dryRun:false`, an `operationId`, write enablement in server configuration, and a permission preflight before the mutation step.

Built-in skills:

| Skill | Mode | Intent |
| --- | --- | --- |
| `export-doc-markdown` | Read-only | Reads document metadata and exports the document as Markdown. |
| `create-draft-doc` | Write, dry-run by default | Creates a draft docx document from Markdown in a target folder. |
| `add-review-comment` | Write, dry-run by default | Adds a review comment to a document after permission preflight. |

Discovery and run request shapes:

```json
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"feishu_skill_list","arguments":{}}}
```

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"feishu_skill_get","arguments":{"name":"export-doc-markdown"}}}
```

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"feishu_skill_run","arguments":{"name":"export-doc-markdown","inputs":{"input":"${FEISHU_DOC_INPUT}"}}}}
```

Write-capable skill dry-run request shape:

```json
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"feishu_skill_run","arguments":{"name":"add-review-comment","inputs":{"input":"${FEISHU_DOC_INPUT}","content":"Review note from MCP skill"}}}}
```

Real write requests set top-level `dryRun:false` and include a unique `operationId` inside `inputs`; use only after confirming the target document or folder and server write policy.

This phase validates manifest loading and unit-level behavior only; real Feishu/Lark skill E2E validation is tracked separately.

### Identity and document coverage

- Accepts docx URLs and raw docx tokens.
- Canonicalizes wiki/drive-style inputs into real docx identities where Feishu/Lark APIs expose the relationship.
- Reads and writes Feishu/Lark docx documents.
- Handles common Markdown block conversion for create/append flows.
- Keeps raw unsupported block data optional so callers can inspect API evolution without breaking normal responses.

### Auth and safety model

Supported credential paths:

- App/tenant credential path using `FEISHU_APP_ID` and `FEISHU_APP_SECRET`.
- Optional pre-provisioned `FEISHU_TENANT_ACCESS_TOKEN` for local testing.
- User OAuth URL generation and token-store plumbing for user-granted credentials.

Safety behavior:

- Write tools are dry-run by default.
- Real writes require `dryRun:false` or `FEISHU_DOC_WRITE_DRY_RUN_DEFAULT=false`.
- Remote deployments can disable caller-supplied `credentialId` so external callers cannot select arbitrary stored credentials.
- Secrets and token values are not returned by MCP tools.
- Production deployments should run behind HTTPS and require `MCP_SERVER_API_KEY`.

### Validated real end-to-end behavior

A real Feishu/Lark E2E run has validated:

- Remote MCP health and authenticated ping.
- Unauthenticated `/mcp` requests fail closed with HTTP 401.
- Tool listing over MCP.
- Rejection of caller-supplied credential selection when disabled.
- Real docx creation.
- Real docx raw content read.
- Real whole-document comment creation.
- Real comment listing.
- Real comment resolution and verification of `is_solved=true`.

See `docs/phase8-e2e-validation-log.md` for the redacted validation log.

Known observed limitation: the separate add-reply endpoint may return Feishu code `1069302` when the target comment section does not allow additional replies. Creating a whole-document comment with an initial reply list has been validated.

## What it is good for

- Giving an MCP-capable AI assistant controlled access to Feishu/Lark Docs.
- Reading a Feishu/Lark document into Markdown for summarization, review, or transformation.
- Creating draft documents from generated Markdown.
- Appending generated sections to an existing document.
- Listing, creating, and resolving document comments for review workflows.
- Deploying a self-hosted Remote MCP endpoint for internal teams.

## What it does not do yet

- It is not a hosted SaaS; you deploy and operate it yourself.
- It does not yet provide a full multi-tenant user admin console.
- It does not bypass Feishu/Lark app scopes or document sharing rules. The app or user token must have access.

## Build and test

```bash
go test ./...
go build ./cmd/feishu-doc-mcp-server
go build ./cmd/feishu-doc-mcp-http-server
go build ./cmd/feishu-doc-cli
```

If your environment uses a project-local Go toolchain, put it on `PATH` first, for example:

```bash
export PATH=/opt/data/tools/go/bin:$PATH
go test ./...
```

## Run remote HTTP MCP server

```bash
export FEISHU_APP_ID="your Feishu/Lark app id"
export FEISHU_APP_SECRET="your Feishu/Lark app secret"
export MCP_SERVER_API_KEY="a-long-random-string"
export MCP_HTTP_ADDR=":8080"
go run ./cmd/feishu-doc-mcp-http-server
```

Deploy this server behind HTTPS and configure your MCP client with the public `/mcp` URL:

```text
MCP URL: https://your-domain.example/mcp
Authorization: Bearer <MCP_SERVER_API_KEY>
```

## Run local stdio MCP server

```bash
go run ./cmd/feishu-doc-mcp-server
```

## Run CLI

```bash
go run ./cmd/feishu-doc-cli resolve "https://..."
go run ./cmd/feishu-doc-cli metadata "https://..."
go run ./cmd/feishu-doc-cli read "https://..."
go run ./cmd/feishu-doc-cli create "New title" "# Hello"
go run ./cmd/feishu-doc-cli append "https://..." "## Added from CLI"
```

## Prepare Feishu/Lark permissions

1. Create an internal app in Feishu Open Platform or Lark Developer Console.
2. Grant the required document, drive, comment, and OAuth scopes for the operations you need.
3. If using the app/tenant credential path, share the target document or parent folder with the app when required.
4. Set `FEISHU_APP_ID` and `FEISHU_APP_SECRET` in the server environment.
5. Keep `FEISHU_DOC_WRITE_DRY_RUN_DEFAULT=true` until you have verified scopes and document sharing.

## Common environment variables

| Environment variable | Default | Description |
| --- | --- | --- |
| `MCP_HTTP_ADDR` | `:8080` | Remote MCP HTTP listen address. |
| `MCP_SERVER_API_KEY` | empty | Bearer token for `/mcp`. Recommended for any remote deployment. |
| `MCP_ALLOW_UNAUTHENTICATED` | `false` | Allow unauthenticated `/mcp`; only use for local development. |
| `MCP_ALLOWED_ORIGINS` | empty | Comma-separated CORS origins. |
| `MCP_MAX_BODY_BYTES` | `16777216` | Max HTTP request body size. |
| `MCP_MAX_BATCH_REQUESTS` | `50` | Max JSON-RPC batch size. |
| `FEISHU_PROVIDER` | `feishu` | `feishu` or `lark`. |
| `FEISHU_BASE_URL` | provider default | Feishu/Lark OpenAPI base URL. |
| `FEISHU_APP_ID` | empty | Feishu/Lark app ID. |
| `FEISHU_APP_SECRET` | empty | Feishu/Lark app secret. |
| `FEISHU_TENANT_ACCESS_TOKEN` | empty | Optional pre-provisioned tenant token for local testing. |
| `FEISHU_DOC_WRITE_DRY_RUN_DEFAULT` | `true` | Whether write/comment tools default to dry-run. |
| `FEISHU_DOC_MAX_BLOCKS` | `3000` | Maximum blocks read per document. |
| `FEISHU_DOC_MAX_DEPTH` | `20` | Maximum recursive block depth. |
| `FEISHU_OAUTH_REDIRECT_URI` | empty | OAuth redirect URI for user authorization URL generation. |
| `FEISHU_OAUTH_SCOPES` | doc scopes | Default OAuth scopes. |
| `FEISHU_TOKEN_STORE_PATH` | `.data/feishu_tokens.json` | Local token store path. |
| `FEISHU_TOKEN_ENCRYPT_KEY` | empty | AES-GCM key for token store encryption. |

See `.env.example` for the full configurable endpoint list.

## Security recommendations

- Always expose `/mcp` through HTTPS in production.
- Set `MCP_SERVER_API_KEY` for remote deployments.
- Keep Feishu/Lark app secrets and OAuth token stores out of Git.
- Keep write tools in dry-run mode until the deployment is verified.
- Disable caller-supplied credential selection for untrusted remote clients.
- Prefer least-privilege Feishu/Lark scopes.
- Redact real document tokens, comment IDs, app IDs, and user IDs in public logs.

## Repository

```bash
git clone git@github.com:holtmiu/lark-docs-mcp.git
```

## License

MIT
