# Kế hoạch: nhiều tài khoản Zalo cá nhân (giai đoạn 1)

Mục tiêu: đăng nhập nhiều tài khoản Zalo, nhận tin của tất cả vào một hộp thư, trả lời đúng tài khoản đã nhận.
Chiến dịch/workflow chọn **một** tài khoản gửi cố định (mặc định: tài khoản đầu tiên). Luân phiên để giai đoạn 2.

## Hiện trạng (đã đọc code)
- Bridge giữ 1 `ZaloSession` ([zalo-bridge/src/server.js:22](../zalo-bridge/src/server.js)); mọi route không có `accountId`.
- Cookie/IMEI/user-agent chỉ đến từ **biến môi trường** (`ZALO_PERSONAL_*`), `EnsureConnected` đăng nhập lại từ env
  ([platform/zalo_bridge.go:46](../backend/internal/platform/zalo_bridge.go)). Đăng nhập QR **không được lưu**: bridge khởi động lại là mất.
- `PersonalZalo` ([service/zalo.go:39](../backend/internal/service/zalo.go)) và `ZaloBridge` handler không có account.
- `Conversation` không biết thuộc tài khoản nào; inbound `/internal/zalo/inbound` không có `accountId`.
- Các chỗ gọi `Personal()`: inbox_zalo.go, outreach.go, zalo_link.go, campaign_runner.go, campaign_sequence.go, admin.go.

## Thiết kế
1. **Collection `zalo_accounts`** (Mongo): `_id, label, zaloUid, displayName, cookie, imei, userAgent, disabledByUser, isDefault, createdAt`.
   Cookie là bí mật: mã hóa khi lưu (khóa từ config), không bao giờ trả ra API.
2. **Seed**: lần chạy đầu, nếu có `ZALO_PERSONAL_*` trong env thì tạo tài khoản mặc định từ đó.
3. **Bridge**: `Map<accountId, ZaloSession>`. Mọi route nhận `accountId`. Thêm `GET /accounts`.
   Inbound kèm `accountId`. Khi đăng nhập QR thành công, bridge gọi `POST /internal/zalo/session` để backend lưu
   credentials (lấy từ `api.getCookie()` + context imei/userAgent của zca-js).
4. **Backend**
   - `PersonalZalo` nhận `accountID` ở mọi method. `ZaloBridge` giữ trạng thái backoff/disconnectedByUser **theo tài khoản**.
   - `KeepConnected` lặp qua tất cả tài khoản trong DB.
   - `Conversation.ZaloAccountID`. Hội thoại cũ (không có trường này) coi là tài khoản mặc định.
   - Gửi/react/kết bạn/tìm số dùng tài khoản của hội thoại.
5. **API**: `GET/POST /zalo/accounts`, `DELETE /zalo/accounts/:id`, `POST /zalo/accounts/:id/qr/start|disconnect|connect`, `GET .../status`.
   Giữ nguyên `/zalo/status`... (trỏ tài khoản mặc định) để frontend cũ không vỡ trong lúc chuyển.
6. **Frontend**: trang quản lý tài khoản (danh sách, trạng thái, quét QR, ngắt), bộ lọc hộp thư theo tài khoản,
   nhãn tài khoản trên mỗi hội thoại, ô chọn tài khoản trong chiến dịch.

## Thứ tự làm (mỗi bước chạy được và có test)
1. Bridge nhiều phiên + test (Node).
2. Domain + repo `zalo_accounts`, seed từ env, mã hóa cookie + test.
3. `PersonalZalo` nhận `accountID`, sửa toàn bộ chỗ gọi, `ZaloBridge` theo tài khoản + test (đã có zalo_reconnect_test, zalo_bridge_test).
4. `Conversation.ZaloAccountID`, upsert inbound theo (tài khoản, người liên hệ), migrate dữ liệu cũ.
5. API quản lý tài khoản + callback lưu session sau QR.
6. Frontend.
7. Chiến dịch/workflow: trường `zaloAccountId`.

