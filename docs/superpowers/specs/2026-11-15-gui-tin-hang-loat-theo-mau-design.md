# Gửi tin nhắn hàng loạt theo mẫu qua Zalo: thiết kế

Ngày: 2026-11-15. Trạng thái: đã duyệt, đã làm (test tự động qua; chưa chạy thử với Zalo thật; chưa commit).

## Ý định và lý do

Muốn nhắn cho nhiều KOC hiện phải mở từng hội thoại, chọn mẫu (chữ `{ten}` tự thay bằng tên KOC), rồi bấm gửi, lặp lại
cho từng người. Người dùng muốn: chọn một nhóm KOC, chọn một mẫu tin, thấy danh sách người sẽ nhận, bỏ tích vài người
không muốn gửi, rồi gửi cho tất cả qua Zalo một lần. Số lượng KOC không cố định.

Lý do (suy ra từ yêu cầu, người dùng chưa nói rõ): đỡ thao tác lặp lại khi có cả nhóm cần nhắn cùng một nội dung.

## Mục tiêu

- Nút "Gửi tin hàng loạt" ở trang Danh bạ KOC. Người nhận là các KOC đang hiển thị theo bộ lọc hiện tại, mặc định tích hết.
- Chọn một mẫu tin đã có, xem trước tin cho một KOC mẫu. Chữ `{ten}` được thay riêng cho từng KOC.
- Bỏ tích từng người. KOC không gửi được hiện mờ kèm lý do và tự bị bỏ qua.
- Gửi theo từng lô nhỏ, có thanh tiến độ và nút Dừng.
- Hết hạn mức ngày hoặc ngoài khung giờ thì dừng cả đợt; các KOC còn lại báo "chưa gửi" để người dùng gửi lại hôm sau
  (hướng A do người dùng chọn).
- Báo cáo cuối cho từng KOC: đã gửi, chưa gửi, bỏ qua (kèm lý do).
- Thêm cột "Liên hệ lần cuối" ở trang Danh bạ KOC (lấy từ `lastContactedAt`), để hôm sau biết KOC nào đã nhận và KOC nào
  chưa, rồi gửi tiếp.
- Gửi hàng loạt cần công tắc tổng của tin tự động đang bật; tắt thì cửa sổ báo rõ lý do và không gửi.

## Không làm (đợt này)

- Không xếp hàng rồi tự gửi dần qua nhiều ngày, không có tiến trình chạy nền.
- Không gửi email, chỉ Zalo.
- Không sửa nội dung riêng cho từng KOC trong cửa sổ này. Mẫu vẫn soạn và sửa ở hộp soạn tin như hiện nay.
- Không hẹn giờ gửi.
- Không đổi bộ chặn spam hiện có.
- Không lưu kết quả từng đợt để xem lại sau. Báo cáo chỉ nằm trong cửa sổ; nhật ký kiểm toán chỉ ghi tổng mỗi lô. Đã cân
  nhắc, sẽ làm sau nếu thật sự cần xem lịch sử.

## Điều tuyệt đối không được xảy ra

- Gửi vượt 80 tin/ngày, ngoài khung 08:30 đến 20:30, hoặc cho KOC thuộc danh sách không liên hệ / đã hợp tác. Mọi kiểm
  tra phải làm lại ở server cho từng KOC, không tin giao diện.
- Một KOC nhận cùng tin hai lần vì bấm đúp, vì lô bị gửi lại sau lỗi mạng, hoặc vì mở lại cửa sổ hôm sau mà không biết
  KOC đó đã nhận hôm trước.
- Báo "chưa gửi" cho tin thực ra đã đi (lô bị lỗi mạng không rõ kết quả).

## Ràng buộc đã kiểm trong code

- Đường gửi tay (`SendMessage` rồi `deliver`, `handler/inbox.go:282`) **không** đi qua bộ chặn spam.
- Bộ chặn spam là `CanSendProactiveMessage` (`service/outreach.go:115`): chặn KOC `Dnc` hoặc `DaHopTac`, hạn mức 80 tin
  mỗi ngày cho cả hệ thống, khung 08:30 đến 20:30 giờ Việt Nam, và không gửi lại trong 24 giờ tính từ `LastContactedAt`.
- `DeliverMessageToKoc` (`outreach.go:202`) tự nó **không** gọi bộ chặn. Với `isProactive=true` nó tăng bộ đếm hạn mức,
  kể cả khi Zalo chưa nhận tin (khi đó tin có trạng thái `pending`).
