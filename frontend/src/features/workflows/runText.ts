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

/** The report of an ended workflow: what it did on each block, or why it stopped. */
export function finishText(wf: Workflow): { message: string; type: 'success' | 'error' } {
	if (wf.status === 'stopped') return { message: workflowStopText(wf.stopReason) || 'Quy trình đã dừng.', type: 'error' };
	const count = (type: string) => (wf.nodes ?? []).filter((n) => n.type === type).map((n) => wf.states?.find((s) => s.nodeId === n.id)?.count ?? 0);
	const imported = count('import')[0] ?? 0;
	const sent = count('send');
	const parts = [`nhập ${imported} KOC`];
	if (sent.length) parts.push(`gửi tin cho ${sent.join(', ')} KOC (theo từng bước gửi)`);
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
