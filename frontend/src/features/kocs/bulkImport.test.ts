import { describe, expect, it } from 'vitest';
import { BULK_BATCH, BULK_MAX_ROWS, finishedRows, RATE_MAX_WAITS, RATE_WAIT_SECONDS, type BulkReport, type BulkRow, parseBulkSheet, runBulkImport, toBulkItem, withTimeout } from './bulkImport';

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
	it('sendsBatchSizeRowsPerRequestAndMapsTheReportBackToRows', async () => {
		const calls: Record<string, unknown>[][] = [];
		const progress: [number, number][] = [];
		const out = await runBulkImport(ready(25), fakeServer(calls), { shouldStop: () => false, onProgress: (d, t) => progress.push([d, t]) });
		expect(calls.map((c) => c.length)).toEqual(Array.from({ length: Math.ceil(25 / BULK_BATCH) }, (_, i) => Math.min(BULK_BATCH, 25 - i * BULK_BATCH)));
		expect(progress.at(-1)).toEqual([25, 25]);
		expect(out).toHaveLength(25);
		expect(out[0]).toMatchObject({ status: 'duplicate', reason: 'Trùng ID kênh' });
		expect(out[1].status).toBe('collaborated');
		expect(out[1].reason).toBe('KOC đã từng hợp tác với shop nên bị chặn');
		expect(out[2]).toMatchObject({ status: 'notListed', reason: 'Không có trong Nhà sáng tạo' });
		expect(out[0].status).toBe('duplicate'); // the first batch went out
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
		out.slice(BULK_BATCH, 2 * BULK_BATCH).forEach((r) => expect(r).toMatchObject({ status: 'failed', reason: 'Mất kết nối' }));
		expect(out[0].status).toBe('duplicate'); // first batch, untouched
		expect(out[29].status).toBe('added'); // last batch
		expect(out.filter((r) => r.status === 'failed')).toHaveLength(BULK_BATCH);
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
		out.slice(BULK_BATCH).forEach((r) => expect(r).toMatchObject({ status: 'skipped', reason: 'Chưa nhập, đã dừng' }));
		expect(out[0].status).toBe('duplicate'); // the first batch went out
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

	it('theShopNotBeingReachableAlsoStopsTheFile', async () => {
		let calls = 0;
		const post = async (items: Record<string, unknown>[]): Promise<BulkReport> => {
			calls += 1;
			return batchWith('unavailable', String(items[0].tiktokId));
		};
		const out = await runBulkImport(ready(25), post, { shouldStop: () => false, onProgress: () => undefined });
		expect(calls).toBe(1);
		out.slice(BULK_BATCH).forEach((r) => expect(r).toMatchObject({ status: 'skipped', reason: 'Chưa xác minh được với TikTok Shop lúc này, hãy thử lại sau ít phút' }));
	});

});

describe('withTimeout', () => {
	it('givesUpOnAReadThatNeverEnds', async () => {
		await expect(withTimeout(new Promise(() => undefined), 20)).rejects.toThrow('timeout');
		await expect(withTimeout(Promise.resolve(7), 20)).resolves.toBe(7);
	});
});

describe('BULK_BATCH', () => {
	it('keeps one request well under the 60 seconds the browser waits', () => {
		// each row is one TikTok call, spaced 5 to 10 seconds by the server: a batch takes up to BULK_BATCH x 10 s
		expect(BULK_BATCH * 10).toBeLessThanOrEqual(30);
	});
});

