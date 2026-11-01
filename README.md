# Tiktoday Booking (KocFlow)

Quản lý & tự động hóa booking KOC: danh bạ KOC, hộp thư Zalo, chatbot phân loại ý định, nhắc lịch tự động, theo dõi vận đơn J&T, đồng bộ kênh TikTok.

| Thành phần | Công nghệ |
|---|---|
| `backend/` | Go 1.26 · Fiber v2 · MongoDB |
| `frontend/` | React 19 · TypeScript · Vite |
| `zalo-bridge/` | Node 22 sidecar bọc `zca-js` (Zalo cá nhân) |

## Cấu trúc

```text
backend/
├── cmd/
│   ├── api/main.go          # entrypoint HTTP API (wiring + graceful shutdown)
│   └── seed/main.go         # dữ liệu mẫu
└── internal/
    ├── config/              # đọc biến môi trường
    ├── database/            # kết nối Mongo + index
    ├── domain/              # entity: Koc, Conversation, Message, User
    ├── repository/          # truy cập MongoDB
    ├── service/             # nghiệp vụ: auth, outreach, chatbot, followup, sweep, koc, jt, tiktok, zalo, email
    ├── handler/             # Fiber handlers (mỏng) + validate request
    ├── middleware/          # JWT + kiểm tra phiên, phân quyền theo vai trò, bridge secret
    ├── metrics/             # bộ đếm request cho trang giám sát
    ├── router/              # đăng ký route /api/v1
    ├── scheduler/           # job nền: quét 4h/lần, tóm tắt 08:00
    └── httpx/               # response + error handler chung
frontend/src/
├── features/{inbox,kocs}/   # màn hình theo tính năng
├── layouts/ pages/ stores/ lib/ types/
zalo-bridge/src/             # server.js (HTTP) + zalo.js (phiên Zalo)
```

## Chạy nhanh toàn bộ hệ thống (1 Lệnh duy nhất)

```bash
# Cách 1: Chạy song song cả 3 dịch vụ trong 1 Terminal (Khuyên dùng)
npm run dev

# Cách 2 (Windows): Đúp chuột vào file dev.bat hoặc gõ trong terminal:
.\dev.bat

# Cách 3 (PowerShell):
.\dev.ps1
```

* Backend Go-Fiber cũng được tích hợp cơ chế **tự động kích hoạt Zalo Bridge ngầm** nếu bạn chỉ chạy `cd backend && go run ./cmd/api`.

## Chạy bằng Docker

```bash
cp .env.example .env     # điền JWT_SECRET, ZALO_BRIDGE_SECRET ...
docker compose up --build
docker compose exec backend ./seed   # (tuỳ chọn) dữ liệu mẫu — admin@tiktoday.local / Admin12345!
```

- Frontend: http://localhost:5173 · API health: http://localhost:5000/health · Mongo Express: http://localhost:8081

## Chạy local (dev)

```bash
# 1. MongoDB đang chạy ở 127.0.0.1:27017
# 2. API
cd backend && cp .env.example .env && make run      # hoặc: go run ./cmd/api
make seed                                           # dữ liệu mẫu
make test                                           # unit + HTTP tests
make test-live                                      # gọi thật TikTok / J&T (cần internet)

# 3. Zalo bridge (chỉ cần khi dùng Zalo cá nhân)
cd zalo-bridge && cp .env.example .env && npm install && npm start

# 4. Frontend
cd frontend && cp .env.example .env && npm install && npm run dev   # npm run typecheck | build
```

## Zalo

- **Zalo OA**: đặt `ZALO_OA_ACCESS_TOKEN`; webhook `POST /api/v1/webhooks/zalo`.
- **Zalo cá nhân**: Go không có thư viện tương đương `zca-js`, nên phần này chạy trong `zalo-bridge` (HTTP nội bộ, bảo vệ bằng `ZALO_BRIDGE_SECRET`). API Go gọi bridge để đăng nhập (cookie/IMEI hoặc QR), tìm user theo SĐT, gửi tin; bridge đẩy tin đến về `POST /internal/zalo/inbound` để lưu + chạy chatbot. Giao diện và endpoint `/api/v1/zalo/*` không đổi.

