# Demo inbox, demo workflow, sign-up form Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** (1) a sign-up form on the login page; (2) guests see the Hộp thư tab filled with fake data; (3) the Quy trình tab ships a ready-made demo diagram that guests (and anyone) can "run" as a simulation with fake data.

**Architecture:** Demo data and the simulator live ONLY in the frontend (`frontend/src/demo/`). In demo mode no inbox/zalo/gmail/workflow API is called, so the backend gate (`middleware/guest.go`: guests get 403 on /inbox, /zalo, /gmail, writes) stays unchanged and real data can never leak. The look is the real components fed by a demo data source, so the UI stays identical.

**Tech Stack:** React 19 + TS + vitest, Go/Fiber, Playwright MCP for the final check.

**Spec:** the user's message of 2026-11-26 (3 bullets). Related: `2026-11-26-guest-readonly-honeypot-playwright.md`.

## Global Constraints

- Reply Vietnamese; every demo screen shows the banner `GUEST_BANNER_TEXT` and a visible "DỮ LIỆU DEMO" tag so fake data is never mistaken for real.
- Guest backend rules do not change: no new guest-allowed route; 403 on /inbox, /zalo, /gmail, /ttshop, /admin, and on every write stays.
- UI must reuse existing components/classes (`lockProps`, `tm-btn-*`, `cp-btn`, tokens in `styles.css`); no new visual language.
- Verify: `cd backend && go build ./... && go vet ./... && go test ./internal/...`; `cd frontend && npm run build && npm test`; `graphify update .`.

## Review Focus

- Demo mode must make zero requests to /inbox, /zalo, /gmail, /workflows (check `browser_network_requests`).
- Demo simulation must never call `POST /workflows`, send Zalo/email, or write localStorage keys the real editor reads (`workflow-draft`).
- Registration: honeypot + captcha + rate limit stay; a new account is `member`/pending, never `guest`/`admin`; error messages never reveal whether an email exists.
- A real (non-guest) user must never see demo data in Hộp thư.
- Guest cannot reach the demo-only UI paths to trigger real sends (all send buttons are demo-simulated or disabled).

## Task 1: Sign-up form (login page)

**Files:** Modify `frontend/src/pages/LoginPage.tsx`, `frontend/src/lib/loginForm.ts` (+ `loginForm.test.ts`), `frontend/src/styles.css` (reuse existing login classes).
**Interfaces:** Produces `buildRegisterBody(form): {name,email,password,website,captchaToken}`. Consumes `GET /auth/options` (sign-up open? captcha site key) and `POST /auth/register`.

- [ ] Test (`loginForm.test.ts`): `buildRegisterBody` always sends `website` (""), trims name, never sends role/status.
- [ ] Add a "Tạo tài khoản" mode toggle on `LoginPage` (name, email, password ≥ 8, hidden honeypot, captcha widget when `siteKey` present); hide the toggle when `/auth/options` says sign-up is closed.
- [ ] On 202 show the server notice (account waits for admin approval); never auto-login.
- [ ] Verify with Playwright: register a new email → pending notice, then login is refused until approved.

## Task 2: Demo inbox for guests

**Files:** Create `frontend/src/demo/inboxData.ts` (≈6 fake KOCs, conversations, messages, labels, zalo accounts — fictional names/handles, no real phones/emails), `frontend/src/demo/useDemoInbox.ts`. Modify `frontend/src/App.tsx` (remove `Hộp thư` from the guest branch at the `isGuest ? <ViewLoading/>` fallback), `frontend/src/lib/guest.ts` (`GUEST_HIDDEN_NAV` loses `'Hộp thư'`; update `guest.test.ts`), `frontend/src/features/inbox/MessagingWorkspace.tsx` (accept `demo` prop → read state from demo source, skip every `api.*`), `Composer.tsx` (send = append a local fake bubble marked demo).
**Interfaces:** Produces `useDemoInbox(): { conversations, messages(id), labels, zaloAccounts, send(id,text) }` — in-memory only, resets on reload.

- [ ] Test: `isNavVisibleFor('Hộp thư','guest') === true`; demo data has no field shaped like a real phone/email (regex guard test).
- [ ] Implement demo source + wire `MessagingWorkspace demo` (App passes `demo={isGuest}`); all write controls beyond "gửi thử" stay locked via `lockProps`.
- [ ] Backend: keep `/inbox` blocked for guests (add a test asserting it still 403s — already in `guest_test.go`, extend if missing).
- [ ] Playwright: guest opens Hộp thư → sees demo conversations; network log shows no /inbox, /zalo, /gmail request.

## Task 3: Demo workflow diagram + simulated run

**Files:** Create `frontend/src/demo/demoWorkflow.ts` (ready diagram: pick KOCs → send_text → wait 1 day → if_replied → [replied: label "Quan tâm"] / [unanswered: email nhắc]), `frontend/src/demo/simulateRun.ts` (pure function stepping node states with timers and fake counts). Modify `frontend/src/features/workflows/WorkflowPage.tsx` / `WorkflowEditor.tsx` (guest and a "Mở sơ đồ demo" button load the demo graph; "Chạy thử demo" drives `simulateRun`), `WorkflowRunView.tsx` (render the simulated state with the DEMO tag).
**Interfaces:** Produces `DEMO_WORKFLOW: {nodes: WorkflowNode[]; edges: WorkflowEdge[]}` (valid per `graph.ts` validation) and `simulateRun(wf, onTick): () => void` (returns cancel).

- [ ] Test: `DEMO_WORKFLOW` passes the same validation the editor uses; `simulateRun` visits every node, ends all `done`, and never calls `api` (inject a spy).
- [ ] Wire guest: editor opens with the demo graph preloaded (not saved to `workflow-draft`); "Chạy thử demo" is enabled for guests, real "Bắt đầu" stays locked.
- [ ] Playwright: guest clicks "Chạy thử demo" → nodes progress to done with fake counts; no POST/PUT request.

## Task 4: Security + UI consistency check, ship

- [ ] Playwright full pass as guest and as a member (member sees real, empty inbox, no demo data); `browser_network_requests` shows no forbidden calls; snapshot compare of buttons/spacing with the real tabs.
- [ ] Full verification commands, `graphify update .`, commit per task, push to `main` only after the user confirms.

## Self-review

Spec coverage: bullet 1 → Task 1 (+ guest sees Hộp thư → Task 2); bullet 2 → Task 3; bullet 3 → Global Constraints, Review Focus, Task 4. Types: `DEMO_WORKFLOW`, `simulateRun`, `useDemoInbox`, `buildRegisterBody` are named once and reused consistently.
