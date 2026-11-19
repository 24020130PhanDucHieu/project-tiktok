import '@xyflow/react/dist/style.css';
import {
	Background, type Connection, Controls, type Edge, MarkerType, MiniMap, ReactFlow, ReactFlowProvider, addEdge, useEdgesState, useNodesState, useReactFlow,
} from '@xyflow/react';
import { type DragEvent, useCallback, useEffect, useMemo, useState } from 'react';
import type { Campaign } from '../../types/campaign';
import type { WorkflowDraft, WorkflowEdge, WorkflowNode, WorkflowNodeType, WorkflowPort } from '../../types/workflow';
import { startRequest } from '../kocs/bulkImport';
import { SWITCH_HINT } from './runText';
import { MAX_NODES, loadDraft, nextNodeId, saveDraft, validateGraph } from './graph';
import NodeSettings, { type SheetInfo } from './NodeSettings';
import WorkflowNodeCard, { NODE_HINTS, NODE_TITLES, NodeIcon, type NodeData, type WfNode } from './WorkflowNodeCard';

const DRAG_TYPE = 'application/x-workflow-node';
const nodeTypes = { wf: WorkflowNodeCard };
const BLOCKS: WorkflowNodeType[] = ['import', 'wait', 'send', 'if_replied'];

function toRfNodes(nodes: WorkflowNode[]): WfNode[] {
	return nodes.map((n) => ({
		id: n.id, type: 'wf', position: { x: n.x, y: n.y },
		data: { kind: n.type, waitMinutes: n.waitMinutes, campaignId: n.campaignId },
	}));
}

function toRfEdges(edges: WorkflowEdge[]): Edge[] {
	return edges.map((e) => ({
		id: e.id, source: e.from, sourceHandle: e.port, target: e.to,
		type: 'smoothstep', markerEnd: { type: MarkerType.ArrowClosed },
	}));
}

/** True when following the wires from `from` leads to `to`. */
function reaches(edges: Edge[], from: string, to: string): boolean {
	const seen = new Set<string>();
	const stack = [from];
	while (stack.length) {
		const id = stack.pop()!;
		if (id === to) return true;
		if (seen.has(id)) continue;
		seen.add(id);
		for (const e of edges) if (e.source === id) stack.push(e.target);
	}
	return false;
}

/** True when the screen is wide enough to draw on: the canvas needs a mouse and room. */
function useWide(): boolean {
	const query = '(min-width: 900px)';
	const [wide, setWide] = useState(() => (typeof window === 'undefined' ? true : window.matchMedia(query).matches));
	useEffect(() => {
		const mq = window.matchMedia(query);
		const on = () => setWide(mq.matches);
		mq.addEventListener('change', on);
		return () => mq.removeEventListener('change', on);
	}, []);
	return wide;
}

interface Props {
	campaigns: Campaign[];
	folders: string[];
	isAdmin: boolean;
	starting: boolean;
	/** starts the workflow; resolves true when the server took it on */
	onStart: (payload: Record<string, unknown>) => Promise<boolean>;
	notify: (message: string, type?: 'success' | 'error') => void;
}

