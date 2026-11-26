import { useEffect, useMemo, useRef, useState } from 'react';
import { DEMO_WORKFLOW, DEMO_WORKFLOW_NAME } from '../../demo/demoWorkflow';
import { DEMO_LABELS, DEMO_ZALO_ACCOUNT } from '../../demo/inboxData';
import { simulateRun } from '../../demo/simulateRun';
import type { Workflow } from '../../types/workflow';
import WorkflowRunView from './WorkflowRunView';

const ZALO_ACCOUNTS = [DEMO_ZALO_ACCOUNT];

function idleWorkflow(): Workflow {
	return { _id: 'demo-run', name: DEMO_WORKFLOW_NAME, nodes: DEMO_WORKFLOW.nodes, edges: DEMO_WORKFLOW.edges, status: 'done', by: 'Demo', states: [], log: [], createdAt: new Date().toISOString() };
}

/** The ready-made diagram of the Quy trình tab, with a button that plays a made-up run of it. Nothing leaves the browser. */
export default function WorkflowDemo({ onClose }: { onClose: () => void }) {
	const idle = useMemo(idleWorkflow, []);
	const [run, setRun] = useState<Workflow>(idle);
	const [phase, setPhase] = useState<'idle' | 'running' | 'done'>('idle');
	const cancel = useRef<(() => void) | null>(null);
	useEffect(() => () => cancel.current?.(), []);

	function start() {
		cancel.current?.();
		setPhase('running');
		cancel.current = simulateRun(DEMO_WORKFLOW, (w) => {
			setRun(w);
			if (w.status === 'done') setPhase('done');
		});
	}
	function stop() {
		cancel.current?.();
		setRun(idle);
		setPhase('idle');
	}

	return (
		<WorkflowRunView
			workflow={run}
			labels={DEMO_LABELS}
			zaloAccounts={ZALO_ACCOUNTS}
			isAdmin
			onStop={stop}
			onNew={onClose}
			newLabel="‹ Về trình vẽ"
			demo={{ phase, onRun: start }}
		/>
	);
}
