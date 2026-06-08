# Feishu User OAuth, Permission, and Comment Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Upgrade the Feishu/Lark MCP connector from tenant-token docx read/create/append MVP to a user-authorized document collaboration connector with OAuth, permission checks, comment operations, and explicit verification artifacts for each phase.

**Architecture:** Keep `internal/mcp` as the tool boundary and `internal/feishu` as the Feishu/Lark capability layer. Introduce a credential/token abstraction so API calls can use either tenant tokens or user OAuth tokens, then layer permission and comment services on top of the existing resolver/client/service structure. Keep write/comment execution safe by default: dry-run or explicit authorization, no token logging, no document-content logging by default.

**Tech Stack:** Go 1.23, standard library HTTP/JSON, existing MCP JSON-RPC server, Feishu/Lark OpenAPI, table-driven Go tests with `httptest`.

---

## Current Baseline

Repository: `/opt/data/workspace/github_repos/ChatGPT_MCP_Connectors`

Observed baseline before this plan:

- `go test ./...` passes.
- `go vet ./...` passes.
- `go build ./cmd/...` passes.
- Existing tools: `feishu_doc_resolve`, `feishu_doc_get_metadata`, `feishu_doc_read`, `feishu_doc_create`, `feishu_doc_append`.
- Existing auth: tenant/app token only, implemented in `internal/feishu/http_client.go`.
- Existing permission model: `PermissionSnapshot` exists, but values are effectively placeholder.
- Existing comments: no comment list/create/reply/resolve APIs or MCP tools.
- Existing dirty worktree risk: many files currently show mode-only changes `100644 => 100755`; fix or isolate this before implementation commits.

---

## Phase Verification Matrix

| Phase | Capability | Verification Standard | Verification Artifacts |
| --- | --- | --- | --- |
| 0 | Baseline hygiene and safety guardrails | Working tree content changes are intentional; mode-only noise is removed or documented; existing tests/build pass | `git diff --summary`, `git status --short`, `gofmt -l`, `go test ./...`, `go vet ./...`, `go build ./cmd/...` output saved in PR/commit notes |
| 1 | User OAuth config and auth URL generation | OAuth config loads from env; auth URL contains app id, redirect URI, state, scopes, provider-specific base URL; invalid config returns structured error | Unit tests for config loading and auth URL; sample `.env.example` diff; local CLI/tool call output with redacted URL fields |
| 2 | OAuth code exchange, refresh, token store | User token can be exchanged and refreshed via mocked Feishu endpoints; tokens are stored encrypted/opaque-at-rest interface; expired token refresh is concurrency-safe | `httptest` tests for exchange/refresh/error paths; race-sensitive refresh test; token store fixture with no plaintext secrets in logs |
| 3 | Token source integration | Read/metadata calls can use user token when actor/credential is provided, otherwise tenant fallback remains compatible | HTTP client tests asserting the Authorization header uses the selected user credential; regression tests for tenant token fallback; MCP request tests with credential id |
| 4 | Permission check | `feishu_doc_check_permission` returns `canRead/canWrite/canComment/visibility/reason/suggestedAction`; write/comment operations block on insufficient permission | Unit tests for permission mapping; MCP schema tests; negative tests proving write/comment not attempted when denied |
| 5 | Comment operations | List/create/reply/resolve comment tools work through service layer; dry-run or confirmation rules are explicit for mutating comment operations | `httptest` integration tests for comment API paths/body; MCP tool tests; dry-run output snapshot for create/reply/resolve |
| 6 | Wiki/Drive canonicalization | Wiki and Drive links resolve to real docx identity before read/permission/comment; unsupported resource returns actionable error | Resolver + mocked API tests; end-to-end service test from wiki URL to docx token; error fixture for unsupported type |
| 7 | Remote MCP security hardening | Production HTTP MCP refuses empty API key; CORS origin is allowlisted; request actor/credential is auditable without leaking secrets | HTTP server tests for empty key policy, CORS allowlist, auth failure; audit log tests with redacted token/content fields |
| 8 | Real end-to-end validation | With a test Feishu app/user, user authorizes, connector reads a private doc, checks permission, creates/replies/resolves a comment, and refuses unauthorized docs | Manual validation log with doc URL/token redacted; screenshots or copied JSON outputs; list of app scopes used; rollback notes |

