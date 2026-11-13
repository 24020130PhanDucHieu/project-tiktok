// Bulk import of KOCs from an Excel sheet: reading and checking the rows (pure, no network). The window that uses it
// is BulkImportModal.tsx.

export const BULK_MAX_ROWS = 200; // rows per file
export const BULK_BATCH = 10; // rows per request to the server

export const NO_FOLDER = 'Chưa phân loại';

export type BulkRowStatus = 'ready' | 'invalid' | 'added' | 'duplicate' | 'collaborated' | 'notListed' | 'failed' | 'skipped';

export interface BulkRow {
	/** the row number in Excel (the header is row 1) */
	line: number;
	tiktokId: string;
	email: string;
	phone: string;
	recipient: string;
	shippingAddress: string;
	castFee: number;
	category: string;
	status: BulkRowStatus;
	reason?: string;
}

const COLUMNS = {
	tiktokId: 'tiktok id',
	email: 'email',
	phone: 'so dien thoai',
	recipient: 'nguoi nhan mau',
	shippingAddress: 'dia chi nhan mau',
	castFee: 'phi cast',
	category: 'thu muc',
} as const;

/** lowercase, no accents, single spaces: "Số  điện thoại" and "so dien thoai" are the same column */
const plain = (s: string) =>
	s
		.normalize('NFD')
		.replace(/[̀-ͯ]/g, '')
		.replace(/đ/g, 'd')
		.replace(/Đ/g, 'D')
		.toLowerCase()
		.replace(/\s+/g, ' ')
		.trim();

const text = (v: unknown): string => (v === null || v === undefined ? '' : String(v).trim());

