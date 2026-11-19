import { describe, expect, it } from 'vitest';
import { ReactFlow } from '@xyflow/react';
import {
	MAX_NODES, MAX_WAIT_MINUTES, emptyDraft, loadDraft, portsOf, saveDraft, stateLabel, validateGraph, waitLabel, waitLeftText,
} from './graph';
import type { WorkflowEdge, WorkflowNode } from '../../types/workflow';

const node = (id: string, type: WorkflowNode['type'], over: Partial<WorkflowNode> = {}): WorkflowNode => ({
	id, type, x: 0, y: 0,
	...(type === 'wait' ? { waitMinutes: 60 } : {}),
	...(type === 'send' ? { campaignId: 'c1' } : {}),
	...over,
});
const edge = (from: string, port: WorkflowEdge['port'], to: string): WorkflowEdge => ({ id: `${from}>${to}`, from, port, to });

const messages = (nodes: WorkflowNode[], edges: WorkflowEdge[]) => validateGraph(nodes, edges).problems.map((p) => p.message.toLowerCase());
const has = (nodes: WorkflowNode[], edges: WorkflowEdge[], word: string) => messages(nodes, edges).some((m) => m.includes(word));

describe('theCanvasLibrary', () => {
	it('loadsWithReact19', () => {
		expect(typeof ReactFlow).toBe('object'); // a memo component: the module and its dependencies load
	});
});

describe('validateGraph accepts', () => {
	it('aChainAndASplitAndAlonelmportBlock', () => {
		expect(validateGraph([node('i', 'import'), node('w', 'wait'), node('s', 'send')], [edge('i', 'out', 'w'), edge('w', 'out', 's')]).ok).toBe(true);
		expect(validateGraph(
			[node('i', 'import'), node('s', 'send'), node('q', 'if_replied'), node('s2', 'send'), node('w', 'wait')],
			[edge('i', 'out', 's'), edge('s', 'out', 'q'), edge('q', 'unanswered', 's2'), edge('q', 'replied', 'w')],
		).ok).toBe(true);
		expect(validateGraph([node('i', 'import')], []).ok).toBe(true);
	});

	it('aWaitOfExactlyFourteenDaysAndTwoLongBranches', () => {
		expect(validateGraph([node('i', 'import'), node('w', 'wait', { waitMinutes: MAX_WAIT_MINUTES })], [edge('i', 'out', 'w')]).ok).toBe(true);
		expect(validateGraph(
			[node('i', 'import'), node('q', 'if_replied'), node('a', 'wait', { waitMinutes: 10 * 24 * 60 }), node('b', 'wait', { waitMinutes: 10 * 24 * 60 })],
			[edge('i', 'out', 'q'), edge('q', 'replied', 'a'), edge('q', 'unanswered', 'b')],
		).ok).toBe(true);
	});
});

describe('validateGraph refuses', () => {
	it('noImportAndTwoImports', () => {
		expect(has([node('w', 'wait')], [], 'chưa có khối nhập')).toBe(true);
		expect(has([node('a', 'import'), node('b', 'import')], [], 'chỉ được có một khối nhập')).toBe(true);
	});

	it('aBlockThatIsNotReached', () => {
		const r = validateGraph([node('i', 'import'), node('w', 'wait')], []);
		expect(r.ok).toBe(false);
		expect(r.problems.some((p) => p.nodeId === 'w' && p.message.toLowerCase().includes('chưa được nối'))).toBe(true);
	});

	it('twoWiresIntoOneBlockAndTwoWiresOutOfOnePort', () => {
		const nodes = [node('i', 'import'), node('q', 'if_replied'), node('w', 'wait')];
		const r = validateGraph(nodes, [edge('i', 'out', 'q'), edge('q', 'replied', 'w'), edge('q', 'unanswered', 'w')]);
		expect(r.problems.some((p) => p.nodeId === 'w' && p.message.toLowerCase().includes('nhiều hơn một dây vào'))).toBe(true);
		expect(has([node('i', 'import'), node('a', 'wait'), node('b', 'wait')], [edge('i', 'out', 'a'), edge('i', 'out', 'b')], 'nhiều hơn một dây ra')).toBe(true);
	});

	it('aCycle', () => {
		expect(has([node('i', 'import'), node('a', 'wait'), node('b', 'wait')], [edge('a', 'out', 'b'), edge('b', 'out', 'a')], 'vòng')).toBe(true);
	});

	it('aWireIntoTheImportBlock', () => {
		expect(has([node('i', 'import'), node('w', 'wait')], [edge('i', 'out', 'w'), edge('w', 'out', 'i')], 'không nhận dây vào')).toBe(true);
	});

	it('aWireToAMissingBlockAndAPortTheBlockDoesNotHave', () => {
		expect(has([node('i', 'import')], [edge('i', 'out', 'ghost')], 'không tồn tại')).toBe(true);
		expect(has([node('i', 'import'), node('w', 'wait')], [edge('i', 'out', 'w'), edge('w', 'replied', 'i')], 'cổng')).toBe(true);
	});

	it('aWaitOutOfRangeNamingTheBlock', () => {
		for (const minutes of [0, MAX_WAIT_MINUTES + 1]) {
			const r = validateGraph([node('i', 'import'), node('w', 'wait', { waitMinutes: minutes })], [edge('i', 'out', 'w')]);
			expect(r.problems.some((p) => p.nodeId === 'w' && p.message.toLowerCase().includes('thời gian chờ'))).toBe(true);
		}
	});

	it('aSendBlockWithNoCampaignNamingTheBlock', () => {
		const r = validateGraph([node('i', 'import'), node('s', 'send', { campaignId: '' })], [edge('i', 'out', 's')]);
		expect(r.problems.some((p) => p.nodeId === 's' && p.message.toLowerCase().includes('chiến dịch'))).toBe(true);
	});

	it('moreThanFifteenBlocks', () => {
		const nodes = [node('n0', 'import')];
		const edges: WorkflowEdge[] = [];
		for (let i = 1; i <= MAX_NODES; i++) {
			nodes.push(node(`n${i}`, 'wait'));
			edges.push(edge(`n${i - 1}`, 'out', `n${i}`));
		}
		expect(nodes).toHaveLength(MAX_NODES + 1);
		expect(has(nodes, edges, 'tối đa 15')).toBe(true);
	});

	it('theWaitsOfOneBranchAddingUpPastFourteenDays', () => {
		expect(has(
			[node('i', 'import'), node('a', 'wait', { waitMinutes: 10 * 24 * 60 }), node('b', 'wait', { waitMinutes: 5 * 24 * 60 })],
			[edge('i', 'out', 'a'), edge('a', 'out', 'b')], 'tổng thời gian chờ',
		)).toBe(true);
	});

	it('reportsEveryProblemNotOnlyTheFirst', () => {
		const r = validateGraph([node('i', 'import'), node('w', 'wait', { waitMinutes: 0 }), node('s', 'send', { campaignId: '' })], []);
		expect(r.problems.length).toBeGreaterThanOrEqual(3); // two blocks not reached, a bad wait, no campaign
	});
});

