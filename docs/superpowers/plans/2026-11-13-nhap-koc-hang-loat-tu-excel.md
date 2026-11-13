# Nhập KOC hàng loạt từ Excel: kế hoạch triển khai

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Trang Danh bạ KOC có nút "Nhập từ Excel": chọn file `.xlsx` (có file mẫu tải về), xem trước từng dòng, nhập theo lô 10 dòng có tiến độ, và nhận báo cáo từng dòng.

**Architecture:** Giao diện đọc và kiểm tra file bằng một module thuần (`bulkImport.ts`, có test), rồi gửi từng lô nhỏ tới `POST /kocs/bulk` có sẵn. Phía máy chủ mở rộng đường dẫn đó: nhận thêm email, người nhận mẫu, địa chỉ nhận mẫu và dùng hồ sơ xác minh của TikTok Shop (tên, ảnh, follower). File mẫu là một file `.xlsx` tạo sẵn bằng script và lưu trong `frontend/public`.

**Tech Stack:** Go (Fiber) backend; React + TypeScript + Vite frontend; `read-excel-file` và `write-excel-file` (chạy ở trình duyệt, nạp khi cần); `vitest` (chỉ để test).

**Spec:** `docs/superpowers/specs/2026-11-13-nhap-koc-hang-loat-tu-excel-design.md`

## Global Constraints

Giá trị lấy nguyên từ spec; mọi task đều phải tuân theo.

- Chỉ nhận `.xlsx`. Tối đa **200** dòng dữ liệu mỗi file. Giao diện gửi mỗi lô **10** dòng, thời gian chờ **60 giây** mỗi yêu cầu.
- Máy chủ nhận tối đa **20** dòng mỗi yêu cầu `POST /kocs/bulk`; giới hạn tốc độ route này là **40 yêu cầu mỗi 10 phút**.
- Cột file mẫu theo thứ tự: `TikTok ID | Email | Số điện thoại | Người nhận mẫu | Địa chỉ nhận mẫu | Phí cast | Thư mục`. Chỉ `TikTok ID` bắt buộc.
- TikTok ID: bỏ `@`, chữ thường, 2 đến 100 ký tự. Số điện thoại: chỉ giữ chữ số, số 9 chữ số không có 0 đầu thì thêm 0, phải 10 hoặc 11 chữ số, đọc ô dạng văn bản. Phí cast chấp nhận `500000`, `500.000`, `500.000đ`, không âm. Thư mục trống là "Chưa phân loại", tên không có trong danh sách thư mục hiện có là lỗi (không tự tạo).
- Không bao giờ nhập KOC chưa được xác minh trong Nhà sáng tạo của TikTok Shop. Một lô lỗi hoặc bị TikTok giới hạn tốc độ không được làm mất các lô khác.
- Không dùng gói npm `xlsx` (SheetJS). Không nhận `.xls`, `.csv`. Không cập nhật KOC đã có.
- Chữ hiển thị cho người dùng bằng tiếng Việt, ngắn, không dùng dấu ngoặc kép hay gạch đầu dòng trong thông báo.
- Mọi file Go phải qua `gofmt -l .` trên bản LF (CI kiểm tra); chạy kiểm tra trên cả cây trước khi đẩy. Sau khi sửa code chạy `graphify update .`.
- Mỗi task kết thúc bằng một commit trên máy. Chỉ đẩy lên `main` khi người dùng bảo.

## Review Focus

Các đầu vào spec không nói nhưng dễ gặp nhất; mỗi dòng có test ở task nêu trong ngoặc.

1. File chỉ có dòng tiêu đề, hoặc thiếu cột `TikTok ID`, hoặc cột đổi thứ tự / có cột thừa: báo lỗi rõ, không nhập. Cột nhận theo tên, không theo vị trí (Task 2).
2. Dòng trống xen giữa dữ liệu (Excel hay để lại): bỏ qua, không báo lỗi, số dòng báo lỗi vẫn khớp số dòng trong Excel (Task 2).
3. TikTok ID dán là cả đường link kênh (`https://www.tiktok.com/@ten_kenh?lang=vi`): lấy ra `ten_kenh` (Task 2).
4. Số điện thoại viết `+84 976 451 571` hoặc `84976451571`: đổi thành `0976451571` (Task 2).
5. Một lô thất bại giữa chừng (mất mạng, hết thời gian chờ, TikTok giới hạn tốc độ): các dòng của lô đó thành "Chưa nhập" kèm lý do, các lô sau vẫn chạy, các dòng đã nhập trước đó không bị ảnh hưởng (Task 4).

