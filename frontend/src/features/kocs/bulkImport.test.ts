import { describe, expect, it } from 'vitest';
import { type BulkRow, type ImportJob, NO_FOLDER, BULK_MAX_ROWS, STOPPED_REASON, finishText, finishedRows, jobProgress, jobRows, parseBulkSheet, shouldShowFinished, startRequest, stopText, toBulkItem, waitSecondsLeft, waitText, withTimeout } from './bulkImport';

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


describe('withTimeout', () => {
	it('givesUpOnAReadThatNeverEnds', async () => {
		await expect(withTimeout(new Promise(() => undefined), 20)).rejects.toThrow('timeout');
		await expect(withTimeout(Promise.resolve(7), 20)).resolves.toBe(7);
	});
});


const sheetRow = (over: Partial<BulkRow> = {}): BulkRow => ({
	line: 2, tiktokId: 'k0', email: '', phone: '', recipient: '', shippingAddress: '', castFee: 0, category: NO_FOLDER, status: 'ready', ...over,
});

const job = (over: Partial<ImportJob> = {}): ImportJob => ({
	_id: 'j1', status: 'running', by: 'a@x.vn', createdAt: '2026-11-18T03:00:00Z',
	rows: [
		{ line: 2, tiktokId: 'a', status: 'added' },
		{ line: 3, tiktokId: 'b', status: 'duplicate', reason: 'Trùng ID kênh' },
		{ line: 4, tiktokId: 'c', status: 'not_listed', reason: 'Không có' },
		{ line: 5, tiktokId: 'd', status: 'queued' },
	],
	...over,
});

describe('startRequest', () => {
	it('sendsOnlyTheReadyRowsEachWithItsLineInTheSheet', () => {
		const rows = [sheetRow({ line: 2, tiktokId: 'a' }), sheetRow({ line: 3, tiktokId: 'bad', status: 'invalid', reason: 'x' }), sheetRow({ line: 4, tiktokId: 'c', email: 'c@x.vn' })];
		const { items } = startRequest(rows);
		expect(items.map((i) => [i.tiktokId, i.line])).toEqual([['a', 2], ['c', 4]]);
		expect(items[1]).toMatchObject({ email: 'c@x.vn', category: NO_FOLDER });
	});
});

describe('jobRows', () => {
	it('mapsTheServerStatusesToTheStatusesOfTheWindow', () => {
		const rows = jobRows(job({ rows: [
			{ line: 2, tiktokId: 'a', status: 'added' },
			{ line: 3, tiktokId: 'b', status: 'duplicate', reason: 'Trùng ID kênh' },
			{ line: 4, tiktokId: 'c', status: 'collaborated' },
			{ line: 5, tiktokId: 'd', status: 'not_listed', reason: 'Không có' },
			{ line: 6, tiktokId: 'e', status: 'failed', reason: 'Lỗi hệ thống' },
		] }));
		expect(rows.map((r) => r.status)).toEqual(['added', 'duplicate', 'collaborated', 'notListed', 'failed']);
		expect(rows[1].reason).toBe('Trùng ID kênh');
	});

	it('aQueuedRowIsWaitingWhileTheJobRunsAndNotSentOnceItEnded', () => {
		expect(jobRows(job())[3].status).toBe('ready');
		const stopped = jobRows(job({ status: 'stopped', stopReason: 'rate_limited' }))[3];
		expect(stopped).toMatchObject({ status: 'skipped', reason: 'TikTok đang giới hạn tốc độ, hãy thử lại sau ít phút.' });
		expect(jobRows(job({ status: 'stopped' }))[3].reason).toBe(STOPPED_REASON);
	});

	it('fillsWhatTheServerLeftOutSoTheReportCanBeDownloaded', () => {
		const r = jobRows(job({ rows: [{ line: 2, tiktokId: 'a', status: 'queued', email: 'a@x.vn' }] }))[0];
		expect(r).toMatchObject({ email: 'a@x.vn', phone: '', castFee: 0, category: NO_FOLDER });
	});
});

describe('jobProgress and waitSecondsLeft', () => {
	it('countsTheRowsThatHaveAnAnswerAndTheOnesAdded', () => {
		expect(jobProgress(job({ current: 'd' }))).toEqual({ done: 3, total: 4, added: 1, current: 'd', waitAttempt: undefined });
	});

	it('knowsWhichWaitItIsOnlyWhileItWaits', () => {
		expect(jobProgress(job({ waitUntil: '2026-11-18T03:00:50Z', waitAttempt: 2 })).waitAttempt).toBe(2);
		expect(jobProgress(job({ waitAttempt: 2 })).waitAttempt).toBeUndefined();
	});

	it('roundsTheSecondsLeftUpAndNeverGoesBelowZero', () => {
		const j = job({ waitUntil: '2026-11-18T03:00:50Z' });
		expect(waitSecondsLeft(j, new Date('2026-11-18T03:00:00Z'))).toBe(50);
		expect(waitSecondsLeft(j, new Date('2026-11-18T03:00:49.2Z'))).toBe(1);
		expect(waitSecondsLeft(j, new Date('2026-11-18T03:01:00Z'))).toBe(0);
		expect(waitSecondsLeft(job(), new Date())).toBe(0);
	});
});

