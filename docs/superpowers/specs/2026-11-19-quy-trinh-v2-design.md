# Quy trình v2: giao diện kiểu n8n, lưu quy trình, thêm khối

Ngày: 2026-11-19. Trạng thái: chờ chủ dự án duyệt. Nối tiếp bản `2026-11-19-quy-trinh-tu-dong-design.md` (đã làm và đã đẩy lên main).

## Mục đích

Bản đầu của tab Quy trình chạy được nhưng khó dùng và chỉ ghép được một kiểu quy trình. Chủ dự án muốn: (1) giao diện giống n8n, khối vuông có tên bên dưới, bấm vào thì hiện cổng vào/ra và cài đặt ngay cạnh khối, kéo dây dễ và mượt; (2) **lưu** quy trình để mở lại; (3) thêm khối để ghép được nhiều quy trình khác nhau, như "nhắc KOC im lặng" và "chăm sóc sau khi gửi mẫu".

## Mục tiêu

- **A. Giao diện:** khối vuông, tên dưới khối; bấm vào khối thì hiện khung cài đặt nổi cạnh khối (bỏ cột cài đặt bên phải); cổng to, vùng bắt dây rộng, thả dây gần cổng là dính, có nút "+" ở cổng ra để thêm khối kế tiếp ngay tại chỗ.
- **B. Lưu quy trình:** danh sách quy trình đã lưu dùng chung cả shop: lưu, mở, đổi tên, nhân bản, xóa.
- **C. Sáu khối mới**, xếp theo nhu cầu của hai quy trình trên:
  - `pick` **Lấy KOC có sẵn** (khối bắt đầu thứ hai): lấy KOC theo giai đoạn và/hoặc thư mục, không cần Excel. **Mình thêm khối này ngoài danh sách đã bàn**, vì không có nó thì "chăm sóc sau khi gửi mẫu" không làm được: hiện chỉ có Excel là nguồn KOC.
  - `send_text` **Gửi tin Zalo lẻ**: một tin chữ tự viết, không cần tạo chiến dịch.
  - `email` **Gửi email cho KOC**.
  - `label` **Gắn nhãn hội thoại**.
  - `stage` **Đổi giai đoạn KOC**.
  - `wait_until` **Chờ đến giờ cố định** (ví dụ 9 giờ sáng).
- Mọi luật an toàn của bản đầu giữ nguyên và áp dụng cho các khối gửi mới.

## Không phải mục tiêu

- Kích hoạt theo sự kiện, chạy định kỳ (cron), chạy nhiều quy trình cùng lúc, sửa quy trình đang chạy.
- Lưu file Excel (có số điện thoại, địa chỉ): mỗi lần chạy chọn lại file.
- Khối duyệt mẫu, theo dõi vận đơn, báo cáo định kỳ (cần sự kiện hoặc người duyệt): để sau.
- Quy trình riêng tư theo từng người; nhập/xuất file n8n.

## Tránh bằng mọi giá

- Một khối gửi mới (Zalo lẻ, email) gửi vượt khung giờ, vượt hạn mức hoặc mà không có quản trị viên và xác nhận.
- Gửi hai lần cho một KOC, kể cả sau khi khởi động lại.
- Gửi cho KOC đã trả lời kể từ lần gửi gần nhất.
- Khối "Đổi giai đoạn" làm hỏng dữ liệu KOC (chuyển sai, đè lên KOC đang bị chặn liên hệ).
- Một quy trình đã lưu làm lộ dữ liệu cá nhân.

## Các cách đã xem

- **Cài đặt của khối:** (1) khung nổi cạnh khối khi bấm vào *(chọn, giống n8n)*; (2) hộp thoại giữa màn hình khi bấm đúp (dễ làm hơn nhưng che mất đồ thị).
- **Thêm khối vào động cơ:** (1) thêm một hàm xử lý cho mỗi loại khối trong `WorkflowRunner`, giống cách bốn khối đầu đã làm *(chọn)*; (2) viết lại thành bộ đăng ký khối tổng quát (làm khi số khối lớn hơn nhiều; hiện chưa đáng).

## Thiết kế

### A. Giao diện

