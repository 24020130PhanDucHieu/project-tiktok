# Auth page: visible sign-up + tidier login/sign-up UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Visitors can always see and use "Đăng ký" on the auth page, and the login / sign-up card looks clean: one segmented tab switch, aligned fields, one clear primary button, a quiet secondary "Xem Demo" button.

**Architecture:** The sign-up code already exists (`LoginPage.tsx` mode `register`, `POST /auth/register`, pending-approval flow) but the toggle only renders when `GET /auth/options` returns `allowRegistration: true`, and production has `ALLOW_REGISTRATION=false` (the default), so the user sees no sign-up. Fix = (a) turn registration on in the deployed env, (b) restyle `LoginPage` into a tabbed card in its own `login.css`, (c) stop the login path from silently creating accounts.

**Tech Stack:** React 19 + TS + Vite + vitest (frontend), Go/Fiber (backend), Playwright MCP for the visual check.

**Spec:** the user's message of 2026-11-27 + screenshot of `project-tiktok-do4u.vercel.app` (login card with 3 stacked full-width buttons, no sign-up link). Related: `2026-11-26-demo-inbox-workflow-register.md` (Task 1 built the form).

## Global Constraints

- Reply Vietnamese; UI copy Vietnamese.
- Sign-up stays: honeypot `website`, Turnstile captcha when `captchaSiteKey` is set, rate limit 5/h/IP, new account = `member` + `pending`, never auto-login, same notice whether or not the email exists.
- Guest button keeps calling `POST /auth/guest`; guest backend rules unchanged.
- Reuse existing tokens/colors in `styles.css` (`#1d4ed8` primary, `#15233d` intro panel, radius 8px); no new visual language, no new dependency.
- Verify: `cd backend && go build ./... && go vet ./... && go test ./internal/...`; `cd frontend && npm run build && npm test`; `graphify update .`.

## Review Focus

- Sign-up closed (`allowRegistration:false` or `/auth/options` fails): the "Đăng ký" tab must not be a dead end — it shows "Đăng ký đang đóng, liên hệ quản trị viên", no form.
- Typing an unknown email in the login tab must NOT create an account (today `/auth/email` creates an ACTIVE member, bypassing admin approval once registration is on).
- Switching tabs must clear error, notice, password and captcha token (a captcha token is single-use).
- Honeypot field stays off-screen and unfocusable; password manager must not autofill it.
- Narrow phone (≤ 480px): intro panel collapses, card fills width, no horizontal scroll, buttons stay ≥ 44px tall.

## File Structure

- Create `frontend/src/pages/login.css` — all auth-page styles (replaces `.login-*`, `.auth-link*`, `.login-guest`, `.login-notice`, `.login-captcha` rules scattered in `styles.css` ~L2226-2350, ~L3235-3258; keep `.hp-field`).
- Modify `frontend/src/pages/LoginPage.tsx` — tabs, field layout, password show/hide, button hierarchy; imports `./login.css`.
- Modify `frontend/src/styles.css` — delete the moved rules.
- Modify `frontend/src/lib/loginForm.ts` (+ `loginForm.test.ts`) — login body no longer carries `name`.
- Modify `backend/internal/service/auth.go` `EmailAuth` (+ `auth_email_test.go` / existing tests that assert creation) — only if the user chooses "close the bypass" (Task 3).
- Config only (no code): production env `ALLOW_REGISTRATION=true`, `TURNSTILE_SITE_KEY`, `TURNSTILE_SECRET` (Task 1).

## Task 1: Turn sign-up on in the deployed backend (ops, user-owned)

**Interfaces:** Produces `GET /api/v1/auth/options` → `{allowRegistration:true, captchaSiteKey:"<key>"}`.

- [ ] Verify the cause: `curl <backend>/api/v1/auth/options` returns `allowRegistration:false` today.
- [ ] User sets `ALLOW_REGISTRATION=true`, `TURNSTILE_SITE_KEY`, `TURNSTILE_SECRET` on the backend host and redeploys (`config.go:108` refuses to start with registration on and no captcha keys).
- [ ] Verify: the same `curl` now returns `allowRegistration:true` and a site key.

