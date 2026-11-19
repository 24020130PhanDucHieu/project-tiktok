import '@xyflow/react/dist/style.css';
import { Background, type Edge, MarkerType, ReactFlow } from '@xyflow/react';
import { useEffect, useMemo, useState } from 'react';
import type { Workflow } from '../../types/workflow';
import { SWITCH_HINT, finishText } from './runText';
import WorkflowNodeCard, { type WfNode } from './WorkflowNodeCard';

const nodeTypes = { wf: WorkflowNodeCard };
const STATUS_TEXT = { running: 'Đang chạy', done: 'Đã xong', stopped: 'Đã dừng' } as const;

interface Props {
	workflow: Workflow;
	isAdmin: boolean;
	onStop: () => void;
	/** leaves the report and goes back to drawing a new workflow */
	onNew: () => void;
}

/** A workflow that has started or ended: the same canvas, read-only, with the state of every block on it. */
export default function WorkflowRunView({ workflow, isAdmin, onStop, onNew }: Props) {
	const running = workflow.status === 'running';
	const [now, setNow] = useState(() => Date.now());
	useEffect(() => {
		if (!running) return;
		const timer = setInterval(() => setNow(Date.now()), 1000); // the countdown of a waiting block
		return () => clearInterval(timer);
	}, [running]);

	const states = useMemo(() => new Map((workflow.states ?? []).map((s) => [s.nodeId, s])), [workflow.states]);
	const nodes: WfNode[] = useMemo(() => workflow.nodes.map((n) => ({
		id: n.id, type: 'wf', position: { x: n.x, y: n.y }, draggable: false, selectable: false, connectable: false,
		data: { kind: n.type, waitMinutes: n.waitMinutes, campaignId: n.campaignId, campaignName: n.campaignName, state: states.get(n.id) ?? { nodeId: n.id, status: 'pending', count: 0 }, nowMs: now },
	})), [workflow.nodes, states, now]);
	const edges: Edge[] = useMemo(() => workflow.edges.map((e) => {
		const target = states.get(e.to)?.status;
		return {
			id: e.id, source: e.from, sourceHandle: e.port, target: e.to, type: 'smoothstep', selectable: false,
			animated: running && (target === 'running' || target === 'waiting'), markerEnd: { type: MarkerType.ArrowClosed },
		};
	}), [workflow.edges, states, running]);

	const report = running ? null : finishText(workflow);
	return (
		<div className="wf-run">
			<div className="wf-run-bar">
				<div>
					<h2>{workflow.name}</h2>
					<p className="wf-run-sub">Bắt đầu {new Date(workflow.createdAt).toLocaleString('vi-VN')} bởi {workflow.by}. Máy chủ tự chạy, bạn có thể đóng trang.</p>
				</div>
				<span className={`wf-run-status ${workflow.status}`}>{STATUS_TEXT[workflow.status]}</span>
				<span className="wf-run-spacer" />
				{running
					? isAdmin && <button type="button" className="tm-btn-danger" onClick={onStop}>Dừng quy trình</button>
					: <button type="button" className="tm-btn-submit" onClick={onNew}>Vẽ quy trình mới</button>}
			</div>
			{report && <p className={`wf-run-note ${report.type === 'error' ? 'bad' : ''}`} role="status">{report.message}</p>}
			{running && workflow.nodes.some((n) => n.type === 'send') && <p className="wf-run-note">{SWITCH_HINT}</p>}
			<div className="wf-canvas wf-run-canvas">
				<ReactFlow
					nodes={nodes} edges={edges} nodeTypes={nodeTypes} nodesDraggable={false} nodesConnectable={false} elementsSelectable={false}
					fitView fitViewOptions={{ padding: 0.25, maxZoom: 1 }} proOptions={{ hideAttribution: true }} minZoom={0.3} maxZoom={1.4}
				>
					<Background gap={20} color="#e2e8f0" />
				</ReactFlow>
			</div>
		</div>
	);
}
