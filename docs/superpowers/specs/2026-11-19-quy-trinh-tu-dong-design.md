# Quy trình tự động (tab "Quy trình", kéo thả): thiết kế

Ngày: 2026-11-19. Trạng thái: chờ chủ dự án duyệt. Bản này thay bản "danh sách bước" trước đó: chủ dự án chọn khung vẽ kéo thả như n8n (mức 1: khung vẽ đầy đủ, động cơ dạng cây).

## Mục đích

Hiện nay một việc như "nhập KOC từ Excel, gửi tin hàng loạt, vài ngày sau nhắc người chưa trả lời" phải làm tay qua nhiều cửa sổ, và người làm phải nhớ quay lại đúng lúc. Tab **Quy trình** cho phép **vẽ** quy trình bằng cách kéo các khối chức năng đã có ra một khung vẽ rồi nối dây, bấm bắt đầu một lần, và máy chủ tự chạy, kể cả khi đóng trang hay máy chủ khởi động lại.

Người dùng là booker và chủ shop (hiện là shop Cooltech). Đây là cách giảm việc tay lặp lại, không gắn với bài tập môn học nào.

## Mục tiêu

- Khung vẽ kéo thả: kéo khối từ danh sách ra khung, nối dây, bấm vào khối để cài đặt, xóa khối hoặc dây, phóng to thu nhỏ.
- Vẽ được, ví dụ: **Nhập Excel → Chờ 2 ngày → Gửi chiến dịch → Chờ 2 ngày → "KOC đã trả lời chưa?" → (chưa trả lời) → Gửi chiến dịch nhắc; (đã trả lời) → để yên.**
- Rẽ nhánh một lần tại khối điều kiện, hai nhánh chạy **độc lập về thời gian** (mục "Người điều phối").
- Chạy nền trên máy chủ, đóng trang vẫn chạy, khởi động lại máy chủ thì chạy tiếp đúng chỗ, không gửi hai lần.
- Khi chạy, **chính khung vẽ** hiện tiến độ: khối nào xong, đang chạy, đang chờ (còn bao lâu), kèm số KOC. Dừng được bất kỳ lúc nào.
- Dùng lại hai bộ chạy nền đã có (`ImportRunner`, `CampaignRunner`) mà không sửa cách chúng làm việc.

## Không phải mục tiêu (bản đầu)

- Gộp nhánh, vòng lặp, chạy song song hai lượt gửi, dữ liệu tùy ý chuyền giữa các khối. (Dữ liệu lưu dạng đồ thị để sau này nâng cấp mà không phải làm lại.)
- Hệ thống sự kiện ("KOC vừa trả lời thì làm ngay"). Điều kiện chỉ được kiểm tra **đúng lúc khối đó đến lượt**.
- Chạy nhiều quy trình cùng lúc. Chỉ một quy trình chạy tại một thời điểm.
- Lưu mẫu quy trình trên máy chủ, sửa quy trình đang chạy, các khối khác (mời hợp tác, duyệt mẫu, vận đơn, báo cáo).
- Dùng khung vẽ trên điện thoại. Điện thoại chỉ xem tiến độ (chỉ đọc).

## Tránh bằng mọi giá (kể cả khi chạy được)

- Gửi tin vượt công tắc, khung giờ, hạn mức đang có.
- Gửi hai lần cho cùng một KOC trong một khối vì máy chủ khởi động lại.
- Một quy trình bị bỏ quên rồi tự gửi tin lúc không ai biết.
- Một đồ thị không hợp lệ (nối vòng, khối bị bỏ trơ) mà vẫn được chạy.

## Các cách đã xem

