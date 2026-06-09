# Skills Support Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add a first-class skills layer so Lark Docs MCP can expose reusable document workflows, not only low-level document tools.

**Architecture:** Keep the current Feishu/Lark tool layer as the execution substrate. Add a small skill registry that loads declarative skill packs from local files, exposes them through MCP tools, and executes skill steps by composing existing document operations with explicit dry-run and permission checks. Start with read-only/listing support, then add execution with strict allowlists.

**Tech Stack:** Go, MCP JSON-RPC, local YAML/JSON skill manifests, existing `internal/feishu` service, existing `internal/mcp` server and tool registry.

---

## Phase Verification Matrix

| Phase | Capability | Verification Standard | Verification Artifacts |
| --- | --- | --- | --- |
| 1 | Define skill manifest schema | Invalid manifests fail with actionable errors; valid manifests parse into typed structs. | Unit test output for parser and validation cases. |
| 2 | Load local skill registry | Server loads skills from configured directories without exposing file traversal. | Unit tests with temporary skill directories and blocked path traversal fixtures. |
| 3 | Expose skill discovery over MCP | MCP clients can list skills and inspect required inputs/capabilities. | `tools/list` output plus `feishu_skill_list` / `feishu_skill_get` test responses. |
| 4 | Execute read-only skills | A skill can call read/metadata/permission tools and return structured results. | Integration tests using fake Feishu service. |
| 5 | Execute write/comment skills safely | Skills that write require dry-run by default, explicit `dryRun:false`, and permission preflight. | Tests proving dry-run default, operation IDs, and permission failures. |
| 6 | Document built-in skills | README describes skill packs and includes examples. | README diff and example skill files. |
| 7 | Real E2E validation | One read-only and one write/comment skill run against a real Feishu doc. | Redacted E2E log with document/comment IDs removed. |

## Proposed Skill Concept

A skill should be a reusable workflow recipe that an MCP client can discover and run. Examples:

- `summarize_doc_for_review`: read a doc and return Markdown sections for summary/review.
- `create_meeting_notes_doc`: create a new doc from a meeting notes template.
- `append_review_comment`: add a standardized review comment with dry-run default.
- `resolve_review_comments`: list comments, filter by text/status, and resolve matching comments.
- `export_doc_as_markdown`: read and normalize a doc for downstream storage.

Skills should not contain secrets. They should describe:

- Name, title, description, version.
- Required inputs and JSON schema.
- Required Feishu/Lark capabilities: read, create, append, comment, resolve_comment.
- Whether writes are allowed.
- Ordered steps that call existing internal operations.
- Output shape.

## Manifest Draft

```yaml
name: append_review_comment
version: 0.1.0
title: Append Review Comment
description: Add a standardized review comment to a Feishu/Lark doc.
capabilities:
  - doc.comment.create
write: true
inputs:
  type: object
  required: [input, content]
  properties:
    input:
      type: string
      maxLength: 2048
    content:
      type: string
      minLength: 1
      maxLength: 20000
steps:
  - tool: feishu_doc_check_permission
    args:
      input: ${input}
  - tool: feishu_doc_create_comment
    args:
      input: ${input}
      content: ${content}
      dryRun: ${dryRun}
outputs:
  type: object
```

## Task 1: Add manifest structs and parser tests

**Objective:** Create typed skill manifest parsing and validation.

**Files:**
- Create: `internal/skills/manifest.go`
- Create: `internal/skills/manifest_test.go`

**Step 1: Write failing tests**

Add tests for:

- Valid manifest parses.
- Missing name fails.
- Invalid step tool fails.
- Write skill without `write: true` fails when a write capability is present.
- Input schema must be an object.

Run:

```bash
go test ./internal/skills -run TestParseManifest -v
```

Expected: FAIL because package does not exist.

**Step 2: Implement minimal parser**

Use Go structs and `gopkg.in/yaml.v3` only if adding a dependency is acceptable. If avoiding dependencies, start with JSON manifests and add YAML later.

**Step 3: Verify**

```bash
go test ./internal/skills -v
go test ./...
```

**Verification artifacts:** Unit test output.

## Task 2: Add local skill registry loader

**Objective:** Load validated skill manifests from configured local directories.

**Files:**
- Create: `internal/skills/registry.go`
- Create: `internal/skills/registry_test.go`
- Modify: `internal/config/config.go`
- Modify: `.env.example`

**Step 1: Write failing tests**

Test:

- Loads multiple manifests from a temp directory.
- Rejects duplicate skill names.
- Ignores non-manifest files.
- Blocks path traversal and symlink escapes if supported by the platform.

**Step 2: Add config**

Proposed env vars:

- `FEISHU_SKILLS_DIRS`: comma-separated list of skill directories.
- `FEISHU_SKILLS_ENABLE_WRITE`: default `false`.

