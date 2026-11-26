# Guest read-only audit, /auth/email honeypot, Playwright access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close the 3 remaining gaps of Email login + Guest mode: no write control visible/enabled for guests on any page, honeypot on `/auth/email` proven end to end, Playwright MCP allowed for real verification.

**Architecture:** Frontend gating goes through `lib/guest.ts` (`canEdit`, `lockProps`) and a `readOnly` prop from `App.tsx`. Backend enforcement is `middleware/guest.go`. Honeypot is the `website` field in `emailAuthRequest`.

**Tech Stack:** Go/Fiber, React 19 + Vite + vitest, Playwright MCP.

## Findings from survey (state on 2026-11-26)

- **Honeypot: already implemented and passing.** `handler/auth.go:117-134` (202 + audit `auth.email_bot`, never reaches service), tests `TestEmailAuthWithTheHoneypot*`, frontend `lib/loginForm.ts` + hidden `website` input in `LoginPage.tsx:94`. `go test -run EmailAuth` and `vitest src/lib` pass. Only gap: the route has a rate limit (`router.go:146`) but no test that the real route wires the handler, and no browser check.
- **Guest read-only: mostly done.** `readOnly` is wired into Account, KocDirectory, TargetCollab(+InviteModal), Workflow(+Editor, TemplateBar). Dashboard, CollabAdmin, KocContent have only read/refresh/pagination buttons (no writes). Inbox/Users/Audit/System are hidden from guests. Not yet proven: modals reachable from read-only pages (AddKoc, BulkImport, PreviewDialog, SendingSwitch, AutomationSwitch) and keyboard/enter paths.
- **Playwright:** blocked only by the project rule in `.claude/CLAUDE.md` ("Only invoke Playwright MCP when explicitly asked"); `.claude/settings.json` has no permission entry.

## Global Constraints

- Reply Vietnamese; guest tooltip text stays `GUEST_TOOLTIP = 'Đăng nhập để chỉnh sửa'`.
- Guest backend 403 for writes already enforced; do not weaken.
- Verify with `cd backend && go build ./... && go vet ./... && go test ./internal/...` and `cd frontend && npm run build && npm test`.
- After code changes run `graphify update .`.

## Review Focus

- Guest presses Enter in a form on a read-only page (submit bypasses disabled button) → no request sent.
- Guest opens a modal via deep state (e.g. `isAddModalOpen`) → blocked (`App.tsx:384` already requires `editable`).
- Bot sends `website` with only whitespace → treated as empty (handler uses TrimSpace; confirm intended).
- Honeypot hit must not consume/leak account existence (same 202 body as register).
- Rate limit 10/min still applies before honeypot.

### Task 1: Guest write-control audit (Playwright-driven, then fix)

**Files:**
- Modify (only where audit finds a gap): files under `frontend/src/features/**` and `frontend/src/App.tsx`
- Test: `frontend/src/lib/guest.test.ts` (extend if a new helper is added)

**Interfaces:**
- Consumes: `canEdit(user)`, `lockProps(readOnly)`, `isNavVisibleFor(label, role)` from `lib/guest.ts`.
- Produces: none new unless a gap needs a shared helper.

- [ ] Step 1: Start backend + frontend, open `/` via Playwright, click "Dùng thử Khách"; for each visible nav item take `browser_snapshot` and list every `button`/`input`/`select` that is enabled and mutates data.
- [ ] Step 2: For each gap, add `readOnly` prop passthrough + `{...lockProps(readOnly)}` (or hide) and guard the submit handler with `if (readOnly) return;`.
- [ ] Step 3: Re-run the snapshots; expected: zero enabled write controls; try Enter-submit and confirm no write request in `browser_network_requests`.
- [ ] Step 4: `cd frontend && npm run build && npm test` → PASS.
- [ ] Step 5: Commit `fix(guest): lock remaining write controls`.

### Task 2: Honeypot end-to-end proof

**Files:**
- Test: `backend/internal/router/router_test.go` (add `TestAuthEmailRouteHoneypot`)

**Interfaces:**
- Consumes: existing router test harness in `router_test.go`.

- [ ] Step 1: Write `TestAuthEmailRouteHoneypot`: POST `/api/auth/email` (use the actual prefix from `router.go`) with `{"email":"bot@x.vn","password":"Matkhau123","website":"spam"}`; assert 202, body `success:true`, no `token`, service `EmailAuth` not called.
- [ ] Step 2: Run `go test ./internal/router -run AuthEmailRouteHoneypot` → expect PASS (or FAIL showing a wiring gap to fix in `router.go`).
- [ ] Step 3: Playwright: on login page, use `browser_evaluate` to set the hidden `website` field and submit; expect neutral message and no token in localStorage.
- [ ] Step 4: Commit `test(auth): honeypot is enforced through the /auth/email route`.

### Task 3: Allow Playwright MCP

**Files:**
- Modify: `.claude/CLAUDE.md` (the "Only invoke Playwright MCP..." line)
- Modify: `.claude/settings.json` (add `permissions.allow: ["mcp__playwright__*"]` — confirm exact matcher syntax via `update-config` skill)

- [ ] Step 1: Replace the rule with: Playwright MCP may be used to verify UI/auth flows; prefer `browser_snapshot` over screenshots to save context.
- [ ] Step 2: Add the permission entry; validate JSON.
- [ ] Step 3: Commit `chore: allow Playwright MCP for UI verification`.

## Self-review

Spec coverage: 3 requests → Tasks 1/2/3. Proportion: small; Task 1 size depends on audit outcome.
