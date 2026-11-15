# Gửi tin nhắn hàng loạt theo mẫu qua Zalo: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Từ trang Danh bạ KOC, chọn một mẫu tin, bỏ tích vài KOC, rồi gửi cho số còn lại qua Zalo theo từng lô nhỏ, tuân theo bộ chặn spam của dự án.

**Architecture:** Endpoint mới `POST /api/v1/kocs/message-batch` nhận tối đa 5 KOC mỗi lần; dịch vụ `OutreachService` kiểm tra công tắc tổng, rồi với từng KOC chạy bộ chặn spam sẵn có và gửi qua Zalo. Giao diện chia các KOC đã tích thành lô 5, gọi lần lượt, hiện tiến độ và dừng khi server báo dừng (cùng mô hình với nhập Excel).

**Tech Stack:** Go + Fiber (backend, test chuẩn `go test`), React 19 + TypeScript + axios + vitest (frontend).

**Spec:** `docs/superpowers/specs/2026-11-15-gui-tin-hang-loat-theo-mau-design.md`

## Global Constraints

- Route: `POST /api/v1/kocs/message-batch`, trong nhóm `/kocs` (có sẵn `requireAuth`), thêm `tooMany(40, 10*time.Minute)`.
- Thân yêu cầu: `{ "body": string, "kocIds": string[], "confirm": true }`. `body` không rỗng sau khi cắt khoảng trắng và tối đa 2000 ký tự; `kocIds` từ 1 đến 5 phần tử; `confirm` bắt buộc `true`.
- Cỡ lô ở giao diện: 5 KOC mỗi yêu cầu, gửi lần lượt, không song song.
- Bộ chặn spam dùng nguyên `CanSendProactiveMessage(koc, false)`: 80 tin/ngày cả hệ thống, khung 08:30 đến 20:30 giờ Việt Nam, không gửi lại trong 24 giờ, bỏ qua KOC `Dnc` hoặc `DaHopTac`. Không bỏ qua khung giờ.
- Trạng thái mỗi KOC: `sent`, `pending`, `skipped`, `not_sent`, `failed`. Lý do: `not_found`, `no_phone`, `not_on_zalo`, `dnc_or_blocked`, `already_sent_within_24h`, `daily_quota_reached`, `outside_0830_2030_window`, `zalo_not_ready`, `save_failed`.
- Công tắc tổng tắt: trả 409 với `service.ErrAutomationOff`, không gửi gì.
- Mặc định bỏ tích KOC đã liên hệ trong 7 ngày qua (từ 24 giờ đến dưới 7 ngày); KOC liên hệ trong 24 giờ, `dnc`, `daHopTac`, hoặc không có số điện thoại thì mờ, không tích được.
- Chữ `{ten}` thay bằng chữ cuối trong tên KOC, hoặc "bạn" khi không có tên.
- Lô lỗi mạng hoặc quá thời gian chờ: không tự gửi lại, các KOC của lô đó hiện "không rõ kết quả", dừng cả đợt.
- Giao diện chỉ thấy 100 KOC đầu (`limit: 100` ở `App.tsx`), đợt này không đổi giới hạn đó.
- Không gửi thật cho Zalo trong test tự động. Chỉ commit khi người dùng đồng ý.

## Review Focus

- **Một KOC có mặt hai lần trong cùng yêu cầu** không được nhận hai tin (Task 4 test `TestMessageBatchSendsEachKocOnlyOnceEvenIfListedTwice`).
- **KOC không có tên** nhận lời chào "Chào bạn" chứ không phải "Chào ," (Task 1 test `TestRenderTemplateFallsBackToBanWhenTheNameIsEmpty`).
- **Zalo mất kết nối giữa chừng**: tin đầu `pending` thì cả đợt dừng, không tiêu thêm hạn mức (Task 3 test `TestAPendingMessageStopsTheRestOfTheBatch`).
- **KOC có số điện thoại nhưng không có tài khoản Zalo** không được báo "đã gửi" và không tốn hạn mức (Task 2 test `TestRequireZaloSendsNothingToAThreadWithoutAZaloAccount`, Task 3 test `TestAKocWithoutAZaloAccountIsSkippedAndCostsNoQuota`).
- **Hết hạn mức giữa lô**: KOC còn lại của lô là `not_sent` và giao diện không gọi lô tiếp theo (Task 3 test `TestTheQuotaRunningOutStopsTheBatch`, Task 6 test `stops calling the server when it says stop`).

---

### Task 1: Thay `{ten}` và kiểu dữ liệu của lô

**Files:**
- Create: `backend/internal/service/outreach_batch.go`
- Test: `backend/internal/service/outreach_batch_test.go`

