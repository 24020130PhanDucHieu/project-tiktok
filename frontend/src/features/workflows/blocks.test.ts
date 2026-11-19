import { describe, expect, it } from 'vitest';
import type { Campaign } from '../../types/campaign';
import type { WorkflowEdge, WorkflowNode, WorkflowNodeType } from '../../types/workflow';
import { BLOCKS, BLOCK_BY_TYPE, STAGE_LABELS, STAGE_TARGETS, formatClock, isSendingType, isStartType, nodeSummary } from './blocks';
import { MAX_NODES, loadDraft, validateGraph } from './graph';

const n = (id: string, type: WorkflowNodeType, over: Partial<WorkflowNode> = {}): WorkflowNode => ({ id, type, x: 0, y: 0, ...over });
const edge = (from: string, to: string, port: WorkflowEdge['port'] = 'out'): WorkflowEdge => ({ id: `${from}>${to}`, from, port, to });
const chain = (...nodes: WorkflowNode[]): [WorkflowNode[], WorkflowEdge[]] => [nodes, nodes.slice(1).map((x, i) => edge(nodes[i].id, x.id))];

const LABEL = '65a1b2c3d4e5f60718293a4b';
const messages = (nodes: WorkflowNode[], edges: WorkflowEdge[]) => validateGraph(nodes, edges).problems.map((p) => p.message.toLowerCase());
const refused = (nodes: WorkflowNode[], edges: WorkflowEdge[], word: string) => messages(nodes, edges).some((m) => m.includes(word));

const pick = n('p', 'pick', { stages: ['SHIPPING'], limit: 50 });
const start = n('i', 'import');

describe('the six new blocks are accepted when they are set up', () => {
	it.each([
		['a pick, a wait until and a text', [pick, n('u', 'wait_until', { atMinute: 540 }), n('t', 'send_text', { text: 'Chào {ten}' })]],
		['a pick by folder only', [n('p', 'pick', { folder: 'Làm đẹp', limit: 10 })]],
		['a label and a stage', [start, n('l', 'label', { labelId: LABEL }), n('s', 'stage', { stage: 'NO_REPLY' })]],
		['an email without a subject', [start, n('e', 'email', { text: 'Xin chào' })]],
		['midnight', [start, n('u', 'wait_until', { atMinute: 0 })]],
		['the last minute of the day', [start, n('u', 'wait_until', { atMinute: 1439 })]],
	])('%s', (_name, nodes) => {
		const [ns, es] = chain(...(nodes as WorkflowNode[]));
		expect(validateGraph(ns, es).ok).toBe(true);
	});
});

describe('one block starts a workflow: an import or a pick', () => {
	it('refusesTwoStartsAndNoStart', () => {
		expect(refused([start, pick], [], 'chỉ được có một khối bắt đầu')).toBe(true);
		expect(refused([n('w', 'wait', { waitMinutes: 5 })], [], 'chưa có khối bắt đầu')).toBe(true);
	});
	it('doesNotLetAWireEnterAPickBlock', () => {
		expect(refused([start, pick], [edge('i', 'p')], 'không nhận dây vào')).toBe(true);
	});
});

