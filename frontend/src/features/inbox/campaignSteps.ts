// The logic of the bulk message window that has nothing to do with React: the editable list of steps of a campaign
// (tick, order, edit), what the steps look like for one KOC, the rules that lock the send button, and how a run is
// read for the "Theo dõi" tab. The window that uses it is BulkMessageModal.tsx.

import type { BulkRun, Step, StepKind } from '../../types/campaign';

export const MAX_STEPS = 12;
export const MAX_STEP_TEXT = 2000;
export const MAX_RUN_KOCS = 50;

/** A step in the editor: the step itself, a key that follows it when it moves, and whether it is sent in this run. */
export type DraftStep = Step & { key: string; on: boolean };

let counter = 0;
const nextKey = () => `step-${++counter}`;

const hasImage = (s: Step) => s.kind !== 'image' || !!s.imageId;

/** Every step of the campaign as a draft; all are on, except a picture that has no image yet (it cannot be sent). */
export function toDraft(steps: Step[]): DraftStep[] {
	return steps.map((s) => ({ ...s, key: nextKey(), on: hasImage(s) }));
}

/** Turns one step on or off. A picture without an image stays off. */
export function toggleStep(d: DraftStep[], key: string): DraftStep[] {
	return d.map((s) => (s.key === key ? { ...s, on: hasImage(s) ? !s.on : false } : s));
}

/** Moves the step at `from` to `to`; a position outside the list returns the list as it is. */
export function moveStep(d: DraftStep[], from: number, to: number): DraftStep[] {
	if (from < 0 || to < 0 || from >= d.length || to >= d.length || from === to) return d;
	const next = [...d];
	const [item] = next.splice(from, 1);
	next.splice(to, 0, item);
	return next;
}

export function editStep(d: DraftStep[], key: string, patch: Partial<Step>): DraftStep[] {
	return d.map((s) => {
		if (s.key !== key) return s;
		const merged = { ...s, ...patch };
		return { ...merged, on: merged.on && hasImage(merged) };
	});
}

/** Adds an empty text step (on), or a picture step (off until an image is chosen), at the end. Never over MAX_STEPS. */
export function addStep(d: DraftStep[], kind: StepKind): DraftStep[] {
	if (d.length >= MAX_STEPS) return d;
	const step: DraftStep = kind === 'text' ? { kind, text: '', key: nextKey(), on: true } : { kind, caption: '', key: nextKey(), on: false };
	return [...d, step];
}

export function removeStep(d: DraftStep[], key: string): DraftStep[] {
	return d.filter((s) => s.key !== key);
}

/** The steps to send: only the ones that are on, in their order, as the server expects them. */
export function stepsToSend(d: DraftStep[]): Step[] {
	return d
		.filter((s) => s.on)
		.map((s): Step => (s.kind === 'image' ? { kind: 'image', imageId: s.imageId, caption: s.caption || undefined } : { kind: 'text', text: s.text }));
}

/** What is wrong with the steps that would be sent, in words, or null. Positions count the whole list, as the numbers on screen do. */
export function stepProblem(d: DraftStep[]): string | null {
	if (!d.some((s) => s.on)) return 'Hãy tích ít nhất một tin.';
	for (let i = 0; i < d.length; i++) {
		const s = d[i];
		if (!s.on || s.kind !== 'text') continue;
		const len = (s.text ?? '').trim().length;
		if (len === 0) return `Tin số ${i + 1} đang trống.`;
		if (len > MAX_STEP_TEXT) return `Tin số ${i + 1} dài quá ${MAX_STEP_TEXT} ký tự.`;
	}
	return null;
}

/** {ten} and {kenh} for one KOC: the last word of the name ("bạn" without a name), the channel with its @ (nothing without one). */
export function fillTags(text: string, koc: { name: string; handle: string }): string {
	const words = (koc.name || '').trim().split(/\s+/).filter(Boolean);
	const handle = (koc.handle || '').trim();
	const channel = handle ? (handle.startsWith('@') ? handle : `@${handle}`) : '';
	return text.replaceAll('{ten}', words.length ? words[words.length - 1] : 'bạn').replaceAll('{kenh}', channel);
}