---

## Phase 0: Baseline Hygiene and Commit Safety

### Task 0.1: Remove mode-only permission noise before feature work

**Objective:** Ensure implementation commits contain semantic changes, not accidental chmod noise.

**Files:**
- No content changes expected.

**Steps:**

1. Inspect mode-only changes:

```bash
git diff --summary
```

Expected: shows mode changes only for files unintentionally marked executable.

2. Reset normal file permissions for tracked non-script files:

```bash
git diff --name-only | xargs chmod 644
chmod 755 scripts/*.sh 2>/dev/null || true
```

3. Verify status:

```bash
git diff --summary
git status --short
```

Expected: no mode-only changes remain. If content changes remain, inspect each before continuing.

**Verification artifacts:**
- Paste `git diff --summary` before and after into the PR/implementation log.
- Paste `git status --short` after cleanup.

### Task 0.2: Capture baseline checks

**Objective:** Confirm the existing codebase is green before adding OAuth/permission/comment work.

**Commands:**

```bash
gofmt -l $(find . -name '*.go')
go test ./...
go vet ./...
go build ./cmd/...
```

Expected:
- `gofmt -l` prints nothing.
- Test/vet/build pass.

**Verification artifacts:**
- Command output saved in implementation notes.

---

## Phase 1: User OAuth Configuration and Authorization URL

### Task 1.1: Add OAuth config fields

**Objective:** Load the minimum OAuth settings without changing runtime behavior.

**Files:**
- Modify: `internal/config/config.go`
- Modify: `.env.example`
- Test: create `internal/config/config_test.go` if absent

**Fields to add to `config.Config`:**

```go
OAuthRedirectURI string
OAuthScopes      []string
OAuthStateSecret string
OAuthAuthPath    string
OAuthTokenPath   string
OAuthRefreshPath string
TokenStorePath   string
TokenEncryptKey  string
```

**Env defaults:**

```env
FEISHU_OAUTH_REDIRECT_URI=
FEISHU_OAUTH_SCOPES=offline_access,docs:doc:readonly,docs:doc:write,drive:drive:readonly
FEISHU_OAUTH_STATE_SECRET=
FEISHU_OAUTH_AUTH_PATH=/open-apis/authen/v1/authorize
FEISHU_OAUTH_TOKEN_PATH=/open-apis/authen/v2/oauth/token
FEISHU_OAUTH_REFRESH_PATH=/open-apis/authen/v2/oauth/token
FEISHU_TOKEN_STORE_PATH=.data/feishu_tokens.json
FEISHU_TOKEN_ENCRYPT_KEY=
```

**TDD steps:**

1. Write failing tests in `internal/config/config_test.go`:
   - `TestLoadOAuthConfigDefaults`
   - `TestLoadOAuthScopesTrimsAndDropsEmptyValues`
   - `TestLoadOAuthConfigOverrides`
2. Run:

```bash
go test ./internal/config -run 'TestLoadOAuth' -v
```

Expected: FAIL because fields do not exist.

3. Add fields and env loading.
4. Re-run the same command.

Expected: PASS.

**Verification artifacts:**
- Test failure output from RED.
- Test pass output from GREEN.
- `.env.example` diff showing scopes and token storage settings.

### Task 1.2: Add OAuth auth URL builder

**Objective:** Generate a safe authorization URL that a user can open to grant document permissions.

**Files:**
- Create: `internal/feishu/oauth.go`
- Create: `internal/feishu/oauth_test.go`
- Modify later only if needed: `internal/feishu/types.go`