**Interfaces:**
- Produces (Task 3, 4 dùng):
  - `func RenderTemplate(body string, koc *domain.Koc) string`
  - hằng `BatchSent = "sent"`, `BatchPending = "pending"`, `BatchSkipped = "skipped"`, `BatchNotSent = "not_sent"`, `BatchFailed = "failed"`
  - `type BatchTarget struct { ID string; Koc *domain.Koc }` (`Koc == nil` nghĩa là không có KOC đang hoạt động với id đó)
  - `type BatchResult struct { KocID string `json:"kocId"`; Status string `json:"status"`; Reason string `json:"reason,omitempty"` }`
  - `type BatchReport struct { Results []BatchResult `json:"results"`; Stop bool `json:"stop"`; StopReason string `json:"stopReason,omitempty"` }`

- [ ] **Step 1: Viết test thất bại** trong `outreach_batch_test.go`:
  - `TestRenderTemplateUsesTheLastWordOfTheName`: `DisplayName: "Nguyễn Thị Lan"`, body `"Chào {ten}, {ten} ơi"` cho `"Chào Lan, Lan ơi"`.
  - `TestRenderTemplateFallsBackToBanWhenTheNameIsEmpty`: `DisplayName: "  "`, body `"Chào {ten}!"` cho `"Chào bạn!"`.
  - `TestRenderTemplateLeavesATemplateWithoutTheTagUntouched`: body `"Xin chào"` cho `"Xin chào"`.
- [ ] **Step 2: Chạy, xác nhận thất bại.** Run: `cd backend && go test ./internal/service/ -run RenderTemplate -v`. Expected: FAIL "undefined: RenderTemplate".
- [ ] **Step 3: Cài `RenderTemplate` và các kiểu trên** trong `outreach_batch.go`. Quy tắc tên: chữ cuối của `strings.Fields(koc.DisplayName)`, rỗng thì `"bạn"`, thay mọi `{ten}` bằng `strings.ReplaceAll`.
- [ ] **Step 4: Chạy, xác nhận qua.** Run: cùng lệnh. Expected: PASS.
- [ ] **Step 5: Commit** (khi được đồng ý): `git add backend/internal/service/outreach_batch.go backend/internal/service/outreach_batch_test.go && git commit -m "feat(outreach): render {ten} per KOC and batch result types"`

### Task 2: Đồng hồ thay thế được và gửi "chỉ khi có Zalo"

**Files:**
- Modify: `backend/internal/service/outreach.go` (struct `OutreachService` dòng 24-37; `CanSendProactiveMessage` dòng 115-129; `DeliverMessageToKoc` dòng 201-239)
- Test: `backend/internal/service/outreach_requirezalo_test.go` (mới)

**Interfaces:**
- Consumes: Task 1 không cần. Dùng `fakeZaloLine`, `fakeKnownThread`, `deliveryService`, `fakeOutreachMsgs`, `fakeOutreachConvs` có sẵn trong `outreach_delivery_test.go` và `chatbot_flow_test.go`.
- Produces (Task 3 dùng):
  - trường `clock func() time.Time` trong `OutreachService` và `func (s *OutreachService) now() time.Time` (trả `time.Now()` khi `clock == nil`); `CanSendProactiveMessage` dùng `s.now()` cho khung giờ và quy tắc 24 giờ
  - `const DeliveryNotOnZalo = "not_on_zalo"`
  - `func (s *OutreachService) deliverToKoc(ctx context.Context, koc *domain.Koc, text string, isProactive, requireZalo bool) (*Delivery, error)`; `DeliverMessageToKoc` giữ nguyên chữ ký và gọi `deliverToKoc(..., false)`

- [ ] **Step 1: Viết test thất bại** trong `outreach_requirezalo_test.go`:
  - `TestTheSendWindowFollowsTheServiceClock`: `svc := &OutreachService{msgs: &fakeOutreachMsgs{}}`; `svc.clock` cố định 22:00 giờ Việt Nam (`time.Date(2026, 11, 16, 22, 0, 0, 0, vnZone)`) cho `CanSendProactiveMessage(domain.NewKoc(), false)` có `Reason == "outside_0830_2030_window"`; đổi sang 10:00 cho `Allowed == true`.
  - `TestTheTwentyFourHourRuleFollowsTheServiceClock`: clock 10:00; KOC có `LastContactedAt` cách 23 giờ cho `already_sent_within_24h`; cách 25 giờ cho `Allowed`.
  - `TestRequireZaloSendsNothingToAThreadWithoutAZaloAccount`: dùng kiểu mới `fakeInternalThread` (nhúng `OutreachConvs`, `UpsertKocMessage` trả `&domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelInternal}` không có `ContactPhone` nên không tra Zalo); gọi `deliverToKoc(ctx, koc, "Chào", true, true)`: `Status == DeliveryNotOnZalo`, `len(msgs.all) == 0`, `svc.dayCount == 0`.
  - `TestRequireZaloStillSendsWhenTheThreadIsOnZalo`: `deliveryService(&fakeZaloLine{connected: true})`, `requireZalo=true` cho `Status == domain.MessageSent`, một tin trong kho, `line.sent` có một phần tử.
