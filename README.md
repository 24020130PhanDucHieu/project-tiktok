# Tiktoday Booking

## Chạy local bằng Docker

```bash
docker compose up --build
```

- Frontend: http://localhost:5173
- Backend health: http://localhost:5000/health
- Mongo Express: http://localhost:8081

Để tạo dữ liệu mẫu cho danh bạ KOC và hộp thư:

```bash
docker compose exec backend npm run seed
```

## API hiện có

- `GET /api/v1/kocs?q=&category=&page=&limit=`
- `POST /api/v1/kocs/lookup`
- `POST /api/v1/kocs`
- `GET /api/v1/inbox/conversations`
- `GET /api/v1/inbox/conversations/:conversationId/messages`
- `POST /api/v1/inbox/conversations/:conversationId/messages`

API lookup hiện chỉ tìm các kênh đã có trong MongoDB. Tích hợp TikTok data provider sẽ được thêm riêng, không dùng scraping tài khoản cá nhân.

## Tích hợp Zalo Official Account

Backend đã có sẵn:

- Gửi tin nhắn text qua Zalo OA khi conversation có `channel=zalo` và `externalContactId`.
- Nhận tin nhắn từ OA tại `POST /api/v1/webhooks/zalo`.
- Tự tạo conversation và message inbound từ webhook.

Trên Render, thêm biến môi trường:

```env
ZALO_OA_ACCESS_TOKEN=your-zalo-oa-access-token
```

Trong Zalo Developers, đăng ký webhook bằng URL:

```text
https://project-tiktok-xqcy.onrender.com/api/v1/webhooks/zalo
```

Không commit access token vào GitHub. Tài khoản nhận tin phải tương tác với Official Account theo chính sách của Zalo thì OA mới được phép nhắn chăm sóc khách hàng.

## Tích hợp Zalo cá nhân bằng zca-js

Backend cũng hỗ trợ tài khoản Zalo cá nhân theo hướng dẫn cookie/IMEI/user-agent:

1. Đặt `ZALO_PERSONAL_COOKIE` là JSON array cookie export từ Zalo Web, cùng `ZALO_PERSONAL_IMEI` và `ZALO_PERSONAL_USER_AGENT` trong `.env`.
2. Khởi động backend. Nếu đủ ba biến, server tự đăng nhập và bắt đầu listener nhận tin.

Có thể kết nối trong lúc server đang chạy bằng endpoint đã xác thực:

```http
POST /api/v1/zalo/connect
Content-Type: application/json
Authorization: Bearer <jwt>

{"cookie": [{"name": "z_uuid", "value": "...", "domain": ".zalo.me"}], "imei": "...", "userAgent": "..."}
```

Trạng thái xem tại `GET /api/v1/zalo/status`. Cookie chỉ được dùng ở backend và không bao giờ được trả về response. Khi conversation có `contactPhone` nhưng chưa có `externalContactId`, lần gửi đầu tiên sẽ tự gọi `api.findUser(phone)`, lưu `uid` Zalo vào conversation rồi gửi tin. Các lần sau gửi thẳng bằng ID đã lưu; nếu chưa kết nối tài khoản cá nhân, API trả lỗi yêu cầu kết nối trước.

# project-tiktok
