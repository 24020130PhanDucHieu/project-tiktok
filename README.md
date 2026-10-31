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
    ├── middleware/          # JWT, bridge secret
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