- [ ] **Step 2: Chạy, xác nhận thất bại.** Run: `cd backend && go test ./internal/service/ -run "ServiceClock|RequireZalo" -v`. Expected: FAIL (không biên dịch: `clock`, `deliverToKoc`).
- [ ] **Step 3: Cài** trường `clock`, hàm `now()`, đổi `CanSendProactiveMessage` sang `s.now()` (cả `IsWithinAllowedSendWindow` và `time.Since`), tách thân `DeliverMessageToKoc` thành `deliverToKoc` với `requireZalo`: sau khối tra Zalo theo số điện thoại và **trước** khi tính `status`, nếu `requireZalo` và hội thoại không phải (`Channel == domain.ChannelZalo && ExternalContactID != ""`) thì trả `&Delivery{Conversation: conv, Status: DeliveryNotOnZalo}, nil` (không tạo tin, không `countProactive`). Giới hạn đã biết: `UpsertKocMessage` chạy trước đó nên dòng xem trước của hội thoại KOC không có Zalo bị ghi đè bằng chữ mẫu; chấp nhận ở đợt này.
- [ ] **Step 4: Chạy toàn bộ test của gói service.** Run: `cd backend && go test ./internal/service/ -v 2>&1 | tail -20`. Expected: PASS, các test cũ (`outreach_delivery_test.go`, `outreach_quota_test.go`) vẫn qua.
- [ ] **Step 5: Commit** (khi được đồng ý): `git add backend/internal/service/outreach.go backend/internal/service/outreach_requirezalo_test.go && git commit -m "feat(outreach): injectable clock and a Zalo-only delivery mode"`

### Task 3: `SendTemplateBatch`

**Files:**
- Modify: `backend/internal/service/outreach_batch.go`
- Test: `backend/internal/service/outreach_batch_test.go`

**Interfaces:**
- Consumes: Task 1 (`RenderTemplate`, `BatchTarget`, `BatchResult`, `BatchReport`, hằng `Batch*`); Task 2 (`clock`, `deliverToKoc`, `DeliveryNotOnZalo`); có sẵn `ErrAutomationOff`, `s.automationOn(ctx)`, `CanSendProactiveMessage`, `NormalizePhone`, `s.kocs.Save`.
- Produces (Task 4 dùng): `func (s *OutreachService) SendTemplateBatch(ctx context.Context, body string, targets []BatchTarget) (*BatchReport, error)`

Thứ tự xử lý: tắt công tắc thì trả `nil, ErrAutomationOff`. Với từng target theo thứ tự: `Koc == nil` thì `skipped` `not_found`; `NormalizePhone(koc.Phone) == ""` thì `skipped` `no_phone`; `CanSendProactiveMessage(koc, false)` không cho phép: lý do `daily_quota_reached` hoặc `outside_0830_2030_window` thì KOC này và mọi KOC sau là `not_sent` kèm lý do đó, `Stop = true`, `StopReason` là lý do đó, dừng; lý do khác thì `skipped` với lý do của bộ chặn. Rồi `deliverToKoc(ctx, koc, RenderTemplate(body, koc), true, true)`: lỗi thì `failed` (reason là `err.Error()`), đi tiếp; `DeliveryNotOnZalo` thì `skipped` `not_on_zalo`; `"blocked_dnc"` thì `skipped` `dnc_or_blocked`; `domain.MessagePending` thì `pending`, các KOC sau là `not_sent` `zalo_not_ready`, `Stop`, dừng (không ghi `LastContactedAt`); `domain.MessageSent` thì đặt `koc.LastContactedAt = &now` (`now` từ `s.now()`) và `s.kocs.Save`; lưu lỗi thì giữ `sent` kèm `Reason: "save_failed"`, các KOC sau `not_sent` `save_failed`, `Stop`, dừng.

