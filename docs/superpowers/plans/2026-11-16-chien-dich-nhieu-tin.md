# Chiến dịch nhiều tin gửi hàng loạt qua Zalo — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gửi một chuỗi nhiều tin (chữ và ảnh) cho tới 50 KOC, do máy chủ tự chạy chậm và không đều, có tự dừng, có theo dõi trực tiếp, và có trình sửa bước (tích, sắp xếp, sửa) trong cửa sổ "Gửi tin hàng loạt".

**Architecture:** Chiến dịch và ảnh lưu trong MongoDB. Bấm gửi tạo một `BulkRun` (bản chụp các bước + danh sách KOC); một goroutine trong backend (`CampaignRunner`) gửi từng KOC, từng bước qua `ZaloService`, ghi trạng thái từng bước trước và sau khi gửi. Trình duyệt chỉ tạo đợt và đọc tiến độ. Đường `POST /kocs/message-batch` và vòng lặp lô 5 bị gỡ.

**Tech Stack:** Go + Fiber, MongoDB, zalo-bridge (Node, `zca-js` 2.2), React 19 + TS + vitest.

**Spec:** `docs/superpowers/specs/2026-11-16-chien-dich-nhieu-tin-design.md`

## Global Constraints

- Không bao giờ gửi lặp một bước cho cùng một KOC: bước ghi `sending` trước khi gọi Zalo; mọi kết quả không chắc → `unknown`, không thử lại.
- Mỗi lúc một đợt chạy. Hạn mức KOC/ngày: một trong 10, 20, 30, 50; **mặc định 30**. Trần 600 tin chiến dịch/ngày. Khung 08:30–20:30 giờ VN (`IsWithinAllowedSendWindow`).
- Tin chiến dịch lưu `Proactive=false`: không tính vào `maxProactivePerDay = 80`.
- Nghỉ trước mỗi bước: ngẫu nhiên 4–15 giây; nghỉ giữa hai KOC: ngẫu nhiên 3–8 phút.
- Tối đa 12 bước/chiến dịch, bước chữ ≤ 2000 ký tự, ảnh JPG/PNG ≤ 2 MB, tối đa 50 KOC/đợt.
- Tạo đợt cần công tắc "Gửi hàng loạt" bật và `confirm: true`; chỉ admin bật công tắc, sửa chiến dịch, tải ảnh, đổi hạn mức. Công tắc tự tắt khi đợt kết thúc.
- Nội dung có sẵn của "Liên hệ KOC mới" lấy nguyên văn từ Phụ lục của spec. Lời văn do mình viết thêm theo quy tắc dự án: không "tặng", không khen "dùng thật", câu thường, không ngoặc giải thích.
- Giao diện dùng lại lớp `bm-*`, `ti-switch`; chữ tiếng Việt.
- Kiểm tra trước khi xong mỗi task: `cd backend && go vet ./... && go test ./...`; frontend: `cd frontend && npm test && npm run typecheck`. `gofmt -l` chạy trên blob đã stage (máy Windows báo sai do CRLF).
- Sau khi sửa mã: `graphify update .`.

## Review Focus

1. Máy chủ khởi động lại khi một bước đang `sending` → KOC đó thành `unknown`, đợt chạy tiếp KOC kế, không gửi lại bước đó. (Task 6)
2. KOC trả lời giữa chuỗi → các bước sau không gửi. (Task 5)
3. Admin tắt công tắc khi đang nghỉ 3–8 phút → đợt dừng ngay, không chờ hết giấc nghỉ. (Task 6)
4. Hai người bấm "Gửi" cùng lúc → chỉ một đợt được tạo, người kia nhận 409. (Task 4)
5. Bước ảnh trỏ tới ảnh đã bị xóa hoặc không tồn tại → từ chối lúc tạo đợt, không phải lúc gửi cho KOC thứ 20. (Task 7)

---

### Task 1: Gửi ảnh qua cầu nối Zalo

**Files:**
- Modify: `zalo-bridge/src/zalo.js`, `zalo-bridge/src/server.js`
- Modify: `backend/internal/service/zalo.go`, `backend/internal/platform/zalo_bridge.go`
- Test: `backend/internal/platform/zalo_bridge_test.go`, mọi fake của `PersonalZalo` trong `*_test.go`

**Interfaces:**
- Produces:
  - Bridge: `POST /send-image` body `{userId, filename, dataBase64, width, height, caption}` → `{delivered: true}`. Giới hạn thân request của riêng đường này là 4 MB.
  - `type ZaloImage struct { Filename string; Data []byte; Width, Height int; Caption string }` (package `service`)
  - `PersonalZalo.SendImage(ctx context.Context, userID string, img ZaloImage) error`
  - `func (z *ZaloService) SendImage(ctx context.Context, userID string, img ZaloImage) (ZaloDelivery, error)` — chỉ đi đường cá nhân; không kết nối thì `Delivered=false, Reason="personal_not_connected"`.
  - `var ErrZaloUnreachable = errors.New("zalo bridge unreachable")` (package `service`): `ZaloBridge.call` bọc lỗi vận chuyển (không nhận được phản hồi HTTP) bằng `%w` với lỗi này. Lỗi do bridge trả về (HTTP ≥ 300) thì không bọc.

