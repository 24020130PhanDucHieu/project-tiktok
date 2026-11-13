import { describe, expect, it } from 'vitest';
import { BULK_MAX_ROWS, parseBulkSheet, toBulkItem } from './bulkImport';

const HEADER = ['TikTok ID', 'Email', 'Số điện thoại', 'Người nhận mẫu', 'Địa chỉ nhận mẫu', 'Phí cast', 'Thư mục'];

/** A sheet as read-excel-file gives it: the header row, then one array per row. */
const sheet = (...rows: unknown[][]) => [HEADER, ...rows];
const row = (tiktokId: unknown, over: Partial<Record<number, unknown>> = {}) => {
	const r: unknown[] = [tiktokId, '', '', '', '', '', ''];
	Object.entries(over).forEach(([i, v]) => (r[Number(i)] = v));
	return r;
};

describe('parseBulkSheet', () => {
	it('readsTheSevenColumnsByNameNotByPosition', () => {
		const cells = [
			['Ghi chú', 'Thư mục', 'Phí cast', 'TikTok ID', 'Địa chỉ nhận mẫu', 'Người nhận mẫu', 'Số điện thoại', 'Email'],
			['bỏ qua', 'Làm đẹp', '500000', 'abc', '1 A, Hà Nội', 'Lan', '0976451571', 'A@X.vn'],
		];
		const { rows, fatal } = parseBulkSheet(cells, ['Làm đẹp']);
		expect(fatal).toBeUndefined();
		expect(rows).toHaveLength(1);
		expect(rows[0]).toMatchObject({
			line: 2, tiktokId: 'abc', email: 'a@x.vn', phone: '0976451571', recipient: 'Lan', shippingAddress: '1 A, Hà Nội',
			castFee: 500000, category: 'Làm đẹp', status: 'ready',
		});
	});

	it('tiktokIdLosesTheAtAndGoesLowercase', () => {
		const { rows } = parseBulkSheet(sheet(row('@GiaDinh_1'), row('https://www.tiktok.com/@Ten_Kenh?lang=vi'), row('')), []);
		expect(rows[0].tiktokId).toBe('giadinh_1');
		expect(rows[1].tiktokId).toBe('ten_kenh');
		// a row whose only content is a blank id but another cell is filled is an error, not skipped
		const { rows: filled } = parseBulkSheet(sheet(row('', { 3: 'Lan' })), []);
		expect(filled[0]).toMatchObject({ status: 'invalid', reason: 'Thiếu TikTok ID' });
	});

	it('phoneKeepsItsLeadingZeroAndAcceptsInternationalForms', () => {
		const phones = [976451571, '+84 976 451 571', '84976451571', '0976 451 571'];
		const { rows } = parseBulkSheet(sheet(...phones.map((p, i) => row(`k${i}`, { 2: p }))), []);
		rows.forEach((r) => {
			expect(r.phone).toBe('0976451571');
			expect(r.status).toBe('ready');
		});
		const bad = parseBulkSheet(sheet(row('abc', { 2: '12345' })), []).rows[0];
		expect(bad).toMatchObject({ status: 'invalid', reason: 'Số điện thoại phải có 10 hoặc 11 chữ số' });
	});

	it('castFeeAcceptsTheThreeForms', () => {
		const fees = [500000, '500.000', '500.000đ'];
		const { rows } = parseBulkSheet(sheet(...fees.map((f, i) => row(`k${i}`, { 5: f }))), []);
		rows.forEach((r) => expect(r.castFee).toBe(500000));
		const invalid = parseBulkSheet(sheet(row('a1', { 5: -5 }), row('a2', { 5: 'abc' })), []).rows;
		invalid.forEach((r) => expect(r).toMatchObject({ status: 'invalid', reason: 'Phí cast không hợp lệ' }));
		expect(parseBulkSheet(sheet(row('a3')), []).rows[0].castFee).toBe(0);
	});

	it('unknownFolderIsAnErrorAndBlankIsUncategorised', () => {
		const { rows } = parseBulkSheet(sheet(row('a1', { 6: 'Làm đẹp' }), row('a2', { 6: 'Lam dep' }), row('a3')), ['Làm đẹp']);
		expect(rows[0]).toMatchObject({ status: 'ready', category: 'Làm đẹp' });
		expect(rows[1]).toMatchObject({ status: 'invalid', reason: 'Thư mục chưa có' });
		expect(rows[2]).toMatchObject({ status: 'ready', category: 'Chưa phân loại' });
	});

	it('anEmailThatIsNotAnEmailIsAnError', () => {
		const r = parseBulkSheet(sheet(row('abc', { 1: 'abc' })), []).rows[0];
		expect(r).toMatchObject({ status: 'invalid', reason: 'Email không đúng dạng' });
	});

	it('blankRowsInTheMiddleAreSkippedAndLineNumbersMatchExcel', () => {
		const cells = sheet(row('a1'), row(''), ['  ', null, undefined, '', '', '', ''], row('a2'), row('a3'));
		const { rows } = parseBulkSheet(cells, []);
		expect(rows.map((r) => [r.line, r.tiktokId])).toEqual([[2, 'a1'], [5, 'a2'], [6, 'a3']]);
	});

	it('aKocTwiceInTheFileIsFlaggedOnTheSecondLine', () => {
		const { rows } = parseBulkSheet(sheet(row('abc'), row('x'), row('@ABC')), []);
		expect(rows[0].status).toBe('ready');
		expect(rows[2].status).toBe('invalid');
		expect(rows[2].reason).toContain('Trùng trong file');
	});

	it('fatalWhenThereIsNoTikTokIdColumnOrNoDataRows', () => {
		expect(parseBulkSheet([['Email', 'Phí cast'], ['a@x.vn', 1]], []).fatal).toContain('TikTok ID');
		expect(parseBulkSheet([], []).fatal).toBeTruthy();
		expect(parseBulkSheet([HEADER], []).fatal).toContain('dòng dữ liệu');
		expect(parseBulkSheet(sheet(row(''), ['', '', '', '', '', '', '']), []).fatal).toContain('dòng dữ liệu');
	});

	it('tooManyWhenMoreThan200Rows', () => {
		const many = Array.from({ length: BULK_MAX_ROWS + 1 }, (_, i) => row(`kenh${i}`));
		const out = parseBulkSheet(sheet(...many), []);
		expect(out.tooMany).toBe(true);
		expect(out.rows).toEqual([]);
		expect(parseBulkSheet(sheet(...many.slice(0, BULK_MAX_ROWS)), []).rows).toHaveLength(BULK_MAX_ROWS);
	});
});

describe('toBulkItem', () => {
	it('toBulkItemSendsOnlyTheApiFields', () => {
		const { rows } = parseBulkSheet(sheet(row('abc', { 1: 'a@x.vn', 2: '0976451571', 3: 'Lan', 4: '1 A', 5: 500000, 6: 'Làm đẹp' })), ['Làm đẹp']);
		const item = toBulkItem(rows[0]);
		expect(Object.keys(item).sort()).toEqual(['castFee', 'category', 'email', 'phone', 'recipient', 'shippingAddress', 'tiktokId']);
		expect(item).toMatchObject({ tiktokId: 'abc', email: 'a@x.vn', phone: '0976451571', recipient: 'Lan', shippingAddress: '1 A', castFee: 500000, category: 'Làm đẹp' });
	});
});