---

## File Structure

| File | Việc |
|---|---|
| `backend/internal/service/koc.go` | `BulkItem` thêm trường; `BulkImport` dùng hồ sơ xác minh và lưu email, người nhận, địa chỉ |
| `backend/internal/handler/koc.go` | `bulkItemRequest` thêm trường kiểm tra; `bulkRequest` tối đa 20 dòng |
| `backend/internal/router/router.go` | giới hạn route `/kocs/bulk` lên 40 mỗi 10 phút |
| `backend/internal/service/koc_bulk_test.go`, `backend/internal/handler/koc_bulk_test.go` | test mới |
| `frontend/src/features/kocs/bulkImport.ts` | module thuần: đọc bảng, kiểm tra, chia lô, chạy nhập |
| `frontend/src/features/kocs/bulkImport.test.ts` | test của module |
| `frontend/scripts/make-koc-template.mjs`, `frontend/public/mau-nhap-koc.xlsx` | script tạo và file mẫu |
| `frontend/src/features/kocs/BulkImportModal.tsx` | cửa sổ 4 bước |
| `frontend/src/features/kocs/KocDirectoryPage.tsx`, `frontend/src/App.tsx` | nút và nối cửa sổ |
| `frontend/src/styles.css` | kiểu `.bi-*` |
| `frontend/package.json`, `.github/workflows/ci.yml` | thư viện, lệnh `test`, chạy test trong CI |

---

### Task 1: Máy chủ: mở rộng `POST /kocs/bulk`

**Files:**
- Modify: `backend/internal/service/koc.go` (`BulkItem`, `BulkImport`), `backend/internal/handler/koc.go` (`bulkItemRequest`, `bulkRequest`, `BulkImport`), `backend/internal/router/router.go:174`
- Test: `backend/internal/service/koc_bulk_test.go`, `backend/internal/handler/koc_bulk_test.go`

**Interfaces:**
- Produces: `service.BulkItem` có thêm `Email, Recipient, ShippingAddress string`. Body của `POST /kocs/bulk`: `{items:[{tiktokId, phone, email, recipient, shippingAddress, castFee, category, ...}]}` tối đa 20. Báo cáo giữ nguyên dạng cũ; mọi mục trong `skippedDuplicates`, `blockedCollaborated`, `notVerified` có khoá `tiktokId` và `reason`; mục trong `imported` là KOC đã lưu (có `tiktokId`).

- [ ] **Step 1: Viết test dịch vụ (fail)** trong `koc_bulk_test.go`, với các fake tự định nghĩa trong file (thư mục nhúng `KocDirectory` và chỉ cài `FindByIdentity`, `Create`, `Save`; luồng nhúng `KocThreads` cài `EnsureKocContact`; người xác minh cài `ResolveCreator`):
  - `TestBulkImportKeepsTheContactFieldsAndTakesTheProfileFromTheMarketplace`: dòng `{TiktokID:"@Abc", Email:" Abc@X.vn ", Recipient:"Lan", ShippingAddress:"1 A, Hà Nội", Phone:"0976451571", CastFee:500000}`, người xác minh trả `{Nickname:"Abc Shop", AvatarURL:"https://a/b.jpg", Followers:3100}`. Khẳng định KOC lưu có `TiktokID=="abc"`, `Email=="abc@x.vn"`, `Recipient=="Lan"`, `ShippingAddress=="1 A, Hà Nội"`, `DisplayName=="Abc Shop"`, `AvatarURL`, `Followers==3100`, `CastFee==500000`, `CastType=="cast"`, `Category=="Chưa phân loại"`, và `EnsureKocContact` được gọi một lần.
  - `TestBulkImportReportsAnUnlistedCreatorWithItsTikTokIDAndSavesNothing`: người xác minh trả `ErrCreatorNotFound`; `NotVerified` có một mục `tiktokId=="abc"` với `reason` không rỗng; `Create` không được gọi.
  - `TestBulkImportSkipsDuplicatesAndBlocksCollaborated`: hai dòng, `FindByIdentity` trả KOC đang hoạt động cho dòng một và KOC `DaHopTac` cho dòng hai; mỗi dòng vào đúng nhóm báo cáo, không gọi người xác minh.
