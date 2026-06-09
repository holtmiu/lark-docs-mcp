# Agent Breakdown for Skills Support Development

> **For Hermes:** Use subagent-driven-development skill to implement `docs/plans/2026-06-09-skills-support-plan.md` with fresh subagents per task and two-stage review.

**Goal:** Turn the skills support plan into concrete agent work packets that can be executed safely and reviewed independently.

**Architecture:** One implementation agent owns one narrow phase at a time. Every implementation phase is followed by a spec-review agent and a code-quality/security-review agent. Tasks that touch the same files run sequentially, not in parallel.

**Tech Stack:** Go, MCP JSON-RPC, local skill manifests, existing `internal/feishu`, `internal/mcp`, and `internal/config` packages.

---

## Execution Rules

1. Work from repository root: `/opt/data/workspace/github_repos/lark-docs-mcp`.
2. Before each implementation agent starts, capture:
   - `git status --short`
   - the exact task text from `docs/plans/2026-06-09-skills-support-plan.md`
3. Each implementation agent must follow TDD:
   - write focused failing tests first
   - run the focused test and capture RED output
   - implement the minimum code
   - run focused tests and full `go test ./...`
4. After each implementation agent, run two independent reviews:
   - spec compliance review: does it match the plan exactly, with no scope creep?
   - quality/security review: maintainability, safety gates, token redaction, traversal protection, dry-run semantics.
5. Do not start the next implementation phase until both reviews pass.
6. Commit after each approved phase with a small, descriptive commit.
7. For Feishu write/comment behavior, real mutation remains blocked until the explicit E2E phase.

## Agent Roster

| Agent | Phase | Primary Responsibility | May Touch | Must Not Touch |
| --- | --- | --- | --- | --- |
| A1 Schema Agent | Task 1 | Manifest structs, parser, validation tests | `internal/skills/manifest.go`, `internal/skills/manifest_test.go`, `go.mod`, `go.sum` | MCP handlers, Feishu API code |
| A2 Registry Agent | Task 2 | Local registry loading, config env vars, traversal/symlink safety tests | `internal/skills/registry.go`, `internal/skills/registry_test.go`, `internal/config/config.go`, `internal/config/config_test.go`, `.env.example` | Skill execution, write gates |
| A3 MCP Discovery Agent | Task 3 | `feishu_skill_list` and `feishu_skill_get` tool registration and tests | `internal/mcp/feishu_tools.go`, `internal/mcp/feishu_tools_test.go` | Executor semantics, Feishu API adapters |
| A4 Read-only Executor Agent | Task 4 | `feishu_skill_run` for read-safe skills, interpolation, fake tool caller tests | `internal/skills/executor.go`, `internal/skills/executor_test.go`, `internal/mcp/feishu_tools.go`, `internal/mcp/feishu_tools_test.go` | Real writes, arbitrary scripting, loops/conditionals |
| A5 Write Safety Agent | Task 5 | Dry-run default, global write enablement, operation IDs, permission preflight | `internal/skills/executor.go`, `internal/skills/executor_test.go`, `internal/config/config.go`, `internal/mcp/feishu_tools.go` | Any real E2E write without explicit user approval |
| A6 Built-in Skills Agent | Task 6 | Example skill manifests and README documentation | `skills/*.yaml` or `skills/*.json`, `README.md`, `README.zh-CN.md`, registry fixture tests | Core executor changes unless examples reveal parser gaps |
| A7 E2E Validation Agent | Task 7 | Redacted real Feishu validation log for one read-only and one comment skill | `docs/skills-e2e-validation-log.md`, optional scripts under `scripts/` | Publicly logging tokens, full doc contents, raw document/comment IDs |
| A8 Final Integration Reviewer | Final | Whole-feature consistency, tests, docs, public-readiness review | Review only, or small docs/test fixes after approval | Large feature rewrites |

## Phase Dependency Graph

```text
A1 Schema
  -> A2 Registry
      -> A3 MCP Discovery
          -> A4 Read-only Executor
              -> A5 Write Safety
                  -> A6 Built-in Skills
                      -> A7 E2E Validation
                          -> A8 Final Integration Review
```

A1 and A2 must be sequential because registry depends on manifest validation. A3 must wait for A2 because MCP discovery needs the registry interface. A4 and A5 must be sequential because write safety depends on executor boundaries. A6 must wait until the manifest/executor semantics are stable. A7 must be last because it depends on working read and write/comment paths.

## Agent Work Packets

### A1 Schema Agent — Manifest Parser

**Objective:** Add typed skill manifest parsing and validation.

**Input Plan Section:** Task 1 from `docs/plans/2026-06-09-skills-support-plan.md`.

**Files:**
- Create: `internal/skills/manifest.go`
- Create: `internal/skills/manifest_test.go`
- Modify if needed: `go.mod`, `go.sum`

**Required tests:**
- valid manifest parses
- missing `name` fails
- invalid step `tool` fails
- write capability without `write: true` fails
- input schema must be an object