- **A. Người điều phối đứng trên các bộ chạy nền sẵn có (đã chọn).** Dùng lại code đã test, thêm khối mới sau này dễ.
- **B. Móc nối trong từng bộ chạy.** Hai bộ chạy bị buộc vào nhau, khó tiếp tục sau khi khởi động lại. Bỏ.
- **C. Động cơ đồ thị đầy đủ như n8n** (gộp nhánh, vòng lặp, song song). Quá nhiều chỗ dễ gửi nhầm hoặc gửi hai lần. Để sau.
- **Giao diện:** dùng thư viện **React Flow (`@xyflow/react`)** cho khung vẽ, thay vì tự viết kéo thả. Hiện frontend không có thư viện kéo thả nào (chỉ `CampaignStepList` có kéo sắp xếp đơn giản).

## Thiết kế

### Các khối của bản đầu

| Khối | Cổng ra | Làm gì | Ra gì |
|---|---|---|---|
| `import` (Nhập Excel) | `out` | Nhập danh sách qua `ImportRunner`. Khối bắt đầu: không có dây vào. | Danh sách KOC của các dòng `added` (cần mã KOC, xem dưới) |
| `wait` (Chờ) | `out` | Chờ một khoảng (giờ hoặc ngày) | Cùng danh sách |
| `send` (Gửi chiến dịch) | `out` | Gửi một chiến dịch đã lưu cho cả danh sách, qua `CampaignRunner` | Danh sách các KOC đã được gửi |
| `if_replied` (KOC đã trả lời chưa?) | `replied`, `unanswered` | Chia danh sách theo việc KOC đã có tin nhắn đến sau lần gửi gần nhất hay chưa | Hai danh sách, mỗi cổng một danh sách |

Một cổng ra có **nhiều nhất một dây**. Cổng không nối là nhánh kết thúc. Khối nào cũng có đúng **một dây vào** (trừ `import` thì không có).

### Đồ thị và kiểm tra hợp lệ

Một quy trình là một đồ thị: các **nút** (mã, loại, cài đặt, vị trí x y trên khung) và các **dây** (từ nút nào, cổng nào, đến nút nào). Máy chủ kiểm tra lại khi bắt đầu (không tin giao diện):

- Đúng **một** khối `import`, và nó là khối bắt đầu.
- Mọi khối khác đến được từ khối bắt đầu; mỗi khối có đúng một dây vào; không có vòng.
- Mỗi cổng ra có tối đa một dây.
- Mỗi `send` chọn một chiến dịch đã lưu và có ít nhất một bước tin; `wait` từ 1 phút đến 14 ngày.
- Tối đa 15 khối; tổng thời gian chờ dọc mọi nhánh không vượt 14 ngày.

### Dữ liệu

Một collection mới `workflows`. Một bản ghi gồm: tên, người tạo, đồ thị (nút và dây, kèm dòng Excel của khối `import`), trạng thái (`running`, `done`, `stopped`), lý do dừng, **các "dấu vị trí" đang hoạt động** (mục dưới), trạng thái từng khối (`pending`, `running`, `waiting`, `done`, `stopped`; số KOC; giờ chờ xong; mã bước con), thời điểm tạo và kết thúc. Có chỉ mục duy nhất một bản ghi `running` (như `import_jobs` và `bulk_runs`) và chỉ mục TTL để dọn bản ghi cũ.

Hai thay đổi nhỏ ở bộ chạy sẵn có, để điều phối tìm lại bước con sau khi khởi động lại:
- `ImportRow` ghi thêm **mã KOC** đã tạo (hiện chỉ lưu trạng thái). Dòng cũ không có thì để trống.
- `ImportJob` và `BulkRun` ghi thêm trường tùy chọn `workflowId` và `nodeId`.

### Người điều phối

`WorkflowRunner` là một goroutine, cùng kiểu với `ImportRunner`. Chỗ khác với bản danh sách: vì có rẽ nhánh, điều phối **không** chạy lần lượt từng nhánh (nếu làm vậy, nhánh "đã trả lời, chờ 3 ngày rồi cảm ơn" sẽ chặn nhánh "chưa trả lời, nhắc ngay"). Thay vào đó:

