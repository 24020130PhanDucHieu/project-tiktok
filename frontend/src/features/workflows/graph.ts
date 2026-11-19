import type { NodeStatus, WorkflowDraft, WorkflowEdge, WorkflowNode, WorkflowNodeType, WorkflowPort } from '../../types/workflow';

// The rules of a graph are the ones of the server (domain.ValidateWorkflowGraph): the screen applies them to show what is
// wrong, the server applies them again because the screen is not trusted.
export const MAX_NODES = 25;
export const MAX_WAIT_MINUTES = 14 * 24 * 60;

const PORTS: Record<WorkflowNodeType, WorkflowPort[]> = {
	import: ['out'],
	pick: ['out'],
	wait: ['out'],
	wait_until: ['out'],
	send: ['out'],
	send_text: ['out'],
	email: ['out'],
	label: ['out'],
	stage: ['out'],
	if_replied: ['replied', 'unanswered'],
};

/** The block types that start a workflow: a workflow has exactly one. */
export function isStartType(type: WorkflowNodeType): boolean {
	return type === 'import' || type === 'pick';
}

/** The block types that message people by themselves: a workflow with one needs an admin and a confirmation. */
export function isSendingType(type: WorkflowNodeType): boolean {
	return type === 'send' || type === 'send_text' || type === 'email';
}

export const MAX_TEXT = 2000;
export const MAX_SUBJECT = 150;
export const MAX_PICK = 200;
export const MAX_PICK_STAGES = 5;
/** The stages a stage block may move KOCs to (the server's domain.AllowedStageTargets). */
export const STAGE_TARGET_VALUES = ['CONTACTED', 'NO_REPLY', 'REPLIED', 'DORMANT', 'STALLED'];
/** Every stage a KOC can be in (the server's domain.KocStages). */
export const KOC_STAGE_VALUES = [
	'SOURCED', 'QUALIFIED', 'AWAITING_APPROVAL', 'FILTERED_OUT', 'CONTACTED', 'NO_REPLY', 'REPLIED', 'DORMANT', 'AGREED', 'DECLINED',
	'SHIPPING', 'DELIVERED', 'VIDEO_POSTED', 'NOT_FIT', 'RETURNING', 'RETURNED', 'STALLED', 'COMPLETED',
];

const LABEL_ID = /^[0-9a-f]{24}$/;

/** What is wrong with the settings of one block, or '' when they are fine (the server's configProblem). */
function configProblem(n: WorkflowNode): string {
	const chars = (v?: string) => Array.from(v ?? '').length;
	switch (n.type) {
		case 'wait':
			return (n.waitMinutes ?? 0) >= 1 && (n.waitMinutes ?? 0) <= MAX_WAIT_MINUTES ? '' : 'Thời gian chờ phải từ 1 phút đến 14 ngày.';
		case 'wait_until':
			return Number.isInteger(n.atMinute) && (n.atMinute ?? -1) >= 0 && (n.atMinute ?? -1) <= 1439 ? '' : 'Giờ chờ không hợp lệ.';
		case 'send':
			return n.campaignId ? '' : 'Khối Gửi chiến dịch chưa chọn chiến dịch.';
		case 'send_text':
			return chars(n.text) >= 1 && chars(n.text) <= MAX_TEXT ? '' : `Khối Gửi tin Zalo lẻ cần nội dung từ 1 đến ${MAX_TEXT} ký tự.`;
		case 'email':
			if (!(chars(n.text) >= 1 && chars(n.text) <= MAX_TEXT)) return `Khối Gửi email cần nội dung từ 1 đến ${MAX_TEXT} ký tự.`;
			return chars(n.subject) <= MAX_SUBJECT ? '' : `Chủ đề email tối đa ${MAX_SUBJECT} ký tự.`;
		case 'label':
			return LABEL_ID.test(n.labelId ?? '') ? '' : 'Khối Gắn nhãn chưa chọn nhãn.';
		case 'stage':
			return STAGE_TARGET_VALUES.includes(n.stage ?? '') ? '' : 'Khối Đổi giai đoạn chưa chọn giai đoạn.';
		case 'pick': {
			const stages = n.stages ?? [];
			if (stages.length === 0 && !n.folder) return 'Khối Lấy KOC có sẵn cần chọn giai đoạn hoặc thư mục.';
			if (stages.length > MAX_PICK_STAGES) return `Khối Lấy KOC có sẵn chọn tối đa ${MAX_PICK_STAGES} giai đoạn.`;
			if (stages.some((s) => !KOC_STAGE_VALUES.includes(s))) return 'Khối Lấy KOC có sẵn có giai đoạn không hợp lệ.';
			if (!((n.limit ?? 0) >= 1 && (n.limit ?? 0) <= MAX_PICK)) return `Giới hạn số KOC của khối Lấy KOC có sẵn từ 1 đến ${MAX_PICK}.`;
			return '';
		}
		default:
			return '';
	}
}

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
		const own = n.type === 'wait' ? n.waitMinutes ?? 0 : n.type === 'wait_until' ? 24 * 60 : 0; // a wait until holds a branch a day at most
		return own + best;
	};
	const start = nodes.find((n) => isStartType(n.type));
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
	const starts = nodes.filter((n) => isStartType(n.type));
	if (starts.length === 0) add('Quy trình chưa có khối bắt đầu (Nhập Excel hoặc Lấy KOC có sẵn).');
	if (starts.length > 1) add('Quy trình chỉ được có một khối bắt đầu.', starts[1].id);

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
		if (isStartType(byId.get(id)!.type)) add('Khối bắt đầu không nhận dây vào.', id);
		else if (count > 1) add(`Khối "${id}" có nhiều hơn một dây vào.`, id);
	}

	for (const n of nodes) {
		const problem = configProblem(n);
		if (problem) add(problem, n.id);
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
		for (const n of starts) walk(n.id);
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
