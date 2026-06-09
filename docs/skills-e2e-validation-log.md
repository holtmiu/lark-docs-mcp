# Skills Real Feishu E2E Validation Log

Date: 2026-06-09

## Scope

Real Feishu OpenAPI validation for the built-in skill layer added in the skills support workstream.

This log intentionally redacts document tokens, operation IDs, app IDs, API keys, and credential values.

## Runtime Setup

- Built and tested the Go project locally before provider calls.
- Started the HTTP MCP server on a loopback-only address with a temporary local API key.
- Enabled the repository skill registry with `FEISHU_SKILLS_DIRS=./skills`.
- Enabled write-capable skill loading with `FEISHU_SKILLS_ENABLE_WRITE=true` for this local validation run.
- Kept write operations dry-run by default.

## Transport and Discovery Checks

- `/healthz`: PASS, HTTP 200.
- Unauthenticated `/mcp` ping: PASS, HTTP 401 fail-closed.
- Authenticated `/mcp` ping: PASS.
- `tools/list`: PASS, returned the skill tools:
  - `feishu_skill_list`
  - `feishu_skill_get`
  - `feishu_skill_run`
- `feishu_skill_list`: PASS, returned the three built-in skills:
  - `add-review-comment`
  - `create-draft-doc`
  - `export-doc-markdown`
- `feishu_skill_get` for `export-doc-markdown`: PASS, returned the expected read-only manifest summary and steps.

## Real Feishu Artifact

A temporary Feishu docx was created through the low-level MCP document create tool to provide a real target for skill validation.

- Temporary document title prefix: `lark-docs-mcp skill e2e`
- Temporary document token: redacted
- Low-level document create without body append: PASS
- Low-level document create with Markdown body append: BLOCKED by provider permission, returning a write-permission denial during append.

## Read-only Skill E2E

Skill: `export-doc-markdown`

Input target: temporary Feishu docx token, redacted.

Result: PASS.

Observed evidence:

- Step 0 `feishu_doc_get_metadata`: PASS.
- Step 1 `feishu_doc_read` with Markdown format: PASS.
- Returned metadata included:
  - `resourceType`: `docx`
  - `revisionId`: `1`
  - `permissions.canRead`: `true`
- Returned read result without exposing secrets in this log.

## Write Skill Dry-run E2E

Skill: `add-review-comment`

Input target: temporary Feishu docx token, redacted.

Result: PASS for dry-run behavior.

Observed evidence:

- `feishu_doc_check_permission` ran before the mutation step.
- `feishu_doc_create_comment` was invoked with executor-injected `dryRun:true`.
- The dry-run result included the warning `dry-run only: no comment mutation was sent to Feishu/Lark`.
- No real comment was created during the dry-run path.

## Real Write Skill Attempt

Skill: `add-review-comment`

Input target: temporary Feishu docx token, redacted.

Requested with:

- top-level `dryRun:false`
- a unique operation ID, redacted
- the same target used by the permission preflight

Result: BLOCKED / FAIL-CLOSED.

Observed error:

- Structured skill error code: `permission_preflight_denied`
- Message summary: permission preflight denied the mutation before the comment creation step.

The permission preflight result for the target reported that write/comment permission was not available to the configured provider actor. The skill executor therefore stopped before sending the real comment mutation, which confirms the A5 write-safety gate fails closed under this provider permission state.

## Current Phase Status

A7 is partially validated but not complete.

Completed:

- Local build/test verification.
- HTTP MCP transport/security probes.
- Skill discovery over MCP.
- Real read-only built-in skill execution against a real Feishu docx.
- Write skill dry-run execution against a real Feishu docx.
- Real write path fail-closed verification when permission preflight denies mutation.

Blocked:

- A successful real `dryRun:false` `add-review-comment` skill mutation could not be completed because the configured Feishu actor did not pass the permission preflight for the temporary document target.

Required next step:

- Provide or configure a Feishu document/folder target for which the configured actor passes `feishu_doc_check_permission` with comment/write permission, then rerun the real `dryRun:false` `add-review-comment` skill path and update this log.
