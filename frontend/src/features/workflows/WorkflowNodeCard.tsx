import { Handle, type Node, type NodeProps, Position } from '@xyflow/react';
import type { ReactNode } from 'react';
import type { NodeState, WorkflowNodeType } from '../../types/workflow';
import { BLOCKS } from './blocks';
import { stateLabel, waitLabel, waitLeftText } from './graph';

/** What a block on the canvas carries. A type alias, not an interface: the canvas library needs an index signature. */
export type NodeData = {
	kind: WorkflowNodeType;
	waitMinutes?: number;
	campaignId?: string;
	campaignName?: string;
	/** import: how many rows of the chosen file will be imported */
	rowsCount?: number;
	/** what is wrong with this block (the editor), shown as a warning border and a tooltip */
	problems?: string[];
	/** while a workflow runs: what the block is doing; the screen is then read-only */
	state?: NodeState;
	nowMs?: number;
};

export type WfNode = Node<NodeData, 'wf'>;

export const NODE_TITLES = Object.fromEntries(BLOCKS.map((x) => [x.type, x.title])) as Record<WorkflowNodeType, string>;

export const NODE_HINTS = Object.fromEntries(BLOCKS.map((x) => [x.type, x.hint])) as Record<WorkflowNodeType, string>;

const SVG = { width: 18, height: 18, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.8, strokeLinecap: 'round', strokeLinejoin: 'round' } as const;

export function NodeIcon({ kind }: { kind: WorkflowNodeType }) {
	switch (kind) {
		case 'import':
			return <svg {...SVG}><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" /><path d="M12 18v-6" /><polyline points="9 15 12 12 15 15" /></svg>;
		case 'wait':
			return <svg {...SVG}><circle cx="12" cy="12" r="9" /><polyline points="12 7 12 12 15.5 14" /></svg>;
		case 'send':
			return <svg {...SVG}><line x1="22" y1="2" x2="11" y2="13" /><polygon points="22 2 15 22 11 13 2 9 22 2" /></svg>;
		default:
			return <svg {...SVG}><path d="M6 3v6a3 3 0 0 0 3 3h6a3 3 0 0 1 3 3v6" /><polyline points="15 18 18 21 21 18" /><circle cx="6" cy="3" r="1.2" /></svg>;
	}
}

function summary(d: NodeData): ReactNode {
	if (d.state) { // a workflow that runs: what was drawn is fixed, so nothing here is "not chosen yet"
		if (d.kind === 'import') return 'Nhập KOC từ file Excel';
		if (d.kind === 'send') return d.campaignName ?? 'Chiến dịch đã chọn';
	}
	switch (d.kind) {
		case 'import':
			return d.rowsCount ? `${d.rowsCount} dòng sẽ nhập` : <em>Chưa chọn file Excel</em>;
		case 'wait':
			return d.waitMinutes ? `Chờ ${waitLabel(d.waitMinutes)}` : <em>Chưa đặt thời gian</em>;
		case 'send':
			return d.campaignName ? d.campaignName : <em>Chưa chọn chiến dịch</em>;
		default:
			return 'Chia theo việc đã trả lời';
	}
}

/** One block of the canvas: the editor draws it, and so does the run view (with a state on it). */
export default function WorkflowNodeCard({ data, selected }: NodeProps<WfNode>) {
	const running = data.state;
	const problems = data.problems ?? [];
	const classes = ['wf-node', `wf-${data.kind}`, selected ? 'is-selected' : '', problems.length ? 'has-problem' : '', running ? `st-${running.status}` : ''].filter(Boolean).join(' ');
	const left = running?.status === 'waiting' ? waitLeftText(running.waitUntil, data.nowMs ?? Date.now()) : '';
	return (
		<div className={classes} title={problems.join('\n') || undefined}>
			{data.kind !== 'import' && <Handle type="target" position={Position.Left} className="wf-handle" isConnectable={!running} />}
			<div className="wf-node-head">
				<span className="wf-node-icon"><NodeIcon kind={data.kind} /></span>
				<strong>{NODE_TITLES[data.kind]}</strong>
			</div>
			<div className="wf-node-body">{summary(data)}</div>
			{running && (
				<div className="wf-node-state">
					<span className={`wf-badge wf-badge-${running.status}`}>{stateLabel(running.status)}</span>
					{(running.count > 0 || running.status === 'done') && <span className="wf-count">{running.count} KOC</span>}
					{left && <span className="wf-left">{left}</span>}
					{running.reason && <span className="wf-reason">{running.reason}</span>}
				</div>
			)}
			{!running && problems.length > 0 && <div className="wf-node-problem">{problems[0]}</div>}
			{data.kind === 'if_replied' ? (
				<>
					<Handle type="source" id="replied" position={Position.Right} className="wf-handle" style={{ top: 'auto', bottom: 38 }} isConnectable={!running} />
					<Handle type="source" id="unanswered" position={Position.Right} className="wf-handle" style={{ top: 'auto', bottom: 12 }} isConnectable={!running} />
					<span className="wf-port-label" style={{ bottom: 37 }}>đã trả lời</span>
					<span className="wf-port-label" style={{ bottom: 11 }}>chưa trả lời</span>
				</>
			) : (
				<Handle type="source" id="out" position={Position.Right} className="wf-handle" isConnectable={!running} />
			)}
		</div>
	);
}