- [ ] **Step 1: Viết test thất bại.** Tạo trợ giúp test `batchService(line *fakeZaloLine, gateOn bool) (*OutreachService, *fakeOutreachKocs, *fakeOutreachMsgs)`: `OutreachService{kocs: kocs, convs: fakeKnownThread{}, msgs: msgs, zalo: NewZaloService(config.Config{}, line, nil), gate: fakeGate{on: gateOn}, clock: 10:00 giờ Việt Nam}`, với `fakeGate{on bool}` có `Enabled(context.Context) (bool, error)`. Hàm `kocWith(name, phone string) *domain.Koc` đặt `DisplayName`, `Phone`. Các test:
  - `TestABatchIsRefusedWhenTheAutomationSwitchIsOff`: `errors.Is(err, ErrAutomationOff)`, `len(msgs.all) == 0`.
  - `TestAllEligibleKocsAreSentAndMarkedContacted`: 3 KOC có số: ba kết quả `sent`; mỗi `LastContactedAt != nil`; `kocs.saves == 3`; `line.sent` có 3 chuỗi, chuỗi đầu có tên đã thay.
  - `TestEachSkipReasonIsReportedAndTheRestStillSend`: các KOC: `Koc: nil`, không điện thoại, `Dnc: true`, `LastContactedAt` cách 2 giờ, và một KOC hợp lệ cho các kết quả `skipped` với lý do `not_found`, `no_phone`, `dnc_or_blocked`, `already_sent_within_24h`, và `sent`; chỉ một tin trong kho.
  - `TestTheQuotaRunningOutStopsTheBatch`: `msgs` có sẵn `maxProactivePerDay-1` tin chủ động hôm nay (dùng `sentToday` có sẵn): KOC đầu `sent`, KOC thứ hai và thứ ba `not_sent` `daily_quota_reached`, `Stop == true`, `StopReason == "daily_quota_reached"`.
  - `TestOutsideTheSendWindowNothingIsSentAndTheBatchStops`: clock 22:00: mọi KOC `not_sent` `outside_0830_2030_window`, `Stop`, không tin nào.
  - `TestAKocWithoutAZaloAccountIsSkippedAndCostsNoQuota`: thread `fakeInternalThread` (Task 2): `skipped` `not_on_zalo`, `len(msgs.all) == 0`, `svc.dayCount == 0`.
  - `TestAPendingMessageStopsTheRestOfTheBatch`: `fakeZaloLine{connected: false}`: KOC đầu `pending`, các KOC sau `not_sent` `zalo_not_ready`, `Stop`, `LastContactedAt == nil`, `kocs.saves == 0`.
  - `TestAFailedSaveKeepsSentButStopsTheBatch`: kho KOC có `Save` trả lỗi: KOC đầu `sent` với `Reason == "save_failed"`, KOC sau `not_sent` `save_failed`, `Stop`.
  - `TestAStoreErrorForOneKocIsFailedAndTheNextOneIsStillSent`: thread của KOC đầu trả lỗi (`flakyThreads` trả lỗi cho handle `@bad`): kết quả `failed` kèm lý do, rồi `sent`.
- [ ] **Step 2: Chạy, xác nhận thất bại.** Run: `cd backend && go test ./internal/service/ -run "ABatch|EligibleKocs|SkipReason|QuotaRunningOut|SendWindow|WithoutAZalo|APendingMessage|AFailedSave|StoreErrorForOne" -v`. Expected: FAIL "undefined: SendTemplateBatch".
- [ ] **Step 3: Cài `SendTemplateBatch`** theo thứ tự xử lý nêu trên, trong `outreach_batch.go`. Mỗi `BatchResult.KocID` là `target.ID`.
- [ ] **Step 4: Chạy, xác nhận qua.** Run: `cd backend && go test ./internal/service/ -v 2>&1 | tail -30 && gofmt -l internal/service && go vet ./internal/service/`. Expected: PASS, không in tên file từ `gofmt`.
- [ ] **Step 5: Commit** (khi được đồng ý): `git add backend/internal/service && git commit -m "feat(outreach): send a template to a batch of KOCs through the anti-spam guard"`

### Task 4: Handler `SendMessageBatch`

**Files:**
- Modify: `backend/internal/handler/koc.go` (interface `OutreachAPI` dòng 39-47; thêm kiểu yêu cầu và hàm xử lý cạnh `RunOutreach` dòng 353)
- Test: `backend/internal/handler/koc_message_batch_test.go` (mới); có thể thêm trường `details []string` vào `fakeAudit` ở `handlers_iface_test.go` nếu nó chỉ giữ `actions`

**Interfaces:**
- Consumes: Task 3 `SendTemplateBatch`, `BatchTarget`, `BatchReport`; có sẵn `KocAPI.FindActive`, `h.audit.Log`, `middleware.ActorOf`, `httpx.BindAndValidate`, `httpx.OK`, `httpx.NewError`.
- Produces (Task 5 dùng): `func (h *KocHandler) SendMessageBatch(c *fiber.Ctx) error`; thêm vào `OutreachAPI`: `SendTemplateBatch(ctx context.Context, body string, targets []service.BatchTarget) (*service.BatchReport, error)`.

Kiểu yêu cầu: `type messageBatchRequest struct { Body string `json:"body" validate:"required,max=2000"`; KocIDs []string `json:"kocIds" validate:"required,min=1,max=5,dive,required"`; Confirm bool `json:"confirm"` }`.

