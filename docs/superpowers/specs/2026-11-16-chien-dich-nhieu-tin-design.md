# Chiến dịch nhiều tin gửi hàng loạt qua Zalo — bản thiết kế

Ngày: 2026-11-16. Nối tiếp `2026-11-15-gui-tin-hang-loat-theo-mau-design.md`.

## Mục đích

Hiện mỗi KOC chỉ nhận được một tin chữ. Thực tế một lần mời hợp tác là một chuỗi khoảng 9 tin, có ảnh (chào, giới thiệu thương hiệu, đề xuất kèm link, ảnh sản phẩm, thông tin bán hàng, hoa hồng, lời nhắn cuối). Người dùng cần gửi chuỗi đó cho khoảng 50 KOC trong một buổi sáng 5–6 tiếng mà không phải ngồi giữ trình duyệt, và giảm hết mức khả năng tài khoản Zalo bị chặn vì spam.

## Mục tiêu

- Một "chiến dịch" là chuỗi bước có thứ tự, mỗi bước là chữ hoặc ảnh.
- Gửi một chiến dịch cho khoảng 50 KOC trong 5–6 tiếng, máy chủ tự chạy, đóng tab vẫn chạy.
- Nhịp gửi chậm, không đều, một KOC tại một thời điểm.
- Tự dừng khi có dấu hiệu nguy hiểm; admin dừng được ngay.
- Theo dõi tiến độ trực tiếp và xem lại từng KOC, từng bước.
- Không bao giờ gửi lặp một bước cho cùng một KOC.

## Không làm

- Không chia chuỗi thành hai chặng chờ KOC trả lời (người dùng đã chọn hướng A: gửi cả chuỗi một lượt).
- Không gửi video, file, sticker; không gửi lời mời kết bạn.
- Không hẹn giờ chạy trước; đợt bắt đầu khi bấm gửi.
- Không tự chạy tiếp sang ngày hôm sau.
- Không biến thể nội dung tự động (spintax).

## Điều sẽ coi là thất bại

- Một KOC nhận cùng một bước hai lần.
- Chuỗi vẫn gửi tiếp sau khi KOC đã trả lời.
- Đợt vẫn chạy sau khi admin tắt công tắc hoặc Zalo đã báo lỗi liên tiếp.
- Bản thiết kế hay giao diện khiến người dùng tin rằng "sẽ không bị chặn".

## Ràng buộc và sự thật đã kiểm

- Cầu nối Zalo dùng `zca-js` (không chính thức, tài khoản cá nhân), hiện chỉ gửi chữ: `zalo-bridge/src/zalo.js:165`. Zalo không công bố ngưỡng chặn; mọi con số dưới đây là lựa chọn thận trọng, không phải ngưỡng an toàn đã biết.
- Hạn mức hiện tại `maxProactivePerDay = 80` đếm theo tin (`backend/internal/service/outreach.go:16`): 9 tin × 50 KOC = 450 tin, không đủ.
- Backend chưa có chỗ nào nhận file tải lên.
- Quy tắc 24 giờ (`ClaimContact`) hiện chặn tin thứ hai tới cùng một KOC.
- Giả định: backend chạy đúng một bản trên máy chủ Oracle.
- **Chưa kiểm:** `zca-js` gửi ảnh qua `sendMessage({ msg, attachments })`. Việc đầu tiên của kế hoạch là một bản thử nhỏ gửi 1 ảnh tới chính tài khoản của người dùng. Nếu không gửi được, phần ảnh bị cắt khỏi đợt này và báo lại người dùng.

## Các hướng đã cân nhắc

1. **Trình duyệt điều khiển như hiện tại**, thêm vòng lặp theo bước. Ít việc nhất, nhưng tab phải mở 5–6 tiếng, máy ngủ là đứt, không dừng tự động được khi KOC trả lời. Loại.
2. **Hàng đợi trên máy chủ, gửi cả chuỗi một lượt cho từng KOC (hướng A — được chọn).** Đúng với cách người dùng đang làm tay. Đổi lại: khoảng 450 tin tới người chưa từng trả lời, rủi ro bị báo xấu cao nhất trong ba hướng.
3. **Hàng đợi trên máy chủ, chia hai chặng** (mở đầu 3–4 tin, phần còn lại sau khi KOC trả lời). An toàn hơn, nhưng người dùng muốn KOC nhận đủ thông tin ngay. Không chọn.