/** "@Ten_Kenh", "ten_kenh" or a link like https://www.tiktok.com/@ten_kenh?lang=vi -> "ten_kenh" */
function tiktokIdOf(v: string): string {
	const link = v.match(/tiktok\.com\/@([^/?#\s]+)/i);
	return (link ? link[1] : v.replace(/^@+/, '')).trim().toLowerCase();
}

/** Excel turns 0976451571 into the number 976451571 unless the cell is text: put the zero back, and accept +84 forms. */
function phoneOf(v: unknown): { value: string; error?: string } {
	let digits = text(v).replace(/\D/g, '');
	if (!digits) return { value: '' };
	// +84 976..., 84976... and +84 0976... are all 0976...; any other number without its 0 lost it in Excel (also landlines)
	if (digits.startsWith('84') && digits.length >= 11) digits = `0${digits.slice(2).replace(/^0+/, '')}`;
	else if (!digits.startsWith('0')) digits = `0${digits}`;
	if (digits.length < 10 || digits.length > 11) return { value: digits, error: 'Số điện thoại phải có 10 hoặc 11 chữ số' };
	return { value: digits };
}

/** 500000, "500.000" and "500.000đ" are all 500000; empty is 0; anything else is null (not a valid fee). */
function feeOf(v: unknown): number | null {
	if (typeof v === 'number') return Number.isFinite(v) && v >= 0 ? Math.round(v) : null;
	const t = text(v).replace(/vnd|đ|d/gi, '').replace(/\s/g, '');
	if (!t) return 0;
	const digits = t.replace(/[.,]/g, '');
	return /^\d+$/.test(digits) ? Number(digits) : null;
}

/** Close to what the server accepts: a row the server would refuse must not reach it, it would fail the whole batch. */
function isEmail(v: string): boolean {
	if (/[\s,;]/.test(v)) return false;
	const parts = v.split('@');
	if (parts.length !== 2) return false;
	const [local, domain] = parts;
	if (!local || local.startsWith('.') || local.endsWith('.') || local.includes('..')) return false;
	const labels = domain.split('.');
	if (labels.length < 2 || labels.some((l) => !l || l.startsWith('-') || l.endsWith('-'))) return false;
	return /^[a-z0-9-]{2,}$/i.test(labels[labels.length - 1]);
}

const MAX_RECIPIENT = 120; // the server's limits for the same fields
const MAX_ADDRESS = 300;

/**
 * Turns the cells of the first sheet into rows to import. Row 1 is the header; columns are found by name, so their order
 * does not matter and extra columns are ignored. Blank rows are skipped (their numbers still count). Each row is
 * `ready` or `invalid` with the reason, and nothing here talks to the server.
 */
export function parseBulkSheet(cells: unknown[][], folders: string[]): { rows: BulkRow[]; fatal?: string; tooMany?: boolean } {
	if (!cells.length) return { rows: [], fatal: 'File trống.' };
	const index: Partial<Record<keyof typeof COLUMNS, number>> = {};
	cells[0].forEach((h, i) => {
		const name = plain(text(h));
		(Object.keys(COLUMNS) as (keyof typeof COLUMNS)[]).forEach((k) => {
			if (COLUMNS[k] === name && index[k] === undefined) index[k] = i;
		});
	});
	if (index.tiktokId === undefined) return { rows: [], fatal: 'Thiếu cột TikTok ID. Hãy điền vào file mẫu.' };

	const cell = (r: unknown[], k: keyof typeof COLUMNS) => (index[k] === undefined ? '' : r[index[k] as number]);
	const data: { line: number; r: unknown[] }[] = [];
	cells.slice(1).forEach((r, i) => {
		if (r.some((c) => text(c) !== '')) data.push({ line: i + 2, r });
	});
	if (data.length === 0) return { rows: [], fatal: 'File không có dòng dữ liệu.' };
	if (data.length > BULK_MAX_ROWS) return { rows: [], tooMany: true };

	const seen = new Map<string, number>();
	const rows = data.map(({ line, r }): BulkRow => {
		const phone = phoneOf(cell(r, 'phone'));
		const fee = feeOf(cell(r, 'castFee'));
		const folder = text(cell(r, 'category'));
		const row: BulkRow = {
			line,
			tiktokId: tiktokIdOf(text(cell(r, 'tiktokId'))),
			email: text(cell(r, 'email')).toLowerCase(),
			phone: phone.value,
			recipient: text(cell(r, 'recipient')),
			shippingAddress: text(cell(r, 'shippingAddress')),
			castFee: fee ?? 0,
			category: folder || NO_FOLDER,
			status: 'ready',
		};
		let reason = '';
		if (!row.tiktokId) reason = 'Thiếu TikTok ID';
		else if (row.tiktokId.length < 2 || row.tiktokId.length > 100) reason = 'TikTok ID phải từ 2 đến 100 ký tự';
		else if (row.email && !isEmail(row.email)) reason = 'Email không đúng dạng';
		else if ([...row.recipient].length > MAX_RECIPIENT) reason = `Người nhận mẫu tối đa ${MAX_RECIPIENT} ký tự`;
		else if ([...row.shippingAddress].length > MAX_ADDRESS) reason = `Địa chỉ nhận mẫu tối đa ${MAX_ADDRESS} ký tự`;
		else if (phone.error) reason = phone.error;
		else if (fee === null) reason = 'Phí cast không hợp lệ';
		else if (folder && folder !== NO_FOLDER && !folders.includes(folder)) reason = 'Thư mục chưa có';
		else if (seen.has(row.tiktokId)) reason = `Trùng trong file, đã có ở dòng ${seen.get(row.tiktokId)}`;
		if (row.tiktokId && !seen.has(row.tiktokId)) seen.set(row.tiktokId, line);
		if (reason) {
			row.status = 'invalid';
			row.reason = reason;
		}
		return row;
	});
	return { rows, tooMany: false };
}

/** The element sent to POST /kocs/bulk. */
export function toBulkItem(row: BulkRow): Record<string, unknown> {
	return {
		tiktokId: row.tiktokId,
		email: row.email,
		phone: row.phone,
		recipient: row.recipient,
		shippingAddress: row.shippingAddress,
		castFee: row.castFee,
		category: row.category,
	};
}

// ---- running the import -------------------------------------------------------------------------------------------

export interface BulkOutcome {
	tiktokId: string;
	reason?: string;
	/** for notVerified rows: not_listed (TikTok has no such creator), rate_limited or unavailable (TikTok could not be asked) */
	kind?: string;
}

/** What POST /kocs/bulk answers: the rows it saved and why it did not save the others. */
export interface BulkReport {
	imported: { tiktokId: string }[];
	skippedDuplicates: BulkOutcome[];
	blockedCollaborated: BulkOutcome[];
	notVerified: BulkOutcome[];
}

export const COLLABORATED_REASON = 'KOC đã từng hợp tác với shop nên bị chặn';
export const STOPPED_REASON = 'Chưa nhập, đã dừng';
export const NO_ANSWER_REASON = 'Máy chủ không trả kết quả cho dòng này';
export const RATE_REASON = 'TikTok đang giới hạn tốc độ, hãy thử lại sau ít phút';
export const UNAVAILABLE_REASON = 'Chưa xác minh được với TikTok Shop lúc này, hãy thử lại sau ít phút';

/** Reads a file without waiting for ever (a workbook the browser cannot unzip must end in a message, not a spinner). */
export function withTimeout<T>(p: Promise<T>, ms: number): Promise<T> {
	return new Promise((resolve, reject) => {
		const timer = setTimeout(() => reject(new Error('timeout')), ms);
		p.then(
			(v) => {
				clearTimeout(timer);
				resolve(v);
			},
			(e) => {
				clearTimeout(timer);
				reject(e);
			},
		);
	});
}

/** The message of a failed request: the server's own sentence when there is one. */
function messageOf(e: unknown): string {
	const err = e as { response?: { data?: { message?: string } }; message?: string } | null;
	return err?.response?.data?.message || err?.message || 'Không gửi được lô này';
}

type Outcome = { status: BulkRowStatus; reason?: string };

/** When TikTok cannot be asked, every row after this one would fail the same way within milliseconds: say why and stop. */
function haltReason(report: BulkReport): string {
	if (report.notVerified.some((o) => o.kind === 'rate_limited')) return RATE_REASON;
	if (report.notVerified.some((o) => o.kind === 'unavailable')) return UNAVAILABLE_REASON;
	return '';
}

/** Which row went where, by TikTok ID, from the server's report. */
function outcomesOf(report: BulkReport): Map<string, Outcome> {
	const map = new Map<string, Outcome>();
	report.skippedDuplicates.forEach((o) => map.set(o.tiktokId, { status: 'duplicate', reason: o.reason }));
	report.blockedCollaborated.forEach((o) => map.set(o.tiktokId, { status: 'collaborated', reason: COLLABORATED_REASON }));
	// only "not_listed" says something about the creator; a rate limit or a shop that is not connected is not their fault
	report.notVerified.forEach((o) => map.set(o.tiktokId, { status: !o.kind || o.kind === 'not_listed' ? 'notListed' : 'failed', reason: o.reason }));
	report.imported.forEach((o) => map.set(o.tiktokId, { status: 'added' }));
	return map;
}

/**
 * Sends the `ready` rows to the server in batches (one after the other, never in parallel: every row asks TikTok Shop)
 * and returns every row with its final status. A batch that fails marks only its own rows `failed` and the next ones
 * still run; stopping leaves the rows not yet sent `skipped`. Rows that are not `ready` are never sent.
 */
export async function runBulkImport(
	rows: BulkRow[],
	post: (items: Record<string, unknown>[]) => Promise<BulkReport>,
	opts: { batchSize?: number; shouldStop: () => boolean; onProgress: (done: number, total: number, rows: BulkRow[]) => void },
): Promise<BulkRow[]> {
	const out = rows.map((r) => ({ ...r }));
	const todo = out.filter((r) => r.status === 'ready');
	const size = opts.batchSize ?? BULK_BATCH;
	let done = 0;
	const snapshot = () => out.map((r) => ({ ...r })); // new objects, so a screen sees every step as a change
	opts.onProgress(0, todo.length, snapshot());

	for (let at = 0; at < todo.length; at += size) {
		if (opts.shouldStop()) {
			todo.slice(at).forEach((r) => Object.assign(r, { status: 'skipped', reason: STOPPED_REASON }));
			break;
		}
		const batch = todo.slice(at, at + size);
		let halt = '';
		try {
			const report = await post(batch.map(toBulkItem));
			const outcomes = outcomesOf(report);
			batch.forEach((r) => Object.assign(r, outcomes.get(r.tiktokId) ?? { status: 'failed', reason: NO_ANSWER_REASON }));
			halt = haltReason(report);
		} catch (e) {
			const reason = messageOf(e);
			batch.forEach((r) => Object.assign(r, { status: 'failed', reason }));
			if ((e as { response?: { status?: number } } | null)?.response?.status === 429) halt = RATE_REASON; // the route's own limit
		}
		done += batch.length;
		if (halt) {
			todo.slice(at + size).forEach((r) => Object.assign(r, { status: 'skipped', reason: halt }));
			opts.onProgress(done, todo.length, snapshot());
			break;
		}
		opts.onProgress(done, todo.length, snapshot());
	}
	return out;
}