- **Đã xác nhận:** `DeliverMessageToKoc` **không** cập nhật `LastContactedAt`; người gọi phải tự làm (như `SendFirstInvite`
  gọi `markInvited` rồi lưu KOC, `outreach.go:278-279`). Gửi hàng loạt phải tự cập nhật sau mỗi tin thật sự đi, nếu không
  quy tắc 24 giờ không chặn được gửi lặp. Việc ghi này là một lần lưu KOC riêng, nên có thể thất bại **sau khi** tin đã đi.
- Zalo tìm người theo số điện thoại (`ResolveByPhone`), nên KOC phải có `Phone`. Code ghi chú Zalo giới hạn số lần tra
  cứu theo số.
- **Cạm bẫy:** `DeliverMessageToKoc` (`outreach.go:223-230`) bắt đầu với `status = sent` và chỉ đổi thành `pending` khi
  KOC **có** tài khoản Zalo mà Zalo không nhận. Nếu số điện thoại **không tra ra** tài khoản Zalo, tin chỉ được lưu nội
  bộ nhưng vẫn trả `sent` và vẫn tính vào hạn mức. Không được dùng thẳng kết quả đó để báo "đã gửi".
- Trình duyệt chờ tối đa 20 giây mỗi yêu cầu (theo thiết kế nhập Excel). Chưa đo thời gian gửi một tin Zalo.
- Mẫu tin có dạng `{_id, name, body}`. `{ten}` hiện được thay ở trình duyệt, lấy từ chữ cuối trong tên
  (`Composer.tsx:127`).
- Cùng loại route trong dự án: `POST /kocs/bulk` giới hạn 40 lần/10 phút, `POST /kocs/outreach/run` 10 lần/10 phút và có
  ghi nhật ký kiểm toán.
- Trang Danh bạ KOC giữ danh sách ở trình duyệt và lọc ở đó (thư mục, follower, GMV, liên hệ, tìm kiếm), nên không cần
  phân trang ở server. **Nhưng danh sách chỉ tải 100 KOC đầu** (`api.get('/kocs', { params: { limit: 100 } })`,
  `App.tsx:82`), nên đợt này người nhận chỉ nằm trong 100 KOC đó. Tải nhiều hơn là việc riêng, không làm ở đây.
- Giao diện đã mang sẵn `lastContactedAt` của server trong từng KOC (`mapItemToCreator` sao chép nguyên các trường), chỉ
  thiếu khai báo kiểu `Creator`.
- Test ở `service` hiện gọi `CanSendProactiveMessage(..., true)` (bỏ qua khung giờ) vì dịch vụ dùng `time.Now()` trực
  tiếp; gửi hàng loạt không bỏ qua khung giờ nên cần cho dịch vụ một đồng hồ thay thế được để test không phụ thuộc giờ chạy.

## Các hướng đã cân nhắc

Về nơi đặt vòng lặp gửi:

- **1. Một request gửi hết.** Giao diện đơn giản nhưng 100 tin chạy rất lâu, dễ quá thời gian chờ, không có tiến độ,
  không dừng được. Loại.
- **2. Giao diện chia lô nhỏ, gọi một endpoint mới; server kiểm tra bộ chặn từng KOC (đã chọn).** Có tiến độ, dừng
  được, mỗi request ngắn, cùng mô hình với nhập Excel.
- **3. Giao diện gọi lặp endpoint gửi tin sẵn có.** Không qua bộ chặn spam, nên mâu thuẫn với mục tiêu. Loại.

Về việc vượt hạn mức:

- **A. Gửi phần được phép, phần còn lại báo chưa gửi (đã chọn).**
- B. Xếp hàng tự gửi dần: cần tiến trình nền, khó dừng, tin đi khi người dùng không để ý. Loại.
- C. Không cho chọn quá hạn mức: người dùng không biết trước hạn mức còn bao nhiêu. Loại.

Người dùng chọn cách 2 và hướng A.

## Thiết kế

**Endpoint mới** `POST /api/v1/kocs/message-batch`, trong nhóm `/kocs` (có sẵn đăng nhập), thêm giới hạn tốc độ
`tooMany(40, 10 phút)` giống `/kocs/bulk`. Thân yêu cầu:

```json
{ "body": "Chào {ten}, ...", "kocIds": ["…", "…"], "confirm": true }
```

Mỗi yêu cầu tối đa 5 KOC. `confirm` bắt buộc là `true` (giống tạo lời mời mục tiêu), `body` không rỗng và tối đa
2000 ký tự.

**Trước hết**, server kiểm tra công tắc tổng của tin tự động (xem mục "Các quyết định mình tự đưa ra"). Tắt thì trả lỗi
**409** với thông báo có sẵn `ErrAutomationOff`, không gửi gì (giống `SendFirstInvite`); giao diện dừng cả đợt và hiện
thông báo đó.