**Implementation guidance:**
- Prefer YAML manifests for user readability if `gopkg.in/yaml.v3` is the only added dependency.
- If avoiding dependencies becomes necessary, use JSON first and record the tradeoff in the plan before continuing.
- Keep allowed tool/capability validation centralized in `internal/skills`.

**Verification commands:**
```bash
go test ./internal/skills -run TestParseManifest -v
go test ./internal/skills -v
go test ./...
```

**Commit:**
```bash
git add internal/skills go.mod go.sum
git commit -m "feat: add skill manifest parser"
```

### A1 Spec Review Agent

Check:
- all required parser tests exist and pass
- validation errors are actionable
- no MCP or Feishu behavior was changed
- no secret-like values were introduced

Output: `PASS` or exact gaps to fix.

### A1 Quality Review Agent

Check:
- parser code is simple and deterministic
- schema validation is not over-engineered
- test fixtures are readable
- dependency addition is justified and minimal

Output: `APPROVED` or requested changes.

---

### A2 Registry Agent — Local Skill Registry

**Objective:** Load validated manifests from configured local directories.

**Files:**
- Create: `internal/skills/registry.go`
- Create: `internal/skills/registry_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `.env.example`

**Required tests:**
- loads multiple manifests from temp directories
- rejects duplicate skill names
- ignores non-manifest files
- blocks `..` path traversal inputs
- blocks or safely skips symlink escapes where platform support allows
- config parses `FEISHU_SKILLS_DIRS`
- config defaults `FEISHU_SKILLS_ENABLE_WRITE=false`

**Implementation guidance:**
- Do not load remote URLs.
- Do not execute skill files while loading.
- Keep registry read-only and deterministic.
- Return sorted skill lists for stable MCP responses.

**Verification commands:**
```bash
go test ./internal/skills -run Registry -v
go test ./internal/config -v
go test ./...
```

**Commit:**
```bash
git add internal/skills internal/config .env.example
git commit -m "feat: load local skill registry"
```

### A2 Spec Review Agent

Check config/env behavior, registry safety, duplicate handling, and deterministic ordering.

### A2 Quality Review Agent

Check path safety, error messages, minimal API surface, and test portability.

---

### A3 MCP Discovery Agent — List/Get Tools

**Objective:** Expose read-only skill discovery through MCP.

**Files:**
- Modify: `internal/mcp/feishu_tools.go`
- Modify: `internal/mcp/feishu_tools_test.go`

**New tools:**
- `feishu_skill_list`
- `feishu_skill_get`

**Required tests:**
- `tools/list` includes both tools when a skill registry is configured
- `feishu_skill_list` returns stable names/titles/descriptions/capabilities
- `feishu_skill_get` returns one full manifest summary by name
- unknown skill returns a structured MCP error

**Implementation guidance:**
- Use an interface so `internal/mcp` does not depend on concrete registry internals more than needed.
- Discovery tools are always read-only.
- Do not include local filesystem paths in public responses unless explicitly useful and safe.

**Verification commands:**
```bash
go test ./internal/mcp -run Skill -v
go test ./...
```

**Commit:**
```bash
git add internal/mcp
git commit -m "feat: expose skill discovery tools"
```

### A3 Spec Review Agent

Check tool names, schemas, and behavior against the plan.

### A3 Quality Review Agent

Check JSON response stability, interface boundaries, and error handling.

---

### A4 Read-only Executor Agent — Skill Run Without Writes

**Objective:** Add `feishu_skill_run` for read-only skills.

**Files:**
- Create/modify: `internal/skills/executor.go`
- Create/modify: `internal/skills/executor_test.go`
- Modify: `internal/mcp/feishu_tools.go`
- Modify: `internal/mcp/feishu_tools_test.go`

**Required tests:**
- read-only skill executes ordered steps against a fake tool caller
- `${inputName}` interpolation works for simple string values
- missing required input fails before any step executes
- unsupported interpolation syntax fails closed
- write-capability skill is rejected in read-only executor mode
- step outputs are returned in a structured response

**Implementation guidance:**
- No loops.
- No conditionals.
- No arbitrary script execution.
- Only compose known internal tools.
- Keep execution context explicit: skill name, inputs, dryRun value, step results.

**Verification commands:**
```bash
go test ./internal/skills -run Executor -v
go test ./internal/mcp -run Skill -v
go test ./...
```

**Commit:**
```bash
git add internal/skills internal/mcp
git commit -m "feat: run read-only skills"
```

### A4 Spec Review Agent

Check step ordering, interpolation scope, read-only enforcement, and response shape.

### A4 Quality Review Agent

Check fail-closed behavior, test fakes, and absence of hidden side effects.

---

### A5 Write Safety Agent — Dry-run and Permission Gates

**Objective:** Add safe execution gates for write/comment skills.

**Files:**
- Modify: `internal/skills/executor.go`
- Modify: `internal/skills/executor_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/mcp/feishu_tools.go`

**Required tests:**
- write skills are rejected when `FEISHU_SKILLS_ENABLE_WRITE=false`
- write steps default to `dryRun:true`
- `dryRun:false` is rejected unless global write skills are enabled
- real write requires `operationId` when underlying tool supports it
- write skill without permission preflight fails validation or execution before mutation
- failing permission preflight stops later write steps

**Implementation guidance:**
- Keep gates in `internal/skills` so all MCP entrypoints inherit the same behavior.
- Prefer explicit allowlists of write-capable tool names.
- Tests must prove unsafe downstream calls are not invoked.

**Verification commands:**
```bash
go test ./internal/skills -run Write -v
go test ./internal/config -v
go test ./internal/mcp -run Skill -v
go test ./...
```

**Commit:**
```bash
git add internal/skills internal/config internal/mcp
git commit -m "feat: add write safety gates for skills"
```

### A5 Spec Review Agent

Check all safety rules from the plan are implemented.

### A5 Quality Review Agent

Check security posture, fail-closed behavior, operation ID semantics, and test proof that writes are blocked when expected.

---

### A6 Built-in Skills Agent — Example Skills and Docs

**Objective:** Add practical local skills and document usage.

**Files:**
- Create: `skills/export-doc-markdown.yaml`
- Create: `skills/create-draft-doc.yaml`
- Create: `skills/add-review-comment.yaml`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify or create fixture tests under `internal/skills` if needed

**Required examples:**
- export document as Markdown
- create a draft doc from Markdown
- add a review comment with dry-run default

**Required tests:**
- repository `skills/` directory loads successfully
- each example declares required inputs and capabilities
- write example cannot run real mutation unless write gates allow it

**Documentation requirements:**
- explain `FEISHU_SKILLS_DIRS`
- explain `FEISHU_SKILLS_ENABLE_WRITE`
- show skill discovery and run examples
- warn that skills do not store secrets
- document dry-run default for write skills

**Verification commands:**
```bash
go test ./internal/skills -run 'Registry|Manifest|Example|Write' -v
go test ./...
```

**Commit:**
```bash
git add skills README.md README.zh-CN.md internal/skills
git commit -m "docs: add built-in skill examples"
```

### A6 Spec Review Agent

Check examples align with actual manifest schema and README claims.

### A6 Quality Review Agent

Check naming polish, user-facing clarity, no placeholder/demo wording in final public names, and no secrets.

---

### A7 E2E Validation Agent — Real Feishu Proof

**Objective:** Validate one read-only skill and one comment/write skill against real Feishu/Lark authorization.

**Files:**
- Create: `docs/skills-e2e-validation-log.md`
- Optional: add reusable local validation commands under `scripts/` only if they do not contain secrets.

**Required sequence:**
1. Confirm target document is safe for testing.
2. Run `export-doc-markdown` against a real readable doc.
3. Run `add-review-comment` with dry-run enabled.
4. Ask for explicit approval before any real comment write if the target is not clearly a temporary test document.
5. Run `add-review-comment` with `dryRun:false` only after approval or on a known temporary doc.
6. Redact document tokens, comment IDs, app IDs, user IDs, and token-like values.
7. Save only redacted evidence to the public log.

**Verification commands:**
```bash
go test ./...
# plus the exact MCP/CLI commands used for the real validation, recorded in redacted form
```

**Commit:**
```bash
git add docs/skills-e2e-validation-log.md scripts
git commit -m "test: document skills e2e validation"
```

### A7 Spec Review Agent

Check the E2E log proves both read-only and write/comment workflows without exposing secrets.

### A7 Quality Review Agent

Check redaction, reproducibility, public-safety wording, and that real mutation evidence is minimal.

---

### A8 Final Integration Reviewer

**Objective:** Verify the complete skills feature is coherent, safe, and public-ready.

**Checklist:**
- `go test ./...` passes
- `gofmt -w $(find . -name '*.go')` produces no diff
- high-confidence secret scan finds no real secrets
- README and README.zh-CN match actual behavior
- all skill examples parse
- write-skill safety is fail-closed
- no local filesystem paths or internal tokens leak in MCP responses
- all commits are small and understandable

**Verification commands:**
```bash
gofmt -w $(find . -name '*.go')
go test ./...
git grep -n -I -E 'xox[baprs]-|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9_]{20,}|u-[0-9a-f]{32,}|cli_[a-z0-9]{16,}|tenant_access_token[[:space:]]*[:=]|refresh_token[[:space:]]*[:=]|access_token[[:space:]]*[:=]' -- . ':!go.sum' || true
git status --short
git log --oneline -8
```

**Final commit if needed:**
```bash
git add -A
git commit -m "chore: finalize skills support"
```

## Ready-to-Dispatch Order

1. Dispatch A1 Schema Agent.
2. Review A1 with spec reviewer.
3. Review A1 with quality reviewer.
4. Fix and re-review if needed.
5. Commit A1.
6. Repeat for A2 through A7.
7. Run A8 final integration review.
8. Push only after full verification passes.

## Immediate Next Step

Start with A1 Schema Agent. Provide it:
- this file
- Task 1 from `docs/plans/2026-06-09-skills-support-plan.md`
- repository root `/opt/data/workspace/github_repos/lark-docs-mcp`
- required check command `go test ./...`

Do not dispatch A2 until A1 implementation and both reviews pass.