/** One step as one KOC would see it (a picture is shown as "Ảnh", with its caption). */
export function previewLine(s: Step, koc: { name: string; handle: string }): string {
	if (s.kind === 'image') {
		const caption = fillTags(s.caption ?? '', koc).trim();
		return caption ? `Ảnh · ${caption}` : 'Ảnh';
	}
	return fillTags(s.text ?? '', koc);
}

/** How long a run takes, in minutes: about 9.5 seconds per step and 5.5 minutes between two KOCs (same as the server). */
export function estimateMinutes(kocs: number, steps: number): number {
	return Math.round((kocs * (steps * 9.5 + 330)) / 60);
}

/** Why the send button is locked, in the order the user should fix it; null when everything is ready. */
export function sendBlockReason(o: { bulkOn: boolean; isAdmin: boolean; selected: number; problem: string | null; running: boolean; limitLeft: number }): string | null {
	if (o.running) return 'Đang có một đợt gửi chạy. Hãy chờ đợt đó xong hoặc dừng nó.';
	if (!o.bulkOn) return o.isAdmin ? 'Bật công tắc "Gửi hàng loạt" để gửi.' : 'Công tắc "Gửi hàng loạt" đang tắt, nhờ quản trị viên bật.';
	if (o.selected === 0) return 'Hãy chọn ít nhất một KOC.';
	if (o.problem) return o.problem;
	if (o.selected > MAX_RUN_KOCS) return `Mỗi đợt tối đa ${MAX_RUN_KOCS} KOC.`;
	if (o.selected > o.limitLeft) return `Hôm nay chỉ còn ${Math.max(o.limitLeft, 0)} KOC trong hạn mức.`;
	return null;
}

const STATUS_LABELS: Record<string, string> = {
	queued: 'Chờ gửi',
	sending: 'Đang gửi',
	sent: 'Đã gửi đủ',
	replied: 'KOC đã trả lời, dừng chuỗi',
	partial: 'Gửi một phần, đợt bị dừng',
	skipped: 'Bỏ qua',
	failed: 'Zalo từ chối',
	unknown: 'Không rõ, hãy kiểm tra hội thoại',
	not_sent: 'Chưa gửi',
};

export function statusLabel(status: string): string {
	return STATUS_LABELS[status] ?? status;
}

const REASONS: Record<string, string> = {
	not_found: 'Không tìm thấy KOC',
	no_phone: 'Chưa có số điện thoại',
	not_on_zalo: 'Số điện thoại không có tài khoản Zalo',
	on_other_account: 'Đang nói chuyện qua tài khoản Zalo khác',
	dnc_or_blocked: 'Không liên hệ hoặc đã hợp tác',
	already_sent_within_24h: 'Vừa liên hệ trong 24 giờ',
	zalo_not_ready: 'Zalo chưa sẵn sàng',
	zalo_not_delivered: 'Zalo chưa nhận tin',
	step_outcome_unknown: 'Không biết tin cuối đã tới chưa, đã không gửi lại',
	restart_during_step: 'Máy chủ khởi động lại giữa lúc gửi, đã không gửi lại',
	image_missing: 'Không đọc được ảnh của chiến dịch',
	message_not_recorded: 'Đã gửi nhưng chưa ghi lại được tin trong hội thoại',
	reply_check_failed: 'Không kiểm tra được KOC đã trả lời chưa',
	store_unavailable: 'Không lưu được trạng thái',
	stopped: 'Đã dừng',
	switch_off: 'Công tắc gửi hàng loạt đã tắt',
	outside_0830_2030_window: 'Ngoài khung giờ gửi 08:30 đến 20:30',
	zalo_errors: 'Zalo lỗi liên tiếp',
	daily_koc_limit_reached: 'Hết hạn mức KOC hôm nay',
	daily_message_cap_reached: 'Hết hạn mức tin hôm nay',
	internal_error: 'Lỗi hệ thống',
};