**Với từng KOC trong lô, theo thứ tự**, server:
1. Tìm KOC đang hoạt động. Không có thì trạng thái `skipped`, lý do `not_found`.
2. Không có số điện thoại thì `skipped`, lý do `no_phone`.
3. Gọi `CanSendProactiveMessage(koc, false)` (không bỏ qua khung giờ).
   - Lý do `dnc_or_blocked` hoặc `already_sent_within_24h`: `skipped` kèm lý do, đi tiếp KOC sau.
   - Lý do `daily_quota_reached` hoặc `outside_0830_2030_window`: **dừng cả lô**, KOC này và các KOC còn lại trong lô
     là `not_sent`, và phản hồi báo `stop: true` kèm lý do để giao diện dừng cả đợt.
4. Thay `{ten}` bằng chữ cuối trong `DisplayName` (cùng quy tắc với `Composer.tsx`), rồi gọi
   `DeliverMessageToKoc(ctx, koc, text, true)`.
5. (Đã đổi sau khi review cuối, làm **trước** bước 4.) Trước khi gửi, **giữ chỗ** KOC trong kho bằng một cập nhật nguyên
   tử `ClaimContact` (đặt `lastContactedAt` nếu chưa ai liên hệ trong 24 giờ, và tăng `version` để bản KOC đã đọc trước đó
   không ghi đè được). Không giữ được thì `skipped` `already_sent_within_24h`, không gửi gì. Tin không tới KOC (không có
   Zalo, `pending`, lỗi trước khi gửi) thì trả chỗ giữ (`ReleaseContact`). Lý do: hai người gửi cùng lúc cho một KOC đều đọc
   KOC "chưa liên hệ", nên chỉ kho dữ liệu mới quyết định được.
6. Trạng thái trả về: `sent` **chỉ khi tin thật sự đi qua Zalo**, `pending` (đã lưu nhưng Zalo chưa nhận), `skipped`
   với lý do `not_on_zalo` khi số điện thoại không có tài khoản Zalo, hoặc `failed` kèm lý do khi server gặp lỗi.
   Lỗi của một KOC không làm hỏng cả lô. Người làm nên tra Zalo **trước** khi gọi `DeliverMessageToKoc` (hoặc kiểm tra
   kênh của hội thoại sau khi gọi) để KOC không có Zalo không tốn hạn mức và không để lại tin nội bộ; cách cụ thể do
   người làm chọn.
   Lỗi mạng hoặc quá thời gian chờ **không phải** `failed`: lúc đó server không trả gì, giao diện tự đánh dấu "không rõ
   kết quả".

**Hai tình huống dừng thêm** (mình bổ sung khi lập kế hoạch, bạn xem lại): (1) tin cho kết quả `pending` (Zalo không nhận
được) thì các KOC còn lại là `not_sent`, `stop: true`, lý do `zalo_not_ready`, vì gửi tiếp chỉ tiêu hao hạn mức mà không tới
ai; (2) tin Zalo đã nhận nhưng không lưu được vào hội thoại thì giữ `sent` kèm lý do `message_not_recorded`, `stop: true`,
chỗ giữ vẫn còn, vì tin đã đi và người dùng không được gửi lại. `pending` **không** giữ chỗ (tin chưa tới KOC). Khi tra
số điện thoại thất bại mà đường Zalo cá nhân đang ngắt, KOC không được báo là "không có Zalo" mà cả đợt dừng với lý do
`zalo_not_ready`. Chữ `{ten}` thay bằng "bạn" khi KOC không có tên.

Phản hồi:

```json
{ "success": true,
  "data": { "results": [ { "kocId": "…", "status": "sent|pending|skipped|not_sent|failed", "reason": "…" } ],
            "stop": false, "stopReason": "" } }
```

Mỗi lô ghi một dòng nhật ký kiểm toán (như `outreach.run`), ghi số KOC và kết quả tổng, không ghi nội dung tin.

**Giao diện:** cửa sổ mới `BulkMessageModal.tsx` (cùng kiểu `BulkImportModal`), logic thuần tách ra `bulkMessage.ts`
(dựng danh sách người nhận, chia lô, thay `{ten}` để xem trước, gộp kết quả) và có test như `bulkImport.ts`.

1. **Chọn mẫu và xem trước.** Chọn một mẫu trong danh sách mẫu có sẵn, thấy tin sẽ gửi cho KOC đầu danh sách.
2. **Danh sách người nhận.** Bảng các KOC đang lọc, mỗi dòng có ô tích. Mặc định tích hết, trừ KOC liên hệ trong 7 ngày
   qua (bỏ tích sẵn, ghi "đã liên hệ N ngày trước", người dùng tích lại được). KOC không gửi được (không có SĐT, không
   liên hệ, đã hợp tác) hiện mờ, không tích được, kèm lý do. Nút "Chọn tất cả", "Bỏ chọn tất cả". Dòng "Sẽ gửi x tin".