## Cần xác nhận / rủi ro
- **Cùng một KOC nhắn cho 2 tài khoản của mình**: tách thành 2 hội thoại (theo tài khoản). Cần kiểm tra uid Zalo của KOC có giống nhau giữa các tài khoản không.
- Index trong Mongo của `UpsertZaloInbound` hiện có thể unique theo `externalContactId`: phải đổi thành (accountId, externalContactId).
- `LinkZaloContact` gắn KOC theo số điện thoại: không phụ thuộc tài khoản, giữ nguyên.
- Hạn mức gửi trong ngày hiện đếm toàn cục: để giai đoạn 2 đếm theo tài khoản.
- Lưu cookie vào DB là bí mật mới: cần khóa mã hóa trong env production.

## Tiến độ và những chỗ khác bản kế hoạch đầu
Đã xong cả 7 bước: 1 (bridge nhiều phiên), 2 (kho tài khoản + mã hóa), 3 (đăng nhập tự động theo từng tài khoản),
4 (hội thoại gắn tài khoản), 5 (API quản lý tài khoản + callback lưu cookie), 6 (giao diện hộp thư), 7 (chiến dịch/workflow chọn tài khoản + hạn mức riêng).

Khác với bản đầu:
- **Không có `ZALO_CRED_KEY`.** Cookie được mã hóa bằng khóa có sẵn của dự án (`TOKEN_ENCRYPTION_KEY`, hoặc `JWT_SECRET` khi chưa đặt),
  trộn thêm nhãn `zalo-account:`. Không cần biến env mới, và tài khoản quét QR luôn lưu được.
- **Tài khoản đi qua `context`, không đổi chữ ký `PersonalZalo`.** `domain.WithZaloAccount(ctx, id)`; `ZaloBridge` gắn nó vào header
  `X-Zalo-Account`; kho hội thoại đọc nó để lọc/đóng dấu. Nơi nào có hội thoại thì dùng `ZaloService.ForConv(ctx, conv)`.
  Hội thoại chưa có tài khoản dùng tài khoản mặc định.
- Tạo/xóa tài khoản chỉ **admin**; xem danh sách, quét QR, ngắt kết nối thì mọi thành viên (như route đơn tài khoản trước đây).

Route mới (backend): `GET/POST /zalo/accounts`, `DELETE /zalo/accounts/:id`, `GET .../status`, `POST .../qr/start|connect|disconnect`,
`POST /internal/zalo/session` (bridge -> backend, cần `X-Bridge-Secret`), `GET /inbox/conversations?account=`.
Route mới (bridge): `GET /accounts`, `POST /forget`; mọi route nhận tài khoản qua header `X-Zalo-Account`.

Chưa kiểm chứng với dịch vụ thật: đăng nhập Zalo thật bằng cookie xuất ra từ QR, và các test Mongo trong
`mongo_integration_test.go` (cần `MONGO_TEST_URI`).

## Bước 7 và việc xóa tài khoản: các quy tắc đã chọn
- **Xóa tài khoản:** bị từ chối (409) khi còn hội thoại (kể cả hội thoại đã ẩn) dùng tài khoản đó. Thứ tự: kiểm tra, rồi bridge quên phiên,
  rồi mới xóa, nên một lần xóa bị từ chối không làm tài khoản mất kết nối. Giao diện khóa nút Xóa và nói rõ lý do.
- **Một đợt gửi hàng loạt = một tài khoản** (`BulkRun.ZaloAccountID`; không chọn thì dùng tài khoản mặc định). Khối `send` / `send_text`
  của workflow có `zaloAccountId` riêng; tài khoản không tồn tại hoặc đã bị ngắt thì workflow bị từ chối trước khi bắt đầu.
- **Hạn mức tính riêng từng tài khoản:** số KOC mỗi ngày (giá trị admin đặt áp dụng cho *mỗi* tài khoản) và 600 tin mỗi ngày.
  Các đợt gửi cũ được gán về tài khoản mặc định khi khởi động để vẫn tính vào hạn mức của nó.
- **KOC đang nói chuyện qua tài khoản khác bị bỏ qua** (`on_other_account`), không bị nhắn từ số thứ hai. Muốn đổi thành "gửi tiếp qua tài khoản
  cũ của KOC" thì sửa điều kiện ở `openThread` (service/outreach.go).
- Vẫn như cũ (chưa đổi): chỉ một đợt gửi chạy tại một thời điểm trên toàn hệ thống; hạn mức tin chủ động của các tin lẻ (`maxProactivePerDay`)
  vẫn tính chung; chưa có chia đều/luân phiên tự động giữa các tài khoản.