- [ ] **Step 1: Test thất bại** trong `zalo_bridge_test.go`:
  - `TestSendImagePostsBase64AndSize`: httptest server giả `/status` (connected) và `/send-image`; gọi `SendImage` với `Data: []byte("abc")`, `Width: 2, Height: 3, Caption: "x"`; khẳng định body nhận được có `dataBase64 == "YWJj"`, `width == 2`, `height == 3`, `caption == "x"`.
  - `TestATransportFailureIsErrZaloUnreachable`: URL trỏ tới server đã đóng; `errors.Is(b.Send(...), service.ErrZaloUnreachable)` là true. Và server trả 500 `{"message":"x"}` → `errors.Is` là false.
- [ ] **Step 2:** `cd backend && go test ./internal/platform/ -run "SendImage|ErrZaloUnreachable"` — Expected: FAIL (không biên dịch: `SendImage` chưa có).
- [ ] **Step 3: Cài đặt.** Go: thêm kiểu, method, sentinel như trên; thêm `SendImage` vào mọi fake `PersonalZalo`. Bridge `zalo.js`:

```js
async sendImage(userId, { filename, dataBase64, width, height, caption }) {
	if (!this.isConnected()) throw new Error('Zalo cá nhân chưa kết nối.');
	const data = Buffer.from(String(dataBase64 || ''), 'base64');
	if (!data.length) throw new Error('Thiếu dữ liệu ảnh.');
	return this.api.sendMessage(
		{ msg: String(caption || ''), attachments: [{ data, filename: String(filename || 'image.png'), metadata: { totalSize: data.length, width: Number(width) || 0, height: Number(height) || 0 } }] },
		String(userId), ThreadType.User,
	);
}
```
  `server.js`: `readJson(request, limit = 1024 * 1024)`; đường `POST /send-image` đọc với 4 MB.
- [ ] **Step 4:** `go test ./...` — Expected: PASS.
- [ ] **Step 5: Thử thật (cần người dùng).** Chạy bridge + backend ở máy, người dùng quét QR, gọi `SendImage` với `backend/internal/campaignseed/anh-san-pham.png` tới chính tài khoản Zalo của người dùng (uid lấy qua `/find-user` với số của họ, hoặc một KOC quen). Expected: ảnh hiện trong Zalo. **Nếu không gửi được: dừng, báo người dùng; các task sau vẫn làm được nhưng bước ảnh bị khóa ở giao diện.** Ghi kết quả vào ledger.
- [ ] **Step 6:** Commit `feat(zalo): send an image through the bridge`.

### Task 2: Chiến dịch và ảnh — domain, kho, dữ liệu có sẵn

**Files:**
- Create: `backend/internal/domain/campaign.go`, `backend/internal/repository/campaign.go`, `backend/internal/campaignseed/seed.go` (cạnh hai file PNG đã có)
- Test: `backend/internal/domain/campaign_test.go`, `backend/internal/campaignseed/seed_test.go`, `backend/internal/repository/campaign_test.go` (tích hợp, bỏ qua khi thiếu `MONGO_TEST_URI`)

