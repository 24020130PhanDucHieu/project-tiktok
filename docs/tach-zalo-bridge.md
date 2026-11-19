# Tách zalo-bridge sang máy riêng

Mục tiêu: `zalo-bridge` chạy trên một VM Oracle thứ hai (cùng vùng Nhật, cùng VCN), deploy độc lập với API. Hai máy nói chuyện với nhau qua **IP private** của VCN, không qua internet.

```
Trình duyệt ──https──▶ Caddy ─▶ backend (VM 1) ──http://IP-private-VM2:5100──▶ zalo-bridge (VM 2) ──▶ Zalo
                                     ▲                                               │
                                     └────── http://IP-private-VM1:5000/internal/zalo/inbound ◀┘
```

Ký hiệu dưới đây: `VM1` là máy đang chạy hệ thống, `VM2` là máy mới. Giao diện Oracle Console có thể khác chút so với mô tả, hãy đối chiếu khi làm.

## Đã chuẩn bị sẵn trong repo (không ảnh hưởng hệ thống đang chạy)

| File | Việc nó làm |
|---|---|
| `backend/internal/platform/zalo_bridge.go` | `KeepConnected`: cứ 30 giây kiểm tra, nếu bridge mất phiên (restart) thì đăng nhập lại. Nếu bạn chủ động bấm ngắt kết nối thì không tự nối lại. Nếu Zalo từ chối thì chờ 1, 2, 4... tối đa 15 phút rồi mới thử lại. |
| `deploy/docker-compose.bridge.yml` | Compose chỉ có `zalo-bridge`, cổng 5100 chỉ mở ở IP private. |
| `deploy/setup-bridge-server.sh` | Cài swap, Docker, tường lửa (chỉ VM1 được vào cổng 5100) cho VM2. |
| `.github/workflows/deploy-bridge.yml` | Deploy bridge riêng. Thẻ ảnh theo nội dung thư mục `zalo-bridge/`, nên commit không đụng bridge thì container không bị tạo lại. Chưa có secret `BRIDGE_*` thì bỏ qua. |
| `deploy/maintenance.sh` | Watchdog giờ chỉ canh các container có trong compose của chính máy đó. |

## Bước 1. Tạo VM2

1. Oracle Console → Compute → Instances → Create instance.
2. Shape `VM.Standard.E2.1.Micro` (Always Free), image Oracle Linux 9. Kiểm tra mục Limits nếu không còn suất.
3. Mạng: chọn **cùng VCN và cùng subnet** với VM1, bật public IP (để SSH và kéo ảnh từ GHCR).
4. Tạo khóa SSH riêng cho máy này: `ssh-keygen -t ed25519 -f $HOME\.ssh\oci_bridge`, dán nội dung `oci_bridge.pub` vào ô SSH keys.
5. Ghi lại **IP private** của VM1 và VM2 (trang chi tiết instance, mục Primary VNIC).

## Bước 2. Mở đường đi trong VCN

Vào VCN → Security Lists → thêm Ingress rule (stateful), hai quy tắc:

| Source CIDR | Cổng đích | Để làm gì |
|---|---|---|
| `<IP private VM1>/32` | TCP 5100 | backend gọi bridge |
| `<IP private VM2>/32` | TCP 5000 | bridge đẩy tin nhắn đến về backend |

Không mở cổng nào cho `0.0.0.0/0`, ngoài SSH có sẵn.

## Bước 3. Dựng VM2

```powershell
scp -i "$HOME\.ssh\oci_bridge" deploy\setup-bridge-server.sh opc@<IP public VM2>:~
ssh -i "$HOME\.ssh\oci_bridge" opc@<IP public VM2>
```

Trên VM2:

```bash
bash setup-bridge-server.sh <IP private VM1>
```

Script in ra IP private của VM2. Đăng xuất rồi đăng nhập lại một lần để nhóm `docker` có hiệu lực.

## Bước 4. File `.env` trên VM2

```bash
cat > /opt/kocflow-bridge/.env <<'EOF'
BRIDGE_BIND_IP=<IP private VM2>
BRIDGE_API_URL=http://<IP private VM1>:5000
ZALO_BRIDGE_SECRET=<đúng giá trị ZALO_BRIDGE_SECRET đang có trong .env của VM1>
EOF
chmod 600 /opt/kocflow-bridge/.env
```

Bridge ở production từ chối chạy nếu secret ngắn hơn 32 ký tự hoặc còn là giá trị mẫu.

## Bước 5. Secret GitHub và deploy bridge

Settings → Secrets and variables → Actions, thêm:

- `BRIDGE_HOST`: IP public của VM2
- `BRIDGE_USER`: `opc`
- `BRIDGE_SSH_KEY`: nội dung file khóa riêng `oci_bridge`

(`GHCR_PULL_TOKEN` dùng chung với VM1.) Sau đó vào Actions → **Deploy zalo-bridge** → Run workflow.

## Bước 6. Kiểm tra hai chiều trước khi chuyển

Từ VM1:

```bash
curl -s http://<IP private VM2>:5100/health        # phải ra {"ok":true}
```

Lệnh trên chưa thể kiểm tra chiều ngược lại, vì backend chưa mở cổng 5000. Chiều đó kiểm tra ở bước chuyển, ngay sau đây.

## Bước 7. Chuyển (cutover)

Đây là phần sửa `docker-compose.prod.yml` của VM1, mình sẽ làm khi bạn xong bước 1 đến 6:

- backend: `ZALO_BRIDGE_URL` đọc từ `.env`, mở cổng `5000` ở IP private của VM1 (`ports: ["<IP private VM1>:5000:5000"]`).
- bỏ service `zalo-bridge` khỏi compose của VM1, và bỏ bước build ảnh bridge khỏi `deploy.yml`.
- trong `.env` của VM1 thêm `ZALO_BRIDGE_URL=http://<IP private VM2>:5100`.
- mở cổng 5000 cho IP của VM2 trong tường lửa của VM1 (`firewall-cmd --permanent --add-rich-rule='rule family=ipv4 source address=<IP VM2> port port=5000 protocol=tcp accept'`, rồi `--reload`).

Sau khi chuyển, backend tự đăng nhập Zalo trong khoảng 30 giây. Nếu Zalo từ chối vì IP mới, vào trang Zalo của web và đăng nhập lại bằng QR hoặc cookie mới.

## Quay lại như cũ (rollback)

Xóa dòng `ZALO_BRIDGE_URL` khỏi `.env` của VM1 và `docker compose up -d`. Chỉ áp dụng trước khi service `zalo-bridge` bị bỏ khỏi compose ở bước 7; sau đó thì deploy lại commit trước bước 7.

## Thử để hiểu hơn (sau khi chuyển)

1. Trên VM2: `docker restart kocflow-bridge-zalo-bridge-1`. Quan sát log backend trên VM1 (`docker compose logs -f backend`): trong khoảng 30 giây có dòng `Personal Zalo connected`.
2. Trên web bấm ngắt kết nối Zalo, restart bridge: backend **không** tự nối lại (vì bạn chủ động ngắt).
3. Tắt hẳn container bridge: trang Zalo hiện bridge offline. Bật lại thì tự nối lại, không cần chạm vào backend.
4. Commit sửa backend và commit sửa bridge: mỗi bên chỉ deploy máy của mình.