**Proposed types:**

```go
type OAuthAuthURLRequest struct {
    RedirectURI string
    State       string
    Scopes      []string
}

type OAuthAuthURLResult struct {
    URL         string   `json:"url"`
    Provider    Provider `json:"provider"`
    Scopes      []string `json:"scopes"`
    RedirectURI string   `json:"redirectUri"`
}
```

**TDD steps:**

1. Write tests:
   - `TestBuildOAuthAuthURLIncludesRequiredParams`
   - `TestBuildOAuthAuthURLUsesProviderBaseURL`
   - `TestBuildOAuthAuthURLRejectsMissingRedirectURI`
2. Run:

```bash
go test ./internal/feishu -run 'TestBuildOAuthAuthURL' -v
```

Expected: FAIL because builder does not exist.

3. Implement `BuildOAuthAuthURL(baseURL, appID string, req OAuthAuthURLRequest) (OAuthAuthURLResult, error)`.
4. Re-run tests.

Expected: PASS.

**Verification artifacts:**
- Red/green test outputs.
- Example auth URL in notes with `client_id` and `state` redacted.

### Task 1.3: Expose `feishu_oauth_auth_url` MCP tool

**Objective:** Let an Agent initiate user authorization through MCP.

**Files:**
- Modify: `internal/mcp/feishu_tools.go`
- Test: create or extend `internal/mcp/feishu_tools_test.go`

**Schema:**

```json
{
  "type": "object",
  "properties": {
    "state": {"type": "string", "maxLength": 256},
    "redirectUri": {"type": "string", "maxLength": 2048},
    "scopes": {"type": "array", "items": {"type": "string"}, "maxItems": 20}
  },
  "additionalProperties": false
}
```

**TDD steps:**

1. Write MCP tool-list test asserting `feishu_oauth_auth_url` exists.
2. Write call test asserting result contains `url`, `provider`, `scopes`, `redirectUri`.
3. Run:

```bash
go test ./internal/mcp -run 'OAuthAuthURL|Tools' -v
```

Expected: FAIL.

4. Add tool registration and call handler.
5. Re-run tests.

Expected: PASS.

**Verification artifacts:**
- MCP tool schema JSON from test or manual call.
- Red/green test outputs.

---

## Phase 2: OAuth Code Exchange, Refresh, and Token Store

### Task 2.1: Define credential and token source models

**Objective:** Represent tenant and user credentials explicitly.

**Files:**
- Modify: `internal/feishu/types.go`
- Create: `internal/feishu/auth_types.go`
- Test: `internal/feishu/auth_types_test.go`

**Types:**

```go
type AuthType string

const (
    AuthTypeTenant AuthType = "tenant"
    AuthTypeUser   AuthType = "user"
)

type CredentialBinding struct {
    ID           string   `json:"id"`
    Provider     Provider `json:"provider"`
    AuthType     AuthType `json:"authType"`
    TenantKey    string   `json:"tenantKey,omitempty"`
    UserID       string   `json:"userId,omitempty"`
    OpenID       string   `json:"openId,omitempty"`
    AccessToken  string   `json:"-"`
    RefreshToken string   `json:"-"`
    ExpiresAt    time.Time `json:"expiresAt"`
    Scopes       []string `json:"scopes"`
}

type ActorContext struct {
    CredentialID string `json:"credentialId,omitempty"`
    UserID       string `json:"userId,omitempty"`
    OpenID       string `json:"openId,omitempty"`
    AuthType     AuthType `json:"authType,omitempty"`
}
```

**TDD steps:**

1. Test JSON does not serialize tokens.
2. Test expired-token helper behavior if implemented.
3. Run:

```bash
go test ./internal/feishu -run 'Credential|Actor' -v
```

Expected: FAIL until types/helpers exist.

**Verification artifacts:**
- Test output proving tokens are not serialized.