- [ ] **Step 1: Viết test thất bại.** Dựng `batchApp(api *batchKocAPI, out *batchOutreach)` giống `bulkApp` trong `koc_bulk_test.go`, đăng ký `app.Post("/kocs/message-batch", NewKocHandler(api, out, nil, audit).SendMessageBatch)`. `batchKocAPI` nhúng `KocAPI` và trả `FindActive` là KOC có sẵn hoặc `domain.NotFound("x")` cho id trong danh sách "không có"; `batchOutreach` nhúng `OutreachAPI`, ghi lại `body` và `targets`, trả `BatchReport` đặt sẵn (hoặc lỗi đặt sẵn). Các test:
  - `TestMessageBatchNeedsConfirmAndABody`: thiếu `confirm` cho 400; `confirm:true` với `body:"   "` cho 400; cả hai không gọi dịch vụ.
  - `TestMessageBatchRefusesMoreThanFiveKocsOrNone`: 6 id cho 400; mảng rỗng cho 400.
  - `TestMessageBatchRefusesABodyOverTwoThousandCharacters`: 2001 ký tự (ký tự có dấu) cho 400; 2000 ký tự cho 200.
  - `TestMessageBatchHandsTheLoadedKocsToTheService`: id `a` có KOC, id `b` không có: dịch vụ nhận `[]BatchTarget{{ID:"a", Koc: koc}, {ID:"b", Koc: nil}}` và `body` đã cắt khoảng trắng hai đầu.
  - `TestMessageBatchSendsEachKocOnlyOnceEvenIfListedTwice`: ids `["a","a","b"]` cho dịch vụ nhận đúng 2 target, giữ thứ tự.
  - `TestMessageBatchAnswersWithTheServicesReport`: phản hồi 200, `data.results[0].status == "sent"`, `data.stop` đúng như dịch vụ trả.
  - `TestMessageBatchPassesTheSwitchOffErrorThrough`: dịch vụ trả `service.ErrAutomationOff` cho mã 409.
  - `TestMessageBatchLogsOneAuditLineWithTheCountsNotTheText`: `audit` có đúng một hành động `outreach.message_batch`, phần chi tiết chứa số `sent`/`skipped`/..., và **không** chứa nội dung `body`.
- [ ] **Step 2: Chạy, xác nhận thất bại.** Run: `cd backend && go test ./internal/handler/ -run MessageBatch -v`. Expected: FAIL (không biên dịch: `SendMessageBatch`).
- [ ] **Step 3: Cài** hàm xử lý: `BindAndValidate`; `Confirm` phải true (400 "Cần xác nhận trước khi gửi tin hàng loạt."); cắt khoảng trắng `Body`, rỗng thì 400; bỏ id trùng giữ thứ tự; mỗi id gọi `h.kocs.FindActive`: lỗi là `*domain.Fault` có `Kind == domain.FaultNotFound` thì `Koc: nil`, lỗi khác thì trả ngay; gọi `h.outreach.SendTemplateBatch`; ghi một dòng kiểm toán `h.audit.Log(nil, a.ID, a.Email, "outreach.message_batch", "", "", chi_tiết_đếm, a.IP)`; trả `httpx.OK(c, report)`.
- [ ] **Step 4: Chạy, xác nhận qua, và biên dịch cả dự án** (các bản giả khác có thể cần thêm phương thức). Run: `cd backend && go build ./... && go vet ./... && go test ./internal/handler/ -v 2>&1 | tail -20`. Expected: PASS.
- [ ] **Step 5: Commit** (khi được đồng ý): `git add backend/internal/handler && git commit -m "feat(kocs): POST /kocs/message-batch handler"`

### Task 5: Route và giới hạn tốc độ

**Files:**
- Modify: `backend/internal/router/router.go` (cạnh dòng `kocs.Post("/bulk", ...)`, khoảng dòng 174)
- Test: `backend/internal/router/ratelimit_test.go` (thêm một dòng vào bảng `TestRoutesThatSpendMoneyOrReachRealPeopleAreRateLimited`); `backend/internal/router/message_batch_route_test.go` (mới)

**Interfaces:**
- Consumes: Task 4 `h.Koc.SendMessageBatch`; `tooMany`; `newApp`, `do` trong `router_test.go` (Mongo giả không kết nối, chỉ đi được các đường không chạm database).
- Produces: route `POST /api/v1/kocs/message-batch`.