**Interfaces:**
- Produces:
```go
// domain
const ( StepText = "text"; StepImage = "image"; MaxCampaignSteps = 12; MaxStepText = 2000; MaxImageBytes = 2 << 20 )
type CampaignStep struct { Kind string `bson:"kind" json:"kind"`; Text string `bson:"text,omitempty" json:"text,omitempty"`; ImageID ID `bson:"imageId,omitempty" json:"imageId,omitempty"`; Caption string `bson:"caption,omitempty" json:"caption,omitempty"` }
type Campaign struct { ID ID `bson:"_id,omitempty" json:"_id"`; Name string `bson:"name" json:"name"`; Steps []CampaignStep `bson:"steps" json:"steps"`; UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"` }
type CampaignImage struct { ID ID `bson:"_id,omitempty" json:"_id"`; Filename, ContentType string; Width, Height, Size int; Data []byte `bson:"data" json:"-"`; CreatedAt time.Time }
func ValidateSteps(steps []CampaignStep) error // domain.Invalid(...) với câu tiếng Việt
// repository
func NewCampaignRepository(db *mongo.Database) *CampaignRepository   // collections "campaigns", "campaign_images"
func (r *CampaignRepository) List(ctx) ([]domain.Campaign, error)                       // theo name
func (r *CampaignRepository) Find(ctx, id string) (*domain.Campaign, error)             // NotFound
func (r *CampaignRepository) SaveSteps(ctx, id string, steps []domain.CampaignStep, at time.Time) error
func (r *CampaignRepository) AddImage(ctx, img *domain.CampaignImage) error
func (r *CampaignRepository) Image(ctx, id string) (*domain.CampaignImage, error)       // NotFound
func (r *CampaignRepository) ImagesExist(ctx, ids []domain.ID) (bool, error)
func (r *CampaignRepository) EnsureSeed(ctx, seed []campaignseed.Campaign) error        // chỉ khi "campaigns" rỗng
// campaignseed
type Step struct { Kind, Text, Caption, File string }; type Campaign struct { Name string; Steps []Step }
func Campaigns() []Campaign; func Image(file string) ([]byte, error) // go:embed *.png
```

- [ ] **Step 1: Test thất bại.**
  - `ValidateSteps`: rỗng → lỗi; 13 bước → lỗi; bước chữ toàn dấu cách → lỗi; chữ 2001 rune → lỗi; bước ảnh `ImageID` rỗng → lỗi; `Kind: "video"` → lỗi; 1 chữ + 1 ảnh hợp lệ → nil.
  - `campaignseed`: `Campaigns()` có đúng 3 chiến dịch tên "Liên hệ KOC mới", "Nhắc KOC phản hồi thông tin hợp tác", "Gửi thêm thông tin sản phẩm"; chiến dịch đầu có 9 bước, bước 4 và 5 (đếm từ 1) là ảnh `anh-san-pham.png`, `loi-moi-hop-tac.png`; bước 3 chứa `{kenh}` và không chứa `emilyhuong69`; bước 1 == `"Chào bạn"`; mọi file ảnh nêu trong seed đọc được bằng `Image` và ≤ `MaxImageBytes`; không bước chữ nào do mình viết chứa "tặng".
  - Repository (tích hợp): `EnsureSeed` hai lần → vẫn 3 chiến dịch; `SaveSteps` rồi `Find` trả đúng thứ tự; `ImagesExist` false khi một id lạ.
- [ ] **Step 2:** `go test ./internal/domain/ ./internal/campaignseed/` — Expected: FAIL (chưa có kiểu/hàm).
- [ ] **Step 3: Cài đặt.** Nội dung seed:
  - "Liên hệ KOC mới": 9 bước đúng Phụ lục của spec.
  - "Nhắc KOC phản hồi thông tin hợp tác": 1 bước chữ: `Chào {ten}, bên mình là Cooltech, hôm trước bên mình có gửi bạn thông tin hợp tác về sản phẩm khăn lạnh thể thao. Bạn xem qua rồi phản hồi giúp bên mình nha.`
  - "Gửi thêm thông tin sản phẩm": chữ `Mình gửi thêm thông tin sản phẩm để bạn tham khảo nha`, rồi hai ảnh.
  - `EnsureSeed` nạp ảnh trước (kích thước lấy bằng `image.DecodeConfig`), rồi chiến dịch trỏ tới id ảnh.
- [ ] **Step 4:** `go test ./...` — Expected: PASS (test tích hợp SKIP ở máy).
- [ ] **Step 5:** Commit `feat(campaign): campaigns of text and image steps, with the first three seeded`.

### Task 3: API chiến dịch và ảnh

**Files:**
- Create: `backend/internal/handler/campaign.go`; Modify: `backend/internal/router/router.go`, `backend/cmd/api/main.go`
- Test: `backend/internal/handler/campaign_test.go`, `backend/internal/router/campaign_route_test.go`, `router_test.go` (thêm handler vào `newApp`)

**Interfaces:**
- Consumes: `CampaignRepository` (Task 2) qua interface `CampaignStore` khai báo trong handler.
- Produces (dưới `/api/v1`, đều `requireAuth`):
  - `GET /campaigns` → `[]Campaign`
  - `PUT /campaigns/:id` (admin; `tooMany(60, time.Hour)`) body `{steps}` → `ValidateSteps` + `ImagesExist`; audit `campaign.update`
  - `POST /campaigns/images` (admin; `tooMany(30, time.Hour)`) multipart field `file` → 201 `{_id, width, height}`; chỉ nhận nội dung thật là PNG/JPEG (`http.DetectContentType` + `image.DecodeConfig`), ≤ 2 MB, nếu không → 400
  - `GET /campaigns/images/:id` → bytes, `Content-Type` đúng, `Cache-Control: private, max-age=86400`, `X-Content-Type-Options: nosniff`
- `main.go`: gọi `EnsureSeed` lúc khởi động (lỗi thì log, không chặn khởi động).

- [ ] **Step 1: Test thất bại:** không đăng nhập → 401 cả bốn đường; member `PUT` và `POST images` → 403; admin `PUT` với 0 bước → 400; admin `PUT` với ảnh không tồn tại → 400; tải file `.png` có nội dung là chữ → 400; tải PNG 1×1 hợp lệ → 201 và `GET` lại trả đúng bytes; file 2 MB + 1 byte → 400 hoặc 413.
- [ ] **Step 2:** `go test ./internal/handler/ ./internal/router/ -run Campaign` — Expected: FAIL.
- [ ] **Step 3:** Cài đặt theo mẫu `httpx.OK/Created/Fail` và `BindAndValidate` sẵn có. Kiểm tra `BodyLimit` của Fiber trong `router.go`; nếu nhỏ hơn 3 MB thì nâng riêng cho đường tải ảnh, không nâng toàn cục.
- [ ] **Step 4:** `go test ./...` — Expected: PASS.
- [ ] **Step 5:** Commit `feat(campaign): routes to read and edit campaigns and their images`.

### Task 4: Đợt gửi — domain và kho

**Files:**
- Create: `backend/internal/domain/bulk_run.go`, `backend/internal/repository/bulk_run.go`, `backend/internal/repository/bulk_limit.go`
- Test: `backend/internal/repository/bulk_run_test.go` (tích hợp)

**Interfaces:**
- Produces:
```go
const ( RunRunning = "running"; RunDone = "done"; RunStopped = "stopped" )
const ( TargetQueued = "queued"; TargetSending = "sending"; TargetSent = "sent"; TargetReplied = "replied"
        TargetSkipped = "skipped"; TargetFailed = "failed"; TargetUnknown = "unknown"; TargetNotSent = "not_sent" )
type RunTarget struct { KocID, KocName, Handle string; Status, Reason string; StepsSent int; StartedAt *time.Time }   // json camelCase
type BulkRun struct { ID ID; CampaignName string; Steps []CampaignStep; Targets []RunTarget; Status, StopReason string
                      By string; CreatedAt time.Time; NextAt, FinishedAt *time.Time }