### Task 2.2: Implement encrypted/opaque token store interface

**Objective:** Add a local token store abstraction while keeping external dependencies minimal.

**Files:**
- Create: `internal/feishu/token_store.go`
- Create: `internal/feishu/token_store_test.go`

**Interface:**

```go
type TokenStore interface {
    Save(ctx context.Context, binding CredentialBinding) error
    Get(ctx context.Context, id string) (CredentialBinding, error)
    Delete(ctx context.Context, id string) error
}
```

**Implementation guidance:**
- Start with file-backed JSON store under `TokenStorePath`.
- Use AES-GCM if `FEISHU_TOKEN_ENCRYPT_KEY` is configured.
- If no encryption key is configured, allow only test/dev mode and return a warning in health/config validation; do not silently claim encrypted storage.
- Never log token values.

**TDD steps:**

1. Write tests:
   - save/get round trip
   - missing id returns `AUTH_REQUIRED` or `DOCUMENT_NOT_FOUND`-style structured error
   - file content does not contain plaintext access/refresh token when encryption key exists
   - concurrent save/get does not corrupt JSON
2. Run:

```bash
go test ./internal/feishu -run 'TokenStore' -v
```

Expected: FAIL.

3. Implement minimal store.
4. Re-run tests.

Expected: PASS.

**Verification artifacts:**
- Test output.
- Temporary token-store fixture proving no plaintext token when encryption key is set.

### Task 2.3: Implement OAuth exchange and refresh client

**Objective:** Exchange authorization code and refresh user access tokens through mocked Feishu endpoints.

**Files:**
- Create/modify: `internal/feishu/oauth.go`
- Create/modify: `internal/feishu/oauth_test.go`

**Methods:**

```go
func (c *HTTPClient) ExchangeOAuthCode(ctx context.Context, code, redirectURI string) (CredentialBinding, error)
func (c *HTTPClient) RefreshUserToken(ctx context.Context, binding CredentialBinding) (CredentialBinding, error)
```

**TDD steps:**

1. Use `httptest.Server` to simulate token endpoints.
2. Test successful code exchange maps `access_token`, `refresh_token`, `expires_in`, `scope`, `open_id`, `user_id`.
3. Test refresh updates token and expiry while preserving credential identity.
4. Test Feishu non-zero `code` maps to structured auth error.
5. Run:

```bash
go test ./internal/feishu -run 'OAuth|RefreshUserToken|ExchangeOAuthCode' -v
```

Expected: FAIL.

6. Implement exchange/refresh.
7. Re-run tests.

Expected: PASS.

**Verification artifacts:**
- Mock request/response assertions.
- Red/green test outputs.

---

## Phase 3: Token Source Integration

### Task 3.1: Introduce request-scoped token source

**Objective:** Allow API calls to choose user token or tenant token without duplicating HTTP logic.

**Files:**
- Modify: `internal/feishu/http_client.go`
- Create: `internal/feishu/token_source.go`
- Test: `internal/feishu/http_client_test.go`

**Design:**

```go
type TokenSource interface {
    Token(ctx context.Context, actor ActorContext) (token string, source string, err error)
}
```

Implementation should provide:
- `TenantTokenSource`
- `UserFirstTokenSource` using `TokenStore`, with tenant fallback only when explicitly allowed

**TDD steps:**

1. Test `GetJSONWithActor` or equivalent sends an Authorization header derived from the selected user credential when actor context exists.
2. Test tenant fallback still works for existing code paths.
3. Test missing user credential returns `AUTH_REQUIRED` when fallback disabled.
4. Run:

```bash
go test ./internal/feishu -run 'TokenSource|Authorization' -v
```

Expected: FAIL.

**Verification artifacts:**
- `httptest` assertion output showing user-token vs tenant-token paths.

### Task 3.2: Thread actor context through service and MCP tools

**Objective:** Let MCP requests specify which user credential should be used.