- **Khối vuông** 96×96 px, biểu tượng ở giữa, tên khối ở bên dưới và một dòng tóm tắt nhỏ dưới tên (ví dụ "Chờ 2 ngày", tên chiến dịch). Màu viền theo loại khối. Khối lỗi có vòng đỏ; khi chạy có vòng xanh/vàng/xanh lá theo trạng thái và số KOC ở góc.
- **Cổng:** chấm 16 px nhìn thấy, vùng bắt chuột 40 px; `connectionRadius` lớn để thả dây gần cổng là tự dính; đường nối cong mượt; khối "KOC đã trả lời chưa?" có hai cổng ra có nhãn. Chấm nối xong thì đặc, chưa nối thì rỗng.
- **Bấm vào khối:** khối được chọn, hiện thanh công cụ nhỏ phía trên (cài đặt, nhân bản, xóa) và **khung cài đặt nổi** cạnh khối (bấm ra ngoài hoặc Esc để đóng). Cổng vào/ra được làm nổi bật.
- **Nút "+" ở cổng ra chưa nối:** bấm thì hiện danh sách khối; chọn một khối thì nó được tạo ngay bên phải và tự nối vào cổng đó.
- **Cột bên trái** (kéo khối ra) vẫn giữ, gọn hơn, nhóm theo loại: Bắt đầu, Điều khiển, Hành động. Cột cài đặt bên phải **bỏ**; khung vẽ rộng ra.
- **Chế độ theo dõi:** bấm vào khối thì hiện chi tiết chỉ đọc (số KOC, trạng thái, lý do, còn bao lâu).
- Đồ thị tối đa **25 khối** (nâng từ 15).

### B. Lưu quy trình

- Collection mới `workflow_templates`: `_id`, `name` (1 đến 100 ký tự), `nodes`, `edges` (chỉ cài đặt của khối; **không** lưu nội dung bước của chiến dịch, chỉ `campaignId`), `createdBy`, `createdAt`, `updatedAt`. Tối đa 50 mẫu. Chỉ mục theo `updatedAt`.
- API dưới `/workflow-templates`: `GET /` (danh sách: id, tên, người tạo, số khối, ngày sửa), `GET /:id`, `POST /` (tạo), `PUT /:id` (đổi tên hoặc ghi đè đồ thị), `DELETE /:id`. Người đã đăng nhập đều xem và tạo được; **sửa/xóa mẫu của mình, quản trị viên sửa/xóa mọi mẫu**. Giới hạn lượt gọi như các API ghi khác.
- Lưu mẫu chỉ kiểm tra **cấu trúc** (số khối, loại khối, dây trỏ đúng khối, kích thước), không bắt buộc quy trình đã hợp lệ để chạy: người dùng được lưu bản đang vẽ dở.
- Giao diện: thanh phía trên khung vẽ có **Mở** (danh sách mẫu), **Lưu**, **Lưu thành bản mới**, và dấu "chưa lưu". Mở mẫu có chiến dịch đã bị xóa thì khối đó hiện "chọn lại chiến dịch". Bản nháp trong trình duyệt vẫn còn như cũ.

### C. Các khối mới

| Khối | Cài đặt | Làm gì | Gửi tin cho người thật? |
|---|---|---|---|
| `pick` Lấy KOC có sẵn | giai đoạn (một hoặc nhiều), thư mục (tùy chọn), tối đa N (≤ 200) | Lấy KOC đang hoạt động, không bị chặn liên hệ, rồi chuyền đi như `import`. **Danh sách cố định lúc bắt đầu** (hộp xác nhận nói "sẽ lấy N KOC giai đoạn X"). | Không |
| `send_text` Gửi tin Zalo lẻ | một tin chữ ≤ 2000 ký tự, có `{ten}` `{kenh}` | Dùng đúng đường của khối `send` (`CampaignRunner`) với một bước chữ: khung giờ, hạn mức ngày, giãn cách, dừng khi KOC trả lời. | **Có** |
| `email` Gửi email | chủ đề, nội dung ≤ 2000 ký tự, có `{ten}` `{kenh}` | Gửi qua SMTP hiện có cho KOC có email; KOC không có email thì bỏ qua. Giãn cách 3 đến 8 giây, chỉ trong khung giờ 08:30 đến 20:30, tối đa 100 email mỗi quy trình. Một KOC không nhận hai email trong 24 giờ. | **Có** |
| `label` Gắn nhãn | một nhãn có sẵn | Thêm nhãn vào cuộc trò chuyện của từng KOC (không có cuộc trò chuyện thì bỏ qua). Không đổi danh sách. | Không |
| `stage` Đổi giai đoạn | giai đoạn đích, chọn trong một danh sách cố định an toàn | Gọi `KocService.ApplyStage` (`set_stage`); KOC đang bị chặn liên hệ (`da_hop_tac`) bị `ApplyStage` từ chối và được đếm là bỏ qua. Không đổi danh sách. | Không |
| `wait_until` Chờ đến giờ | giờ:phút (giờ Việt Nam) | Chờ đến lần kế tiếp của giờ đó (hôm nay nếu chưa tới, ngược lại ngày mai). Không đổi danh sách. | Không |