describe('portsOf', () => {
	it('everyBlockHasOutAndIfRepliedHasTwo', () => {
		expect(portsOf('import')).toEqual(['out']);
		expect(portsOf('send')).toEqual(['out']);
		expect(portsOf('if_replied')).toEqual(['replied', 'unanswered']);
	});
});

describe('waitLeftText', () => {
	const now = Date.parse('2026-11-20T10:00:00Z');
	const at = (minutes: number) => new Date(now + minutes * 60000).toISOString();
	it('writesTheRemainingTimeInTheLargestUnits', () => {
		expect(waitLeftText(at(90), now)).toBe('còn 1 giờ 30 phút');
		expect(waitLeftText(at(120), now)).toBe('còn 2 giờ');
		expect(waitLeftText(at(45), now)).toBe('còn 45 phút');
		expect(waitLeftText(at(2 * 24 * 60 + 3 * 60), now)).toBe('còn 2 ngày 3 giờ');
		expect(waitLeftText(at(3 * 24 * 60), now)).toBe('còn 3 ngày');
		expect(waitLeftText(at(0.5), now)).toBe('còn dưới 1 phút');
	});
	it('saysItIsAboutToGoOnceTheTimeHasPassed', () => {
		expect(waitLeftText(at(-5), now)).toBe('sắp tới');
		expect(waitLeftText(undefined, now)).toBe('');
	});
});

describe('waitLabel and stateLabel', () => {
	it('usesTheLargestExactUnit', () => {
		expect(waitLabel(30)).toBe('30 phút');
		expect(waitLabel(120)).toBe('2 giờ');
		expect(waitLabel(3 * 24 * 60)).toBe('3 ngày');
		expect(waitLabel(90)).toBe('1 giờ 30 phút');
	});
	it('namesEveryStatus', () => {
		for (const s of ['pending', 'running', 'waiting', 'done', 'stopped'] as const) expect(stateLabel(s)).not.toBe('');
	});
});

describe('draft', () => {
	it('startsWithAnImportBlock', () => {
		const d = emptyDraft();
		expect(d.nodes).toHaveLength(1);
		expect(d.nodes[0].type).toBe('import');
		expect(d.edges).toEqual([]);
	});

	it('survivesAStorageThatThrowsAndGarbageInIt', () => {
		const original = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
		Object.defineProperty(globalThis, 'localStorage', { configurable: true, get() { throw new Error('blocked'); } });
		try {
			expect(loadDraft().nodes[0].type).toBe('import');
			expect(() => saveDraft(emptyDraft())).not.toThrow();
		} finally {
			if (original) Object.defineProperty(globalThis, 'localStorage', original);
			else delete (globalThis as { localStorage?: unknown }).localStorage;
		}
		const store: Record<string, string> = { 'workflow-draft': '{not json' };
		Object.defineProperty(globalThis, 'localStorage', {
			configurable: true,
			value: { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; } },
		});
		try {
			expect(loadDraft().nodes[0].type).toBe('import');
			const d = { nodes: [node('i', 'import', { x: 5 }), node('w', 'wait')], edges: [edge('i', 'out', 'w')] };
			saveDraft(d);
			expect(loadDraft()).toEqual(d);
		} finally {
			delete (globalThis as { localStorage?: unknown }).localStorage;
		}
	});
});
