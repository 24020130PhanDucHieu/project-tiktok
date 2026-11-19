import '@xyflow/react/dist/style.css';
import { Background, type Edge, MarkerType, ReactFlow } from '@xyflow/react';
import { useEffect, useMemo, useState } from 'react';
import type { InboxLabel, Workflow } from '../../types/workflow';
import { BLOCK_BY_TYPE, nodeDetailLines } from './blocks';
import { stateLabel, waitLeftText } from './graph';
import NodeIcon from './NodeIcon';
import { SWITCH_HINT, finishText } from './runText';
import { type WfNode, toRfNode } from './convert';
import WorkflowNodeCard from './WorkflowNodeCard';
import { READ_ONLY_UI, WorkflowUiContext } from './WorkflowContext';

const nodeTypes = { wf: WorkflowNodeCard };
const STATUS_TEXT = { running: 'Đang chạy', done: 'Đã xong', stopped: 'Đã dừng' } as const;

interface Props {
	workflow: Workflow;
	/** the labels of the inbox, to name the label a block puts on */
	labels: InboxLabel[];
	isAdmin: boolean;
	onStop: () => void;
	/** leaves the report and goes back to drawing a new workflow */
	onNew: () => void;
}

/** A workflow that has started or ended: the same canvas, read-only, with the state of every block on it. */
export default function WorkflowRunView({ workflow, labels, isAdmin, onStop, onNew }: Props) {
	const [selectedId, setSelectedId] = useState<string | null>(null);
	const running = workflow.status === 'running';
	const [now, setNow] = useState(() => Date.now());
	const anyWaiting = running && (workflow.states ?? []).some((s) => s.status === 'waiting');
	useEffect(() => {
		if (!anyWaiting) return;
		const timer = setInterval(() => setNow(Date.now()), 1000); // the countdown of a waiting block
		return () => clearInterval(timer);
	}, [anyWaiting]);

	const states = useMemo(() => new Map((workflow.states ?? []).map((s) => [s.nodeId, s])), [workflow.states]);
	const nodes: WfNode[] = useMemo(() => workflow.nodes.map((n) => {
		const base = toRfNode(n);
		return {
			...base, draggable: false, selectable: false, connectable: false,
			data: { ...base.data, state: states.get(n.id) ?? { nodeId: n.id, status: 'pending' as const, count: 0 }, nowMs: now },
		};
	}), [workflow.nodes, states, now]);
	const edges: Edge[] = useMemo(() => workflow.edges.map((e) => {
		const target = states.get(e.to)?.status;
		return {
			id: e.id, source: e.from, sourceHandle: e.port, target: e.to, type: 'default', selectable: false,
			animated: running && (target === 'running' || target === 'waiting'), markerEnd: { type: MarkerType.ArrowClosed },
		};
	}), [workflow.edges, states, running]);

	const report = running ? null : finishText(workflow);
	const picked = workflow.nodes.find((n) => n.id === selectedId);
	const pickedState = picked ? states.get(picked.id) : undefined;
	const meta = picked ? BLOCK_BY_TYPE[picked.type] : undefined;
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
			{running && workflow.nodes.some((n) => n.type === 'send' || n.type === 'send_text') && <p className="wf-run-note">{SWITCH_HINT}</p>}
			<div className="wf-canvas wf-run-canvas">
				<WorkflowUiContext.Provider value={READ_ONLY_UI}>
				<ReactFlow
					nodes={nodes} edges={edges} nodeTypes={nodeTypes} nodesDraggable={false} nodesConnectable={false} elementsSelectable={false}
					onNodeClick={(_, n) => setSelectedId(n.id)} onPaneClick={() => setSelectedId(null)}
					fitView fitViewOptions={{ padding: 0.25, maxZoom: 1 }} proOptions={{ hideAttribution: true }} minZoom={0.3} maxZoom={1.4}
				>
					<Background gap={20} color="#e2e8f0" />
				</ReactFlow>
				</WorkflowUiContext.Provider>
				{picked && meta && (
					<aside className="wf-run-detail" aria-label={`Chi tiết khối ${meta.title}`}>
						<header style={{ '--accent': meta.color } as React.CSSProperties}>
							<span className="wfp-icon"><NodeIcon kind={picked.type} size={20} /></span>
							<strong>{meta.title}</strong>
							<button type="button" className="wfp-close" onClick={() => setSelectedId(null)} aria-label="Đóng">×</button>
						</header>
						<dl>
							<dt>Trạng thái</dt><dd>{pickedState ? stateLabel(pickedState.status) : 'Chưa tới lượt'}</dd>
							{pickedState && <><dt>Số KOC</dt><dd>{pickedState.count}</dd></>}
							{pickedState?.status === 'waiting' && pickedState.waitUntil && <><dt>Còn</dt><dd>{waitLeftText(pickedState.waitUntil, now).replace(/^còn /, '')}</dd></>}
							{pickedState?.reason && <><dt>Ghi chú</dt><dd>{pickedState.reason}</dd></>}
							{nodeDetailLines(picked, { labels }).map(([k, v]) => <><dt key={`k-${k}`}>{k}</dt><dd key={`v-${k}`} className="wf-run-text">{v}</dd></>)}
						</dl>
						<div className="wfp-io">
							<div><span>VÀO</span><p>{meta.input}</p></div>
							<div><span>RA</span><p>{meta.output}</p></div>
						</div>
					</aside>
				)}
			</div>
		</div>
	);
}