- [ ] **Step 2: Chạy `go test ./internal/service -run BulkImport -count=1`**; kỳ vọng FAIL (trường chưa có).
- [ ] **Step 3: Cài đặt** trong `service/koc.go`: thêm 3 trường vào `BulkItem`. Trong `BulkImport`, nhận kết quả `verifyCreator` (hiện bị bỏ `_`), đổi tên hiển thị, ảnh, follower sang lấy từ `marketplaceProfile(creator, tiktokID)` khi dòng không có giá trị (tên rỗng thì dùng `Nickname`; follower bằng 0 hoặc thiếu thì dùng của TikTok; ảnh luôn từ TikTok). Lưu `Email` (chữ thường, cắt khoảng trắng), `Recipient`, `ShippingAddress` cho cả nhánh tạo mới và nhánh kích hoạt lại KOC đã lưu trữ. Không gọi `FetchChannel` (kênh công khai) trong lô.
- [ ] **Step 4: Viết test handler (fail)** trong `handler/koc_bulk_test.go` với một `KocAPI` giả nhúng interface và ghi lại `items` nhận được; dựng app Fiber như các test handler khác (`httpx.ErrorHandler`), đăng ký `app.Post("/kocs/bulk", h.BulkImport)`:
  - `TestBulkImportRefusesMoreThan20ItemsInOneRequest`: 21 dòng → 400, service không được gọi.
  - `TestBulkImportRefusesABadEmail`: `email:"abc"` → 400.
  - `TestBulkImportPassesTheContactFieldsToTheService`: 1 dòng đủ trường → 201 và `BulkItem` nhận có `Email`, `Recipient`, `ShippingAddress`.
- [ ] **Step 5: Cài đặt** trong `handler/koc.go`: `bulkItemRequest` thêm `Email` (`omitempty,email`), `Recipient` (`max=120`), `ShippingAddress` (cùng giới hạn độ dài với `createKocRequest`); `bulkRequest.Items` đổi thành `validate:"required,min=1,max=20,dive"`; chuyển các trường mới vào `service.BulkItem`. Trong `router.go` đổi `tooMany(10, 10*time.Minute)` của `/bulk` thành `tooMany(40, 10*time.Minute)`.
- [ ] **Step 6: Chạy** `go vet ./... && go test ./internal/service ./internal/handler ./internal/router -count=1`; kỳ vọng tất cả `ok`. Chạy kiểm tra gofmt trên bản LF của cả cây `backend` (xuất ra rỗng là đạt).
- [ ] **Step 7: Commit** `feat(kocs): bulk import takes contact fields and the TikTok profile, 20 rows per request`.

---

### Task 2: Giao diện: module đọc và kiểm tra bảng

**Files:**
- Modify: `frontend/package.json` (thêm dependency `read-excel-file`, `write-excel-file`; devDependency `vitest`; script `"test": "vitest run"`)
- Create: `frontend/src/features/kocs/bulkImport.ts`
- Test: `frontend/src/features/kocs/bulkImport.test.ts`

**Interfaces:**
- Produces (từ `bulkImport.ts`, dùng ở Task 3 đến 5):
  - `BULK_MAX_ROWS = 200`, `BULK_BATCH = 10`
  - `type BulkRowStatus = 'ready' | 'invalid' | 'added' | 'duplicate' | 'collaborated' | 'notListed' | 'failed' | 'skipped'`
  - `interface BulkRow { line: number; tiktokId: string; email: string; phone: string; recipient: string; shippingAddress: string; castFee: number; category: string; status: BulkRowStatus; reason?: string }` (`line` là số dòng trong Excel, dòng tiêu đề là 1)
  - `parseBulkSheet(cells: unknown[][], folders: string[]): { rows: BulkRow[]; fatal?: string; tooMany?: boolean }`: dòng đầu là tiêu đề; trả `fatal` khi không có cột `TikTok ID` hoặc không có dòng dữ liệu; `tooMany` khi quá 200 dòng (khi đó `rows` rỗng).
  - `toBulkItem(row: BulkRow): Record<string, unknown>`: phần tử gửi lên `POST /kocs/bulk` (`tiktokId, email, phone, recipient, shippingAddress, castFee, category`).