/** The sentence for a reason code from the server; a code it does not know is shown as it is (Zalo's own error text, for one). */
export function reasonText(reason?: string): string {
	if (!reason) return '';
	return REASONS[reason] ?? reason;
}

/** A status with its reason in words. */
export function targetText(status: string, reason?: string): string {
	const r = reasonText(reason);
	return r ? `${statusLabel(status)} · ${r}` : statusLabel(status);
}

/** Why a whole run stopped, as a sentence; empty when it did not stop for a reason. */
export function stopText(reason?: string): string {
	return reasonText(reason);
}

export interface RunView {
	done: number;
	total: number;
	/** the KOC whose steps are going out now */
	current?: { name: string; step: number; steps: number };
	nextInMinutes?: number;
	counts: Record<string, number>;
}

/** What the live card of the "Theo dõi" tab shows for a run. */
export function runView(run: BulkRun, now: Date): RunView {
	const counts: Record<string, number> = {};
	let done = 0;
	for (const t of run.targets) {
		counts[t.status] = (counts[t.status] ?? 0) + 1;
		if (t.status !== 'queued' && t.status !== 'sending') done++;
	}
	const sending = run.targets.find((t) => t.status === 'sending');
	const current = sending
		? { name: sending.kocName || sending.handle || sending.kocId, step: Math.min(sending.stepsSent + 1, run.steps.length), steps: run.steps.length }
		: undefined;
	let nextInMinutes: number | undefined;
	if (run.nextAt) {
		const ms = new Date(run.nextAt).getTime() - now.getTime();
		if (ms > 0) nextInMinutes = Math.ceil(ms / 60000);
	}
	return { done, total: run.targets.length, current, nextInMinutes, counts };
}

/** All the steps in their current order, including the ones that are off: what "Lưu làm mặc định" stores in the campaign. */
export function stepsToSave(d: DraftStep[]): Step[] {
	return d.map((s): Step => (s.kind === 'image' ? { kind: 'image', imageId: s.imageId, caption: s.caption || undefined } : { kind: 'text', text: s.text ?? '' }));
}

const vnDay = (d: Date) => d.toLocaleDateString('en-CA', { timeZone: 'Asia/Ho_Chi_Minh' });

/** How many KOCs the runs started on today's date (Vietnam time): what the daily limit counts. */
export function startedToday(runs: BulkRun[], now: Date, accountId?: string, defaultAccountId?: string): number {
	const today = vnDay(now);
	let n = 0;
	for (const run of runs) {
		// every Zalo account has its own daily limit; a run made before there were accounts belongs to the default one
		if (accountId && (run.zaloAccountId || defaultAccountId) !== accountId) continue;
		for (const t of run.targets) if (t.startedAt && vnDay(new Date(t.startedAt)) === today) n++;
	}
	return n;
}

/** "3 giờ 28 phút", "45 phút": a number of minutes for people. */
export function formatDuration(minutes: number): string {
	const m = Math.max(0, Math.round(minutes));
	const h = Math.floor(m / 60);
	const rest = m % 60;
	if (h === 0) return `${rest} phút`;
	return rest === 0 ? `${h} giờ` : `${h} giờ ${rest} phút`;
}

const normal = (s: Step) => (s.kind === 'image' ? { kind: 'image', imageId: s.imageId ?? '', caption: s.caption ?? '' } : { kind: 'text', text: s.text ?? '' });

/** True when two lists hold the same steps in the same order (a missing caption and an empty one are the same). */
export function sameSteps(a: Step[], b: Step[]): boolean {
	return a.length === b.length && a.every((s, i) => JSON.stringify(normal(s)) === JSON.stringify(normal(b[i])));
}