// repository, collection "bulk_runs"
func (r *BulkRunRepository) Create(ctx, run *domain.BulkRun) error            // domain.Conflict khi đã có đợt running
func (r *BulkRunRepository) Running(ctx) (*domain.BulkRun, error)             // nil, nil khi không có
func (r *BulkRunRepository) Recent(ctx, limit int64) ([]domain.BulkRun, error) // createdAt giảm dần
func (r *BulkRunRepository) SetTarget(ctx, runID domain.ID, index int, t domain.RunTarget) error
func (r *BulkRunRepository) SetNextAt(ctx, runID domain.ID, at *time.Time) error
func (r *BulkRunRepository) Finish(ctx, runID domain.ID, status, reason string, at time.Time) error // target còn queued → not_sent + reason
func (r *BulkRunRepository) StartedSince(ctx, since time.Time) (kocs int, messages int, err error)   // target có startedAt >= since; messages = tổng stepsSent
// bulk_limit.go, doc "bulk_daily_limit" trong app_settings
func (r *BulkLimitRepository) Get(ctx) int            // 30 khi chưa đặt hoặc giá trị lạ
func (r *BulkLimitRepository) Set(ctx, n int) error   // chỉ 10, 20, 30, 50; khác → domain.Invalid
```
- "Một đợt running" được bảo đảm bằng unique index một phần: `{status: 1}` với `partialFilterExpression: {status: "running"}`.

- [ ] **Step 1: Test tích hợp thất bại:** `Create` hai đợt running → cái thứ hai là Conflict; `Finish` đổi mọi target `queued` thành `not_sent` kèm reason và cho phép `Create` đợt mới; `SetTarget` chỉ đổi đúng index; `StartedSince` đếm đúng 2 KOC, 11 tin từ dữ liệu dựng sẵn; `BulkLimit.Get` mặc định 30, `Set(25)` lỗi, `Set(50)` rồi `Get` == 50.
- [ ] **Step 2:** `go vet ./... && go test ./internal/repository/` — Expected: biên dịch được, test SKIP ở máy (thiếu `MONGO_TEST_URI`); ghi rõ trong ledger rằng chỉ CI chạy thật.
- [ ] **Step 3:** Cài đặt.
- [ ] **Step 4:** Commit `feat(bulk-run): a stored run of a campaign and the daily KOC limit`.

### Task 5: Gửi chuỗi cho một KOC

**Files:**
- Create: `backend/internal/service/campaign_sequence.go`; Modify: `backend/internal/service/outreach.go` (tách phần mở hội thoại), `outreach_batch.go` (`RenderTemplate`), `ports.go`, `backend/internal/repository/chat.go`
- Test: `backend/internal/service/campaign_sequence_test.go`

**Interfaces:**
- Consumes: `ZaloService.SendText/SendImage`, `ErrZaloUnreachable` (Task 1); `domain.CampaignStep`, `RunTarget` (Task 2, 4); `ClaimContact/ReleaseContact`.
- Produces:
```go
// RenderTemplate điền thêm {kenh} = "@" + koc.TiktokID ("" khi không có TiktokID).
// OutreachMsgs thêm:
InboundSince(ctx context.Context, convID domain.ID, since time.Time) (bool, error)
// outreach.go: phần đầu của deliverToKoc được tách ra, deliverToKoc gọi lại nó (hành vi cũ không đổi):
func (s *OutreachService) openThread(ctx context.Context, koc *domain.Koc, preview string) (conv *domain.Conversation, status string, err error) // status: "" | DeliveryNotOnZalo | DeliveryZaloNotReady
// campaign_sequence.go
type ImageLoader interface { Image(ctx context.Context, id string) (*domain.CampaignImage, error) }
type SequenceHooks struct {
    Pause    func(ctx context.Context) error                   // nghỉ trước mỗi bước; lỗi = bị dừng
    Progress func(t domain.RunTarget)                           // lưu trạng thái; gọi TRƯỚC và SAU mỗi bước
}
// SendSequence gửi các bước cho một KOC và trả trạng thái cuối. zaloFault = true khi dừng vì Zalo từ chối/không tới được.
func (s *OutreachService) SendSequence(ctx context.Context, koc *domain.Koc, steps []domain.CampaignStep, images ImageLoader, t domain.RunTarget, h SequenceHooks) (out domain.RunTarget, zaloFault bool)
```
- Quy tắc của `SendSequence` (theo thứ tự):
  1. `koc == nil` → skipped `not_found`; không số → skipped `no_phone`; `Dnc || DaHopTac` → skipped `dnc_or_blocked`.
  2. `ClaimContact(now-24h, now)`: lỗi → failed; không giữ được → skipped `already_sent_within_24h`.
  3. `openThread`: `DeliveryNotOnZalo` → release, skipped `not_on_zalo`; `DeliveryZaloNotReady` → release, not_sent `zalo_not_ready`, `zaloFault = true`.
  4. Đặt `StartedAt = now`. Với mỗi bước i: `h.Pause` (lỗi → trả về `not_sent`/`stopped` nếu `StepsSent == 0` thì release, ngược lại giữ `sent`-một-phần với `Reason: "stopped"`); `InboundSince(conv.ID, *StartedAt)` true → status `replied`, dừng; `h.Progress` với status `sending`; gửi (chữ: `RenderTemplate`; ảnh: nạp ảnh, chú thích cũng qua `RenderTemplate`); kết quả:
     - thành công → lưu `domain.Message{Direction: outbound, Channel: zalo, Text: nội dung chữ hoặc "[Ảnh] " + chú thích, Status: sent, Proactive: false}`, `StepsSent++`, `h.Progress`.
     - `errors.Is(err, ErrZaloUnreachable)` hoặc ctx hết hạn → status `unknown`, `Reason: "step_outcome_unknown"`, `zaloFault = true`, dừng.
     - lỗi khác hoặc `Delivered == false` → status `failed`, `Reason` = lỗi, `zaloFault = true`, dừng. Nếu `StepsSent == 0` thì release.
  5. Hết các bước → status `sent`; cập nhật `SetLastMessageAt` của hội thoại.
  - Lưu tin thất bại sau khi Zalo đã nhận: vẫn tính bước đã gửi (`StepsSent++`), `Reason: "message_not_recorded"`, tiếp tục.

- [ ] **Step 1: Test thất bại** (fake Zalo ghi lại thứ tự gọi; `Pause` không ngủ):
  - `TestSequenceSendsEveryStepInOrderAndClaimsOnce`: 3 bước (chữ, ảnh, chữ) → Zalo nhận đúng 3 lần đúng thứ tự, `ClaimContact` gọi 1 lần, kết quả `sent`, `StepsSent == 3`, chữ có `{ten}`/`{kenh}` đã thay.
  - `TestSequenceStopsWhenTheKocReplies`: `InboundSince` true từ bước 2 → Zalo nhận 1 lần, status `replied`, `StepsSent == 1`.
  - `TestProgressSaysSendingBeforeZaloIsCalled`: tại thời điểm fake Zalo được gọi, `Progress` gần nhất có status `sending`.
  - `TestAnUnreachableBridgeLeavesTheStepUnknownAndIsNotRetried`: bước 2 trả `ErrZaloUnreachable` → status `unknown`, Zalo nhận đúng 2 lần, `zaloFault`.
  - `TestARefusedFirstStepReleasesTheClaim`: bước 1 lỗi → `failed`, `ReleaseContact` gọi 1 lần.
  - `TestAStopDuringThePauseSendsNothingMore`: `Pause` trả lỗi ở bước 2 → Zalo nhận 1 lần, `Reason == "stopped"`.
  - `TestCampaignMessagesDoNotSpendTheProactiveQuota`: sau một chuỗi 3 bước, `dayCount` không đổi và các tin lưu có `Proactive == false`.
  - `TestRenderTemplateFillsTheChannel`: `"{kenh}"` với `TiktokID: "abc"` → `"@abc"`.
- [ ] **Step 2:** `go test ./internal/service/ -run "Sequence|Campaign|RenderTemplate"` — Expected: FAIL.
- [ ] **Step 3:** Cài đặt; chạy lại toàn bộ test cũ của `deliverToKoc` để chắc việc tách `openThread` không đổi hành vi.
- [ ] **Step 4:** `go test ./...` — Expected: PASS.
- [ ] **Step 5:** Commit `feat(outreach): send a sequence of steps to one KOC, stopping on a reply`.

### Task 6: Tiến trình nền chạy đợt

**Files:**
- Create: `backend/internal/service/campaign_runner.go`; Modify: `backend/internal/service/automation.go` (thêm khả năng tắt công tắc), `backend/cmd/api/main.go`
- Test: `backend/internal/service/campaign_runner_test.go`

**Interfaces:**
- Consumes: `SendSequence` (Task 5), kho đợt và hạn mức (Task 4).
- Produces:
```go
type RunStore interface { /* đúng các method của BulkRunRepository ở Task 4 */ }
type DailyLimit interface { Get(ctx context.Context) int }
type BulkSwitch interface { IsOn(ctx context.Context) bool; TurnOff(ctx context.Context) error } // bọc repo công tắc sẵn có
const ( MaxRunKocs = 50; MaxCampaignMessagesPerDay = 600 )
var ErrRunInProgress = domain.Conflict("Đang có một đợt gửi chạy. Hãy chờ đợt đó xong hoặc dừng nó.")
func NewCampaignRunner(o *OutreachService, runs RunStore, images ImageLoader, limit DailyLimit, sw BulkSwitch, kocs func(ctx context.Context, id string) (*domain.Koc, error)) *CampaignRunner
func (r *CampaignRunner) Start(ctx context.Context, by, campaignName string, steps []domain.CampaignStep, kocs []*domain.Koc) (*domain.BulkRun, error) // ErrBulkOff, ErrRunInProgress; tạo đợt rồi `go r.work`
func (r *CampaignRunner) Resume(ctx context.Context) // lúc khởi động
func (r *CampaignRunner) Estimate(kocs, steps int) time.Duration // kocs * (steps*9.5s + 5.5 phút)
// test thay được: r.sleep func(ctx, d) error; r.between func(min, max time.Duration) time.Duration
```
- Vòng `work` cho từng target `queued`, trước mỗi KOC kiểm tra theo thứ tự, điều kiện đầu tiên sai thì `Finish(stopped, reason)`:
  `switch_off` · `outside_0830_2030_window` · `zalo_not_ready` (`!IsConnected`) · `daily_koc_limit_reached` (`kocs >= limit.Get`) · `daily_message_cap_reached` (`messages + len(steps) > 600`).
  Rồi nạp lại KOC theo id (bản mới nhất), gọi `SendSequence` với `Pause` = ngủ `between(4s, 15s)` nhưng thức dậy mỗi ≤ 5 giây để xem công tắc (tắt → trả lỗi), `Progress` = `SetTarget`.
  Sau mỗi KOC: `zaloFault` hai KOC liên tiếp → `Finish(stopped, "zalo_errors")`; một KOC không lỗi đặt lại bộ đếm. KOC `skipped` không nghỉ dài. Ngược lại `SetNextAt(now + between(3m, 8m))`, ngủ từng nhịp ≤ 5 giây có xem công tắc.
  Hết danh sách → `Finish(done, "")`. **Mọi lối ra đều gọi `sw.TurnOff`.**
- `Resume`: có đợt running → mọi target `sending` thành `unknown` (`Reason: "restart_during_step"`), rồi `go r.work`. Công tắc đang tắt → `Finish(stopped, "switch_off")`.
- `work` được bọc `recover`: panic → `Finish(stopped, "internal_error")` + log.

- [ ] **Step 1: Test thất bại** (`sleep` giả trả ngay và ghi lại thời lượng; đồng hồ giả trong khung giờ):
  - `TestARunSendsToEveryKocThenTurnsTheSwitchOff`: 3 KOC × 2 bước → 6 lần gửi, đợt `done`, công tắc tắt, `NextAt` được đặt 2 lần với thời lượng trong [3, 8] phút.
  - `TestStartIsRefusedWhileTheSwitchIsOff` → `ErrBulkOff`; `TestASecondStartIsRefused` → `ErrRunInProgress`.
  - `TestTurningTheSwitchOffDuringTheLongPauseStopsTheRun`: tắt công tắc trong lần ngủ dài đầu tiên → KOC 2, 3 là `not_sent` lý do `switch_off`, Zalo không nhận thêm.
  - `TestTwoZaloFaultsInARowStopTheRun`; `TestAFaultThenASuccessDoesNotStop`.
  - `TestTheDailyKocLimitStopsTheRun`: limit 10, đã có 9 KOC hôm nay, đợt 3 KOC → 1 gửi, 2 `not_sent` `daily_koc_limit_reached`.
  - `TestTheRunStopsOutsideTheWindow`: đồng hồ nhảy sang 20:31 sau KOC 1.
  - `TestResumeMarksAStepInFlightUnknownAndGoesOn`: đợt có target 0 `sending` (StepsSent 1), target 1 `queued` → sau `Resume`: target 0 `unknown`, Zalo chỉ nhận các bước của target 1.
  - `TestASkippedKocDoesNotCostALongPause`.
- [ ] **Step 2:** `go test ./internal/service/ -run "Run|Resume|Start"` — Expected: FAIL.
- [ ] **Step 3:** Cài đặt. Test đợi goroutine bằng kênh `done` do runner đóng khi `work` kết thúc (chỉ lộ ra cho test trong cùng package), không dùng `time.Sleep`.
- [ ] **Step 4:** `go test ./...` — Expected: PASS. `main.go`: dựng runner, gọi `Resume` sau khi kết nối Zalo từ env.
- [ ] **Step 5:** Commit `feat(bulk-run): a background runner that paces a campaign and stops itself`.

### Task 7: API đợt gửi; gỡ đường lô 5

**Files:**
- Create: `backend/internal/handler/bulk_run.go`; Modify: `router.go`, `main.go`, `handler/koc.go` (gỡ `SendMessageBatch`, `messageBatchRequest`, `recordBatch`, `SendTemplateBatch` khỏi `OutreachAPI`), `service/outreach_batch.go` (gỡ `SendTemplateBatch`, giữ `RenderTemplate`), xóa các test của đường cũ
- Test: `backend/internal/handler/bulk_run_test.go`, `backend/internal/router/bulk_run_route_test.go` (thay `message_batch_route_test.go`, giữ hai test về công tắc và nhật ký nếu còn đúng)

**Interfaces:**
- Produces (dưới `/api/v1`, `requireAuth`):
  - `POST /bulk-runs` (`tooMany(10, time.Hour)`) body `{campaignName string required max=120, steps []CampaignStep required, kocIds []string required min=1 max=50, confirm bool}` → 201 `{run, estimateMinutes}`. Thứ tự kiểm tra: `confirm` (400) → `ValidateSteps` (400) → `ImagesExist` (400) → bỏ id trùng → nạp KOC (id không thấy vẫn vào đợt, runner ghi `not_found`) → `runner.Start` (`ErrBulkOff` → 409, `ErrRunInProgress` → 409). Audit `bulk_run.start` chỉ ghi số lượng.
  - `GET /bulk-runs/current` → `{run: BulkRun | null}`
  - `GET /bulk-runs?limit=20` → `[]BulkRun`
  - `GET /bulk-limit` → `{value}`; `PUT /bulk-limit` (admin, `tooMany(20, time.Hour)`) body `{value}` → 400 nếu không thuộc 10/20/30/50; audit `bulk_limit.set`.
- Dừng đợt = `PUT /bulk-switch {enabled:false}` sẵn có; không thêm đường mới.
- `GET /kocs/message-log` và bảng `bulk_send_log`: giữ đường đọc để xem các đợt cũ; không ghi thêm.

- [ ] **Step 1: Test thất bại:** không đăng nhập → 401 mọi đường; thiếu `confirm` → 400 và runner không được gọi; 0 bước / 13 bước / 51 KOC → 400; ảnh không tồn tại → 400; công tắc tắt → 409; đang có đợt → 409; hợp lệ → 201, runner nhận đúng thứ tự bước và danh sách KOC đã bỏ trùng; member `PUT /bulk-limit` → 403; admin `PUT {value: 25}` → 400; `POST /kocs/message-batch` → 404 hoặc 405.
- [ ] **Step 2:** `go test ./internal/handler/ ./internal/router/ -run "BulkRun|BulkLimit|MessageBatch"` — Expected: FAIL.
- [ ] **Step 3:** Cài đặt và gỡ mã cũ.
- [ ] **Step 4:** `go vet ./... && go test ./...` — Expected: PASS, không còn tham chiếu `SendTemplateBatch`.
- [ ] **Step 5:** Commit `feat(bulk-run): routes to start and follow a run; the batch-of-five route is gone`.

### Task 8: Logic trình sửa bước (frontend)

**Files:**
- Create: `frontend/src/features/inbox/campaignSteps.ts`; Test: `frontend/src/features/inbox/campaignSteps.test.ts`
- Modify: `frontend/src/features/kocs/bulkMessage.ts` (gỡ `runBulkMessage`, `MSG_BATCH`, `SUGGESTED_TEMPLATES`, `groupRuns` và test của chúng; giữ `buildRecipients`, `filterRecipients`, `reasonText`)

**Interfaces:**
- Produces:
```ts
export type Step = { kind: 'text' | 'image'; text?: string; imageId?: string; caption?: string }
export type DraftStep = Step & { key: string; on: boolean }
export const MAX_STEPS = 12, MAX_STEP_TEXT = 2000
export function toDraft(steps: Step[]): DraftStep[]                      // mọi bước on, trừ ảnh thiếu imageId
export function toggleStep(d: DraftStep[], key: string): DraftStep[]     // ảnh thiếu imageId không bật được
export function moveStep(d: DraftStep[], from: number, to: number): DraftStep[] // ngoài khoảng → trả nguyên
export function editStep(d: DraftStep[], key: string, patch: Partial<Step>): DraftStep[]
export function addStep(d: DraftStep[], kind: Step['kind']): DraftStep[] // không vượt MAX_STEPS
export function removeStep(d: DraftStep[], key: string): DraftStep[]
export function stepsToSend(d: DraftStep[]): Step[]                      // chỉ bước on, đúng thứ tự, bỏ key/on
export function stepProblem(d: DraftStep[]): string | null               // câu tiếng Việt hoặc null
export function previewLine(s: Step, koc: { name: string; handle: string }): string // {ten}, {kenh} đã thay; ảnh → "Ảnh" + chú thích
export function estimateMinutes(kocs: number, steps: number): number     // khớp Runner.Estimate
export function targetText(status: string, reason?: string): string      // nhãn tiếng Việt cho mọi status/reason của Task 4–6
export function sendBlockReason(i: { bulkOn: boolean; isAdmin: boolean; selected: number; problem: string | null; running: boolean; limitLeft: number }): string | null
```
- `stepProblem`: không bước nào bật → "Hãy tích ít nhất một tin."; bước chữ đang bật mà rỗng → "Tin số N đang trống."; quá 2000 ký tự → "Tin số N dài quá 2000 ký tự."
- `{ten}` trong xem trước dùng cùng quy tắc với máy chủ: từ cuối của tên, không có tên thì "bạn".

- [ ] **Step 1: Test thất bại** cho từng hàm, gồm: `moveStep([a,b,c], 0, 2)` → `[b,c,a]`; `toggleStep` trên ảnh thiếu `imageId` không đổi gì; `stepsToSend` bỏ bước tắt và giữ thứ tự sau khi `moveStep`; `stepProblem` ba trường hợp trên, và bước rỗng nhưng đang tắt thì không báo; `previewLine` thay `{kenh}` bằng handle; `estimateMinutes(30, 9)` trong [200, 215]; `sendBlockReason` trả lý do đúng thứ tự ưu tiên: đang có đợt chạy → công tắc tắt → chưa chọn người nhận → vấn đề của bước → vượt hạn mức còn lại; `targetText` có nhãn cho cả 8 status và mọi reason liệt kê ở Task 5–6.
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/inbox/campaignSteps.test.ts` — Expected: FAIL (module chưa có).
- [ ] **Step 3:** Cài đặt hàm thuần, không đụng React.
- [ ] **Step 4:** `npm test && npm run typecheck` — Expected: PASS (typecheck có thể đỏ ở `BulkMessageModal.tsx` vì các export đã gỡ; nếu vậy gộp commit với Task 9).
- [ ] **Step 5:** Commit `feat(inbox): the logic of the campaign step editor`.