- [ ] **Step 1: Cài thư viện** `cd frontend && npm install read-excel-file write-excel-file && npm install -D vitest`. Kiểm tra `package.json` không có gói `xlsx`.
- [ ] **Step 2: Viết test (fail)** trong `bulkImport.test.ts`, gọi `parseBulkSheet` với bảng dựng tay (tiêu đề đúng tên cột). Các test:
  - `readsTheSevenColumnsByNameNotByPosition`: cột đổi thứ tự và thêm cột thừa `Ghi chú` vẫn đọc đúng.
  - `tiktokIdLosesTheAtAndGoesLowercase`: `@GiaDinh_1` → `giadinh_1`; `https://www.tiktok.com/@ten_kenh?lang=vi` → `ten_kenh`; trống → `status:'invalid'`, `reason:'Thiếu TikTok ID'`.
  - `phoneKeepsItsLeadingZeroAndAcceptsInternationalForms`: số `976451571` (kiểu số) → `0976451571`; `'+84 976 451 571'` và `'84976451571'` → `0976451571`; `'12345'` → invalid `Số điện thoại phải có 10 hoặc 11 chữ số`.
  - `castFeeAcceptsTheThreeForms`: `500000`, `'500.000'`, `'500.000đ'` → 500000; `-5` và `'abc'` → invalid `Phí cast không hợp lệ`; trống → 0.
  - `unknownFolderIsAnErrorAndBlankIsUncategorised`: `folders=['Làm đẹp']`; `'Làm đẹp'` ok, `'Lam dep'` invalid `Thư mục chưa có` (so khớp chính xác tên có sẵn), trống → `'Chưa phân loại'`.
  - `blankRowsInTheMiddleAreSkippedAndLineNumbersMatchExcel`: dữ liệu ở dòng 2, 5, 6 (dòng 3, 4 trống) → `line` lần lượt 2, 5, 6.
  - `aKocTwiceInTheFileIsFlaggedOnTheSecondLine`: dòng sau `invalid` `Trùng trong file`.
  - `fatalWhenThereIsNoTikTokIdColumnOrNoDataRows`, và `tooManyWhenMoreThan200Rows`.
  - `toBulkItemSendsOnlyTheApiFields`.
- [ ] **Step 3: Chạy `npx vitest run src/features/kocs/bulkImport.test.ts`**; kỳ vọng FAIL (module chưa có).
- [ ] **Step 4: Cài đặt** `bulkImport.ts` theo các chữ ký trên. Tên cột so khớp bỏ dấu, chữ thường, cắt khoảng trắng thừa. Ô trống toàn dòng thì bỏ qua. Ô số của SĐT chuyển thành chuỗi trước khi xử lý.
- [ ] **Step 5: Chạy lại test**; kỳ vọng PASS toàn bộ. Chạy `npx tsc --noEmit`; kỳ vọng không có lỗi.
- [ ] **Step 6: Thêm `npm test` vào job frontend** trong `.github/workflows/ci.yml` (sau bước build hiện có). Chạy `npm test` ở máy; kỳ vọng PASS.
- [ ] **Step 7: Commit** `feat(kocs): read and check the Excel sheet for bulk import`.

---

### Task 3: File Excel mẫu

**Files:**
- Create: `frontend/scripts/make-koc-template.mjs`, `frontend/public/mau-nhap-koc.xlsx`
- Test: `frontend/src/features/kocs/bulkTemplate.test.ts`

**Interfaces:**
- Consumes: `parseBulkSheet` (Task 2).
- Produces: file tĩnh `/mau-nhap-koc.xlsx` (Vite phục vụ từ `public/`). Sheet 1 tên "Nhập KOC": dòng 1 là 7 tiêu đề theo thứ tự trong Global Constraints, 2 dòng ví dụ với ID dạng `vi_du_ten_kenh_1` và SĐT dạng văn bản bắt đầu bằng 0; hai cột `TikTok ID` và `Số điện thoại` đặt kiểu văn bản. Sheet 2 tên "Hướng dẫn": ý nghĩa từng cột, cột nào bắt buộc, ví dụ giá trị, dặn xoá 2 dòng ví dụ trước khi nhập.

