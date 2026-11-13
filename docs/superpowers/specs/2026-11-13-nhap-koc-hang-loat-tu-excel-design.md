# Nhập KOC hàng loạt từ file Excel: thiết kế

Ngày: 2026-11-13. Trạng thái: đã duyệt, đã làm (chưa chạy thử với dữ liệu thật).

## Ý định và lý do

Trang Danh bạ KOC hiện chỉ cho thêm từng KOC bằng tay (form "Thêm KOC vào danh bạ": TikTok ID, Email, Số điện thoại,
Người nhận mẫu, Địa chỉ nhận mẫu, Phí cast, Thư mục, và bắt buộc bấm "Tìm kênh" để xác minh kênh có trong Nhà sáng tạo
của TikTok Shop). Người dùng muốn thêm nhiều KOC một lần từ file Excel, kèm một file Excel mẫu để điền theo.

Lý do (suy ra từ yêu cầu, người dùng chưa nói rõ): đỡ gõ từng KOC khi có cả danh sách.

## Mục tiêu

- Nút "Nhập từ Excel" cạnh "+ Thêm KOC" ở trang Danh bạ KOC.
- File mẫu `.xlsx` tải về được, các cột trùng với các ô của form thủ công.
- Xem trước từng dòng trước khi nhập, dòng lỗi báo rõ lý do ngay trên bảng.
- Nhập theo từng lô nhỏ, có thanh tiến độ, và báo cáo cuối cùng từng dòng.
- Mỗi KOC vẫn phải được xác minh có trong Nhà sáng tạo của TikTok Shop, đúng như khi bấm "Tìm kênh".

## Không làm (đợt này)

- Không nhập file `.xls` cũ, `.csv` hay Google Sheet. Chỉ `.xlsx`.
- Không cập nhật KOC đã có trong danh bạ bằng file (chỉ thêm mới, trùng thì bỏ qua).
- Không điền video hay GMV từ file.
- Không tạo thư mục mới từ file.

## Điều tuyệt đối không được xảy ra

- Nhập một KOC chưa được xác minh trong TikTok Shop.
- Mất cả file vì một lô bị lỗi hoặc bị TikTok giới hạn tốc độ.
- Số điện thoại bị mất số 0 đầu vì Excel tự biến thành số.

## Ràng buộc đã kiểm trong code

- `POST /kocs/bulk` đã có: bỏ qua trùng ID kênh hoặc SĐT, chặn KOC đã hợp tác, và gọi TikTok một lần cho mỗi dòng để
  xác minh (`verifyCreator`, tuần tự). Nhưng nó chỉ nhận `tiktokId, displayName, phone, source, category, followers,
  gmv, castFee`. Thiếu email, người nhận mẫu, địa chỉ nhận mẫu, và nó bỏ phí kết quả xác minh (tên, ảnh, follower).
- Route đó đang bị giới hạn 10 yêu cầu mỗi 10 phút. Nhập 200 dòng theo lô 10 cần 20 yêu cầu nên sẽ bị chặn.
- Trình duyệt chờ tối đa 20 giây mỗi yêu cầu. Một lô 10 dòng gọi TikTok 10 lần, bị giới hạn thì còn thử lại sau 2 giây.
- Giới hạn tốc độ của TikTok chung cho cả nhóm ứng dụng.
- `NormalizePhone` chỉ bỏ khoảng trắng, không sửa số 0 đầu. Excel mất số 0 khi ô không phải dạng văn bản.
- Chưa có thư viện đọc Excel trong dự án; form thủ công chỉ cho chọn thư mục đã có.

## Các hướng đã cân nhắc

- **A. Đọc file trên trình duyệt, nhập theo lô (đã chọn).** Xem trước được, có tiến độ, một lô lỗi không mất cả file.
  Đánh đổi: thêm một thư viện đọc Excel ở giao diện.
- **B. Gửi nguyên file lên máy chủ.** Một lần bấm, nhưng 100 dòng gọi TikTok sẽ quá thời gian chờ nếu không làm việc
  chạy nền, không xem trước, không tiến độ. Loại.
- **C. Chỉ CSV.** Loại vì người dùng muốn Excel.

Người dùng chọn A.

## Thiết kế