### Task 9: Giao diện tab "Soạn tin"

**Files:**
- Create: `frontend/src/features/inbox/CampaignStepList.tsx`
- Modify: `frontend/src/features/inbox/BulkMessageModal.tsx`, `frontend/src/styles.css`

**Interfaces:**
- Consumes: mọi export của Task 8; API Task 3 và 7.
- Produces: `<CampaignStepList steps={DraftStep[]} onChange={(d: DraftStep[]) => void} isAdmin={boolean} preview={{name, handle}} onUpload={(file: File) => Promise<string>} imageUrl={(id: string) => string | undefined} />`

Hành vi (bố cục đúng bản vẽ trong spec, mục Giao diện):
- Dropdown "Chiến dịch" (`GET /campaigns`) thay các nút mẫu gợi ý và ô soạn một tin. Đổi chiến dịch → `toDraft` lại.
- Mỗi dòng: tay nắm kéo (HTML5 drag-and-drop, không thêm thư viện), ô tích `bm-check`, số thứ tự, một dòng xem trước (`previewLine`, cắt bằng CSS), nút ✎ ↑ ↓ (`aria-label` tiếng Việt; ↑ của dòng đầu và ↓ của dòng cuối bị vô hiệu). Dòng tắt mờ đi.
- ✎ mở ô soạn tại chỗ (mỗi lúc một dòng): textarea, bộ đếm `/2000`, nút chèn `{ten}`, `{kenh}` tại vị trí con trỏ; bước ảnh: hình nhỏ, ô chú thích, nút "Đổi ảnh" (admin).
- Ảnh hiển thị bằng blob tải qua axios (`responseType: 'blob'`) rồi `URL.createObjectURL`, vì `<img src>` không gửi được token; thu hồi URL khi đóng cửa sổ.
- Admin: "+ Thêm tin chữ", "+ Thêm ảnh", nút xóa bước, "Lưu làm mặc định" (`PUT /campaigns/:id` với toàn bộ bước theo thứ tự hiện tại, kể cả bước đang tắt).
- Chân cửa sổ: công tắc như cũ; dòng tóm tắt "N KOC · M tin mỗi người · khoảng X giờ Y phút · hôm nay còn Z KOC" (`GET /bulk-limit`, `GET /bulk-runs?limit=20` để tính đã dùng hôm nay); admin có chọn hạn mức 10/20/30/50.
- "Gửi" → hộp xác nhận nêu số KOC, số tin, thời gian ước tính → `POST /bulk-runs` → chuyển sang tab Theo dõi. Lỗi 409/400/429 hiện bằng `apiMessage`.
- Đóng cửa sổ: chỉ tắt công tắc khi **không** có đợt đang chạy.