- [ ] **Step 1: Viết test thất bại.** Thêm vào bảng `ratelimit_test.go` dòng `{"bulk template message", "POST", "/api/v1/kocs/message-batch", `{}`, 40}` (thân `{}` không qua kiểm tra dữ liệu nên không chạm database). Trong `message_batch_route_test.go`: `TestMessageBatchNeedsALogin` (không token cho 401) và `TestMessageBatchRefusesAnUnconfirmedRequestForAMember` (token thành viên, thân `{"body":"Chào","kocIds":["x"]}` cho 400).
- [ ] **Step 2: Chạy, xác nhận thất bại.** Run: `cd backend && go test ./internal/router/ -run "MessageBatch|RateLimited" -v`. Expected: FAIL (404 vì chưa có route).
- [ ] **Step 3: Thêm dòng** `kocs.Post("/message-batch", tooMany(40, 10*time.Minute), h.Koc.SendMessageBatch)` ngay sau dòng `/bulk`, kèm chú thích một dòng nói đây là gửi tới người thật. Đường dẫn chỉ có một đoạn nên không đụng các route `/:id/...`.
- [ ] **Step 4: Chạy, xác nhận qua.** Run: `cd backend && go test ./internal/router/ -v 2>&1 | tail -20`. Expected: PASS.
- [ ] **Step 5: Commit** (khi được đồng ý): `git add backend/internal/router && git commit -m "feat(kocs): route and rate limit for message-batch"`

### Task 6: Logic thuần của gửi hàng loạt ở giao diện

**Files:**
- Create: `frontend/src/features/kocs/bulkMessage.ts`
- Test: `frontend/src/features/kocs/bulkMessage.test.ts`

**Interfaces:**
- Consumes: `Creator` từ `frontend/src/types/koc.ts` (Task 7 thêm `lastContactedAt?: string | null`; viết test dùng ép kiểu `as Creator` trong lúc chờ).
- Produces (Task 8 dùng):
  - `export const MSG_BATCH = 5`, `export const MSG_RECENT_DAYS = 7`, `export const MSG_MAX_BODY = 2000`
  - `export type MsgStatus = 'queued' | 'sent' | 'pending' | 'skipped' | 'not_sent' | 'failed' | 'unknown'`
  - `export interface Recipient { creator: Creator; checked: boolean; blocked?: string; recentDaysAgo?: number }`
  - `export interface SendRow { id: string; name: string; status: MsgStatus; reason?: string }`
  - `export interface BatchResponse { results: { kocId: string; status: string; reason?: string }[]; stop: boolean; stopReason?: string }`
  - `export function reasonText(reason?: string): string`
  - `export function renderPreview(body: string, creator: Creator): string`
  - `export function buildRecipients(creators: Creator[], now?: Date): Recipient[]`
  - `export async function runBulkMessage(rows: SendRow[], post: (kocIds: string[]) => Promise<BatchResponse>, opts: { batchSize?: number; shouldStop: () => boolean; onProgress: (done: number, total: number, rows: SendRow[]) => void }): Promise<SendRow[]>`

- [ ] **Step 1: Viết test thất bại** (vitest, mẫu như `bulkImport.test.ts`):
  - `renderPreview`: tên "Nguyễn Thị Lan" cho "Chào Lan"; tên rỗng cho "Chào bạn".
  - `buildRecipients` (với `now` cố định): `dnc` cho `blocked: 'Không liên hệ'`, `checked: false`; `daHopTac` cho `'Đã hợp tác'`; thiếu `phone` cho `'Chưa có số điện thoại'`; `lastContactedAt` cách 2 giờ cho `'Vừa liên hệ trong 24 giờ'`; cách 3 ngày cho `checked: false`, `recentDaysAgo: 3`, không `blocked`; cách 10 ngày hoặc chưa liên hệ cho `checked: true`.
  - `runBulkMessage`: `sends in order in batches of 5` (12 id cho các lần gọi `post` có 5, 5, 2 id, lần lượt); `merges the server's results by kocId` (kết quả `sent`, `skipped` kèm `reasonText` của lý do); `stops calling the server when it says stop` (lô đầu trả `stop: true`, `stopReason: 'daily_quota_reached'`, `post` chỉ gọi 1 lần, các dòng còn lại `not_sent` với `reasonText('daily_quota_reached')`); `stops when shouldStop turns true` (các dòng chưa gửi `not_sent`, lý do `'Chưa gửi, đã dừng'`); `a network error marks the batch unknown and stops` (lỗi không có `response`: dòng của lô là `unknown` với lý do `'Không rõ kết quả, hãy kiểm tra hội thoại'`, các dòng sau `not_sent`); `a 5xx answer is also unknown`; `a 409 or 429 answer marks the batch not_sent with the server's message (429: 'Thao tác quá nhanh, hãy thử lại sau ít phút') and stops`; `an id missing from the response is unknown`.