**File mẫu** `frontend/public/mau-nhap-koc.xlsx`, tạo một lần bằng một script (công cụ chỉ dùng lúc phát triển, không
đi vào bản chạy), rồi lưu sẵn trong dự án. Sheet "Nhập KOC": dòng 1 là tiêu đề
`TikTok ID | Email | Số điện thoại | Người nhận mẫu | Địa chỉ nhận mẫu | Phí cast | Thư mục`, ô cột Số điện thoại và
TikTok ID đặt sẵn dạng văn bản, 2 dòng ví dụ. Sheet "Hướng dẫn": ý nghĩa từng cột, dòng nào bắt buộc (chỉ TikTok ID),
ví dụ giá trị.

**Giao diện** (cửa sổ "Nhập KOC từ Excel", cùng kiểu với `AddKocModal`):
1. Chọn file (hoặc kéo thả), kèm liên kết "Tải file mẫu". Thư viện đọc Excel chỉ nạp khi bấm vào đây.
2. Xem trước: bảng các dòng, mỗi dòng có trạng thái "Hợp lệ" hay lý do lỗi. Đếm "x hợp lệ, y lỗi". Bấm "Nhập x KOC".
   Dòng lỗi không được nhập, người dùng sửa file rồi chọn lại.
3. Đang nhập: thanh tiến độ "30/100", chạy từng lô 10, có nút Dừng giữa chừng (dừng sau lô đang chạy).
4. Báo cáo cuối: số đã thêm, và danh sách từng dòng không thêm được kèm lý do (trùng, đã hợp tác, không có trong
   TikTok Shop, TikTok đang giới hạn tốc độ). Nút "Tải danh sách dòng lỗi" xuất lại `.xlsx` để sửa và nhập lại.
   Danh bạ tự tải lại.

**Kiểm tra ở giao diện trước khi nhập** (mỗi dòng):
- TikTok ID: bắt buộc, bỏ `@`, đổi thành chữ thường, 2 đến 100 ký tự; trùng với dòng khác trong file thì dòng sau báo "Trùng trong file".
- Email: nếu có phải đúng dạng.
- Số điện thoại: đọc dưới dạng văn bản; số 9 chữ số không có 0 đầu (Excel làm mất) thì thêm 0; chỉ giữ chữ số; phải 10 hoặc 11 chữ số.
- Phí cast: chấp nhận `500000`, `500.000`, `500.000đ`; không âm.
- Thư mục: để trống thì "Chưa phân loại"; tên không có trong danh sách thư mục hiện có thì báo lỗi (tránh gõ sai tạo thư mục rác).
- Tối đa 200 dòng dữ liệu mỗi file.

**Phía máy chủ** (mở rộng `POST /kocs/bulk`, không viết đường dẫn mới):
- Nhận thêm `email, recipient, shippingAddress`, kiểm tra email, giới hạn độ dài như form thủ công.
- Dùng kết quả xác minh TikTok: tên hiển thị, ảnh, follower lấy từ Nhà sáng tạo, như khi bấm "Tìm kênh".
- Mỗi lô tối đa 20 dòng; nâng giới hạn tốc độ của route này đủ cho 200 dòng (đề xuất 40 yêu cầu mỗi 10 phút).
- Báo cáo trả về vẫn theo nhóm (đã thêm, trùng, đã hợp tác, chưa xác minh), mỗi dòng kèm `tiktokId` để giao diện
  ghép lại với dòng trong file.
- Giao diện gửi từng lô 10 dòng, đặt thời gian chờ riêng 60 giây cho các yêu cầu này.

## Các quyết định do mình tự đặt (người dùng chưa duyệt, có thể sửa)

- Chỉ `.xlsx`, tối đa 200 dòng, lô 10 dòng.
- Thư mục sai tên là lỗi, không tự tạo.
- Thư viện đọc Excel: ưu tiên `read-excel-file` (nhỏ, còn được duy trì, nạp khi cần). Không dùng gói `xlsx` (SheetJS)
  trên npm vì bản đó đã cũ và có lỗ hổng đã biết.
- Giới hạn tốc độ route: 40 yêu cầu mỗi 10 phút.

## Để người làm quyết định lúc viết kế hoạch

Tên và vị trí chính xác của các thành phần giao diện, cách vẽ thanh tiến độ, cách sinh file mẫu (script nào), cách kéo thả.

## Cách kiểm chứng

- Test backend: các trường mới được lưu; xác minh trả về tên/ảnh/follower; lô vượt 20 dòng bị từ chối; báo cáo kèm `tiktokId`.
- Test giao diện cho phần đọc và kiểm tra file: số 0 đầu của SĐT, phí cast các dạng, trùng trong file, thư mục lạ, quá 200 dòng.
- Chạy thật với file mẫu: nhập vài KOC thật trên máy local và xem báo cáo, danh bạ có KOC mới.