- Mỗi nhánh đang chạy là một **dấu vị trí** (token): đang ở khối nào, cầm danh sách KOC nào, giờ nào được đi tiếp, mã bước con nếu có.
- Điều phối lặp theo chu kỳ ngắn, đi qua các dấu vị trí **đã đến giờ**. Các nhánh đang **chờ** thì chờ song song, không cản nhau.
- `import` và `send` dùng chung tài nguyên "một lượt chạy một lúc" của hai bộ chạy cũ. Dấu vị trí nào đến khối này mà đang có lượt khác chạy (của quy trình hoặc do người bấm tay) thì chờ rồi thử lại mỗi phút, tối đa 60 phút, quá thì dừng quy trình với lý do rõ ràng.
- `wait`: ghi giờ chờ xong, kiểm tra yêu cầu dừng mỗi vài giây (cùng kiểu `waitFor` của `ImportRunner`).
- `send`: chờ đến khung giờ gửi, bật công tắc gửi hàng loạt, bắt đầu `CampaignRunner` với danh sách, rồi chờ lượt chạy kết thúc.
- `import`: bắt đầu `ImportRunner`, hỏi trạng thái vài giây một lần. Job dừng thì quy trình dừng, lý do giữ nguyên lý do của job.
- `if_replied`: với mỗi KOC trong danh sách xem đã có tin nhắn đến sau lần gửi gần nhất hay chưa, dùng đúng cách `CampaignRunner` đang quyết định "KOC đã trả lời" để hai nơi không bất đồng. Tách thành hai danh sách, sinh ra hai dấu vị trí cho hai cổng (cổng không nối thì kết thúc ở đó).
- Quy trình `done` khi không còn dấu vị trí nào.
- Khởi động lại: `Resume` đọc quy trình `running` và các dấu vị trí. Dấu nào đã có bước con thì tìm bước con theo `workflowId/nodeId` và chờ tiếp, **không bắt đầu lại**. Chưa có bước con thì bắt đầu mới. Đây là điểm quan trọng nhất để không gửi hai lần: ghi mã bước con trước rồi mới bắt đầu, cùng cách an toàn mà `ClaimContact` đang dùng.

### Luật an toàn (không nới lỏng)

- Chỉ **quản trị viên** được bắt đầu quy trình có khối `send`, và phải xác nhận rõ ("quy trình này sẽ tự gửi tin cho tối đa N KOC, qua M lượt gửi"). Thành viên thường chỉ xem.
- Công tắc gửi hàng loạt hiện tự tắt sau mỗi lượt gửi. Quy trình bật nó ngay trước mỗi khối `send` và lượt gửi vẫn tự tắt sau đó, như cũ. Việc bật là do quản trị viên đã xác nhận khi bắt đầu quy trình. Quản trị viên dừng quy trình bất kỳ lúc nào, kể cả khi đang chờ; dừng thì cũng dừng bước con đang chạy bằng cách dừng sẵn có của nó.
- Khung giờ 08:30 đến 20:30, hạn mức mỗi KOC, hạn mức 30 KOC mỗi ngày và mức 600 tin mỗi ngày vẫn do `CampaignRunner` áp dụng. Khối `send` luôn dùng khung giờ thường, không có tùy chọn bỏ qua khung giờ.
- Một lúc chỉ có một quy trình `running`. Quy trình chạy tối đa 14 ngày rồi tự dừng với lý do `expired`.
- Mọi lần bắt đầu, dừng, chuyển khối và lý do dừng được ghi vào nhật ký kiểm toán.

### API

Mới, dưới `/workflows` (cùng kiểu với `/import-jobs`): `POST /` (bắt đầu, giới hạn lượt gọi mỗi giờ; gửi cả đồ thị và dòng Excel, cần `confirm=true`), `GET /latest`, `GET /:id`, `PUT /:id/stop`. Dòng Excel dùng lại luật kiểm tra từng dòng của `/import-jobs`.

### Màn hình

