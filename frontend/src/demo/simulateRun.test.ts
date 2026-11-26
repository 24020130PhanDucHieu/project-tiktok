import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { validateGraph } from '../features/workflows/graph';
import type { Workflow } from '../types/workflow';
import { DEMO_KOC_IDS, DEMO_REPLIED, DEMO_WORKFLOW } from './demoWorkflow';
import { simulateRun } from './simulateRun';

describe('demo workflow', () => {
	it('is a diagram the editor itself accepts', () => {
		const check = validateGraph(DEMO_WORKFLOW.nodes, DEMO_WORKFLOW.edges);
		expect(check.problems).toEqual([]);
		expect(check.ok).toBe(true);
	});
});

describe('simulateRun', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('walks every block to done, splits the KOCs at if_replied and ends as done', () => {
		const ticks: Workflow[] = [];
		simulateRun(DEMO_WORKFLOW, (w) => ticks.push(w));
		vi.advanceTimersByTime(60_000);
		const last = ticks.at(-1)!;
		expect(last.status).toBe('done');
		const byId = new Map((last.states ?? []).map((s) => [s.nodeId, s]));
		for (const n of DEMO_WORKFLOW.nodes) expect(byId.get(n.id)?.status).toBe('done');
		expect(byId.get('pick-1')?.count).toBe(DEMO_KOC_IDS.length);
		expect(byId.get('label-1')?.count).toBe(DEMO_REPLIED);
		expect(byId.get('email-1')?.count).toBe(DEMO_KOC_IDS.length - DEMO_REPLIED);
		const branch = (last.log ?? []).filter((e) => e.nodeId === 'if_replied-1' && e.kind === 'out');
		expect(branch.map((e) => e.port).sort()).toEqual(['replied', 'unanswered']);
	});

	it('reports a block as running before it is done, and the wait block as waiting with a countdown', () => {
		const ticks: Workflow[] = [];
		simulateRun(DEMO_WORKFLOW, (w) => ticks.push(w));
		vi.advanceTimersByTime(60_000);
		const seen = (id: string, status: string) => ticks.some((t) => t.states?.some((s) => s.nodeId === id && s.status === status));
		expect(seen('send_text-1', 'running')).toBe(true);
		expect(seen('wait-1', 'waiting')).toBe(true);
		expect(ticks.find((t) => t.states?.some((s) => s.status === 'waiting'))?.states?.find((s) => s.status === 'waiting')?.waitUntil).toBeTruthy();
	});

	it('cancel stops the run where it is and nothing else is ticked', () => {
		const ticks: Workflow[] = [];
		const cancel = simulateRun(DEMO_WORKFLOW, (w) => ticks.push(w));
		vi.advanceTimersByTime(1500);
		cancel();
		const n = ticks.length;
		vi.advanceTimersByTime(60_000);
		expect(ticks.length).toBe(n);
		expect(ticks.at(-1)!.status).toBe('running');
	});

	it('the demo stays in the browser: the module does not use the api client', async () => {
		const src = await import('node:fs').then((fs) => fs.readFileSync(new URL('./simulateRun.ts', import.meta.url), 'utf8'));
		expect(src).not.toMatch(/lib\/axios|fetch\(|XMLHttpRequest/);
	});
});
