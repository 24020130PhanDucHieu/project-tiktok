// Builds public/mau-nhap-koc.xlsx, the sample file people fill in for "Nhập từ Excel" (KOC directory).
// Run `npm run template` after changing the columns, and keep src/features/kocs/bulkImport.ts in step: the test in
// bulkTemplate.test.ts reads this file with that code.
import { fileURLToPath } from 'node:url';
import writeExcelFile from 'write-excel-file/node';

const out = fileURLToPath(new URL('../public/mau-nhap-koc.xlsx', import.meta.url));

const bold = (value) => ({ value, fontWeight: 'bold', backgroundColor: '#eef2ff' });
const text = (value) => ({ value, type: String, format: '@' }); // text: Excel must not turn 0912345678 into a number

const header = ['TikTok ID', 'Email', 'Số điện thoại', 'Người nhận mẫu', 'Địa chỉ nhận mẫu', 'Phí cast', 'Thư mục'].map(bold);

const examples = [
	[text('vi_du_ten_kenh_1'), 'vi_du_1@example.com', text('0912345678'), 'Nguyễn Văn A', '12 Lê Lợi, Quận 1, TP.HCM', 500000, ''],
	[text('vi_du_ten_kenh_2'), '', text('0987654321'), 'Trần Thị B', '45 Nguyễn Huệ, Hà Nội', 0, 'Làm đẹp'],
];

// empty cells below the examples are text too, so a phone typed there keeps its leading zero
const blanks = Array.from({ length: 200 }, () => [text(''), null, text(''), null, null, null, null]);

const guide = [
	['Cột', 'Bắt buộc', 'Ý nghĩa', 'Ví dụ'],
	['TikTok ID', 'Có', 'Tên kênh TikTok. Có thể gõ có @ hoặc dán cả link kênh. Phải là nhà sáng tạo có trong TikTok Shop, nếu không dòng sẽ bị từ chối.', 'ten_kenh'],
	['Email', 'Không', 'Email liên hệ của KOC.', 'ten@gmail.com'],
	['Số điện thoại', 'Không', 'Gõ liền hoặc có dấu cách, 10 hoặc 11 chữ số. Nên để ô dạng văn bản để không mất số 0 đầu.', '0912345678'],
	['Người nhận mẫu', 'Không', 'Tên người nhận hàng mẫu.', 'Nguyễn Văn A'],
	['Địa chỉ nhận mẫu', 'Không', 'Địa chỉ gửi hàng mẫu.', '12 Lê Lợi, Quận 1, TP.HCM'],
	['Phí cast', 'Không', 'Số tiền (đồng). Gõ 500000, 500.000 hoặc 500.000đ. Để trống nếu không trả phí.', '500000'],
	['Thư mục', 'Không', 'Tên thư mục đã có trong danh bạ. Để trống thì vào "Chưa phân loại". Tên không có sẽ báo lỗi.', 'Làm đẹp'],
	[],
	['Lưu ý', '', 'Nhớ xoá 2 dòng ví dụ ở sheet "Nhập KOC" trước khi nhập. Mỗi file tối đa 200 dòng, chỉ nhận .xlsx.', ''],
].map((row, i) => (i === 0 ? row.map(bold) : row));

await writeExcelFile([
	{ sheet: 'Nhập KOC', data: [header, ...examples, ...blanks], columns: [{ width: 22 }, { width: 28 }, { width: 16 }, { width: 22 }, { width: 38 }, { width: 12 }, { width: 18 }], stickyRowsCount: 1 },
	{ sheet: 'Hướng dẫn', data: guide, columns: [{ width: 20 }, { width: 11 }, { width: 90 }, { width: 28 }] },
]).toFile(out);

console.log(`Đã tạo ${out}`);