- [ ] **Step 2: Chạy, xác nhận thất bại.** Run: `cd frontend && npx vitest run src/features/kocs/bulkMessage.test.ts`. Expected: FAIL (không tìm thấy module).
- [ ] **Step 3: Cài** theo chữ ký trên. `reasonText` có bảng: `not_found` "Không tìm thấy KOC", `no_phone` "Chưa có số điện thoại", `not_on_zalo` "Số điện thoại không có tài khoản Zalo", `dnc_or_blocked` "Không liên hệ hoặc đã hợp tác", `already_sent_within_24h` "Vừa liên hệ trong 24 giờ", `daily_quota_reached` "Hết hạn mức gửi hôm nay, hãy gửi tiếp vào ngày mai", `outside_0830_2030_window` "Ngoài khung giờ gửi 08:30 đến 20:30", `zalo_not_ready` "Zalo chưa sẵn sàng, tin chưa tới KOC", `save_failed` "Đã gửi nhưng chưa ghi được lần liên hệ"; lý do không có trong bảng thì trả nguyên chuỗi. Lỗi có `response.status` trong {400, 401, 403, 404, 409, 422, 429} là server từ chối trước khi gửi (dòng của lô `not_sent`); mọi lỗi khác (không có phản hồi, 5xx, quá thời gian chờ) là `unknown`.
- [ ] **Step 4: Chạy, xác nhận qua.** Run: cùng lệnh. Expected: PASS.
- [ ] **Step 5: Commit** (khi được đồng ý): `git add frontend/src/features/kocs/bulkMessage.ts frontend/src/features/kocs/bulkMessage.test.ts && git commit -m "feat(kocs): pure logic for the bulk message window"`

### Task 7: Cột "Liên hệ lần cuối"

**Files:**
- Modify: `frontend/src/types/koc.ts` (interface `Creator`, thêm `lastContactedAt?: string | null`); `frontend/src/lib/formatters.ts` (thêm hàm); `frontend/src/features/kocs/KocDirectoryPage.tsx` (tiêu đề cột và ô, quanh dòng 195-240)
- Test: `frontend/src/lib/formatters.test.ts` (mới)

**Interfaces:**
- Produces: `export function formatLastContacted(value?: string | null, now?: Date): string`; trường `Creator.lastContactedAt`. Dữ liệu đã có sẵn ở runtime vì `mapItemToCreator` sao chép nguyên `...item`.

- [ ] **Step 1: Viết test thất bại**: `formatLastContacted(undefined)` và `(null)` cho `'Chưa liên hệ'`; cách 2 giờ cho `'Hôm nay'`; cách 3 ngày cho `'3 ngày trước'`; cách 45 ngày cho `'dd/MM/yyyy'` của ngày đó (đặt `now` cố định, dùng ngày giờ Việt Nam); chuỗi không phải ngày cho `'Chưa liên hệ'`.
- [ ] **Step 2: Chạy, xác nhận thất bại.** Run: `cd frontend && npx vitest run src/lib/formatters.test.ts`. Expected: FAIL.
- [ ] **Step 3: Cài** hàm, thêm trường kiểu, thêm `<th>LIÊN HỆ LẦN CUỐI</th>` sau cột "PHÍ CAST" và `<td>{formatLastContacted(creator.lastContactedAt)}</td>` tương ứng trong `KocDirectoryPage.tsx`.
- [ ] **Step 4: Chạy, xác nhận qua.** Run: `cd frontend && npx vitest run src/lib/formatters.test.ts && npm run typecheck`. Expected: PASS, không lỗi kiểu.
- [ ] **Step 5: Commit** (khi được đồng ý): `git add frontend/src && git commit -m "feat(kocs): last-contacted column in the KOC directory"`

### Task 8: Cửa sổ "Gửi tin hàng loạt" và nối vào trang

**Files:**
- Create: `frontend/src/features/kocs/BulkMessageModal.tsx`
- Modify: `frontend/src/features/kocs/KocDirectoryPage.tsx` (prop mới và nút cạnh "Nhập từ Excel", dòng 145-151); `frontend/src/App.tsx` (trạng thái, cạnh `isBulkOpen` dòng 41 và dòng 374)

**Interfaces:**
- Consumes: Task 6 (`buildRecipients`, `renderPreview`, `runBulkMessage`, `MSG_MAX_BODY`, `reasonText`, kiểu); `SavedTemplate` trong `types/koc.ts`; API có sẵn `GET /inbox/templates` (`data.data: SavedTemplate[]`) và `GET /automation` (`data.data.enabled: boolean`); `api` từ `lib/axios.ts`; kiểu của `showToast` lấy y như `BulkImportModal`.
- Produces: `interface BulkMessageModalProps { creators: Creator[]; onClose: () => void; onDone: () => void; showToast: <cùng kiểu với BulkImportModal> }`; `KocDirectoryPageProps.onOpenBulkMessage: (creators: Creator[]) => void` (trang gọi nó với `filteredCreators`).

