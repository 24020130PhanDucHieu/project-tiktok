import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import readExcelFile from 'read-excel-file/node';
import { describe, expect, it } from 'vitest';
import { parseBulkSheet } from './bulkImport';

// The template the person downloads must be readable by the very code that imports it: if either one changes alone,
// this test fails before a file nobody can import reaches anyone.
describe('the Excel template for bulk import', () => {
	const file = readFileSync(fileURLToPath(new URL('../../../public/mau-nhap-koc.xlsx', import.meta.url)));

	it('hasTheTwoSheetsAndItsExampleRowsAreReadyWithTheLeadingZeroKept', async () => {
		const sheets = await readExcelFile(file);
		const names = sheets.map((s) => s.sheet);
		expect(names).toEqual(['Nhập KOC', 'Hướng dẫn']);

		// 'Làm đẹp' is one of the folders the app starts with (INITIAL_FOLDERS), used by the second example
		const { rows, fatal } = parseBulkSheet(sheets[0].data, ['Làm đẹp']);
		expect(fatal).toBeUndefined();
		expect(rows).toHaveLength(2);
		rows.forEach((r) => {
			expect(r.status).toBe('ready');
			expect(r.phone.startsWith('0')).toBe(true);
			expect(r.tiktokId.startsWith('vi_du_ten_kenh_')).toBe(true);
		});

		const guide = sheets[1].data.flat().map((c) => String(c ?? '')).join(' ');
		expect(guide).toContain('TikTok ID');
		expect(guide).toContain('xoá 2 dòng ví dụ');
	});
});