**Step 3: Verify**

```bash
go test ./internal/skills -v
go test ./internal/config -v
go test ./...
```

**Verification artifacts:** Test output and `.env.example` diff.

## Task 3: Expose MCP skill discovery tools

**Objective:** Add read-only MCP tools for listing and inspecting skills.

**Files:**
- Modify: `internal/mcp/feishu_tools.go`
- Modify: `internal/mcp/feishu_tools_test.go`

**New tools:**

- `feishu_skill_list`
- `feishu_skill_get`

**Step 1: Write failing tests**

Verify `tools/list` includes the new tools when a registry is configured.

**Step 2: Implement minimal wiring**

Extend `FeishuTools` with an optional `SkillRegistry` interface.

**Step 3: Verify**

```bash
go test ./internal/mcp -run Skill -v
go test ./...
```

**Verification artifacts:** MCP tool list test output.

## Task 4: Add read-only skill execution

**Objective:** Execute skills composed only of read-safe operations.

**Files:**
- Create: `internal/skills/executor.go`
- Create: `internal/skills/executor_test.go`
- Modify: `internal/mcp/feishu_tools.go`
- Modify: `internal/mcp/feishu_tools_test.go`

**New tool:**

- `feishu_skill_run`

**Step 1: Write failing tests**

Test a skill that calls `feishu_doc_resolve` and `feishu_doc_read` against a fake tool caller.

**Step 2: Implement interpolation**

Support only simple `${inputName}` interpolation initially. Do not add loops, conditionals, or arbitrary code.

**Step 3: Verify**

```bash
go test ./internal/skills -run Executor -v
go test ./internal/mcp -run Skill -v
go test ./...
```

**Verification artifacts:** Executor test output.

## Task 5: Add write-skill safety gates

**Objective:** Allow write/comment skills only with dry-run defaults, explicit write enablement, and permission checks.

**Files:**
- Modify: `internal/skills/executor.go`
- Modify: `internal/skills/executor_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/mcp/feishu_tools.go`

**Rules:**

- Write skills cannot run unless server config allows write skills.
- Write steps default to `dryRun:true` unless caller passes `dryRun:false` and server config allows it.
- Write skills must declare write capabilities.
- Write skills should require an `operationId` for real mutations when the underlying tool supports it.

**Step 1: Write failing tests**

Test dry-run default, write disabled rejection, real write allowed path, and permission preflight missing rejection.

**Step 2: Implement gates**

Keep logic centralized in `internal/skills` rather than scattering checks across MCP handlers.

**Step 3: Verify**

```bash
go test ./internal/skills -run Write -v
go test ./...
```

**Verification artifacts:** Safety gate test output.

## Task 6: Add example built-in skills

**Objective:** Provide useful examples without requiring users to invent manifest structure.

**Files:**
- Create: `skills/export-doc-markdown.yaml`
- Create: `skills/create-draft-doc.yaml`
- Create: `skills/add-review-comment.yaml`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Step 1: Add examples**

Start with three simple skills:

- Export document as Markdown.
- Create draft document from Markdown.
- Add review comment.

**Step 2: Verify examples parse**

Add a test fixture or registry test that loads the repository `skills/` directory.

**Verification artifacts:** Test output and README examples.

## Task 7: Real E2E validation for skills

**Objective:** Prove skills work with real Feishu/Lark authorization and document operations.

**Files:**
- Create: `docs/skills-e2e-validation-log.md`

**Step 1: Run read-only skill**

Run `export-doc-markdown` against a real docx accessible to the configured app/user token.

**Step 2: Run write/comment skill**

Run `add-review-comment` first with dry-run, then with `dryRun:false` on a temporary doc.

**Step 3: Redact evidence**

Remove document tokens, comment IDs, app IDs, user IDs, and token-like values from the public log.

**Verification artifacts:** Redacted E2E log.

## Open Design Questions

1. Should manifests be JSON-only first to avoid dependencies, or YAML for readability?
2. Should skills be repository-local only, or should the server also load a user data directory?
3. Should write skills require per-skill allowlisting in addition to global `FEISHU_SKILLS_ENABLE_WRITE`?
4. Should skill outputs be raw step outputs or mapped through an output template?
5. Should skill packs be shareable as Git submodules/packages later?

## Recommended First Release Scope

For the first skills release, keep the scope intentionally small:

- Local file-based registry.
- JSON or YAML manifest parsing.
- `feishu_skill_list`, `feishu_skill_get`, `feishu_skill_run`.
- Simple `${input}` interpolation only.
- No arbitrary script execution.
- No network loading of skills.
- Dry-run default for all write skills.
- Real E2E log before marking the feature complete.
