import type { Workflow, WorkflowEvent, WorkflowNode } from '../../types/workflow';
import { BLOCK_BY_TYPE, STAGE_LABELS } from './blocks';

/** One line of the report under a run: what happened, at which block, and to which KOCs (by name). */
export interface ReportLine {
	at: string;
	nodeId: string;
	title: string;
	text: string;
	names: string[];
	tone: 'ok' | 'skip' | 'info';
}

const NO_NAME = 'KOC không còn trong danh sách';

function textOf(ev: WorkflowEvent, node: WorkflowNode): { text: string; tone: ReportLine['tone'] } | null {
	const n = ev.kocs.length;
	if (ev.kind === 'in') return { text: `Nhận ${n} KOC`, tone: 'info' };
	if (ev.kind === 'skip') return { text: `Bỏ qua: ${n} KOC`, tone: 'skip' };
	if (ev.kind === 'did') {
		if (node.type === 'stage') return { text: `Đã đổi sang ${STAGE_LABELS[node.stage ?? ''] ?? node.stage ?? ''}: ${n} KOC`, tone: 'ok' };
		if (node.type === 'label') return { text: `Đã gắn nhãn: ${n} KOC`, tone: 'ok' };
		return { text: `Đã xử lý: ${n} KOC`, tone: 'ok' };
	}
	// "out": for blocks that only mark or move KOCs along, leaving is not news
	switch (node.type) {
		case 'import': return { text: `Nhập được ${n} KOC`, tone: 'ok' };
		case 'pick': return { text: `Lấy được ${n} KOC`, tone: 'ok' };
		case 'send': case 'send_text': return { text: `Đã gửi tin cho ${n} KOC`, tone: 'ok' };
		case 'email': return { text: `Đã gửi email cho ${n} KOC`, tone: 'ok' };
		case 'if_replied':
			return ev.port === 'replied'
				? { text: `Đã trả lời, đi nhánh "đã trả lời": ${n} KOC`, tone: 'ok' }
				: { text: `Chưa trả lời, đi nhánh "chưa trả lời": ${n} KOC`, tone: 'ok' };
		case 'wait': case 'wait_until': return { text: `Chờ xong, đi tiếp: ${n} KOC`, tone: 'ok' };
		default: return null;
	}
}

/** The report of a run: the log of the server, oldest first, with the names of the KOCs and the titles of the blocks. */
export function buildReport(wf: Workflow): ReportLine[] {
	const nodes = new Map(wf.nodes.map((n) => [n.id, n]));
	const names = wf.kocNames ?? {};
	const events = [...(wf.log ?? [])].sort((a, b) => (a.at < b.at ? -1 : a.at > b.at ? 1 : 0));
	const lines: ReportLine[] = [];
	for (const ev of events) {
		const node = nodes.get(ev.nodeId);
		const meta = node ? BLOCK_BY_TYPE[node.type] : undefined;
		if (!node || !meta) continue;
		const t = textOf(ev, node);
		if (!t) continue;
		lines.push({ at: ev.at, nodeId: ev.nodeId, title: meta.title, text: t.text, tone: t.tone, names: ev.kocs.map((id) => names[id] ?? NO_NAME) });
	}
	return lines;
}