(I do not change production env or deploy; this step is the user's, or needs their explicit go-ahead.)

## Task 2: Tabbed, tidier auth card

**Files:** Create `frontend/src/pages/login.css`; Modify `LoginPage.tsx`, `styles.css`.
**Interfaces:** Consumes `options.allowRegistration`, `options.captchaSiteKey`, `buildEmailBody`, `buildRegisterBody` (unchanged names). Produces no new exports.

Layout of the card (top → bottom):
1. Segmented control `Đăng nhập | Đăng ký` (`role="tablist"`, buttons `role="tab"` with `aria-selected`), full width, 40px.
2. Heading + one muted line (login: "Đăng nhập để tiếp tục quản lý KOC."; register: "Tài khoản mới cần quản trị viên duyệt trước khi đăng nhập.").
3. Fields, label above input, 44px inputs, 16px gap: [Tên hiển thị — register only], Email, Mật khẩu (eye toggle inside the input; "Quên mật khẩu?" right-aligned on the label row, login only).
4. Captcha (register only), error / notice box.
5. Primary button `Đăng nhập` / `Tạo tài khoản`, full width, 44px, loading state.
6. Divider "hoặc", then `Xem Demo (Chế độ Khách)` as an outlined secondary button with an eye icon (not the same blue fill family as primary).
7. Footnote, muted, 12px.

- [ ] Step 1: Write `login.css` with the rules above; breakpoint `@media (max-width: 900px)` hides `.login-intro` copy and shows only the brand bar; `@media (max-width: 480px)` card padding 20px.
- [ ] Step 2: Rewrite `LoginPage.tsx` JSX to the layout above. Tabs always render; when `!options.allowRegistration` the Đăng ký tab shows the closed notice instead of the form. `switchMode` also clears `form.password` and `captchaToken`.
- [ ] Step 3: Remove the moved rules from `styles.css`; `grep -n "login-\|auth-link" frontend/src` shows no orphan class names.
- [ ] Step 4: `cd frontend && npm run build && npm test` pass.
- [ ] Step 5: Playwright (`browser_snapshot` first, one screenshot at 1280 and one at 390 wide): both tabs, closed-sign-up state (mock `/auth/options` → false), guest button, no horizontal scroll.
- [ ] Step 6: Commit `feat(auth): tabbed login/sign-up card, always-visible sign-up`.

## Task 3: Login must not create accounts (recommended)

**Files:** Modify `frontend/src/lib/loginForm.ts` + `loginForm.test.ts`, `LoginPage.tsx`; optionally `backend/internal/service/auth.go` (`EmailAuth`).
**Interfaces:** Produces `buildLoginBody(form: {email:string; password:string; website:string}): {email,password,website}`.

- [ ] Step 1: Failing test `buildLoginBody never contains name` and `buildLoginBody always sends website ""`. Run `npm test -- loginForm` → FAIL (not defined).
- [ ] Step 2: Implement `buildLoginBody` in `loginForm.ts`; login tab posts it to `/auth/email` (route kept so the honeypot test still holds).
- [ ] Step 3: Backend (Go test first): `TestEmailAuth_UnknownEmailDoesNotCreateAccount` — unknown email + valid password → same `domain.Invalid`/"sai thông tin" error as a wrong password, `users.FindByEmail` still nil afterwards. Then change `EmailAuth` (`auth.go` ~L294-320) to stop creating; remove the now-dead create branch and update the existing test that expects creation.
- [ ] Step 4: `cd backend && go build ./... && go vet ./... && go test ./internal/...` and `npm test` pass; `graphify update .`.
- [ ] Step 5: Commit `fix(auth): email login never creates an account; sign-up goes through /auth/register`.

## Self-review

- Spec coverage: "thêm lại đăng ký" → Task 1 (root cause) + Task 2 (always-visible tab); "giao diện đẹp hơn, button gọn gàng" → Task 2 layout; security consistency → Task 3.
- Names consistent: `buildLoginBody`, `buildRegisterBody`, `switchMode`, `options.allowRegistration`.
- Proportion: three tasks, no code bodies beyond signatures.