**Files:**
- Modify: `internal/feishu/service.go`
- Modify: `internal/mcp/feishu_tools.go`
- Test: `internal/mcp/feishu_tools_test.go`

**Input field to add to read/write/comment-capable tools:**

```json
"credentialId": {"type": "string", "maxLength": 128}
```

**Service method pattern:**

```go
func (s *Service) ReadDocument(ctx context.Context, input string, options ReadOptions, actor ActorContext) (DocumentReadResult, error)
```

If backward compatibility is preferred, add `ReadDocumentWithActor` and keep existing `ReadDocument` as tenant-default wrapper.

**TDD steps:**

1. Write MCP decode test for `credentialId`.
2. Write service test proving actor reaches token source.
3. Run:

```bash
go test ./internal/mcp ./internal/feishu -run 'CredentialID|Actor' -v
```

Expected: FAIL.

4. Implement minimal thread-through.
5. Re-run tests.

Expected: PASS.

**Verification artifacts:**
- MCP schema diff.
- Test proving actor propagation.

---

## Phase 4: Permission Check

### Task 4.1: Implement permission API adapter

**Objective:** Query and normalize read/write/comment capability for a canonical document identity.

**Files:**
- Create: `internal/feishu/permission.go`
- Create: `internal/feishu/permission_test.go`
- Modify: `internal/feishu/types.go`

**Type extension:**

```go
type PermissionSnapshot struct {
    CanRead         bool     `json:"canRead"`
    CanWrite        bool     `json:"canWrite"`
    CanComment      bool     `json:"canComment,omitempty"`
    Visibility      string   `json:"visibility,omitempty"`
    Reason          string   `json:"reason,omitempty"`
    RequiredScopes  []string `json:"requiredScopes,omitempty"`
    SuggestedAction string   `json:"suggestedAction,omitempty"`
}
```

**TDD steps:**

1. Test allowed response maps to `canRead=true`, `canWrite=true`, `canComment=true`.
2. Test denied response maps to `PERMISSION_DENIED` or `PermissionSnapshot` with `suggestedAction`.
3. Test upstream 403 does not attempt write/comment.
4. Run:

```bash
go test ./internal/feishu -run 'Permission' -v
```

Expected: FAIL.

**Verification artifacts:**
- Permission fixture JSON.
- Red/green test output.

### Task 4.2: Expose `feishu_doc_check_permission` MCP tool

**Objective:** Give Agent a safe preflight before read/write/comment.

**Files:**
- Modify: `internal/mcp/feishu_tools.go`
- Test: `internal/mcp/feishu_tools_test.go`

**Schema:**

```json
{
  "type": "object",
  "properties": {
    "input": {"type": "string", "maxLength": 2048},
    "credentialId": {"type": "string", "maxLength": 128}
  },
  "required": ["input"],
  "additionalProperties": false
}
```

**TDD steps:**

1. Tool-list test asserts tool exists.
2. Call test asserts input maps to service `CheckPermission`.
3. Run:

```bash
go test ./internal/mcp -run 'CheckPermission|Tools' -v
```

Expected: FAIL, then PASS after implementation.

**Verification artifacts:**
- MCP tool schema output.
- Example permission JSON for allowed and denied cases.

### Task 4.3: Gate mutating operations on permission

**Objective:** Prevent writes/comments when user lacks permission.

**Files:**
- Modify: `internal/feishu/service.go`
- Modify later: comment service files from Phase 5
- Test: `internal/feishu/service_test.go`

**Rule:**
- `AppendDocument`, `CreateDocument` with target folder, comment create/reply/resolve must check permission when not dry-run.
- If permission cannot be checked, fail closed unless config explicitly permits legacy tenant behavior.

**TDD steps:**

1. Test append with denied `CanWrite=false` returns `PERMISSION_DENIED` and does not call append endpoint.
2. Test dry-run append returns preview without calling permission endpoint if safe, or returns permission warning if configured.
3. Run:

