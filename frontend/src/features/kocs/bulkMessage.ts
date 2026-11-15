// Bulk message to KOCs from a template: who is a recipient, what the preview says, and how a send runs in batches
// (pure, no network apart from the `post` it is given). The window that uses it is BulkMessageModal.tsx.

import type { Creator } from '../../types/koc';

export const MSG_BATCH = 5; // KOCs per request to the server
export const MSG_RECENT_DAYS = 7; // a KOC contacted within this many days starts unticked
export const MSG_MAX_BODY = 2000; // characters in the message

export const MSG_STOPPED_REASON = 'Chưa gửi, đã dừng';
export const MSG_UNKNOWN_REASON = 'Không rõ kết quả, hãy kiểm tra hội thoại';
export const MSG_HALTED_UNKNOWN_REASON = 'Chưa gửi, đã dừng vì lô trước không rõ kết quả';
export const MSG_RATE_REASON = 'Thao tác quá nhanh, hãy thử lại sau ít phút';

export type MsgStatus = 'queued' | 'sent' | 'pending' | 'skipped' | 'not_sent' | 'failed' | 'unknown';

export interface Recipient {
	creator: Creator;
	checked: boolean;
	/** why this KOC cannot be sent to: shown greyed out, cannot be ticked */
	blocked?: string;
	/** contacted this many days ago (inside MSG_RECENT_DAYS): starts unticked but can be ticked */
	recentDaysAgo?: number;
}

export interface SendRow {
	id: string;
	name: string;
	status: MsgStatus;
	reason?: string;
}

/** What POST /kocs/message-batch answers. */
export interface BatchResponse {
	results: { kocId: string; status: string; reason?: string }[];
	stop: boolean;
	stopReason?: string;
}

const REASONS: Record<string, string> = {
	not_found: 'Không tìm thấy KOC',
	no_phone: 'Chưa có số điện thoại',
	not_on_zalo: 'Số điện thoại không có tài khoản Zalo',
	dnc_or_blocked: 'Không liên hệ hoặc đã hợp tác',
	already_sent_within_24h: 'Vừa liên hệ trong 24 giờ',
	daily_quota_reached: 'Hết hạn mức gửi hôm nay, hãy gửi tiếp vào ngày mai',
	outside_0830_2030_window: 'Ngoài khung giờ gửi 08:30 đến 20:30',
	zalo_not_ready: 'Zalo chưa sẵn sàng, tin chưa tới KOC',
	message_not_recorded: 'Đã gửi nhưng chưa ghi lại được tin trong hội thoại',
};

/** The sentence for a server reason code; a code it does not know is shown as it is. */
export function reasonText(reason?: string): string {
	if (!reason) return '';
	return REASONS[reason] ?? reason;
}

/** Why the rest of a batch was not sent, as a sentence about THOSE rows (reasonText may describe the one that was sent). */
const STOP_REASONS: Record<string, string> = {
	message_not_recorded: 'Chưa gửi, đã dừng vì tin trước chưa được ghi lại',
};

export function stopText(code?: string): string {
	if (code && STOP_REASONS[code]) return STOP_REASONS[code];
	return reasonText(code) || MSG_STOPPED_REASON;
}

/** The rows the report must show: everything not sent, and a sent row that carries a warning. */
export function needsAttention(rows: SendRow[]): SendRow[] {
	return rows.filter((r) => r.status !== 'sent' || !!r.reason);
}

/** True when anything may have gone out (even with an unknown outcome): the directory must reload its "last contacted". */
export function shouldRefresh(rows: SendRow[]): boolean {
	return rows.some((r) => r.status === 'sent' || r.status === 'pending' || r.status === 'unknown');
}

/** The message for one KOC: {ten} becomes the last word of the name, or "bạn" without a name (same rule as the server). */
export function renderPreview(body: string, creator: Creator): string {
	const words = (creator.name || '').trim().split(/\s+/).filter(Boolean);
	return body.replaceAll('{ten}', words.length ? words[words.length - 1] : 'bạn');
}

