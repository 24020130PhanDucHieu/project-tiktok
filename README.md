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

# project-tiktok