Hành vi của cửa sổ (theo mục "Giao diện" của spec): bước 1 chọn mẫu và xem trước cho KOC đầu danh sách, có thể sửa chữ trước khi gửi (không sửa mẫu đã lưu), đếm `MSG_MAX_BODY`; bước 2 bảng người nhận với ô tích (`buildRecipients`), KOC bị chặn mờ kèm lý do, KOC liên hệ trong 7 ngày bỏ tích sẵn kèm "đã liên hệ N ngày trước", nút Chọn tất cả/Bỏ chọn tất cả, dòng "Sẽ gửi x tin", nút Gửi bị khóa khi x = 0, khi công tắc tắt, hoặc khi đang chạy; bước 3 thanh tiến độ "30/80" và nút Dừng; bước 4 báo cáo theo từng KOC (đã gửi, chưa gửi, bỏ qua, không rõ kết quả) kèm lý do từ `reasonText`, và `onDone()` được gọi để danh bạ tải lại. Công tắc tắt: nút Gửi khóa, hiện "Công tắc gửi tự động đang tắt, nhờ quản trị viên bật". Mỗi yêu cầu: `api.post('/kocs/message-batch', { body, kocIds, confirm: true })` rồi lấy `data.data` làm `BatchResponse`.

- [ ] **Step 1: Viết cửa sổ** theo mô tả trên, cùng kiểu lớp CSS với `BulkImportModal.tsx` (ví dụ `tm-btn-cancel`); không đóng cửa sổ khi đang gửi (giống `BulkImportModal.tsx:113`).
- [ ] **Step 2: Nối vào trang**: thêm prop `onOpenBulkMessage` và nút "Gửi tin hàng loạt" trong `dir-actions`; trong `App.tsx` thêm `const [bulkMessageCreators, setBulkMessageCreators] = useState<Creator[] | null>(null)`, truyền `onOpenBulkMessage={(list) => (apiReady ? setBulkMessageCreators(list) : showToast('Cần kết nối máy chủ để gửi tin hàng loạt.', 'error'))}`, và hiển thị `{bulkMessageCreators && <BulkMessageModal creators={bulkMessageCreators} onClose={() => setBulkMessageCreators(null)} onDone={refreshAll} showToast={showToast} />}`.
- [ ] **Step 3: Kiểm tra kiểu và test.** Run: `cd frontend && npm run typecheck && npm test`. Expected: không lỗi kiểu, mọi test qua.
- [ ] **Step 4: Chạy thử bằng tay** (backend và frontend đang chạy, công tắc tổng bật, một tài khoản Zalo thử): mở Danh bạ KOC, bấm "Gửi tin hàng loạt"; kiểm tra: danh sách khớp bộ lọc; KOC không có số điện thoại hiện mờ; bỏ tích một KOC rồi số "Sẽ gửi" giảm 1; Gửi cho 2 KOC thử và thấy hội thoại có tin; sau đó cột "Liên hệ lần cuối" của 2 KOC đó là "Hôm nay"; mở lại cửa sổ thì 2 KOC đó mờ ("Vừa liên hệ trong 24 giờ"); tắt công tắc tổng rồi mở cửa sổ thì nút Gửi khóa kèm thông báo. Ghi lại kết quả từng mục.
- [ ] **Step 5: Commit** (khi được đồng ý): `git add frontend/src && git commit -m "feat(kocs): bulk message window on the KOC directory"`

### Task 9: Kiểm tra cuối và cập nhật tài liệu

**Files:**
- Modify: `docs/superpowers/specs/2026-11-15-gui-tin-hang-loat-theo-mau-design.md` (dòng "Trạng thái")

- [ ] **Step 1: Chạy toàn bộ kiểm tra backend.** Run: `cd backend && gofmt -l . && go vet ./... && go test ./...`. Expected: `gofmt` không in gì, `vet` sạch, mọi test qua (test cần Mongo thật có thể bị bỏ qua tùy cấu hình, ghi rõ nếu vậy).
- [ ] **Step 2: Chạy toàn bộ kiểm tra frontend.** Run: `cd frontend && npm run typecheck && npm test && npm run build`. Expected: không lỗi.
- [ ] **Step 3: Cập nhật đồ thị tri thức theo quy định dự án.** Run: `graphify update .` (ở thư mục gốc dự án). Expected: in "Code graph updated".
- [ ] **Step 4: Cập nhật dòng "Trạng thái"** của bản thiết kế thành "đã duyệt, đã làm (chưa chạy thử với Zalo thật)" nếu Task 8 Step 4 chưa chạy với Zalo thật, hoặc "đã chạy thử" kèm ngày nếu đã chạy.
- [ ] **Step 5: Commit** (khi được đồng ý): `git add docs graphify-out && git commit -m "docs: mark the bulk template message design as built"`
