// Bulk import of KOCs from an Excel sheet: reading and checking the rows (pure, no network). The window that uses it
// is BulkImportModal.tsx.

export const BULK_MAX_ROWS = 200; // rows per file

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

/** One row of POST /import-jobs (the fields of the sheet row; the window adds its line). */
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

// ---- the background import ----------------------------------------------------------------------------------------
// The server imports the rows one after the other (POST /import-jobs) and writes each answer into the job; the window only
// starts it, asks for the job every few seconds (GET /import-jobs/:id) and shows what it says. Reloading the page changes
// nothing: GET /import-jobs/current brings the running job back.

export const POLL_MS = 4000; // how often the window asks the server how far the job is
export const RATE_MAX_WAITS = 3; // the server waits and tries again this many times in a row before it stops the job

export type JobStatus = 'running' | 'done' | 'stopped';

/** One row of a job as the server keeps it. */
export interface ImportJobRow {
	line: number;
	tiktokId: string;
	email?: string;
	phone?: string;
	recipient?: string;
	shippingAddress?: string;
	category?: string;
	castFee?: number;
	/** queued, added, duplicate, collaborated, not_listed or failed */
	status: string;
	reason?: string;
}

export interface ImportJob {
	_id: string;
	status: JobStatus;
	stopReason?: string;
	stopRequested?: boolean;
	rows: ImportJobRow[];
	/** the TikTok id being checked now */
	current?: string;
	/** TikTok is limiting calls: the time of the next try, and which wait this is */
	waitUntil?: string;
	waitAttempt?: number;
	by: string;
	createdAt: string;
	finishedAt?: string;
}

export const STOPPED_REASON = 'Chưa nhập, đã dừng';

const STOP_REASONS: Record<string, string> = {
	stopped_by_user: 'Đã dừng theo yêu cầu.',
	rate_limited: 'TikTok đang giới hạn tốc độ, hãy thử lại sau ít phút.',
	tiktok_unavailable: 'Chưa xác minh được với TikTok Shop lúc này, hãy thử lại sau ít phút.',
	internal_error: 'Máy chủ gặp lỗi nên đã dừng. Các dòng chưa nhập vẫn còn nguyên, hãy thử lại.',
};

/** Why a job stopped, as a sentence; empty when it did not stop for a reason. */
export function stopText(reason?: string): string {
	return reason ? (STOP_REASONS[reason] ?? reason) : '';
}

/** The body of POST /import-jobs: the ready rows, each with its line in the sheet. */
export function startRequest(rows: BulkRow[]): { items: Record<string, unknown>[] } {
	return { items: rows.filter((r) => r.status === 'ready').map((r) => ({ ...toBulkItem(r), line: r.line })) };
}

const ROW_STATUS: Record<string, BulkRowStatus> = {
	added: 'added',
	duplicate: 'duplicate',
	collaborated: 'collaborated',
	not_listed: 'notListed',
	failed: 'failed',
};

/**
 * The rows of a job as the window shows them. A row still queued is waiting while the job runs ("ready"); once the job has
 * ended it was never reached ("skipped", with the reason the job stopped).
 */
export function jobRows(job: ImportJob): BulkRow[] {
	return job.rows.map((r) => {
		const base = {
			line: r.line,
			tiktokId: r.tiktokId,
			email: r.email ?? '',
			phone: r.phone ?? '',
			recipient: r.recipient ?? '',
			shippingAddress: r.shippingAddress ?? '',
			castFee: r.castFee ?? 0,
			category: r.category || NO_FOLDER,
		};
		const mapped = ROW_STATUS[r.status];
		if (mapped) return { ...base, status: mapped, reason: r.reason };
		if (job.status === 'running') return { ...base, status: 'ready' as const };
		return { ...base, status: 'skipped' as const, reason: stopText(job.stopReason) || STOPPED_REASON };
	});
}

export interface JobProgress {
	/** the rows that have an answer */
	done: number;
	total: number;
	added: number;
	/** the TikTok id being checked now */
	current?: string;
	/** which wait for TikTok this is (1 to RATE_MAX_WAITS), when the job is waiting */
	waitAttempt?: number;
}

export function jobProgress(job: ImportJob): JobProgress {
	const done = job.rows.filter((r) => r.status !== 'queued').length;
	const added = job.rows.filter((r) => r.status === 'added').length;
	return { done, total: job.rows.length, added, current: job.current, waitAttempt: job.waitUntil ? job.waitAttempt : undefined };
}

/** Whole seconds until the job tries TikTok again (0 when it is not waiting or the time has passed). */
export function waitSecondsLeft(job: ImportJob, now: Date): number {
	if (!job.waitUntil) return 0;
	return Math.max(0, Math.ceil((new Date(job.waitUntil).getTime() - now.getTime()) / 1000));
}

/** The rows of the import that already have an answer (added, refused, failed or skipped), in file order: the live list. */
export function finishedRows(rows: BulkRow[]): BulkRow[] {
	return rows.filter((r) => r.status !== 'ready' && r.status !== 'invalid');
}