- [ ] **Step 1:** Cài đặt component và sửa modal. Logic mới phát sinh nào không thuần hiển thị thì đưa về `campaignSteps.ts` kèm test trước.
- [ ] **Step 2:** `npm test && npm run typecheck && npm run build` — Expected: PASS.
- [ ] **Step 3: Xem thật bằng trình duyệt** với API giả (cách đã dùng ở đợt trước): tích/bỏ tích, kéo đổi thứ tự, ↑ ↓, sửa một tin, chèn `{kenh}`, xem số đếm "7/9", bấm Gửi và kiểm tra thân request `POST /bulk-runs` chỉ chứa bước đang bật theo đúng thứ tự. Chụp màn hình ở 1366 px và 1920 px; cửa sổ không tràn, danh sách bước cuộn bên trong.
- [ ] **Step 4:** Commit `feat(inbox): pick a campaign and tick, order and edit its messages`.

### Task 10: Tab "Theo dõi" trực tiếp

**Files:**
- Modify: `frontend/src/features/inbox/BulkMessageModal.tsx`, `frontend/src/styles.css`, `frontend/src/features/inbox/campaignSteps.ts` (+ test)

**Interfaces:**
- Produces trong `campaignSteps.ts`:
```ts
export type RunView = { done: number; total: number; current?: { name: string; step: number; steps: number }; nextInMinutes?: number; counts: Record<string, number> }
export function runView(run: BulkRun, now: Date): RunView
export function stopText(reason: string): string
```

