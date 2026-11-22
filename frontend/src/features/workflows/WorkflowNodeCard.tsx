import { Handle, type NodeProps, Position, useUpdateNodeInternals } from '@xyflow/react';
import { type CSSProperties, useEffect } from 'react';
import type { WorkflowNode, WorkflowNodeType, WorkflowPort } from '../../types/workflow';
import { BLOCKS, BLOCK_BY_TYPE, nodeSummary } from './blocks';
import type { WfNode } from './convert';
import { stateLabel, waitLeftText } from './graph';
import NodeIcon from './NodeIcon';
import { useWorkflowUi } from './WorkflowContext';

export const NODE_TITLES = Object.fromEntries(BLOCKS.map((x) => [x.type, x.title])) as Record<WorkflowNodeType, string>;
export const NODE_HINTS = Object.fromEntries(BLOCKS.map((x) => [x.type, x.hint])) as Record<WorkflowNodeType, string>;

/** What the block says under its name: its settings in a few words, or while it runs what it has done. */
function summaryOf(id: string, data: WfNode['data'], ui: ReturnType<typeof useWorkflowUi>): string {
	const node = { id, type: data.kind, x: 0, y: 0, ...data } as unknown as WorkflowNode;
	if (ui.mode === 'run') {
		if (data.kind === 'import') return 'Nhập KOC từ file Excel';
		if (data.kind === 'send') return data.campaignName ?? 'Chiến dịch đã chọn';
	}
	const rows = ui.sheet?.rows.filter((r) => r.status === 'ready').length ?? 0;
	return nodeSummary(node, { campaigns: ui.campaigns, labels: ui.labels, zaloAccounts: ui.zaloAccounts, rows });
}

/** One block of the canvas: a square with its name under it, an input on the left and the output(s) on the right. */
export default function WorkflowNodeCard({ id, data, selected }: NodeProps<WfNode>) {
	const ui = useWorkflowUi();
	const meta = BLOCK_BY_TYPE[data.kind];
	const editing = ui.mode === 'edit';
	const state = data.state;
	const problems = data.problems ?? [];
	const used = data.usedPorts ?? [];
	// a connector that stops being a "+" moves: tell the canvas to measure it again so a wire starts at the ring
	const updateInternals = useUpdateNodeInternals();
	const usedKey = used.join(',');
	useEffect(() => { updateInternals(id); }, [id, usedKey, updateInternals]);
	const isStart = data.kind === 'import' || data.kind === 'pick';
	const ports: { port: WorkflowPort; top: string; label?: string }[] =
		data.kind === 'if_replied'
			? [{ port: 'replied', top: '28%', label: 'đã trả lời' }, { port: 'unanswered', top: '72%', label: 'chưa trả lời' }]
			: [{ port: 'out', top: '50%' }];
	const classes = ['wfn', selected ? 'is-selected' : '', problems.length && !state ? 'has-problem' : '', state ? `st-${state.status}` : ''].filter(Boolean).join(' ');
	const left = state?.status === 'waiting' ? waitLeftText(state.waitUntil, data.nowMs ?? Date.now()) : '';

	return (
		<div className={classes} style={{ '--accent': meta.color } as CSSProperties} title={problems.length ? problems.join('\n') : undefined}>
			<div className={`wfn-box${data.kind === 'if_replied' ? ' split' : ''}`}>
				{!isStart && <Handle type="target" position={Position.Left} className="wfn-handle" isConnectable={editing} />}
				<span className="wfn-icon"><NodeIcon kind={data.kind} /></span>
				{state && <span className={`wfn-dot wfn-dot-${state.status}`} aria-label={stateLabel(state.status)} />}
				{state && (state.count > 0 || state.status === 'done') && <span className="wfn-count" key={state.count}>{state.count}</span>}
				{!state && problems.length > 0 && <span className="wfn-warn">!</span>}
				{selected && !isStart && <span className="wfn-io wfn-io-in">vào</span>}
				{ports.map(({ port, top, label }) => (
					<span key={port}>
						<Handle
							type="source" id={port} position={Position.Right} style={{ top }} isConnectable={editing}
							className={`wfn-handle${editing && !used.includes(port) ? ' is-plus' : ''}`}
							title={editing && !used.includes(port) ? 'Kéo để nối, hoặc bấm để thêm khối tiếp theo' : undefined}
							onClick={editing && !used.includes(port) ? (e) => { e.stopPropagation(); ui.quickAdd(id, port, { x: e.clientX, y: e.clientY }); } : undefined}
						>
							{editing && !used.includes(port) && <span className="wfn-plus-mark" aria-label="Thêm khối tiếp theo">+</span>}
						</Handle>
						{label && <span className="wfn-port-label" style={{ top }}>{label}</span>}
						{selected && port === 'out' && <span className="wfn-io wfn-io-out">ra</span>}
					</span>
				))}
			</div>
			<div className="wfn-label">
				<strong>{meta.title}</strong>
				<small>{summaryOf(id, data, ui)}</small>
				{state && left && <small className="wfn-left">{left}</small>}
				{state?.reason && <small className="wfn-reason">{state.reason}</small>}
				{state && <small className={`wfn-status wfn-status-${state.status}`}>{stateLabel(state.status)}</small>}
			</div>
		</div>
	);
}