describe('stopText and finishedRows', () => {
	it('hasASentenceForEveryReasonAJobCanStopFor', () => {
		for (const r of ['stopped_by_user', 'rate_limited', 'tiktok_unavailable', 'tiktok_not_connected', 'internal_error']) expect(stopText(r)).not.toBe(r);
		expect(stopText(undefined)).toBe('');
		expect(stopText('something_new')).toBe('something_new');
	});

	it('finishedRowsLeavesOutTheRowsStillWaitingAndTheOnesNotInTheImport', () => {
		const rows = [sheetRow({ status: 'added' }), sheetRow({ line: 3, status: 'ready' }), sheetRow({ line: 4, status: 'invalid' }), sheetRow({ line: 5, status: 'failed' })];
		expect(finishedRows(rows).map((r) => r.status)).toEqual(['added', 'failed']);
	});
});

describe('rows the server refused', () => {
	it('areShownAsRowsWithAnErrorInTheFileAndNotCountedAsAdded', () => {
		const rows = jobRows(job({ rows: [
			{ line: 2, tiktokId: 'a', status: 'added' },
			{ line: 3, tiktokId: 'bb', status: 'invalid', reason: 'Email không đúng dạng' },
		] }));
		expect(rows[1]).toMatchObject({ status: 'invalid', reason: 'Email không đúng dạng' });
		expect(finishedRows(rows).map((r) => r.status)).toEqual(['added']); // not in the live list
		expect(jobProgress(job({ rows: [{ line: 2, tiktokId: 'a', status: 'invalid' }] })).done).toBe(1); // but it has an answer
	});
});

describe('waitText', () => {
	it('saysWhyTheJobWaitsAndHowLong', () => {
		expect(waitText('rate_limited', 38, 1)).toBe('TikTok đang giới hạn tốc độ. Tự thử lại sau 38 giây (lần 1/3), bạn không cần làm gì.');
		expect(waitText('unavailable', 12, 2)).toBe('TikTok chưa trả lời được. Tự thử lại sau 12 giây (lần 2/3), bạn không cần làm gì.');
		expect(waitText(undefined, 5, undefined)).toMatch(/giới hạn tốc độ/);
	});
});

describe('finishText', () => {
	const row = (status: BulkRow['status']) => sheetRow({ status });
	it('isASuccessWhenSomethingWasAdded', () => {
		expect(finishText([row('added'), row('added'), row('failed')])).toEqual({ message: 'Đã thêm 2 KOC vào danh bạ.', type: 'success' });
	});
	it('isNoAlarmWhenEveryRowWasOnlyAKnownOne', () => {
		const r = finishText([row('duplicate'), row('collaborated')]);
		expect(r.type).toBe('success');
		expect(r.message).toMatch(/Không có KOC mới/);
	});
	it('isAnErrorWhenNothingWasAddedAndSomethingWentWrong', () => {
		expect(finishText([row('duplicate'), row('notListed')]).type).toBe('error');
		expect(finishText([row('skipped')]).type).toBe('error');
		expect(finishText([]).type).toBe('error');
	});
});

describe('shouldShowFinished', () => {
	const ended = (over: Partial<ImportJob> = {}) => job({ status: 'done', finishedAt: '2026-11-18T04:00:00Z', ...over });
	const now = new Date('2026-11-18T10:00:00Z');
	it('offersTheReportOfAJobThatEndedAndWasNotSeen', () => {
		expect(shouldShowFinished(ended(), '', now)).toBe(true);
		expect(shouldShowFinished(ended({ status: 'stopped' }), 'other', now)).toBe(true);
	});
	it('doesNotOfferItAgainOnceSeenOrWhileItRuns', () => {
		expect(shouldShowFinished(ended(), 'j1', now)).toBe(false);
		expect(shouldShowFinished(job(), '', now)).toBe(false);
	});
	it('forgetsAJobThatEndedMoreThanAWeekAgo', () => {
		expect(shouldShowFinished(ended({ finishedAt: '2026-11-01T04:00:00Z' }), '', now)).toBe(false);
	});
});

describe('the preview refuses the emails the server refuses', () => {
	const preview = (email: string) => parseBulkSheet([['TikTok ID', 'Email'], ['abc', email]], []).rows[0];
	it('refusesAnUnderscoreInTheDomainAndALastLabelWithDigits', () => {
		for (const e of ['a@b_c.com', 'a@b.c1', 'a@b.123', 'ab@cd.12']) expect(preview(e)).toMatchObject({ status: 'invalid', reason: 'Email không đúng dạng' });
	});
	it('stillAcceptsOrdinaryEmails', () => {
		for (const e of ['lan@gmail.com', 'a.b+c@shop.com.vn', 'x@mail.co', 'x@b.xn--p1ai']) expect(preview(e).status).toBe('ready');
	});
});
