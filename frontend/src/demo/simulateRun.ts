import type { NodeState, Workflow, WorkflowDraft, WorkflowEdge, WorkflowEvent, WorkflowNode } from '../types/workflow';
import { DEMO_KOC_IDS, DEMO_KOC_NAMES, DEMO_REPLIED, DEMO_WORKFLOW_NAME } from './demoWorkflow';

/** What the simulated "wait" block shows as its countdown, in milliseconds (the real wait is a day). */
const SIMULATED_WAIT_MS = 4000;

interface Step {
	node: WorkflowNode;
	kocs: string[];
	/** for if_replied: the KOCs that took each branch */
	branches?: { replied: string[]; unanswered: string[] };
}

/** Walks the graph from its start block and works out which KOCs reach each block (a pure function, no timers). */
function plan(wf: WorkflowDraft): Step[] {
	const byId = new Map(wf.nodes.map((n) => [n.id, n]));
	const next = (id: string, port: WorkflowEdge['port']) => wf.edges.find((e) => e.from === id && e.port === port)?.to;
	const start = wf.nodes.find((n) => n.type === 'pick' || n.type === 'import');
	const steps: Step[] = [];
	const queue: { id: string; kocs: string[] }[] = start ? [{ id: start.id, kocs: DEMO_KOC_IDS }] : [];
	const seen = new Set<string>();
	while (queue.length) {
		const { id, kocs } = queue.shift()!;
		const node = byId.get(id);
		if (!node || seen.has(id)) continue;
		seen.add(id);
		if (node.type === 'if_replied') {
			const branches = { replied: kocs.slice(0, DEMO_REPLIED), unanswered: kocs.slice(DEMO_REPLIED) };
			steps.push({ node, kocs, branches });
			for (const port of ['replied', 'unanswered'] as const) {
				const to = next(id, port);
				if (to) queue.push({ id: to, kocs: branches[port] });
			}
			continue;
		}
		steps.push({ node, kocs });
		const to = next(id, 'out');
		if (to) queue.push({ id: to, kocs });
	}
	return steps;
}

/**
 * Plays a made-up run of the graph: each block goes running → done (a wait block shows a short countdown) and the
 * report fills in with fake KOCs. It only calls onTick: nothing is sent, saved or requested. Returns a cancel function.
 */
export function simulateRun(wf: WorkflowDraft, onTick: (run: Workflow) => void, stepMs = 1200): () => void {
	const steps = plan(wf);
	const states = new Map<string, NodeState>();
	const log: WorkflowEvent[] = [];
	const createdAt = new Date().toISOString();
	const emit = (status: Workflow['status']) =>
		onTick({
			_id: 'demo-run',
			name: DEMO_WORKFLOW_NAME,
			nodes: wf.nodes,
			edges: wf.edges,
			status,
			by: 'Demo',
			states: [...states.values()],
			log: [...log],
			kocNames: DEMO_KOC_NAMES,
			createdAt,
			...(status === 'running' ? {} : { finishedAt: new Date().toISOString() }),
		});
	const event = (nodeId: string, kind: WorkflowEvent['kind'], kocs: string[], port?: string) => log.push({ at: new Date().toISOString(), nodeId, kind, kocs, ...(port ? { port } : {}) });

	const timers: ReturnType<typeof setTimeout>[] = [];
	let delay = 0;
	const at = (ms: number, fn: () => void) => timers.push(setTimeout(fn, ms));

	steps.forEach(({ node, kocs, branches }) => {
		const start = delay;
		at(start, () => {
			states.set(node.id, { nodeId: node.id, status: node.type === 'wait' ? 'waiting' : 'running', count: kocs.length, ...(node.type === 'wait' ? { waitUntil: new Date(Date.now() + SIMULATED_WAIT_MS).toISOString() } : {}) });
			if (node.type !== 'pick' && node.type !== 'import') event(node.id, 'in', kocs);
			emit('running');
		});
		const length = node.type === 'wait' ? SIMULATED_WAIT_MS : stepMs;
		at(start + length, () => {
			states.set(node.id, { nodeId: node.id, status: 'done', count: kocs.length });
			if (branches) {
				event(node.id, 'out', branches.replied, 'replied');
				event(node.id, 'out', branches.unanswered, 'unanswered');
			} else if (node.type === 'label' || node.type === 'stage') event(node.id, 'did', kocs);
			else event(node.id, 'out', kocs, 'out');
			emit('running');
		});
		delay = start + length;
	});
	at(delay + 300, () => emit('done'));
	return () => timers.forEach(clearTimeout);
}
