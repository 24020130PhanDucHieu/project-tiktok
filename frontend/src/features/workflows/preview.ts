import type { WorkflowEdge, WorkflowNode } from '../../types/workflow';
import { STAGE_LABELS, formatClock } from './blocks';
import { isSendingType, isStartType, waitLabel } from './graph';

const BRANCH_TEXT: Record<string, string> = { replied: 'đã trả lời', unanswered: 'chưa trả lời' };

/**
 * What a workflow will do after its first block, in order, to how many KOCs: the lines the preview shows under the list of
 * names. After a block that may skip some KOCs (a send, the reply check) the number is "at most".
 */
export function previewEffects(nodes: WorkflowNode[], edges: WorkflowEdge[], count: number): { lines: string[]; sends: boolean } {
	const byId = new Map(nodes.map((n) => [n.id, n]));
	const start = nodes.find((n) => isStartType(n.type));
	const lines: string[] = [];
	let sends = false;
	if (!start) return { lines, sends };
	const seen = new Set<string>([start.id]);
	// breadth first, so a block that waits is listed before what comes after it
	const queue: { id: string; exact: boolean; branch: string }[] = [{ id: start.id, exact: true, branch: '' }];
	while (queue.length) {
		const cur = queue.shift()!;
		for (const e of edges.filter((x) => x.from === cur.id)) {
			const node = byId.get(e.to);
			if (!node || seen.has(node.id)) continue;
			seen.add(node.id);
			const branch = cur.branch || (nodes.find((x) => x.id === cur.id)?.type === 'if_replied' ? BRANCH_TEXT[e.port] ?? '' : '');
			const where = branch ? ` (ở nhánh "${branch}")` : '';
			let exact = cur.exact;
			const n = exact ? `${count}` : `tối đa ${count}`;
			switch (node.type) {
				case 'wait': lines.push(`Chờ ${waitLabel(node.waitMinutes ?? 0)}`); break;
				case 'wait_until': lines.push(`Chờ đến ${formatClock(node.atMinute ?? 0)} (giờ Việt Nam)`); break;
				case 'label': lines.push(`Gắn nhãn cho ${n} KOC${where}`); break;
				case 'stage': lines.push(`Đổi giai đoạn sang ${STAGE_LABELS[node.stage ?? ''] ?? node.stage ?? ''} cho ${n} KOC${where}`); break;
				case 'if_replied': lines.push('Tách KOC đã trả lời và chưa trả lời'); exact = false; break;
				case 'send': lines.push(`Gửi chiến dịch Zalo cho tối đa ${count} KOC${where}`); exact = false; break;
				case 'send_text': lines.push(`Gửi tin Zalo cho tối đa ${count} KOC${where}`); exact = false; break;
				case 'email': lines.push(`Gửi email cho tối đa ${count} KOC${where} (người đã được liên hệ trong 24 giờ qua sẽ bị bỏ qua)`); exact = false; break;
				default: break;
			}
			if (isSendingType(node.type)) sends = true;
			queue.push({ id: node.id, exact, branch });
		}
	}
	return { lines, sends };
}
