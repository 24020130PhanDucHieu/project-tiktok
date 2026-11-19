/** How a wire looks in a run: "done" when the block at its end finished, "active" while the KOCs are on their way or being worked on. */
export type EdgeFlow = 'done' | 'active' | 'idle';

export function edgeFlow(source: string | undefined, target: string | undefined, running: boolean): EdgeFlow {
	if (target === 'done') return 'done';
	if (!running) return 'idle';
	if (target === 'running' || target === 'waiting') return 'active';
	if (source === 'done' || source === 'running') return 'active';
	return 'idle';
}

/** How far a run is: the blocks that are done out of all the blocks. */
export function progressOf(nodes: { id: string }[], states: { nodeId: string; status: string }[]): { done: number; total: number; percent: number } {
	const total = nodes.length;
	const ids = new Set(nodes.map((n) => n.id));
	const done = states.filter((s) => s.status === 'done' && ids.has(s.nodeId)).length;
	return { done, total, percent: total === 0 ? 0 : Math.round((done / total) * 100) };
}