- **Khối bắt đầu:** đồ thị có **đúng một** khối bắt đầu, là `import` hoặc `pick`. Quy trình bắt đầu từ `pick` không cần dòng Excel.
- **Quyền:** quy trình có `send`, `send_text` hoặc `email` cần **quản trị viên** và `confirm: true`. Hộp xác nhận nêu số khối gửi và số KOC tối đa.
- **Kiểm tra trả lời:** `send_text` và `email` cũng bỏ KOC đã trả lời kể từ lần gửi gần nhất (như `send`), và KOC không kiểm tra được thì không nhận tin.
- **Thời gian chờ:** `wait_until` tính vào tổng thời gian chờ như tối đa 24 giờ; tổng mỗi nhánh vẫn ≤ 14 ngày.
- **Dừng/khởi động lại:** mọi khối mới theo cùng cách của bốn khối đầu: token lưu trong MongoDB, tiếp tục đúng chỗ, khối gửi ghi số lượt trước khi bắt đầu để không gửi hai lần. Công tắc "Gửi hàng loạt" chỉ liên quan `send` và `send_text` (đều qua `CampaignRunner`); `email` không dùng công tắc đó mà dùng nút Dừng quy trình.

### Các quy trình ghép được (kiểm chứng bằng test chạy cả quy trình)

1. **Nhập rồi gửi:** Nhập Excel → Chờ → Gửi chiến dịch.
2. **Nhắc KOC im lặng:** Nhập Excel hoặc Lấy KOC → Gửi tin lẻ → Chờ 2 ngày → KOC đã trả lời chưa? → (chưa trả lời) Gửi tin lẻ nhắc → Gắn nhãn "không phản hồi"; (đã trả lời) Đổi giai đoạn "đã trả lời".
3. **Chăm sóc sau khi gửi mẫu:** Lấy KOC giai đoạn "đang giao" → Chờ đến 9 giờ → Gửi tin lẻ hỏi đã nhận hàng chưa → Chờ 3 ngày → Gửi email nhắc đăng video.

## Việc để người xây tự quyết

- Danh sách giai đoạn cho khối `stage` (an toàn, đọc `set_stage` để biết giai đoạn nào đổi thẳng được).
- Cách tìm KOC cho khối `pick` (dùng `KocDirectory.Search` hoặc thêm một truy vấn), và có lọc theo thư mục thế nào.
- Cách đặt nhãn: nhãn lưu theo tên hay theo mã; thêm một thao tác "thêm nhãn" an toàn khi hai nơi ghi cùng lúc.
- Chi tiết hình dạng khung cài đặt nổi, cách đặt thanh công cụ, vị trí menu "+".
- Chu kỳ giãn cách chính xác của `email` trong khoảng 3 đến 8 giây.

## Cách kiểm thử

- Từng khối mới có test đơn vị với kho giả: danh sách vào/ra đúng, bỏ qua đúng người, khởi động lại không gửi hai lần, dừng được giữa chừng, luật khung giờ và hạn mức.
- Ba quy trình trên chạy trọn qua bộ chạy thật (như `TestAWholeWorkflowThroughTheRealRunners`).
- Kiểm tra đồ thị: một khối bắt đầu (cả `import` lẫn `pick`), 25 khối, `wait_until`.
- API mẫu: quyền xem/sửa/xóa theo chủ mẫu và quản trị viên, giới hạn 50 mẫu, lưu bản vẽ dở, không lưu nội dung bước chiến dịch.
- Giao diện: kiểm tra đồ thị phía trình duyệt, chạy thật trong trình duyệt (kéo khối, thả dây gần cổng, nút "+", khung nổi, lưu/mở mẫu).
