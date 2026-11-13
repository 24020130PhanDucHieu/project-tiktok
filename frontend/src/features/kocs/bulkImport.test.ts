import { describe, expect, it } from 'vitest';
import { BULK_MAX_ROWS, type BulkReport, type BulkRow, parseBulkSheet, runBulkImport, toBulkItem, withTimeout } from './bulkImport';

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

describe('what the server would refuse is refused in the preview', () => {
	it('phoneRestoresTheZeroOfALandlineAndDropsTheZeroAfter84', () => {
		const phones = [2838123456, '+84 0976 451 571', '840976451571'];
		const { rows } = parseBulkSheet(sheet(...phones.map((p, i) => row(`k${i}`, { 2: p }))), []);
		expect(rows.map((r) => [r.status, r.phone])).toEqual([['ready', '02838123456'], ['ready', '0976451571'], ['ready', '0976451571']]);
	});

	it('anEmailTheServerWouldRefuseIsRefusedHere', () => {
		const bad = ['an..b@gmail.com', 'ten@gmail..com', 'a@gmail.com,b.vn', '.ten@gmail.com', 'ten.@gmail.com', 'ten@.gmail.com', 'ten@gmail', 'a b@gmail.com', 'ten@@gmail.com'];
		bad.forEach((email, i) => {
			const r = parseBulkSheet(sheet(row(`k${i}`, { 1: email })), []).rows[0];
			expect(r, email).toMatchObject({ status: 'invalid', reason: 'Email không đúng dạng' });
		});
		['ten@gmail.com', 'Ten.Kenh+shop@mail.example.vn', 'a_b-c@x.co'].forEach((email, i) => {
			expect(parseBulkSheet(sheet(row(`g${i}`, { 1: email })), []).rows[0].status, email).toBe('ready');
		});
	});

	it('aRecipientOrAddressLongerThanTheServerAllowsIsRefused', () => {
		const r = parseBulkSheet(sheet(row('a1', { 3: 'x'.repeat(121) }), row('a2', { 4: 'x'.repeat(301) }), row('a3', { 3: 'x'.repeat(120), 4: 'x'.repeat(300) })), []).rows;
		expect(r[0]).toMatchObject({ status: 'invalid', reason: 'Người nhận mẫu tối đa 120 ký tự' });
		expect(r[1]).toMatchObject({ status: 'invalid', reason: 'Địa chỉ nhận mẫu tối đa 300 ký tự' });
		expect(r[2].status).toBe('ready');
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

// ---- running the import -------------------------------------------------------------------------------------------

const ready = (n: number): BulkRow[] =>
	Array.from({ length: n }, (_, i) => ({
		line: i + 2, tiktokId: `k${i}`, email: '', phone: '', recipient: '', shippingAddress: '', castFee: 0, category: 'Chưa phân loại', status: 'ready' as const,
	}));

/** a server that files k0, k5, ... as duplicates, k1, k6, ... as collaborated, k2, k7, ... as not listed, the rest imported */
function fakeServer(calls: Record<string, unknown>[][] = []) {
	return async (items: Record<string, unknown>[]): Promise<BulkReport> => {
		calls.push(items);
		const report: BulkReport = { imported: [], skippedDuplicates: [], blockedCollaborated: [], notVerified: [] };
		for (const it of items) {
			const id = String(it.tiktokId);
			const n = Number(id.slice(1)) % 5;
			if (n === 0) report.skippedDuplicates.push({ tiktokId: id, reason: 'Trùng ID kênh' });
			else if (n === 1) report.blockedCollaborated.push({ tiktokId: id, reason: 'da_hop_tac = true' });
			else if (n === 2) report.notVerified.push({ tiktokId: id, reason: 'Không có trong Nhà sáng tạo' });
			else report.imported.push({ tiktokId: id });
		}
		return report;
	};
}

describe('runBulkImport', () => {
	it('sendsTenRowsPerRequestAndMapsTheReportBackToRows', async () => {
		const calls: Record<string, unknown>[][] = [];
		const progress: [number, number][] = [];
		const out = await runBulkImport(ready(25), fakeServer(calls), { shouldStop: () => false, onProgress: (d, t) => progress.push([d, t]) });
		expect(calls.map((c) => c.length)).toEqual([10, 10, 5]);
		expect(progress.at(-1)).toEqual([25, 25]);
		expect(out).toHaveLength(25);
		expect(out[0]).toMatchObject({ status: 'duplicate', reason: 'Trùng ID kênh' });
		expect(out[1].status).toBe('collaborated');
		expect(out[1].reason).toBe('KOC đã từng hợp tác với shop nên bị chặn');
		expect(out[2]).toMatchObject({ status: 'notListed', reason: 'Không có trong Nhà sáng tạo' });
		expect(out[3].status).toBe('added');
		expect(out[24].status).toBe('added');
	});

	it('aBatchThatFailsMarksOnlyItsRowsAndTheNextBatchesStillRun', async () => {
		const server = fakeServer();
		let call = 0;
		const post = async (items: Record<string, unknown>[]) => {
			call += 1;
			if (call === 2) throw new Error('Mất kết nối');
			return server(items);
		};
		const out = await runBulkImport(ready(30), post, { shouldStop: () => false, onProgress: () => undefined });
		out.slice(10, 20).forEach((r) => expect(r).toMatchObject({ status: 'failed', reason: 'Mất kết nối' }));
		expect(out[3].status).toBe('added'); // first batch, k3
		expect(out[23].status).toBe('added'); // third batch, k23
		expect(out.filter((r) => r.status === 'failed')).toHaveLength(10);
	});

	it('aFailureFromTheServerShowsItsOwnMessage', async () => {
		const post = async (): Promise<BulkReport> => {
			throw { response: { data: { message: 'Quá nhiều yêu cầu, thử lại sau' } } };
		};
		const out = await runBulkImport(ready(3), post, { shouldStop: () => false, onProgress: () => undefined });
		out.forEach((r) => expect(r).toMatchObject({ status: 'failed', reason: 'Quá nhiều yêu cầu, thử lại sau' }));
	});

	it('stoppingLeavesTheRestAsSkipped', async () => {
		let batches = 0;
		const server = fakeServer();
		const out = await runBulkImport(ready(25), async (items) => {
			batches += 1;
			return server(items);
		}, { shouldStop: () => batches >= 1, onProgress: () => undefined });
		expect(batches).toBe(1);
		out.slice(10).forEach((r) => expect(r).toMatchObject({ status: 'skipped', reason: 'Chưa nhập, đã dừng' }));
		expect(out[3].status).toBe('added');
	});

	it('rowsThatAreNotReadyAreNeverSent', async () => {
		const rows = ready(3);
		rows[1] = { ...rows[1], status: 'invalid', reason: 'Email không đúng dạng' };
		const calls: Record<string, unknown>[][] = [];
		const out = await runBulkImport(rows, fakeServer(calls), { shouldStop: () => false, onProgress: () => undefined });
		expect(calls.flat().map((i) => i.tiktokId)).toEqual(['k0', 'k2']);
		expect(out[1]).toMatchObject({ status: 'invalid', reason: 'Email không đúng dạng' });
	});

	it('aRowTheServerSaysNothingAboutIsFailedNotAdded', async () => {
		const post = async (): Promise<BulkReport> => ({ imported: [], skippedDuplicates: [], blockedCollaborated: [], notVerified: [] });
		const out = await runBulkImport(ready(2), post, { shouldStop: () => false, onProgress: () => undefined });
		out.forEach((r) => expect(r).toMatchObject({ status: 'failed', reason: 'Máy chủ không trả kết quả cho dòng này' }));
	});
});

describe('when TikTok cannot be asked', () => {
	const batchWith = (kind: string, tiktokId: string): BulkReport => ({
		imported: [], skippedDuplicates: [], blockedCollaborated: [], notVerified: [{ tiktokId, reason: 'TikTok đang giới hạn tốc độ (từ máy chủ)', kind }],
	});

	it('aCreatorTikTokDoesNotKnowIsNotListedButARateLimitIsNotTheCreatorsFault', async () => {
		const post = async (items: Record<string, unknown>[]): Promise<BulkReport> => ({
			imported: [], skippedDuplicates: [], blockedCollaborated: [],
			notVerified: items.map((it) => ({ tiktokId: String(it.tiktokId), reason: 'không có', kind: 'not_listed' })),
		});
		const out = await runBulkImport(ready(2), post, { shouldStop: () => false, onProgress: () => undefined });
		out.forEach((r) => expect(r.status).toBe('notListed'));
	});

	it('aRateLimitStopsTheFileAndLeavesTheRestSkippedNotRejected', async () => {
		const calls: number[] = [];
		const post = async (items: Record<string, unknown>[]): Promise<BulkReport> => {
			calls.push(items.length);
			return batchWith('rate_limited', String(items[0].tiktokId));
		};
		const out = await runBulkImport(ready(30), post, { shouldStop: () => false, onProgress: () => undefined });
		expect(calls).toEqual([10]); // no later batch is sent into a pause
		expect(out[0]).toMatchObject({ status: 'failed', reason: 'TikTok đang giới hạn tốc độ (từ máy chủ)' });
		out.slice(10).forEach((r) => expect(r).toMatchObject({ status: 'skipped', reason: 'TikTok đang giới hạn tốc độ, hãy thử lại sau ít phút' }));
		expect(out.filter((r) => r.status === 'notListed')).toHaveLength(0);
	});

	it('theShopNotBeingReachableAlsoStopsTheFile', async () => {
		let calls = 0;
		const post = async (items: Record<string, unknown>[]): Promise<BulkReport> => {
			calls += 1;
			return batchWith('unavailable', String(items[0].tiktokId));
		};
		const out = await runBulkImport(ready(25), post, { shouldStop: () => false, onProgress: () => undefined });
		expect(calls).toBe(1);
		out.slice(10).forEach((r) => expect(r).toMatchObject({ status: 'skipped', reason: 'Chưa xác minh được với TikTok Shop lúc này, hãy thử lại sau ít phút' }));
	});

	it('aRouteLimitAnswer429StopsTheFileToo', async () => {
		let calls = 0;
		const post = async (): Promise<BulkReport> => {
			calls += 1;
			throw { response: { status: 429, data: { message: 'Quá nhiều yêu cầu' } } };
		};
		const out = await runBulkImport(ready(25), post, { shouldStop: () => false, onProgress: () => undefined });
		expect(calls).toBe(1);
		out.slice(0, 10).forEach((r) => expect(r).toMatchObject({ status: 'failed', reason: 'Quá nhiều yêu cầu' }));
		out.slice(10).forEach((r) => expect(r).toMatchObject({ status: 'skipped', reason: 'TikTok đang giới hạn tốc độ, hãy thử lại sau ít phút' }));
	});
});

describe('withTimeout', () => {
	it('givesUpOnAReadThatNeverEnds', async () => {
		await expect(withTimeout(new Promise(() => undefined), 20)).rejects.toThrow('timeout');
		await expect(withTimeout(Promise.resolve(7), 20)).resolves.toBe(7);
	});
});