Hành vi:
- Khi tab mở và có đợt `running`: gọi `GET /bulk-runs/current` mỗi 5 giây (dừng khi đợt kết thúc hoặc đóng cửa sổ). Thẻ trên cùng: "KOC 12/50 · bước 4/9 · KOC kế tiếp sau khoảng 5 phút", thanh tiến độ, nút "Dừng đợt" (admin; có hỏi xác nhận; gọi `PUT /bulk-switch {enabled:false}`).
- Đợt đã dừng: hiện lý do bằng `stopText`. Bên dưới: các đợt cũ (`GET /bulk-runs`), mở ra thấy từng KOC: "x/y tin", nhãn `targetText`, lý do. KOC `unknown`, `failed`, `replied` xếp lên đầu.
- Đợt cũ kiểu lô 5 (`GET /kocs/message-log`) không còn hiện; ghi vào ledger là thay đổi có chủ ý.

- [ ] **Step 1: Test thất bại** cho `runView`: đợt 3 KOC (sent, sending với `stepsSent: 1` trên 4 bước, queued) → `done: 1, total: 3, current.step: 2, current.steps: 4`; `nextAt` sau 4 phút 20 giây → `nextInMinutes: 5`; `nextAt` đã qua → `undefined`. `stopText` có câu cho mọi lý do dừng của Task 6.
- [ ] **Step 2:** `npx vitest run src/features/inbox/campaignSteps.test.ts` — Expected: FAIL.
- [ ] **Step 3:** Cài đặt.
- [ ] **Step 4:** `npm test && npm run typecheck && npm run build` — Expected: PASS. Xem thật bằng trình duyệt với API giả trả lần lượt ba trạng thái của một đợt; chụp màn hình.
- [ ] **Step 5:** Commit `feat(inbox): follow a running campaign live and stop it`.

### Task 11: Kiểm tra toàn bộ

- [ ] **Step 1:** `cd backend && go vet ./... && go test ./...`; `cd frontend && npm test && npm run typecheck && npm run build` — Expected: tất cả PASS.
- [ ] **Step 2:** `gofmt -l` trên blob đã stage — Expected: không file nào.
- [ ] **Step 3: Chạy thật ở máy (cần người dùng quét QR):** hạn mức 10, đợt 2 KOC quen với chiến dịch "Liên hệ KOC mới" đủ 9 bước. Kiểm: đủ 9 tin đúng thứ tự, ảnh hiện, tab Theo dõi đúng; nhờ một KOC trả lời giữa chuỗi và xem chuỗi dừng; tắt công tắc giữa lúc nghỉ và xem đợt dừng. Ghi kết quả thật vào báo cáo cuối; phần nào không thử được thì nói rõ.
- [ ] **Step 4:** `graphify update .`; cập nhật spec nếu có quyết định lệch.