describe('the settings of the new blocks are checked', () => {
	it.each([
		['a text with no content', n('t', 'send_text'), 'nội dung'],
		['a text that is too long', n('t', 'send_text', { text: 'a'.repeat(2001) }), 'nội dung'],
		['an email with no content', n('e', 'email'), 'nội dung'],
		['a subject that is too long', n('e', 'email', { text: 'x', subject: 'a'.repeat(151) }), 'chủ đề'],
		['a label that is not an id', n('l', 'label', { labelId: 'nhan' }), 'nhãn'],
		['a stage that is not offered', n('s', 'stage', { stage: 'COMPLETED' }), 'giai đoạn'],
		['no stage', n('s', 'stage'), 'giai đoạn'],
		['a time of day that does not exist', n('u', 'wait_until', { atMinute: 1440 }), 'giờ chờ'],
	])('%s', (_name, node, word) => {
		const [ns, es] = chain(start, node);
		const r = validateGraph(ns, es);
		expect(r.ok).toBe(false);
		expect(r.problems.some((p) => p.nodeId === node.id && p.message.toLowerCase().includes(word))).toBe(true);
	});

	it.each([
		['neither a stage nor a folder', n('p', 'pick', { limit: 10 }), 'giai đoạn hoặc thư mục'],
		['a stage that does not exist', n('p', 'pick', { stages: ['NOPE'], limit: 10 }), 'giai đoạn'],
		['six stages', n('p', 'pick', { stages: ['SOURCED', 'QUALIFIED', 'CONTACTED', 'REPLIED', 'AGREED', 'SHIPPING'], limit: 10 }), 'giai đoạn'],
		['no limit', n('p', 'pick', { stages: ['SHIPPING'] }), 'giới hạn'],
		['a limit over 200', n('p', 'pick', { stages: ['SHIPPING'], limit: 201 }), 'giới hạn'],
	])('a pick with %s', (_name, node, word) => {
		const r = validateGraph([node], []);
		expect(r.problems.some((p) => p.nodeId === 'p' && p.message.toLowerCase().includes(word))).toBe(true);
	});

	it('allowsTwentyFiveBlocksAndRefusesTwentySix', () => {
		const make = (count: number): [WorkflowNode[], WorkflowEdge[]] =>
			chain(start, ...Array.from({ length: count - 1 }, (_, i) => n(`l${i}`, 'label', { labelId: LABEL })));
		expect(MAX_NODES).toBe(25);
		expect(validateGraph(...make(25)).ok).toBe(true);
		expect(refused(...make(26), 'tối đa 25')).toBe(true);
	});

	it('countsAWaitUntilAsADayInTheTotalWait', () => {
		const [a, ae] = chain(start, n('u', 'wait_until', { atMinute: 540 }), n('w', 'wait', { waitMinutes: 14 * 24 * 60 }));
		expect(refused(a, ae, 'tổng thời gian chờ')).toBe(true); // a day plus fourteen days
		const [b, be] = chain(start, n('u', 'wait_until', { atMinute: 540 }), n('w', 'wait', { waitMinutes: 13 * 24 * 60 }));
		expect(validateGraph(b, be).ok).toBe(true);
	});
});

describe('the catalogue of blocks', () => {
	const types: WorkflowNodeType[] = ['import', 'pick', 'wait', 'wait_until', 'send', 'send_text', 'email', 'label', 'stage', 'if_replied'];

	it('describesEveryBlockOnce', () => {
		expect(BLOCKS.map((b) => b.type).sort()).toEqual([...types].sort());
		for (const t of types) {
			const b = BLOCK_BY_TYPE[t];
			expect(b.title.length).toBeGreaterThan(2);
			expect(b.hint.length).toBeGreaterThan(10);
			expect(b.input.length).toBeGreaterThan(5); // what comes in, shown on the left of the block when it is selected
			expect(b.output.length).toBeGreaterThan(5); // and what goes out, on the right
			expect(['start', 'control', 'action']).toContain(b.group);
		}
	});

	it('knowsWhichBlocksStartAndWhichMessagePeople', () => {
		expect(types.filter(isStartType)).toEqual(['import', 'pick']);
		expect(types.filter(isSendingType)).toEqual(['send', 'send_text', 'email']);
		expect(BLOCKS.filter((b) => b.sends).map((b) => b.type).sort()).toEqual(['email', 'send', 'send_text']);
	});

	it('offersTheSameStagesAsTheServerForAStageBlock', () => {
		expect(STAGE_TARGETS.map((s) => s.value)).toEqual(['CONTACTED', 'NO_REPLY', 'REPLIED', 'DORMANT', 'STALLED']);
		for (const s of STAGE_TARGETS) expect(s.label).toBe(STAGE_LABELS[s.value]);
		expect(Object.keys(STAGE_LABELS)).toHaveLength(18);
	});

	it('writesTheClockTime', () => {
		expect(formatClock(540)).toBe('09:00');
		expect(formatClock(0)).toBe('00:00');
		expect(formatClock(1439)).toBe('23:59');
	});
});