describe('when TikTok limits the calls', () => {
	const limitedAll = (items: Record<string, unknown>[]): BulkReport => ({
		imported: [], skippedDuplicates: [], blockedCollaborated: [],
		notVerified: items.map((it) => ({ tiktokId: String(it.tiktokId), reason: 'TikTok đang giới hạn tốc độ (từ máy chủ)', kind: 'rate_limited' })),
	});
	const quiet = { sleep: async () => undefined, shouldStop: () => false, onProgress: () => undefined };

	it('waitsAndTriesTheSameRowsAgainThenCarriesOn', async () => {
		const calls: string[][] = [];
		const server = fakeServer();
		const post = async (items: Record<string, unknown>[]): Promise<BulkReport> => {
			calls.push(items.map((i) => String(i.tiktokId)));
			return calls.length === 2 ? limitedAll(items) : server(items); // the 2nd batch is limited once
		};
		const waits: [number, number][] = [];
		const out = await runBulkImport(ready(9), post, { ...quiet, onWait: (s, n) => waits.push([s, n]) });
		expect(calls[1]).toEqual(calls[2]); // the rows that were limited are sent again, as they were
		expect(calls).toHaveLength(4); // 3 batches + 1 retry
		expect(out.filter((r) => r.status === 'failed')).toHaveLength(0);
		expect(waits[0]).toEqual([RATE_WAIT_SECONDS, 1]);
		expect(waits.at(-1)).toEqual([0, 1]); // the wait ends with a 0
		expect(waits.filter(([s]) => s > 0)).toHaveLength(RATE_WAIT_SECONDS);
	});

	it('countsOnlyTheRowsThatWentThroughAsDone', async () => {
		const progress: number[] = [];
		let n = 0;
		const server = fakeServer();
		const post = async (items: Record<string, unknown>[]) => (++n === 1 ? limitedAll(items) : server(items));
		await runBulkImport(ready(BULK_BATCH), post, { ...quiet, onProgress: (d) => progress.push(d) });
		expect(progress).toEqual([0, 0, BULK_BATCH]); // start, after the limited try (nothing done), after the retry
	});

	it('givesUpAfterTheMaximumWaitsAndStopsTheFile', async () => {
		const calls: number[] = [];
		const post = async (items: Record<string, unknown>[]): Promise<BulkReport> => {
			calls.push(items.length);
			return limitedAll(items);
		};
		const out = await runBulkImport(ready(30), post, quiet);
		expect(calls).toEqual(Array(1 + RATE_MAX_WAITS).fill(BULK_BATCH)); // the first try and RATE_MAX_WAITS retries
		out.slice(0, BULK_BATCH).forEach((r) => expect(r).toMatchObject({ status: 'failed', reason: 'TikTok đang giới hạn tốc độ (từ máy chủ)' }));
		out.slice(BULK_BATCH).forEach((r) => expect(r).toMatchObject({ status: 'skipped', reason: 'TikTok đang giới hạn tốc độ, hãy thử lại sau ít phút' }));
		expect(out.filter((r) => r.status === 'notListed')).toHaveLength(0);
	});

	it('theCounterOfWaitsStartsOverAfterABatchGoesThrough', async () => {
		let n = 0;
		const server = fakeServer();
		// every other call is limited: it never limits more than once in a row, so the import must finish
		const post = async (items: Record<string, unknown>[]) => (++n % 2 === 1 ? limitedAll(items) : server(items));
		const out = await runBulkImport(ready(30), post, quiet);
		expect(out.filter((r) => r.status === 'skipped' || r.status === 'failed')).toHaveLength(0);
	});

	it('aRouteLimit429IsWaitedForToo', async () => {
		let calls = 0;
		const server = fakeServer();
		const post = async (items: Record<string, unknown>[]): Promise<BulkReport> => {
			calls += 1;
			if (calls === 1) throw { response: { status: 429, data: { message: 'Quá nhiều yêu cầu' } } };
			return server(items);
		};
		const out = await runBulkImport(ready(BULK_BATCH), post, quiet);
		expect(calls).toBe(2);
		expect(out.filter((r) => r.status === 'failed')).toHaveLength(0);
	});

	it('stoppingDuringTheWaitLeavesTheRestSkippedWithoutAnotherTry', async () => {
		let calls = 0;
		let stop = false;
		const post = async (items: Record<string, unknown>[]): Promise<BulkReport> => {
			calls += 1;
			return limitedAll(items);
		};
		const out = await runBulkImport(ready(9), post, { ...quiet, shouldStop: () => stop, onWait: () => { stop = true; } });
		expect(calls).toBe(1);
		out.forEach((r) => expect(['skipped', 'failed']).toContain(r.status));
		expect(out.some((r) => r.reason === 'Chưa nhập, đã dừng')).toBe(true);
	});
});

describe('onBatch and finishedRows', () => {
	it('tellsWhichBatchIsGoingOutAndWhichIdsItHolds', async () => {
		const seen: { number: number; total: number; ids: string[] }[] = [];
		await runBulkImport(ready(7), fakeServer(), { shouldStop: () => false, onProgress: () => undefined, onBatch: (b) => seen.push(b) });
		const total = Math.ceil(7 / BULK_BATCH);
		expect(seen.map((b) => b.number)).toEqual(Array.from({ length: total }, (_, i) => i + 1));
		expect(seen.every((b) => b.total === total)).toBe(true);
		expect(seen[0].ids).toEqual(['k0', 'k1', 'k2'].slice(0, BULK_BATCH));
	});

	it('keepsTheSameBatchNumberWhenARetryResendsIt', async () => {
		let n = 0;
		const server = fakeServer();
		const limited = (items: Record<string, unknown>[]): BulkReport => ({
			imported: [], skippedDuplicates: [], blockedCollaborated: [],
			notVerified: items.map((it) => ({ tiktokId: String(it.tiktokId), reason: 'x', kind: 'rate_limited' })),
		});
		const numbers: number[] = [];
		await runBulkImport(ready(BULK_BATCH * 2), async (items) => (++n === 2 ? limited(items) : server(items)), {
			shouldStop: () => false, onProgress: () => undefined, sleep: async () => undefined, onBatch: (b) => numbers.push(b.number),
		});
		expect(numbers).toEqual([1, 2, 2]); // the second batch was limited once and is sent again as batch 2
	});

	it('finishedRowsLeavesOutTheRowsStillWaitingAndTheOnesNotSent', () => {
		const rows = [
			{ ...ready(1)[0], status: 'added' },
			{ ...ready(1)[0], tiktokId: 'b', status: 'ready' },
			{ ...ready(1)[0], tiktokId: 'c', status: 'invalid' },
			{ ...ready(1)[0], tiktokId: 'd', status: 'failed' },
		] as BulkRow[];
		expect(finishedRows(rows).map((r) => r.status)).toEqual(['added', 'failed']);
	});
});