Vì chọn hướng 2, phần bù rủi ro nằm ở: nhịp chậm, tăng dần số KOC mỗi ngày, và các điều kiện tự dừng.

## Thiết kế

### Chiến dịch

- Lưu trong MongoDB: tên, danh sách bước (tối đa 12). Bước chữ: tối đa 2000 ký tự, dùng được `{ten}` và `{kenh}` (tên kênh TikTok, có `@`). Bước ảnh: JPG/PNG, tối đa 2 MB, có thể kèm chú thích.
- Ảnh tải lên một lần, lưu trong MongoDB, dùng lại cho mọi KOC. Backend chuyển ảnh cho cầu nối Zalo khi gửi.
- Có sẵn 3 chiến dịch khung: "Liên hệ KOC mới", "Nhắc KOC phản hồi thông tin hợp tác", "Gửi thêm thông tin sản phẩm". Nội dung khung theo quy tắc viết của dự án; người dùng tự sửa thành nội dung thật.
- Chỉ admin tạo, sửa, xóa chiến dịch. Sửa chiến dịch không ảnh hưởng đợt đang chạy: đợt chụp lại các bước lúc bắt đầu.

### Đợt gửi

- Một đợt = một chiến dịch (bản chụp) + danh sách KOC đã tích + người tạo. **Mỗi lúc chỉ có một đợt chạy.**
- Tạo đợt cần công tắc "Gửi hàng loạt" đang bật và xác nhận, như hiện nay. Ô xác nhận nêu rõ số KOC, số tin mỗi KOC, thời gian ước tính.
- Một tiến trình nền trong backend lấy từng KOC theo thứ tự:
  1. Kiểm tra lại: công tắc còn bật, trong khung 08:30–20:30, Zalo còn kết nối, chưa chạm hạn mức ngày.
  2. Kiểm tra KOC như hiện nay (có số, có Zalo, không DNC/đã hợp tác, chưa được liên hệ trong 24 giờ) và giữ chỗ bằng `ClaimContact` **một lần cho cả chuỗi**.
  3. Gửi lần lượt các bước. Trước mỗi bước nghỉ ngẫu nhiên 4–15 giây, bước chữ dài nghỉ lâu hơn. Trước mỗi bước kiểm tra KOC đã trả lời chưa.
  4. Xong một KOC thì nghỉ ngẫu nhiên 3–8 phút.
- Với 50 KOC × 9 bước: khoảng 1,5 phút gửi + trung bình 5,5 phút nghỉ ≈ 7 phút/KOC ≈ 5 tiếng 50 phút.

### Không gửi lặp

Trước khi gọi Zalo, bước được ghi "đang gửi"; gửi xong ghi "đã gửi". Nếu máy chủ khởi động lại và thấy một bước còn "đang gửi" thì không biết nó đã đi hay chưa: chuỗi của KOC đó dừng ở trạng thái "không rõ, cần xem tay", đợt chạy tiếp với KOC kế tiếp. Lỗi mạng hay hết thời gian chờ ở một bước cũng xử lý như vậy.

### Điều kiện dừng

| Điều kiện | Phạm vi dừng |
|---|---|
| KOC trả lời giữa chuỗi | Chuỗi của KOC đó; ghi "KOC đã trả lời", để người thật tiếp |
| Một bước của KOC lỗi rõ ràng (Zalo từ chối) | Chuỗi của KOC đó |
| 2 KOC liên tiếp gặp lỗi từ Zalo | Cả đợt |
| Zalo mất kết nối | Cả đợt |
| Admin tắt công tắc "Gửi hàng loạt" | Cả đợt, sau bước đang gửi |
| Hết khung 20:30 hoặc chạm hạn mức ngày | Cả đợt |

