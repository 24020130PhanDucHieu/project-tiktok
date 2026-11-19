import type { NodeStatus, WorkflowDraft, WorkflowEdge, WorkflowNode, WorkflowNodeType, WorkflowPort } from '../../types/workflow';

// The rules of a graph are the ones of the server (domain.ValidateWorkflowGraph): the screen applies them to show what is
// wrong, the server applies them again because the screen is not trusted.
export const MAX_NODES = 15;
export const MAX_WAIT_MINUTES = 14 * 24 * 60;

const PORTS: Record<WorkflowNodeType, WorkflowPort[]> = {
	import: ['out'],
	wait: ['out'],
	send: ['out'],
	if_replied: ['replied', 'unanswered'],
};

/** The output ports of a block type. */
export function portsOf(type: WorkflowNodeType): WorkflowPort[] {
	return PORTS[type] ?? [];
}

export interface GraphProblem {
	/** the block to mark, when the problem belongs to one */
	nodeId?: string;
	message: string;
}

export interface GraphCheck {
	ok: boolean;
	problems: GraphProblem[];
}

function nextOf(edges: WorkflowEdge[], from: string, port: WorkflowPort): string | undefined {
	return edges.find((e) => e.from === from && e.port === port)?.to;
}

/** The longest time one branch waits, from the import block to the end of the branch. */
function longestWait(nodes: WorkflowNode[], edges: WorkflowEdge[]): number {
	const byId = new Map(nodes.map((n) => [n.id, n]));
	const walk = (id: string, seen: Set<string>): number => {
		const n = byId.get(id);
		if (!n || seen.has(id)) return 0;
		seen.add(id);
		let best = 0;
		for (const port of portsOf(n.type)) {
			const to = nextOf(edges, id, port);
			if (to) best = Math.max(best, walk(to, seen));
		}
		seen.delete(id);
		return (n.type === 'wait' ? n.waitMinutes ?? 0 : 0) + best;
	};
	const start = nodes.find((n) => n.type === 'import');
	return start ? walk(start.id, new Set()) : 0;
}

/** Says in words everything that is wrong with a graph (not only the first thing). */
export function validateGraph(nodes: WorkflowNode[], edges: WorkflowEdge[]): GraphCheck {
	const problems: GraphProblem[] = [];
	const add = (message: string, nodeId?: string) => problems.push({ message, nodeId });

	if (nodes.length > MAX_NODES) add(`Quy trình có tối đa ${MAX_NODES} khối.`);
	const byId = new Map<string, WorkflowNode>();
	for (const n of nodes) {
		if (byId.has(n.id)) add('Có hai khối trùng mã.', n.id);
		byId.set(n.id, n);
	}
	const imports = nodes.filter((n) => n.type === 'import');
	if (imports.length === 0) add('Quy trình chưa có khối Nhập Excel.');
	if (imports.length > 1) add('Quy trình chỉ được có một khối Nhập Excel.', imports[1].id);

	const outs = new Map<string, number>();
	const ins = new Map<string, number>();
	let wiresOk = true;
	for (const e of edges) {
		const from = byId.get(e.from);
		if (!from || !byId.has(e.to)) {
			add('Có dây nối tới một khối không tồn tại.');
			wiresOk = false;
			continue;
		}
		if (!portsOf(from.type).includes(e.port)) {
			add(`Khối "${e.from}" không có cổng ra "${e.port}".`, e.from);
			wiresOk = false;
			continue;
		}
		outs.set(`${e.from}/${e.port}`, (outs.get(`${e.from}/${e.port}`) ?? 0) + 1);
		ins.set(e.to, (ins.get(e.to) ?? 0) + 1);
	}
	for (const [key, count] of outs) {
		if (count > 1) {
			const [from, port] = key.split('/');
			add(`Cổng "${port}" của khối "${from}" có nhiều hơn một dây ra.`, from);
		}
	}
	for (const [id, count] of ins) {
		if (byId.get(id)?.type === 'import') add('Khối Nhập Excel không nhận dây vào.', id);
		else if (count > 1) add(`Khối "${id}" có nhiều hơn một dây vào.`, id);
	}

	for (const n of nodes) {
		if (n.type === 'wait' && !((n.waitMinutes ?? 0) >= 1 && (n.waitMinutes ?? 0) <= MAX_WAIT_MINUTES)) {
			add('Thời gian chờ phải từ 1 phút đến 14 ngày.', n.id);
		}
		if (n.type === 'send' && !n.campaignId) add('Khối Gửi chiến dịch chưa chọn chiến dịch.', n.id);
	}

	let cycle = false;
	if (wiresOk) {
		const state = new Map<string, number>(); // 1 being followed, 2 done
		const visit = (id: string): boolean => {
			state.set(id, 1);
			for (const port of portsOf(byId.get(id)!.type)) {
				const to = nextOf(edges, id, port);
				if (!to) continue;
				if (state.get(to) === 1 || (!state.has(to) && visit(to))) return true;
			}
			state.set(id, 2);
			return false;
		};
		for (const n of nodes) if (!state.has(n.id) && visit(n.id)) cycle = true;
		if (cycle) add('Các khối đang nối thành vòng.');

		const reached = new Set<string>();
		const walk = (id: string) => {
			reached.add(id);
			for (const port of portsOf(byId.get(id)!.type)) {
				const to = nextOf(edges, id, port);
				if (to && !reached.has(to)) walk(to);
			}
		};
		for (const n of imports) walk(n.id);
		for (const n of nodes) if (!reached.has(n.id)) add(`Khối "${n.id}" chưa được nối vào quy trình.`, n.id);
	}

	if (wiresOk && !cycle && longestWait(nodes, edges) > MAX_WAIT_MINUTES) add('Tổng thời gian chờ của một nhánh không được quá 14 ngày.');
	return { ok: problems.length === 0, problems };
}

