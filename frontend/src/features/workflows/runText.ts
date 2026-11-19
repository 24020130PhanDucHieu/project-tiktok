import type { Workflow } from '../../types/workflow';
import { stopText } from '../kocs/bulkImport';

const SIMPLE: Record<string, string> = {
	stopped_by_user: 'Đã dừng theo yêu cầu.',
	expired: 'Quy trình chạy quá 14 ngày nên đã tự dừng.',
	import_busy: 'Một đợt nhập khác chạy quá lâu nên quy trình đã dừng. Hãy chạy lại sau khi đợt đó xong.',
	send_busy: 'Một đợt gửi khác chạy quá lâu nên quy trình đã dừng. Hãy chạy lại sau khi đợt đó xong.',
	import_failed: 'Không bắt đầu được bước nhập KOC. Hãy thử lại.',
	send_failed: 'Không bắt đầu được bước gửi tin. Hãy thử lại.',
	internal_error: 'Máy chủ gặp lỗi nên quy trình đã dừng.',
};

const SEND_REASONS: Record<string, string> = {
	switch_off: 'công tắc Gửi hàng loạt đã bị tắt.',
	zalo_errors: 'Zalo lỗi nhiều lần liên tiếp.',
	zalo_not_ready: 'Zalo cá nhân chưa kết nối.',
	store_unavailable: 'máy chủ chưa đọc được dữ liệu.',
	internal_error: 'máy chủ gặp lỗi.',
};

/**
 * Said wherever a workflow that sends is shown. The switch of "Gửi hàng loạt" turns itself off after every run, so between two
 * runs of a workflow it looks off whatever anybody did: it cannot stop the next run. Only the stop button of the workflow can.
 */
export const SWITCH_HINT =
	'Công tắc "Gửi hàng loạt" tự tắt sau mỗi lượt gửi, và quy trình tự bật lại cho lượt kế tiếp. Muốn dừng việc gửi, hãy bấm "Dừng quy trình" ở tab này, đừng chỉ tắt công tắc.';

/** Why a workflow stopped, as a sentence; empty when it did not stop for a reason. */
export function workflowStopText(reason?: string): string {
	if (!reason) return '';
	if (SIMPLE[reason]) return SIMPLE[reason];
	if (reason.startsWith('import_stopped:')) {
		const detail = stopText(reason.slice('import_stopped:'.length));
		return `Bước nhập KOC đã dừng. ${detail}`.trim();
	}
	if (reason.startsWith('send_stopped:')) {
		const code = reason.slice('send_stopped:'.length);
		return `Bước gửi tin đã dừng vì ${SEND_REASONS[code] ?? 'một lý do chưa rõ.'}`;
	}
	return `Quy trình đã dừng (${reason}).`;
}

/** How long a run took: "40 giây", "7 phút", "2 giờ 20 phút", "2 ngày 3 giờ". */
export function durationText(startIso: string, endIso: string): string {
	const s = Math.max(0, Math.floor((Date.parse(endIso) - Date.parse(startIso)) / 1000));
	if (s < 60) return `${s} giây`;
	const m = Math.floor(s / 60);
	if (m < 60) return `${m} phút`;
	const h = Math.floor(m / 60);
	if (h < 24) return m % 60 ? `${h} giờ ${m % 60} phút` : `${h} giờ`;
	const d = Math.floor(h / 24);
	return h % 24 ? `${d} ngày ${h % 24} giờ` : `${d} ngày`;
}

/** The report of an ended workflow: what it did on each block, or why it stopped. */
export function finishText(wf: Workflow): { message: string; type: 'success' | 'error' } {
	if (wf.status === 'stopped') return { message: workflowStopText(wf.stopReason) || 'Quy trình đã dừng.', type: 'error' };
	const count = (type: string) => (wf.nodes ?? []).filter((n) => n.type === type).map((n) => wf.states?.find((s) => s.nodeId === n.id)?.count ?? 0);
	const parts: string[] = [];
	const imported = count('import');
	if (imported.length) parts.push(`nhập ${imported[0]} KOC`);
	const picked = count('pick');
	if (picked.length) parts.push(`lấy ${picked[0]} KOC có sẵn`);
	const sent = count('send');
	if (sent.length) parts.push(`gửi tin cho ${sent.join(', ')} KOC (theo từng bước gửi)`);
	const texts = count('send_text');
	if (texts.length) parts.push(`gửi tin Zalo cho ${texts.join(', ')} KOC`);
	const mails = count('email');
	if (mails.length) parts.push(`gửi email cho ${mails.join(', ')} KOC`);
	const staged = count('stage');
	if (staged.length) parts.push(`đổi giai đoạn cho ${staged.join(', ')} KOC`);
	const labeled = count('label');
	if (labeled.length) parts.push(`gắn nhãn cho ${labeled.join(', ')} KOC`);
	if (!parts.length) parts.push('không có gì để làm');
	return { message: `Quy trình đã xong: ${parts.join(', ')}.`, type: 'success' };
}

const SEEN_KEY = 'koc_workflow_seen';
const FINISHED_FOR_MS = 7 * 24 * 3600 * 1000; // an ended workflow is offered again for this long, until the person has seen it

/** The id of the last ended workflow the person has seen (kept in this browser). */
export function seenWorkflowId(): string {
	try {
		return localStorage.getItem(SEEN_KEY) ?? '';
	} catch {
		return '';
	}
}

export function markWorkflowSeen(id: string): void {
	try {
		localStorage.setItem(SEEN_KEY, id);
	} catch {
		/* a browser that keeps nothing simply shows the report again */
	}
}

/** True when an ended workflow has a report the person has not seen yet. */
export function shouldShowFinished(wf: Workflow, seenId: string, now: Date): boolean {
	if (wf.status === 'running' || wf._id === seenId) return false;
	const ended = new Date(wf.finishedAt ?? wf.createdAt).getTime();
	return now.getTime() - ended < FINISHED_FOR_MS;
}