Đợt đã dừng không tự chạy lại. KOC chưa tới lượt ghi "chưa gửi" kèm lý do; người dùng tạo đợt mới cho họ.

### Công tắc

Công tắc "Gửi hàng loạt" thành nút dừng khẩn cấp: bật suốt khi đợt chạy, **tự tắt khi đợt kết thúc hoặc dừng**. Đóng cửa sổ không còn tắt công tắc khi có đợt đang chạy.

### Hạn mức

- Hạn mức riêng cho chiến dịch, đếm theo **KOC được mở chuỗi mỗi ngày**: admin chọn 10, 20, 30 hoặc 50; **mặc định 30** (người dùng quyết định). Giao diện khuyên tăng lên 50 sau vài ngày chạy 30 không gặp vấn đề.
- Trần cứng 600 tin chiến dịch mỗi ngày.
- Hạn mức 80 tin/ngày cũ giữ nguyên cho chatbot và tin chủ động lẻ; tin chiến dịch không tính vào đó.

### Giao diện (cửa sổ "Gửi tin hàng loạt" trong Hộp thư)

- Tab **Soạn tin**: các nút mẫu gợi ý được thay bằng một dropdown chọn chiến dịch. Chọn xong, cột trái hiện **danh sách bước** của chiến dịch, cột người nhận giữ nguyên:

  ```
  Chiến dịch  [ Liên hệ KOC mới            ▾ ]      7/9 bước được gửi
  ┌──────────────────────────────────────────────────────────┐
  │ ⠿ [✓] 1  Chào bạn                                   ✎ ↑ ↓ │
  │ ⠿ [✓] 2  Chúng mình xin giới thiệu một chút. Chú…   ✎ ↑ ↓ │
  │ ⠿ [✓] 3  Chúng mình có theo dõi kênh {kenh} và…     ✎ ↑ ↓ │
  │ ⠿ [✓] 4  [ảnh nhỏ] Ảnh sản phẩm                     ✎ ↑ ↓ │
  │ ⠿ [ ] 5  [ảnh nhỏ] Lời mời hợp tác          (bỏ qua) ✎ ↑ ↓ │
  │ ⠿ [✓] 6  Mình gửi lại thông tin SP nha              ✎ ↑ ↓ │
  │ …                                                         │
  └──────────────────────────────────────────────────────────┘
  + Thêm tin chữ   + Thêm ảnh        [Lưu làm mặc định] (admin)
  ```

  - **Ô tích** mỗi bước: bỏ tích thì bước đó không gửi trong đợt này (dòng mờ đi, vẫn nằm trong danh sách). Ô tích dùng cùng kiểu `bm-check` với cột người nhận. Phải còn ít nhất một bước được tích.
  - **Sắp thứ tự**: kéo bằng tay nắm ⠿, hoặc bấm ↑ ↓. Số thứ tự cập nhật ngay.
  - **Sửa** (✎): dòng mở ra thành ô soạn tại chỗ, có đếm ký tự /2000, có nút chèn `{ten}`, `{kenh}`; bước ảnh thì đổi ảnh hoặc sửa chú thích. Mỗi lúc chỉ một dòng mở.
  - Dòng thu gọn hiện một dòng đầu của nội dung, đã thay tên KOC đầu tiên để xem trước.
  - Mọi thay đổi (tích, thứ tự, nội dung) **chỉ áp dụng cho đợt sắp gửi**; mọi thành viên làm được. Chỉ admin có nút "Lưu làm mặc định" để ghi lại vào chiến dịch, cùng nút thêm, xóa bước.
  - Chiến dịch "Liên hệ KOC mới" có sẵn 7 tin chữ do người dùng cung cấp (xem Phụ lục) và 2 ảnh ở vị trí 4, 5 do người dùng cung cấp, nạp sẵn từ `backend/internal/campaignseed/` (`anh-san-pham.png`, `loi-moi-hop-tac.png`). Bước ảnh chưa có ảnh (do admin tự thêm sau) thì không tích được cho tới khi tải ảnh lên.