Tab mới **Quy trình** trong thanh điều hướng:

- **Khung vẽ** (React Flow): bên trái là danh sách bốn khối để kéo ra; giữa là khung vẽ có lưới, phóng to thu nhỏ, bản đồ nhỏ; bên phải là bảng cài đặt của khối đang chọn (chọn file Excel, thời gian chờ, chọn chiến dịch). Khối hiện cổng vào và cổng ra, dây nối bằng cách kéo từ cổng ra sang cổng vào. Khối `if_replied` có hai cổng ra, đánh nhãn "đã trả lời" và "chưa trả lời".
- **Kiểm tra khi vẽ:** khối nào chưa cài đặt hoặc nối sai thì có viền cảnh báo và nút Bắt đầu bị khóa, kèm lý do.
- **Bắt đầu:** hộp xác nhận ghi số KOC và số lượt gửi. Sau khi bắt đầu khung vẽ chuyển sang **chế độ chỉ đọc**: mỗi khối hiện trạng thái (chờ, đang chạy, xong, dừng), số KOC, thời gian chờ còn lại. Có nút Dừng.
- Bản nháp đồ thị đang vẽ lưu trong trình duyệt (không lưu lên máy chủ) để lỡ tải lại trang không mất.
- Tiến độ lấy bằng cách hỏi `GET /workflows/:id` vài giây một lần, như cửa sổ nhập Excel. Giao diện theo đúng phong cách các cửa sổ hiện có, hoạt động tốt trên màn hình máy tính; điện thoại chỉ xem được tiến độ.

## Việc để người xây tự quyết

- Tên chính xác của các trạng thái và lý do dừng của `CampaignRunner` (cần đọc code để biết "KOC đã trả lời" và "dừng vì hạn mức ngày" được ghi thế nào).
- Khi một lượt gửi dừng vì hạn mức 30 KOC mỗi ngày và còn KOC chưa gửi: **đề xuất** khối `send` chờ đến khung giờ ngày hôm sau rồi gửi tiếp cho KOC chưa gửi. Người xây xác nhận được hay nên dừng quy trình với lý do rõ. Nếu chọn dừng thì ghi vào hướng phát triển.
- Phiên bản `@xyflow/react` cần chọn và **kiểm tra chạy với React 19** (frontend đang dùng React 19) trước khi làm giao diện; nếu không hợp thì báo lại để chọn cách khác.
- Chu kỳ vòng lặp điều phối và chu kỳ hỏi trạng thái bước con (đề xuất 5 giây, giống `importStopPoll`).

## Cách kiểm thử

- **Kiểm tra đồ thị:** bảng các đồ thị hợp lệ và không hợp lệ (có vòng, hai dây vào một khối, khối bỏ trơ, hai cổng ra nối vào nhau, quá 15 khối, chờ quá 14 ngày).
- **Điều phối với bộ chạy giả** (như `import_runner` đang được test): chạy đúng thứ tự; `wait` thật sự chờ và dừng được; `if_replied` tách đúng hai danh sách; nhánh đang chờ **không** cản nhánh kia; bước con dừng thì quy trình dừng đúng lý do; lượt gửi khác đang chạy thì chờ.
- **Khởi động lại** giữa hai khối, giữa một khối đang chạy và giữa lúc đang chờ: không bắt đầu lại bước con, không gửi hai lần.
- **Một quy trình chạy từ Excel qua `ImportRunner` thật và `CampaignRunner` thật** với kho dữ liệu giả, theo cách `9c6654c` đã kiểm tra cả lượt nhập.
- **API:** quyền (chỉ quản trị viên bắt đầu), xác nhận, một quy trình một lúc, giới hạn lượt gọi, đồ thị không hợp lệ bị từ chối.
- **Giao diện:** kiểm tra đồ thị phía trình duyệt, khóa nút Bắt đầu khi sai, hiển thị trạng thái khối ở chế độ chỉ đọc.