/** "còn 1 giờ 30 phút": how long a waiting block still waits. */
export function waitLeftText(untilIso: string | undefined, nowMs: number): string {
	if (!untilIso) return '';
	const diff = Date.parse(untilIso) - nowMs;
	if (!(diff > 0)) return 'sắp tới';
	const minutes = Math.floor(diff / 60000);
	if (minutes < 1) return 'còn dưới 1 phút';
	const days = Math.floor(minutes / 1440);
	const hours = Math.floor((minutes % 1440) / 60);
	const mins = minutes % 60;
	if (days >= 1) return `còn ${days} ngày${hours ? ` ${hours} giờ` : ''}`;
	if (hours >= 1) return `còn ${hours} giờ${mins ? ` ${mins} phút` : ''}`;
	return `còn ${mins} phút`;
}

/** "2 giờ", "3 ngày": the wait of a block, in the largest unit that fits exactly. */
export function waitLabel(minutes: number): string {
	if (minutes % 1440 === 0) return `${minutes / 1440} ngày`;
	if (minutes % 60 === 0) return `${minutes / 60} giờ`;
	if (minutes > 60) return `${Math.floor(minutes / 60)} giờ ${minutes % 60} phút`;
	return `${minutes} phút`;
}

const STATE_LABELS: Record<NodeStatus, string> = {
	pending: 'Chưa tới lượt',
	running: 'Đang chạy',
	waiting: 'Đang chờ',
	done: 'Xong',
	stopped: 'Đã dừng',
};

export function stateLabel(status: NodeStatus): string {
	return STATE_LABELS[status] ?? '';
}

export function emptyDraft(): WorkflowDraft {
	return { nodes: [{ id: 'import-1', type: 'import', x: 40, y: 140 }], edges: [] };
}

const DRAFT_KEY = 'workflow-draft';

/** The graph that was being drawn, kept in this browser only. Storage may be blocked or hold garbage: then a new draft. */
export function loadDraft(): WorkflowDraft {
	try {
		const raw = localStorage.getItem(DRAFT_KEY);
		if (raw) {
			const parsed = JSON.parse(raw) as Partial<WorkflowDraft>;
			if (Array.isArray(parsed.nodes) && Array.isArray(parsed.edges) && parsed.nodes.length > 0) {
				return { nodes: parsed.nodes, edges: parsed.edges };
			}
		}
	} catch {
		// blocked storage or a damaged draft: start again
	}
	return emptyDraft();
}

export function saveDraft(draft: WorkflowDraft): void {
	try {
		localStorage.setItem(DRAFT_KEY, JSON.stringify(draft));
	} catch {
		// the draft is a convenience: nothing is lost if it cannot be kept
	}
}

/** An id for a new block: "send-1", "send-2"... not used by another block. */
export function nextNodeId(type: WorkflowNodeType, taken: string[]): string {
	let n = 1;
	while (taken.includes(`${type}-${n}`)) n++;
	return `${type}-${n}`;
}