```bash
go test ./internal/feishu -run 'Append.*Permission|Permission.*Append' -v
```

Expected: FAIL, then PASS.

**Verification artifacts:**
- Test proving no upstream write call on denied permission.

---

## Phase 5: Comment Operations

### Task 5.1: Add comment data models

**Objective:** Define normalized comment request/result types independent of Feishu raw response.

**Files:**
- Modify: `internal/feishu/types.go`
- Create: `internal/feishu/comment_test.go`

**Types:**

```go
type Comment struct {
    ID          string `json:"id"`
    Content     string `json:"content"`
    AuthorID    string `json:"authorId,omitempty"`
    CreatedTime string `json:"createdTime,omitempty"`
    UpdatedTime string `json:"updatedTime,omitempty"`
    Resolved    bool   `json:"resolved,omitempty"`
    Quote       string `json:"quote,omitempty"`
}

type CommentListResult struct {
    DocumentID string    `json:"documentId"`
    Comments   []Comment `json:"comments"`
    HasMore    bool      `json:"hasMore,omitempty"`
    PageToken  string    `json:"pageToken,omitempty"`
}

type CreateCommentRequest struct {
    Content     string `json:"content"`
    BlockID     string `json:"blockId,omitempty"`
    Quote       string `json:"quote,omitempty"`
    DryRun      *bool  `json:"dryRun,omitempty"`
    OperationID string `json:"operationId,omitempty"`
}
```

**TDD steps:**

1. Test JSON field names and zero-value omissions.
2. Test request validation rejects empty content and overly long content.
3. Run:

```bash
go test ./internal/feishu -run 'Comment.*Model|CreateComment.*Validate' -v
```

Expected: FAIL, then PASS.

**Verification artifacts:**
- Test output.

### Task 5.2: Implement comment API adapter

**Objective:** Support list/create/reply/resolve comments through Feishu/Lark API paths.

**Files:**
- Create: `internal/feishu/comment.go`
- Create/modify: `internal/feishu/comment_test.go`
- Modify: `internal/config/config.go`
- Modify: `.env.example`

**Config paths:**

```env
FEISHU_DOCX_COMMENTS_PATH_TEMPLATE=/open-apis/drive/v1/files/%s/comments
FEISHU_DOCX_COMMENT_REPLIES_PATH_TEMPLATE=/open-apis/drive/v1/files/%s/comments/%s/replies
FEISHU_DOCX_COMMENT_RESOLVE_PATH_TEMPLATE=/open-apis/drive/v1/files/%s/comments/%s
```

Confirm exact Feishu endpoint shapes against current Feishu OpenAPI docs before implementation. If endpoint differs for docx vs drive file, isolate it in config/path builder and document the limitation.

**Service methods:**

```go
func (s *Service) ListComments(ctx context.Context, input string, req ListCommentsRequest, actor ActorContext) (CommentListResult, error)
func (s *Service) CreateComment(ctx context.Context, input string, req CreateCommentRequest, actor ActorContext) (CommentWriteResult, error)
func (s *Service) ReplyComment(ctx context.Context, input string, commentID string, req ReplyCommentRequest, actor ActorContext) (CommentWriteResult, error)
func (s *Service) ResolveComment(ctx context.Context, input string, commentID string, req ResolveCommentRequest, actor ActorContext) (CommentWriteResult, error)
```

**TDD steps:**

1. `httptest` for list path, query pagination, response normalization.
2. `httptest` for create body, dry-run behavior, and permission gate.
3. `httptest` for reply body and path escaping.
4. `httptest` for resolve body/path and permission gate.
5. Run:

```bash
go test ./internal/feishu -run 'Comment' -v
```

Expected: FAIL, then PASS.

**Verification artifacts:**
- Captured mock HTTP requests proving method/path/body.
- Dry-run output snapshot for create/reply/resolve.