- Tab **Theo dõi**: nếu có đợt đang chạy, hiện trên cùng: "KOC 12/50 · bước 4/9 · KOC kế tiếp sau khoảng 5 phút", và nút "Dừng đợt" (admin). Bên dưới là các đợt cũ; mở một đợt thấy từng KOC: số bước đã gửi, trạng thái, lý do.
- Khi có đợt đang chạy, nút gửi bị khóa kèm lý do.

### Thay thế cái cũ

Đường `POST /kocs/message-batch` và vòng lặp gửi lô 5 ở trình duyệt bị gỡ; "mẫu một tin" trở thành chiến dịch một bước. Bảng nhật ký `bulk_send_log` được dùng tiếp, thêm thông tin bước.

## Để người làm tự quyết

- Tên collection, tên đường dẫn API, cách tiến trình nền thức dậy, cách chuyển ảnh sang cầu nối.
- Cách phát hiện "KOC đã trả lời" từ dữ liệu tin nhắn đến sẵn có.
- Khoảng thời gian tab Theo dõi tự làm mới.

## Phụ lục: nội dung có sẵn của chiến dịch "Liên hệ KOC mới"

Lời văn do người dùng cung cấp, giữ nguyên; chỉ thay tên kênh cụ thể bằng `{kenh}`. Bước 4 và 5 là ảnh.

1. Chào bạn
2. Chúng mình xin giới thiệu một chút. Chúng mình là thương hiệu đồ thể thao Cooltech. Bên mình là một thương hiệu mới với ưu thế nổi bật ở phần chất liệu: Bên mình tự nghiên cứu và phát triển (R&D) công thức vải, sau đó đặt sản xuất độc quyền, nên kiểm soát được chất lượng từ chất liệu đến thành phẩm
3. Chúng mình có theo dõi kênh {kenh} và nhận thấy định hướng nội dung, hình ảnh của bạn khá phù hợp với hướng phát triển mà Cooltech đang xây dựng. Vì vậy, chúng mình mong muốn đề xuất hợp tác cùng bạn với sản phẩm khăn lạnh thể thao Cooltech của bên mình:https://vt.tiktok.com/ZS9hwE5n8G8AU-9iH4C/
Sau khi nhận sản phẩm, bạn có thể lên video review/giới thiệu sản phẩm, hoặc đưa vào tuyến nội dung sử dụng sản phẩm thật theo phong cách của bạn thường làm.
4. [Ảnh] Ảnh sản phẩm
5. [Ảnh] Lời mời hợp tác
6. Mình gửi lại thông tin SP nha
7. Tuy rằng sản phẩm bên mình là SP mới ra nhưng từ 1/8 - 31/10 bên mình đã bán được gần 4000 khăn và có đánh giá SP là 4.9/5 sao trên cả 2 sàn Shopee và TikTok. Các bạn KOC và khách hàng bên mình đều có nhận xét và trải nghiệm rất tốt về SP của bên mình nha
8. Chúng mình ngỏ ý hợp tác với bạn theo hình thức freecast theo mức hoa hồng là 10% - 3%. Đây là mức hoa hồng hợp tác ban đầu và sẽ tăng dần theo hiệu quả hoạt động giữa bên mình và bạn.
9. Có gì cậu dành chút thời gian của cậu để check thông tin hợp tác và phản hồi cho bên mình biết với nha

## Rủi ro còn lại, không giải quyết được bằng mã

- 450 tin gần giống nhau tới người lạ trong một buổi vẫn có thể khiến Zalo hạn chế tài khoản, dù gửi chậm. Nên dùng một tài khoản Zalo chấp nhận được rủi ro này.
- Tra số điện thoại 50 lần mỗi ngày cũng là một tín hiệu Zalo có thể theo dõi.
- Chưa từng thử với Zalo thật; lần chạy đầu nên là 2–3 KOC quen.