describe('nodeSummary', () => {
	const campaigns = [{ _id: 'c1', name: 'Liên hệ KOC mới', steps: [], updatedAt: '' }] as Campaign[];
	const labels = [{ _id: LABEL, name: 'Không phản hồi', color: '#ef4444' }];
	const s = (node: WorkflowNode) => nodeSummary(node, { campaigns, labels, rows: 0 });

	it('saysWhatEachBlockIsSetTo', () => {
		expect(s(n('w', 'wait', { waitMinutes: 2880 }))).toBe('Chờ 2 ngày');
		expect(s(n('u', 'wait_until', { atMinute: 540 }))).toBe('Chờ đến 09:00');
		expect(s(n('s', 'send', { campaignId: 'c1' }))).toBe('Liên hệ KOC mới');
		expect(s(n('l', 'label', { labelId: LABEL }))).toBe('Không phản hồi');
		expect(s(n('g', 'stage', { stage: 'NO_REPLY' }))).toBe('Chưa phản hồi');
		expect(s(n('p', 'pick', { stages: ['SHIPPING', 'DELIVERED'], limit: 30 }))).toBe('Đang giao, Đã nhận mẫu (tối đa 30)');
		expect(s(n('e', 'email', { subject: 'Hợp tác', text: 'x' }))).toBe('Hợp tác');
		expect(s(n('t', 'send_text', { text: 'Chào {ten}' }))).toBe('Chào {ten}');
	});

	it('cutsALongTextAndSaysWhatIsMissing', () => {
		expect(s(n('t', 'send_text', { text: 'a'.repeat(100) })).length).toBeLessThanOrEqual(34);
		expect(s(n('s', 'send'))).toBe('Chưa chọn chiến dịch');
		expect(s(n('l', 'label'))).toBe('Chưa chọn nhãn');
		expect(s(n('g', 'stage'))).toBe('Chưa chọn giai đoạn');
		expect(s(n('t', 'send_text'))).toBe('Chưa có nội dung');
		expect(s(n('e', 'email', { text: 'x' }))).toBe('Email, không chủ đề');
		expect(s(n('i', 'import'))).toBe('Chưa chọn file Excel');
		expect(s(n('p', 'pick'))).toBe('Chưa chọn giai đoạn');
	});

	it('asksToChooseAgainWhenTheCampaignOrTheLabelWasDeleted', () => {
		expect(s(n('s', 'send', { campaignId: 'da-xoa' }))).toBe('Chọn lại chiến dịch (đã bị xóa)');
		expect(s(n('l', 'label', { labelId: '65a1b2c3d4e5f60718293a4c' }))).toBe('Chọn lại nhãn (đã bị xóa)');
		// the lists have not arrived yet (or this is a running workflow, which carries the name the server took)
		expect(nodeSummary(n('s', 'send', { campaignId: 'c1' }), { campaigns: [], labels, rows: 0 })).toBe('Chiến dịch đã chọn');
		expect(nodeSummary(n('s', 'send', { campaignId: 'c1', campaignName: 'Tên cũ' }), { campaigns: [], labels, rows: 0 })).toBe('Tên cũ');
		expect(nodeSummary(n('l', 'label', { labelId: LABEL }), { campaigns, labels: [], rows: 0 })).toBe('Nhãn đã chọn');
	});

	it('showsTheNumberOfRowsOfTheChosenSheet', () => {
		expect(nodeSummary(n('i', 'import'), { campaigns, labels, rows: 12 })).toBe('12 dòng sẽ nhập');
	});
});

describe('a draft kept by the first version still opens', () => {
	it('loadsABrowserDraftThatOnlyHasTheFourOldBlocks', () => {
		const old = {
			nodes: [n('import-1', 'import'), n('wait-1', 'wait', { waitMinutes: 60 }), n('send-1', 'send', { campaignId: 'c1' }), n('if_replied-1', 'if_replied')],
			edges: [edge('import-1', 'wait-1')],
		};
		const store: Record<string, string> = { 'workflow-draft': JSON.stringify(old) };
		Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: { getItem: (k: string) => store[k] ?? null, setItem: () => undefined } });
		try {
			const d = loadDraft();
			expect(d.nodes).toHaveLength(4);
			expect(validateGraph(d.nodes, d.edges).ok).toBe(false); // not wired yet, as it was saved: it opens, it is just not ready
			expect(d.nodes[0].type).toBe('import');
		} finally {
			delete (globalThis as { localStorage?: unknown }).localStorage;
		}
	});
});