### Task 5.3: Expose comment MCP tools

**Objective:** Let Agent collaborate in Feishu documents via comments.

**Files:**
- Modify: `internal/mcp/feishu_tools.go`
- Test: `internal/mcp/feishu_tools_test.go`

**Tools:**
- `feishu_doc_list_comments`
- `feishu_doc_create_comment`
- `feishu_doc_reply_comment`
- `feishu_doc_resolve_comment`

**Schema rules:**
- `input` required, `maxLength: 2048`
- `credentialId` optional, `maxLength: 128`
- `content` required for create/reply, `minLength: 1`, `maxLength: 20000`
- `commentId` required for reply/resolve, `maxLength: 256`
- `dryRun` defaults to true for mutating comment operations unless explicitly safe

**TDD steps:**

1. Tool-list test for all four names.
2. Schema test for required fields and `additionalProperties=false`.
3. Call-handler test for each tool.
4. Run:

```bash
go test ./internal/mcp -run 'Comment|Tools' -v
```

Expected: FAIL, then PASS.

**Verification artifacts:**
- Tool-list JSON.
- Schema JSON.
- Call-handler test outputs.

---

## Phase 6: Wiki and Drive Canonicalization

### Task 6.1: Add canonical identity resolution

**Objective:** Convert wiki/drive inputs into real docx identities before read/permission/comment.

**Files:**
- Modify: `internal/feishu/resolver.go`
- Create: `internal/feishu/canonicalize.go`
- Create: `internal/feishu/canonicalize_test.go`
- Modify: `internal/feishu/service.go`

**Method:**

```go
func (s *Service) CanonicalizeIdentity(ctx context.Context, identity DocumentIdentity, actor ActorContext) (DocumentIdentity, error)
```

**TDD steps:**

1. Test wiki URL -> wiki token from local resolver.
2. Mock API maps wiki token -> docx token.
3. Test read flow uses docx token after canonicalization.
4. Test unsupported drive file returns `UNSUPPORTED_DOCUMENT_TYPE` with `suggestedAction`.
5. Run:

```bash
go test ./internal/feishu -run 'Canonical|Wiki|Drive' -v
```

Expected: FAIL, then PASS.

**Verification artifacts:**
- Mock wiki-to-docx fixture.
- End-to-end service test output.

---

## Phase 7: Remote MCP Security Hardening and Audit

### Task 7.1: Require API key in production HTTP mode

**Objective:** Prevent accidentally exposing remote MCP without authentication.

**Files:**
- Modify: `cmd/feishu-doc-mcp-http-server/main.go`
- Modify: `internal/config/config.go`
- Modify: `internal/mcp/http_server.go`
- Test: `internal/mcp/http_server_test.go`

**Config:**

```env
MCP_HTTP_REQUIRE_API_KEY=true
MCP_HTTP_ALLOWED_ORIGINS=https://chat.openai.com,https://chatgpt.com
```

**TDD steps:**

1. Test empty API key + require=true refuses `/mcp` or server startup validation fails.
2. Test require=false preserves local/dev behavior.
3. Test allowed origin receives CORS header and disallowed origin does not.
4. Run:

```bash
go test ./internal/mcp -run 'APIKey|CORS|HTTPServer' -v
```

Expected: FAIL, then PASS.

**Verification artifacts:**
- HTTP test outputs.
- Config docs diff.

### Task 7.2: Add redacted audit logging

**Objective:** Record operation metadata without storing secrets or full document contents.

**Files:**
- Create: `internal/feishu/audit.go`
- Create: `internal/feishu/audit_test.go`
- Modify: service methods for read/write/comment

**Audit fields:**
- operation id
- action
- provider
- resource type
- token source: `tenant` or `user`, never token value
- actor id or credential id hash/redacted form
- document id redacted if configured
- latency
- retry count if available
- error code

**TDD steps:**