- [ ] **Step 1: Viết test (fail)** `bulkTemplate.test.ts`: dùng `read-excel-file/node` đọc `public/mau-nhap-koc.xlsx`, khẳng định sheet "Nhập KOC" qua `parseBulkSheet(cells, [])` cho đúng 2 dòng `status:'ready'`, SĐT giữ số 0 đầu; sheet "Hướng dẫn" tồn tại và có nội dung.
- [ ] **Step 2: Chạy test**; kỳ vọng FAIL (file chưa có).
- [ ] **Step 3: Viết script** `make-koc-template.mjs` bằng `write-excel-file/node` tạo hai sheet như trên (độ rộng cột hợp lý, tiêu đề in đậm), ghi ra `public/mau-nhap-koc.xlsx`. Thêm script `"template": "node scripts/make-koc-template.mjs"`. Chạy `npm run template`.
- [ ] **Step 4: Chạy test**; kỳ vọng PASS. Mở file bằng một trình đọc Excel (hoặc chuyển nội dung ra bảng văn bản) để xem hai sheet đúng.
- [ ] **Step 5: Commit** `feat(kocs): Excel template for bulk import`.

---

### Task 4: Chạy nhập theo lô

**Files:**
- Modify: `frontend/src/features/kocs/bulkImport.ts`
- Test: `frontend/src/features/kocs/bulkImport.test.ts`

**Interfaces:**
- Consumes: `BulkRow`, `toBulkItem`, `BULK_BATCH` (Task 2).
- Produces:
  - `interface BulkOutcome { tiktokId: string; reason?: string }`
  - `interface BulkReport { imported: { tiktokId: string }[]; skippedDuplicates: BulkOutcome[]; blockedCollaborated: BulkOutcome[]; notVerified: BulkOutcome[] }`
  - `runBulkImport(rows: BulkRow[], post: (items: Record<string, unknown>[]) => Promise<BulkReport>, opts: { batchSize?: number; shouldStop: () => boolean; onProgress: (done: number, total: number, rows: BulkRow[]) => void }): Promise<BulkRow[]>`: chỉ chạy các dòng `status:'ready'`, trả về toàn bộ dòng với trạng thái cuối. Ánh xạ: `imported` → `added`; `skippedDuplicates` → `duplicate`; `blockedCollaborated` → `collaborated`; `notVerified` → `notListed` (giữ `reason` của máy chủ); dòng không xuất hiện trong báo cáo → `failed` với `reason:'Máy chủ không trả kết quả cho dòng này'`.

- [ ] **Step 1: Viết test (fail)** với `post` giả:
  - `sendsTenRowsPerRequestAndMapsTheReportBackToRows`: 25 dòng ready → `post` được gọi 3 lần với 10, 10, 5 phần tử; kết quả ánh xạ đủ bốn nhóm; `onProgress` cuối là `(25, 25, …)`.
  - `aBatchThatFailsMarksOnlyItsRowsAndTheNextBatchesStillRun`: `post` ném lỗi ở lô 2 → 10 dòng của lô 2 là `failed` với lý do là thông điệp lỗi, lô 1 và 3 vẫn `added`.
  - `stoppingLeavesTheRestAsSkipped`: `shouldStop` trả true sau lô 1 → các dòng còn lại `skipped` với lý do `Chưa nhập, đã dừng`.
  - `rowsThatAreNotReadyAreNeverSent`: dòng `invalid` không có trong bất kỳ lần gọi `post` nào.
- [ ] **Step 2: Chạy test**; kỳ vọng FAIL.
- [ ] **Step 3: Cài đặt** `runBulkImport` tuần tự từng lô (không chạy song song, để không dồn gọi TikTok).
- [ ] **Step 4: Chạy** `npx vitest run && npx tsc --noEmit`; kỳ vọng PASS.
- [ ] **Step 5: Commit** `feat(kocs): run the bulk import in batches of ten`.

---

### Task 5: Cửa sổ nhập, nút và nối vào ứng dụng

**Files:**
- Create: `frontend/src/features/kocs/BulkImportModal.tsx`
- Modify: `frontend/src/features/kocs/KocDirectoryPage.tsx`, `frontend/src/App.tsx`, `frontend/src/styles.css`

**Interfaces:**
- Consumes: `parseBulkSheet`, `runBulkImport`, `toBulkItem`, `BulkRow`, `BulkReport` (Task 2, 4); file `/mau-nhap-koc.xlsx` (Task 3); `api` từ `../../lib/axios`.
- Produces: `BulkImportModal` với props `{ folders: string[]; onClose: () => void; onDone: () => void; showToast: (msg: string, type?: 'success' | 'error') => void }`. `KocDirectoryPage` có thêm prop `onOpenBulkImport: () => void`.