3. **Đang gửi.** Thanh tiến độ "30/80", chạy từng lô 5, có nút Dừng (dừng sau lô đang chạy). Khi server báo `stop: true`
   thì dừng cả đợt.
4. **Báo cáo cuối.** Số đã gửi, số chưa gửi và lý do chung (hết hạn mức hôm nay, ngoài khung giờ), danh sách từng KOC
   không gửi kèm lý do.

**Công tắc tổng tắt:** nút "Gửi tin hàng loạt" vẫn mở được cửa sổ nhưng hiện rõ lý do ("Công tắc gửi tự động đang tắt, nhờ
quản trị viên bật") và nút Gửi bị khóa. Mọi thành viên đăng nhập đều xem được trạng thái công tắc.

**Danh bạ KOC:** thêm cột "Liên hệ lần cuối" hiện ngày của `lastContactedAt`, hoặc "Chưa liên hệ" khi trống. Việc đầu tiên
của người làm là kiểm tra giao diện đã nhận trường này từ backend chưa (backend đã trả `lastContactedAt` trong JSON).
Lưu ý trường này chỉ được ghi khi hệ thống liên hệ (lời mời, nhắc, gửi hàng loạt), không ghi khi người dùng tự gõ tin.

**Lô bị lỗi mạng hoặc quá thời gian chờ:** giao diện **không tự gửi lại**, vì server có thể đã gửi. Các KOC của lô đó
hiện trạng thái "không rõ kết quả, hãy kiểm tra hội thoại", và dừng cả đợt. Nút Gửi bị khóa khi đang chạy để chặn bấm đúp.

## Việc để người làm quyết

- Kích thước lô cuối cùng (đề xuất 5): đo thời gian gửi một tin Zalo thật rồi chỉnh cho mỗi lô nằm trong 20 giây.
- Chữ chính xác của các thông báo và lý do, theo giọng văn của dự án.
- Kiểu dáng cửa sổ, theo `AddKocModal` và `BulkImportModal`.

## Kiểm thử

- Service (có bản giả của Zalo và kho dữ liệu): từng lý do của bộ chặn cho ra đúng trạng thái; hết hạn mức giữa lô thì
  các KOC sau là `not_sent` và `stop`; KOC không SĐT hoặc không có tài khoản Zalo bị bỏ qua và không tốn hạn mức; `pending` vẫn tính vào hạn mức; `{ten}` thay đúng; gửi
  hai lần liên tiếp cho cùng KOC thì lần hai bị chặn bởi quy tắc 24 giờ (cần `LastContactedAt` được cập nhật).
- Handler và router: bắt buộc `confirm`, tối đa 5 KOC, độ dài `body`, cần đăng nhập, 429 khi quá giới hạn.
- Giao diện (hàm thuần): dựng danh sách người nhận, bỏ tích mặc định cho KOC liên hệ trong 7 ngày, chia lô, gộp kết quả,
  dừng khi `stop`.
- Không gửi thật cho Zalo trong test tự động.

## Các quyết định mình tự đưa ra (bạn xem lại)

- Tên route `POST /kocs/message-batch`, theo kiểu "hành động ở đuôi" như `/kocs/bulk` và `/kocs/outreach/run`.
- Cho mọi thành viên đăng nhập dùng, không chỉ admin, vì gửi tin tay và chạy chiến dịch liên hệ hiện cũng là cấp thành
  viên. Ngược lại, tạo lời mời mục tiêu chỉ cho admin vì không rút lại được. Nếu bạn muốn chặt hơn thì đổi thành admin.
- Lô 5 KOC, giới hạn 40 yêu cầu/10 phút, `body` tối đa 2000 ký tự.
- Mặc định bỏ tích KOC đã liên hệ trong 7 ngày, để hôm sau mở lại cửa sổ không gửi lặp cho người đã nhận.
- **Đã chốt với bạn:** gửi hàng loạt theo mẫu **cần công tắc tổng đang bật**, giống `SendFirstInvite` (`outreach.go:265`,
  "bulk path, so it needs the automation switch"), để nhất quán với dự án và để admin có nút tắt khẩn cấp.
- **Đã chốt với bạn:** báo cáo chỉ nằm trong cửa sổ, cộng thêm cột "Liên hệ lần cuối" ở Danh bạ KOC; không lưu kết quả từng
  đợt.
- Khung giờ không được bỏ qua (bypass) ở chế độ hàng loạt.
- Không tự gửi lại lô lỗi mạng, vì không biết tin đã đi hay chưa.