const DAY = 24 * 3600 * 1000;

/** Every KOC of the list as a recipient: blocked ones cannot be ticked, recently contacted ones start unticked. */
export function buildRecipients(creators: Creator[], now: Date = new Date()): Recipient[] {
	return creators.map((creator) => {
		const last = creator.lastContactedAt ? new Date(creator.lastContactedAt).getTime() : NaN;
		const age = Number.isNaN(last) ? Infinity : now.getTime() - last;
		let blocked: string | undefined;
		if (creator.dnc) blocked = 'Không liên hệ';
		else if (creator.daHopTac) blocked = 'Đã hợp tác';
		else if (!creator.phone?.trim()) blocked = 'Chưa có số điện thoại';
		else if (age < DAY) blocked = 'Vừa liên hệ trong 24 giờ';
		if (blocked) return { creator, checked: false, blocked };
		if (age < MSG_RECENT_DAYS * DAY) return { creator, checked: false, recentDaysAgo: Math.floor(age / DAY) };
		return { creator, checked: true };
	});
}

/** The server looked at the request and refused it before sending anything. */
const REFUSED = new Set([400, 401, 403, 404, 409, 422, 429]);

/**
 * Sends the rows to the server in batches, one after the other (never in parallel), and returns every row with its
 * final status. The run stops when the server says stop, when the user stops it, or when a batch fails: a batch with no
 * answer (network error, timeout, 5xx) is `unknown` and is never sent again, because the server may have sent it.
 */
export async function runBulkMessage(
	rows: SendRow[],
	post: (kocIds: string[]) => Promise<BatchResponse>,
	opts: { batchSize?: number; shouldStop: () => boolean; onProgress: (done: number, total: number, rows: SendRow[]) => void },
): Promise<SendRow[]> {
	const out = rows.map((r) => ({ ...r }));
	const size = opts.batchSize ?? MSG_BATCH;
	const snapshot = () => out.map((r) => ({ ...r })); // new objects, so a screen sees every step as a change
	const markRest = (from: number, reason: string) => out.slice(from).forEach((r) => Object.assign(r, { status: 'not_sent', reason }));
	let done = 0;
	opts.onProgress(0, out.length, snapshot());

	for (let at = 0; at < out.length; at += size) {
		if (opts.shouldStop()) {
			markRest(at, MSG_STOPPED_REASON);
			break;
		}
		const batch = out.slice(at, at + size);
		let halt: string | null = null; // set: the rest of the rows are not sent, for this reason
		try {
			const res = await post(batch.map((r) => r.id));
			const byId = new Map(res.results.map((x) => [x.kocId, x]));
			batch.forEach((r) => {
				const x = byId.get(r.id);
				if (x) Object.assign(r, { status: x.status as MsgStatus, reason: (x.status === 'not_sent' ? stopText(x.reason) : reasonText(x.reason)) || undefined });
				else Object.assign(r, { status: 'unknown', reason: MSG_UNKNOWN_REASON });
			});
			if (res.stop) halt = stopText(res.stopReason);
		} catch (e) {
			const err = e as { response?: { status?: number; data?: { message?: string } } } | null;
			const status = err?.response?.status;
			if (status && REFUSED.has(status)) {
				const reason = status === 429 ? MSG_RATE_REASON : err?.response?.data?.message || 'Máy chủ từ chối lô này';
				batch.forEach((r) => Object.assign(r, { status: 'not_sent', reason }));
				halt = MSG_STOPPED_REASON;
			} else {
				batch.forEach((r) => Object.assign(r, { status: 'unknown', reason: MSG_UNKNOWN_REASON }));
				halt = MSG_HALTED_UNKNOWN_REASON;
			}
		}
		done += batch.length;
		if (halt !== null) markRest(at + size, halt);
		opts.onProgress(done, out.length, snapshot());
		if (halt !== null) break;
	}
	return out;
}