- [ ] **Step 1: Cài đặt `BulkImportModal`** (dùng lại lớp `modal-backdrop`, `tiktoday-modal`, `tm-*` của `AddKocModal.tsx`), bốn bước:
  1. **Chọn file:** vùng chọn file / kéo thả, liên kết `Tải file mẫu` trỏ `/mau-nhap-koc.xlsx`. Từ chối đuôi khác `.xlsx` và file quá 2 MB với thông báo ngắn. `read-excel-file` nạp bằng `import()` khi bắt đầu đọc. Lỗi `fatal` / `tooMany` của `parseBulkSheet` hiện ngay trên bước này.
  2. **Xem trước:** bảng gồm số dòng Excel, TikTok ID, SĐT, thư mục, trạng thái (`Hợp lệ` hoặc lý do). Dòng lỗi tô nền đỏ nhạt. Dòng tóm tắt `x hợp lệ, y lỗi`. Nút `Nhập x KOC` (vô hiệu khi x = 0 hoặc đang chạy, chặn bấm hai lần) và `Chọn file khác`.
  3. **Đang nhập:** thanh tiến độ `30/100`, nút `Dừng` (dừng sau lô đang chạy). Yêu cầu gửi bằng `api.post('/kocs/bulk', { items }, { timeout: 60000 })` và lấy `data.data` làm `BulkReport`. Cảnh báo khi đóng cửa sổ giữa chừng.
  4. **Báo cáo:** số đã thêm, và bảng các dòng không thêm được (dòng, TikTok ID, lý do tiếng Việt). Nút `Tải danh sách dòng chưa nhập` ghi file `koc-chua-nhap.xlsx` (cùng 7 cột cộng cột `Lý do`) bằng `write-excel-file/browser`, nạp khi cần. Nút `Xong` gọi `onDone()`.
- [ ] **Step 2: Nối nút:** `KocDirectoryPage` thêm nút `Nhập từ Excel` cạnh `+ Thêm KOC` (kiểu nút phụ, không nổi hơn nút chính). `App.tsx` thêm state mở cửa sổ; khi `!apiReady` thì bấm nút hiện toast `Cần kết nối máy chủ để nhập hàng loạt.` và không mở. Truyền `folders`, `onDone={refreshAll}`, `showToast`. Thêm kiểu `.bi-*` vào `styles.css`.
- [ ] **Step 3: Kiểm tra kiểu và build** `cd frontend && npx tsc --noEmit && npm run build`; kỳ vọng không lỗi, và `read-excel-file` / `write-excel-file` nằm trong các chunk tách riêng (không vào file `index-*.js`).
- [ ] **Step 4: Xem giao diện:** dựng trang thử tạm (file `harness` ngoài `src` chính, xoá sau khi xong) hiển thị `BulkImportModal` với `api` giả cho cả bốn bước, chụp màn hình bằng Chrome không giao diện. Khẳng định: bảng xem trước đọc được trên khung 900 px, dòng lỗi nổi bật, thanh tiến độ và báo cáo đúng. Sửa kiểu cho tới khi gọn.
- [ ] **Step 5: Commit** `feat(kocs): Excel bulk import window on the KOC directory`.

---

### Task 6: Kiểm tra toàn bộ và bàn giao

- [ ] **Step 1:** `cd backend && go vet ./... && go test ./... -count=1` (với `MONGO_TEST_URI` trỏ Mongo Docker như các lần trước); kỳ vọng tất cả `ok`. Kiểm tra gofmt bản LF cả cây `backend` rỗng.
- [ ] **Step 2:** `cd frontend && npm test && npm run build`; kỳ vọng PASS.
- [ ] **Step 3: Chạy thật trên máy:** dựng lại container (`docker compose up -d --build backend frontend`), đăng nhập, tải file mẫu từ nút `Nhập từ Excel`, xoá dòng ví dụ, điền 2 KOC thật có trong Nhà sáng tạo, nhập, kiểm tra danh bạ có KOC mới với đúng tên, follower, SĐT giữ số 0. Nếu không đăng nhập được (mật khẩu local chưa biết), nói rõ với người dùng và đề nghị họ thử trên bản chạy thật sau khi đẩy.
- [ ] **Step 4:** `graphify update .`. Đổi trạng thái trong spec thành "Đã duyệt, đã làm". Commit `docs(kocs): mark the bulk import design as done`.
- [ ] **Step 5:** Báo người dùng kết quả (cái gì đã kiểm, cái gì chưa), và hỏi có đẩy lên `main` không.