function EditorInner({ campaigns, folders, isAdmin, starting, onStart, notify }: Props) {
	const [initial] = useState<WorkflowDraft>(() => loadDraft());
	const [nodes, setNodes, onNodesChange] = useNodesState<WfNode>(toRfNodes(initial.nodes));
	const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>(toRfEdges(initial.edges));
	const [sheet, setSheet] = useState<SheetInfo | null>(null);
	const [name, setName] = useState('Quy trình mới');
	const [confirming, setConfirming] = useState(false);
	const { screenToFlowPosition } = useReactFlow();

	const draftNodes: WorkflowNode[] = useMemo(() => nodes.map((n) => ({
		id: n.id, type: n.data.kind, x: Math.round(n.position.x), y: Math.round(n.position.y),
		...(n.data.kind === 'wait' ? { waitMinutes: n.data.waitMinutes ?? 0 } : {}),
		...(n.data.kind === 'send' ? { campaignId: n.data.campaignId ?? '' } : {}),
	})), [nodes]);
	const draftEdges: WorkflowEdge[] = useMemo(() => edges.map((e) => ({
		id: e.id, from: e.source, port: (e.sourceHandle ?? 'out') as WorkflowPort, to: e.target,
	})), [edges]);
	const check = useMemo(() => validateGraph(draftNodes, draftEdges), [draftNodes, draftEdges]);

	const ready = sheet?.rows.filter((r) => r.status === 'ready').length ?? 0;
	const sendCount = draftNodes.filter((n) => n.type === 'send').length;

	// the graph being drawn is kept in this browser (the sheet is not: it holds phones and addresses)
	useEffect(() => {
		const timer = setTimeout(() => saveDraft({ nodes: draftNodes, edges: draftEdges }), 300);
		return () => clearTimeout(timer);
	}, [draftNodes, draftEdges]);

	// what each block shows: its problems, the file it will import, the name of its campaign
	const viewNodes = useMemo(() => {
		const problems = new Map<string, string[]>();
		for (const p of check.problems) if (p.nodeId) problems.set(p.nodeId, [...(problems.get(p.nodeId) ?? []), p.message]);
		return nodes.map((n) => ({
			...n,
			data: {
				...n.data,
				problems: problems.get(n.id) ?? [],
				rowsCount: n.data.kind === 'import' ? ready : undefined,
				campaignName: n.data.kind === 'send' ? campaigns.find((c) => c._id === n.data.campaignId)?.name : undefined,
			},
		}));
	}, [nodes, check, ready, campaigns]);

	const selected = nodes.find((n) => n.selected);

	const addBlock = useCallback((type: WorkflowNodeType, at?: { x: number; y: number }) => {
		if (type === 'import' && nodes.some((n) => n.data.kind === 'import')) return notify('Quy trình chỉ có một khối Nhập Excel.', 'error');
		if (nodes.length >= MAX_NODES) return notify(`Quy trình có tối đa ${MAX_NODES} khối.`, 'error');
		const right = nodes.reduce((m, n) => Math.max(m, n.position.x), 0);
		const id = nextNodeId(type, nodes.map((n) => n.id));
		const data: NodeData = { kind: type, ...(type === 'wait' ? { waitMinutes: 60 } : {}), ...(type === 'send' ? { campaignId: '' } : {}) };
		setNodes((ns) => [...ns.map((n) => ({ ...n, selected: false })), { id, type: 'wf', position: at ?? { x: right + 260, y: 140 }, data, selected: true }]);
	}, [nodes, notify, setNodes]);

	function onDrop(e: DragEvent<HTMLDivElement>) {
		e.preventDefault();
		const type = e.dataTransfer.getData(DRAG_TYPE) as WorkflowNodeType;
		if (BLOCKS.includes(type)) addBlock(type, screenToFlowPosition({ x: e.clientX, y: e.clientY }));
	}

	// a wire is only allowed where the server would accept it: one wire per output, one wire into a block (none into the
	// import block), and never a loop
	const isValidConnection = useCallback((c: Connection | Edge) => {
		if (!c.source || !c.target || c.source === c.target) return false;
		const target = nodes.find((n) => n.id === c.target);
		if (!target || target.data.kind === 'import') return false;
		const port = c.sourceHandle ?? 'out';
		if (edges.some((e) => e.source === c.source && (e.sourceHandle ?? 'out') === port)) return false;
		if (edges.some((e) => e.target === c.target)) return false;
		return !reaches(edges, c.target, c.source);
	}, [nodes, edges]);

	const onConnect = useCallback((c: Connection) => {
		setEdges((es) => addEdge({
			...c, id: `e-${c.source}-${c.sourceHandle ?? 'out'}-${c.target}`, type: 'smoothstep', markerEnd: { type: MarkerType.ArrowClosed },
		}, es));
	}, [setEdges]);

	function updateSelected(patch: Partial<NodeData>) {
		if (!selected) return;
		setNodes((ns) => ns.map((n) => (n.id === selected.id ? { ...n, data: { ...n.data, ...patch } } : n)));
	}

	function deleteSelected() {
		if (!selected) return;
		const id = selected.id;
		setNodes((ns) => ns.filter((n) => n.id !== id));
		setEdges((es) => es.filter((e) => e.source !== id && e.target !== id));
	}

	const needsAdmin = sendCount > 0 && !isAdmin;
	const blocker = !check.ok ? 'Quy trình chưa hợp lệ.'
		: ready === 0 ? 'Chưa có dòng Excel hợp lệ để nhập.'
		: needsAdmin ? 'Chỉ quản trị viên được chạy quy trình có bước gửi tin.' : '';

	async function go() {
		if (!sheet || blocker) return;
		const ok = await onStart({
			name: name.trim() || 'Quy trình', confirm: true,
			nodes: draftNodes, edges: draftEdges, ...startRequest(sheet.rows),
		});
		if (ok) setConfirming(false);
	}

	return (
		<div className="wf-editor">
			<aside className="wf-palette" aria-label="Các khối">
				<h3>Các khối</h3>
				<p className="wf-hint">Kéo khối ra khung vẽ, rồi kéo từ chấm bên phải của khối này sang chấm bên trái của khối kia để nối.</p>
				{BLOCKS.map((type) => {
					const used = type === 'import' && nodes.some((n) => n.data.kind === 'import');
					return (
						<div
							key={type} className={`wf-palette-item wf-${type}${used ? ' is-used' : ''}`}
							draggable={!used}
							onDragStart={(e) => { e.dataTransfer.setData(DRAG_TYPE, type); e.dataTransfer.effectAllowed = 'move'; }}
							onDoubleClick={() => !used && addBlock(type)}
							title={used ? 'Đã có khối Nhập Excel' : 'Kéo ra khung vẽ (hoặc bấm đúp để thêm)'}
						>
							<span className="wf-node-icon"><NodeIcon kind={type} /></span>
							<span><strong>{NODE_TITLES[type]}</strong><small>{NODE_HINTS[type]}</small></span>
						</div>
					);
				})}
			</aside>

			<div className="wf-canvas" onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; }} onDrop={onDrop}>
				<ReactFlow
					nodes={viewNodes} edges={edges} nodeTypes={nodeTypes}
					onNodesChange={onNodesChange} onEdgesChange={onEdgesChange} onConnect={onConnect} isValidConnection={isValidConnection}
					deleteKeyCode={['Backspace', 'Delete']} fitView fitViewOptions={{ padding: 0.3, maxZoom: 1 }}
					proOptions={{ hideAttribution: true }} minZoom={0.3} maxZoom={1.6}
				>
					<Background gap={20} color="#e2e8f0" />
					<Controls showInteractive={false} />
					<MiniMap pannable zoomable nodeStrokeWidth={2} />
				</ReactFlow>
			</div>

			<aside className="wf-side" aria-label="Cài đặt">
				{selected ? (
					<NodeSettings
						key={selected.id} nodeId={selected.id} data={selected.data} campaigns={campaigns} folders={folders}
						sheet={sheet} onSheet={setSheet} onChange={updateSelected} onDelete={deleteSelected}
					/>
				) : (
					<div className="wf-settings">
						<h3>Cài đặt</h3>
						<p className="wf-hint">Bấm vào một khối để cài đặt: chọn file Excel, thời gian chờ hoặc chiến dịch.</p>
						<p className="wf-hint">Bấm vào dây rồi nhấn Delete để xóa dây. Bản vẽ được giữ trong trình duyệt này khi bạn tải lại trang.</p>
					</div>
				)}
			</aside>

			<footer className="wf-bar">
				<input className="wf-name" value={name} maxLength={100} onChange={(e) => setName(e.target.value)} aria-label="Tên quy trình" />
				<div className="wf-bar-info">
					{check.ok ? <span>{nodes.length} khối{ready > 0 && <>, <b>{ready}</b> KOC sẽ nhập</>}</span>
						: <span className="wf-bar-problem">{check.problems[0]?.message}{check.problems.length > 1 && ` (và ${check.problems.length - 1} lỗi khác)`}</span>}
					{check.ok && blocker && <span className="wf-bar-problem">{blocker}</span>}
				</div>
				<button type="button" className="tm-btn-submit" disabled={!!blocker || starting} onClick={() => setConfirming(true)}>Bắt đầu</button>
			</footer>

			{confirming && (
				<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && !starting && setConfirming(false)}>
					<div className="tiktoday-modal wf-confirm" role="dialog" aria-modal="true" aria-labelledby="wf-confirm-title">
						<div className="tm-header"><div><h2 id="wf-confirm-title">Bắt đầu quy trình?</h2></div></div>
						<div className="tm-body">
							<p>Máy chủ sẽ nhập <b>{ready}</b> KOC từ file <b>{sheet?.fileName}</b>.</p>
							{sendCount > 0 && (
								<p className="wf-warn">
									Sau đó quy trình sẽ <b>tự gửi tin Zalo cho KOC</b> ở {sendCount} bước gửi, chỉ trong khung giờ 08:30 đến 20:30 và theo hạn mức mỗi ngày.
									Quy trình có thể kéo dài nhiều ngày (tối đa 14 ngày). Bạn dừng được bất kỳ lúc nào ở tab này.
									<br />{SWITCH_HINT}
								</p>
							)}
						</div>
						<div className="tm-footer">
							<button type="button" className="tm-btn-cancel" disabled={starting} onClick={() => setConfirming(false)}>Hủy</button>
							<button type="button" className="tm-btn-submit" disabled={starting} onClick={go}>{starting ? 'Đang bắt đầu…' : 'Bắt đầu chạy'}</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
}

/** The canvas where a workflow is drawn: blocks to drag out, wires to connect them, settings for the selected block. */
export default function WorkflowEditor(props: Props) {
	const wide = useWide();
	if (!wide) {
		return <div className="panel-card wf-narrow"><h3 className="section-title">Hãy dùng máy tính để vẽ quy trình</h3><p className="muted-note">Khung vẽ cần màn hình rộng và chuột. Trên điện thoại bạn vẫn xem được tiến độ của quy trình đang chạy.</p></div>;
	}
	return <ReactFlowProvider><EditorInner {...props} /></ReactFlowProvider>;
}