1. Test audit record does not contain access token, refresh token, app secret, raw document markdown, or comment content.
2. Test successful read/comment/write creates expected metadata record.
3. Run:

```bash
go test ./internal/feishu -run 'Audit' -v
```

Expected: FAIL, then PASS.

**Verification artifacts:**
- Audit record fixture with redacted fields.

---

## Phase 8: Real End-to-End Validation

### Task 8.1: Prepare Feishu app and scopes

**Objective:** Validate against real Feishu APIs after unit/integration tests pass.

**Prerequisites:**
- Test Feishu/Lark app with redirect URI configured.
- Test private doc owned/shared by the authorizing user.
- A second doc without permission for negative testing.
- Scopes selected minimally for read/write/comment.

**Verification artifacts:**
- App scope list copied into validation notes.
- Redirect URI value, with secrets redacted.

### Task 8.2: Execute user OAuth flow

**Objective:** Prove a user can authorize and store a usable token.

**Steps:**

1. Call `feishu_oauth_auth_url`.
2. Open generated URL and authorize.
3. Exchange callback code through the planned callback/CLI/manual endpoint.
4. Confirm token store has a credential record without plaintext tokens.

**Verification artifacts:**
- Redacted auth URL.
- Redacted credential id.
- Token store check proving no plaintext access/refresh token.

### Task 8.3: Validate private document read, permission, and comments

**Objective:** Prove the connector solves the target Hermes/Agent gap.

**Steps:**

1. Read private doc with `credentialId`.
2. Run `feishu_doc_check_permission`.
3. Create a dry-run comment.
4. Create a real comment only after explicit user confirmation in the test plan.
5. Reply to that comment.
6. Resolve that comment.
7. Try the unauthorized doc and verify actionable failure.

**Verification standards:**
- Private doc succeeds with user token.
- Same doc fails or is denied without user token if tenant app has no access.
- Permission snapshot includes read/write/comment truthfully.
- Comment appears in Feishu UI.
- Reply appears under the comment.
- Resolve status changes in Feishu UI/API.
- Unauthorized doc returns `PERMISSION_DENIED` or `AUTH_REQUIRED` with suggested action.

**Verification artifacts:**
- Redacted MCP JSON inputs/outputs.
- Feishu UI screenshot or copied comment IDs with document token redacted.
- Negative-case output.

---

## Required Final Quality Gates

Run before marking implementation complete:

```bash
gofmt -w $(find . -name '*.go')
gofmt -l $(find . -name '*.go')
go test ./...
go vet ./...
go build ./cmd/...
git diff --summary
git status --short
```

Expected:
- `gofmt -l` prints nothing.
- tests/vet/build pass.
- `git diff --summary` has no accidental mode-only changes.
- `git status --short` shows only intentional source/docs/config changes.

---

## Scope and Safety Notes

- Do not implement comment/write execution before permission checks and actor/token-source integration exist.
- Do not log access tokens, refresh tokens, app secrets, raw document contents, or raw comment contents.
- Keep tenant-token behavior backward compatible for existing read/create/append tests, but make user token the preferred path when `credentialId` is provided.
- Any real write/comment operation in Phase 8 must be done only on a disposable test document or after explicit user confirmation.
- If Feishu comment API endpoint details differ from the config path assumptions above, update `doc/feishu-doc-module-sdd-spec.md` and this plan before implementation.

---

## Suggested Commit Sequence

1. `chore: clean file mode noise`
2. `test: capture oauth config expectations`
3. `feat: add feishu oauth auth url tool`
4. `feat: add user oauth token exchange and store`
5. `feat: route feishu requests through actor token source`
6. `feat: add feishu permission check tool`
7. `feat: add feishu document comment tools`
8. `feat: canonicalize wiki and drive document identities`
9. `fix: harden remote mcp auth and cors`
10. `feat: add redacted operation audit logs`
11. `docs: add e2e validation evidence for feishu oauth comments`