## API (`/api/v1`, JWT trừ auth & webhook)

`POST /auth/register|login` · `GET /auth/me` · `GET /zalo/status` · `POST /zalo/qr/start|connect|disconnect` ·
`GET|POST /webhooks/zalo` ·
`GET /kocs` · `POST /kocs` · `POST /kocs/lookup|bulk|outreach/run|video/lookup|track-shipment/lookup` · `GET /kocs/digest/morning` ·
`PATCH /kocs/:id/stage` · `POST /kocs/:id/sync-channel|track-shipment|email` · `DELETE /kocs/:id` ·
`GET /inbox/conversations` · `GET|POST|DELETE /inbox/conversations/:id/messages` · `DELETE …/messages/:messageId` · `POST …/inbound` · `POST /inbox/chatbot/parse`


## Vai trò & tài khoản

| | Booker (`member`) | Quản trị viên (`admin`) |
|---|---|---|
| Tổng quan, Hộp thư, Quản lý KOC, tự động hóa | ✅ | ✅ |
| Đổi tên / đổi mật khẩu của mình | ✅ | ✅ |
| Tạo / khóa / xóa / đổi vai trò người dùng, gửi email reset | ❌ | ✅ |
| Nhật ký hoạt động, Giám sát hệ thống | ❌ | ✅ |
| Kết nối / ngắt tài khoản Zalo dùng chung | ❌ | ✅ |

- Đăng ký công khai **tắt** (`ALLOW_REGISTRATION=false`); admin tạo tài khoản, người dùng nhận email mời đặt mật khẩu.
- Admin đầu tiên được tạo từ `ADMIN_EMAIL` / `ADMIN_PASSWORD` khi khởi động lần đầu. **Không chạy `seed` trên production** (seed xóa dữ liệu).
- Quên mật khẩu: mã xác nhận 6 số gửi qua email, hiệu lực 10 phút, tối đa 5 lần nhập sai (cần cấu hình SMTP). Đổi mật khẩu / khóa tài khoản / đổi vai trò sẽ thu hồi mọi phiên đăng nhập cũ.
- Sai mật khẩu 5 lần → khóa 15 phút; giới hạn tần suất theo IP trên các endpoint xác thực.

## Deploy lên Oracle Cloud (thay Render)

Kiến trúc: `Caddy (HTTPS tự động) → frontend (nginx) / backend (Go) → MongoDB`, `zalo-bridge` nằm trong mạng nội bộ. Ảnh Docker được build bằng GitHub Actions và đẩy lên GHCR; server (1 GB RAM) chỉ `pull` và chạy.

1. **OCI Console** → VCN → Security List: mở ingress TCP **80** và **443** (nguồn `0.0.0.0/0`).
2. Trên server: `bash deploy/setup-server.sh` (tạo swap 2 GB, cài Docker, mở firewall, cron backup Mongo hằng ngày).
3. Tạo `/opt/kocflow/.env` từ `deploy/.env.prod.example` (đặt `JWT_SECRET`, `ZALO_BRIDGE_SECRET`, `ADMIN_*`, SMTP...). Chưa có tên miền thì dùng `SITE_ADDRESS=<ip-gạch-ngang>.sslip.io`.
4. GitHub → Settings → Secrets → Actions: `OCI_HOST`, `OCI_USER` (`opc`), `OCI_SSH_KEY` (nội dung khóa riêng), `GHCR_PULL_TOKEN` (PAT quyền `read:packages`).
5. Push lên `main` → CI (vet/test/build) → Deploy tự động. Giám sát: trang **Hệ thống** (admin) hoặc `docker compose logs`.
