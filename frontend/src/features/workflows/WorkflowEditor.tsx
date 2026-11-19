import '@xyflow/react/dist/style.css';
import {
	Background, type Connection, ConnectionLineType, Controls, type Edge, type FinalConnectionState, MarkerType, MiniMap, ReactFlow,
	ReactFlowProvider, addEdge, useEdgesState, useNodesState, useReactFlow,
} from '@xyflow/react';
import { type DragEvent, useCallback, useEffect, useMemo, useState } from 'react';
import type { Campaign } from '../../types/campaign';
import type { InboxLabel, WorkflowDraft, WorkflowEdge, WorkflowNode, WorkflowNodeType, WorkflowPort } from '../../types/workflow';
import { startRequest } from '../kocs/bulkImport';
import { BLOCKS, BLOCK_BY_TYPE, GROUP_TITLES, isSendingType, isStartType } from './blocks';
import { type NodeData, type WfNode, freeSpotRightOf, toRfNode, toWorkflowNode, withoutSettingsOfOtherTypes } from './convert';
import { MAX_NODES, emptyDraft, loadDraft, nextNodeId, saveDraft, validateGraph } from './graph';
import NodeIcon from './NodeIcon';
import QuickAddMenu from './QuickAddMenu';
import TemplateBar from './TemplateBar';
import { type DraftMeta, canChange, fingerprint, loadDraftMeta, saveDraftMeta, saveState, templateToDraft } from './templates';
import { useTemplates } from './useTemplates';
import { SWITCH_HINT } from './runText';
import WorkflowNodeCard from './WorkflowNodeCard';
import { type SheetInfo, type WorkflowUi, WorkflowUiContext } from './WorkflowContext';

const DRAG_TYPE = 'application/x-workflow-node';
const nodeTypes = { wf: WorkflowNodeCard };

function toRfEdges(edges: WorkflowEdge[]): Edge[] {
	return edges.map((e) => ({
		id: e.id, source: e.from, sourceHandle: e.port, target: e.to,
		type: 'default', markerEnd: { type: MarkerType.ArrowClosed },
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

function pointerOf(event: MouseEvent | TouchEvent): { x: number; y: number } {
	if ('changedTouches' in event && event.changedTouches.length) return { x: event.changedTouches[0].clientX, y: event.changedTouches[0].clientY };
	const m = event as MouseEvent;
	return { x: m.clientX, y: m.clientY };
}

interface Props {
	campaigns: Campaign[];
	labels: InboxLabel[];
	folders: string[];
	isAdmin: boolean;
	/** the email of the person who draws: who may change a saved workflow */
	userEmail: string;
	starting: boolean;
	/** starts the workflow; resolves true when the server took it on */
	onStart: (payload: Record<string, unknown>) => Promise<boolean>;
	notify: (message: string, type?: 'success' | 'error') => void;
}

interface MenuAt {
	x: number;
	y: number;
	fromId: string;
	port: WorkflowPort;
}

function EditorInner({ campaigns, labels, folders, isAdmin, userEmail, starting, onStart, notify }: Props) {
	const [initial] = useState<WorkflowDraft>(() => {
		const d = loadDraft();
		return { nodes: d.nodes.map(withoutSettingsOfOtherTypes), edges: d.edges };
	});
	const [nodes, setNodes, onNodesChange] = useNodesState<WfNode>(initial.nodes.map(toRfNode));
	const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>(toRfEdges(initial.edges));
	const [sheet, setSheet] = useState<SheetInfo | null>(null);
	const [meta] = useState<DraftMeta>(() => loadDraftMeta());
	const [name, setName] = useState(meta.name);
	const [templateId, setTemplateId] = useState<string | null>(meta.templateId);
	const [savedPrint, setSavedPrint] = useState(meta.savedPrint);
	const [saving, setSaving] = useState(false);
	const [pendingAction, setPendingAction] = useState<null | { kind: 'open' | 'duplicate'; id: string } | { kind: 'new' }>(null);
	const [confirming, setConfirming] = useState(false);
	const [panelId, setPanelId] = useState<string | null>(null);
	const [menu, setMenu] = useState<MenuAt | null>(null);
	const { screenToFlowPosition, setCenter, getZoom, fitView } = useReactFlow();
	const templates = useTemplates(notify);

	const draftNodes: WorkflowNode[] = useMemo(() => nodes.map(toWorkflowNode), [nodes]);
	const draftEdges: WorkflowEdge[] = useMemo(() => edges.map((e) => ({
		id: e.id, from: e.source, port: (e.sourceHandle ?? 'out') as WorkflowPort, to: e.target,
	})), [edges]);
	const check = useMemo(() => {
		const base = validateGraph(draftNodes, draftEdges);
		// a drawing opened from a saved workflow may point to a campaign or a label that was deleted since
		const extra = draftNodes.flatMap((n) => {
			if (n.type === 'send' && n.campaignId && campaigns.length > 0 && !campaigns.some((c) => c._id === n.campaignId)) return [{ nodeId: n.id, message: 'Chiến dịch của khối này đã bị xóa. Hãy chọn lại.' }];
			if (n.type === 'label' && n.labelId && labels.length > 0 && !labels.some((l) => l._id === n.labelId)) return [{ nodeId: n.id, message: 'Nhãn của khối này đã bị xóa. Hãy chọn lại.' }];
			return [];
		});
		return { ok: base.ok && extra.length === 0, problems: [...base.problems, ...extra] };
	}, [draftNodes, draftEdges, campaigns, labels]);
	const print = useMemo(() => fingerprint(draftNodes, draftEdges), [draftNodes, draftEdges]);
	const state = saveState(templateId, savedPrint, print);
	const openedSummary = templates.items.find((t) => t._id === templateId);
	const user = useMemo(() => ({ email: userEmail, role: isAdmin ? 'admin' : 'member' }), [userEmail, isAdmin]);
	const canSaveOver = !!templateId && (!openedSummary || canChange(openedSummary, user));

	const ready = sheet?.rows.filter((r) => r.status === 'ready').length ?? 0;
	const startNode = draftNodes.find((n) => isStartType(n.type));
	const needsSheet = startNode?.type === 'import';
	const sendNodes = draftNodes.filter((n) => isSendingType(n.type));

	// the graph being drawn is kept in this browser (the sheet is not: it holds phones and addresses)
	useEffect(() => {
		const timer = setTimeout(() => saveDraft({ nodes: draftNodes, edges: draftEdges }), 300);
		return () => clearTimeout(timer);
	}, [draftNodes, draftEdges]);

	useEffect(() => { saveDraftMeta({ templateId, savedPrint, name }); }, [templateId, savedPrint, name]);

	// what each block shows: its problems, and which of its outputs already have a wire
	const viewNodes = useMemo(() => {
		const problems = new Map<string, string[]>();
		for (const p of check.problems) if (p.nodeId) problems.set(p.nodeId, [...(problems.get(p.nodeId) ?? []), p.message]);
		return nodes.map((n) => ({
			...n,
			data: { ...n.data, problems: problems.get(n.id) ?? [], usedPorts: edges.filter((e) => e.source === n.id).map((e) => e.sourceHandle ?? 'out') },
		}));
	}, [nodes, edges, check]);

	/** Opens the settings of a block and slides the canvas so that the block is on the left and its panel has room on the right. */
	const focusAndOpen = useCallback((id: string) => {
		setPanelId(id);
		const n = nodes.find((x) => x.id === id);
		if (n) setCenter(n.position.x + 48 + 190, n.position.y + 60, { zoom: Math.min(Math.max(getZoom(), 0.8), 1.2), duration: 280 });
	}, [nodes, setCenter, getZoom]);

	const closeAll = useCallback(() => { setPanelId(null); setMenu(null); }, []);
	useEffect(() => {
		const key = (e: KeyboardEvent) => e.key === 'Escape' && closeAll();
		document.addEventListener('keydown', key);
		return () => document.removeEventListener('keydown', key);
	}, [closeAll]);

	const canWire = useCallback((fromId: string, port: string, toId: string, current: Edge[], all: WfNode[]) => {
		if (!fromId || !toId || fromId === toId) return false;
		const target = all.find((n) => n.id === toId);
		if (!target || isStartType(target.data.kind)) return false;
		if (current.some((e) => e.source === fromId && (e.sourceHandle ?? 'out') === port)) return false;
		if (current.some((e) => e.target === toId)) return false;
		return !reaches(current, toId, fromId);
	}, []);

	/** Adds a block (at a place, or right of the block it follows) and, when it follows one, wires it to that block's output. */
	const addBlock = useCallback((type: WorkflowNodeType, at?: { x: number; y: number }, from?: { id: string; port: WorkflowPort }) => {
		if (isStartType(type) && nodes.some((n) => isStartType(n.data.kind))) return notify('Quy trình chỉ có một khối bắt đầu.', 'error');
		if (nodes.length >= MAX_NODES) return notify(`Quy trình có tối đa ${MAX_NODES} khối.`, 'error');
		const id = nextNodeId(type, nodes.map((n) => n.id));
		const source = from ? nodes.find((n) => n.id === from.id) : undefined;
		const position = at ?? (source ? freeSpotRightOf(source.position, nodes.map((n) => n.position)) : freeSpotRightOf({ x: -180, y: 140 }, nodes.map((n) => n.position)));
		const data: NodeData = { kind: type, ...(type === 'wait' ? { waitMinutes: 60 } : {}), ...(type === 'pick' ? { stages: [], limit: 50 } : {}), ...(type === 'wait_until' ? { atMinute: 9 * 60 } : {}) };
		setNodes((ns) => [...ns.map((n) => ({ ...n, selected: false })), { id, type: 'wf', position, data, selected: true }]);
		if (from && canWire(from.id, from.port, id, edges, [...nodes, { id, type: 'wf', position, data }])) {
			setEdges((es) => addEdge({ id: `e-${from.id}-${from.port}-${id}`, source: from.id, sourceHandle: from.port, target: id, type: 'default', markerEnd: { type: MarkerType.ArrowClosed } }, es));
		}
		setMenu(null);
		setPanelId(id);
		setCenter(position.x + 48 + 190, position.y + 60, { zoom: Math.min(Math.max(getZoom(), 0.8), 1.2), duration: 280 });
	}, [nodes, edges, canWire, notify, setNodes, setEdges, setCenter, getZoom]);

	function onDrop(e: DragEvent<HTMLDivElement>) {
		e.preventDefault();
		const type = e.dataTransfer.getData(DRAG_TYPE) as WorkflowNodeType;
		if (BLOCK_BY_TYPE[type]) addBlock(type, screenToFlowPosition({ x: e.clientX, y: e.clientY }));
	}

	const isValidConnection = useCallback((c: Connection | Edge) => canWire(c.source ?? '', c.sourceHandle ?? 'out', c.target ?? '', edges, nodes), [canWire, edges, nodes]);

	const onConnect = useCallback((c: Connection) => {
		setEdges((es) => addEdge({
			...c, id: `e-${c.source}-${c.sourceHandle ?? 'out'}-${c.target}`, type: 'default', markerEnd: { type: MarkerType.ArrowClosed },
		}, es));
	}, [setEdges]);

	// a wire dropped on empty canvas opens the menu of the blocks that can follow, like n8n
	const onConnectEnd = useCallback((event: MouseEvent | TouchEvent, state: FinalConnectionState) => {
		if (state.isValid || state.toNode || !state.fromNode || !state.fromHandle) return;
		const port = (state.fromHandle.id ?? 'out') as WorkflowPort;
		if (edges.some((e) => e.source === state.fromNode.id && (e.sourceHandle ?? 'out') === port)) return;
		const at = pointerOf(event);
		setMenu({ ...at, fromId: state.fromNode.id, port });
	}, [edges]);

	const ui: WorkflowUi = useMemo(() => ({
		mode: 'edit', campaigns, labels, folders, sheet, setSheet, panelId, startExists: nodes.some((n) => isStartType(n.data.kind)),
		update: (id, patch) => setNodes((ns) => ns.map((n) => (n.id === id ? { ...n, data: { ...n.data, ...patch } } : n))),
		remove: (id) => {
			setNodes((ns) => ns.filter((n) => n.id !== id));
			setEdges((es) => es.filter((e) => e.source !== id && e.target !== id));
			setPanelId((p) => (p === id ? null : p));
		},
		duplicate: (id) => {
			const src = nodes.find((n) => n.id === id);
			if (!src || isStartType(src.data.kind)) return;
			if (nodes.length >= MAX_NODES) return notify(`Quy trình có tối đa ${MAX_NODES} khối.`, 'error');
			const copy = nextNodeId(src.data.kind, nodes.map((n) => n.id));
			const data: NodeData = { ...toRfNode(toWorkflowNode(src)).data };
			setNodes((ns) => [...ns.map((n) => ({ ...n, selected: false })), { id: copy, type: 'wf', position: { x: src.position.x + 40, y: src.position.y + 150 }, data, selected: true }]);
			setPanelId(copy);
		},
		quickAdd: (id, port, at) => setMenu({ ...at, fromId: id, port }),
		openPanel: (id) => focusAndOpen(id),
		closePanel: () => setPanelId(null),
	}), [campaigns, labels, folders, sheet, panelId, nodes, notify, setNodes, setEdges]);

	const needsAdmin = sendNodes.length > 0 && !isAdmin;
	const blocker = !check.ok ? 'Quy trình chưa hợp lệ.'
		: needsSheet && ready === 0 ? 'Chưa có dòng Excel hợp lệ để nhập.'
		: needsAdmin ? 'Chỉ quản trị viên được chạy quy trình có bước gửi tin.' : '';

	async function go() {
		if (blocker) return;
		const ok = await onStart({
			name: name.trim() || 'Quy trình', confirm: true,
			nodes: draftNodes, edges: draftEdges, ...(needsSheet && sheet ? startRequest(sheet.rows) : {}),
		});
		if (ok) setConfirming(false);
	}

	// ---- saved workflows
	const meaningful = nodes.length > 1 || edges.length > 0;
	const unsaved = state === 'changed' || (state === 'draft' && meaningful);

	const loadIntoCanvas = useCallback((draft: WorkflowDraft, nextName: string, id: string | null) => {
		setNodes(draft.nodes.map(toRfNode));
		setEdges(toRfEdges(draft.edges));
		setName(nextName);
		setTemplateId(id);
		setSavedPrint(id ? fingerprint(draft.nodes, draft.edges) : '');
		setPanelId(null);
		setMenu(null);
		window.setTimeout(() => fitView({ padding: 0.3, maxZoom: 1, duration: 250 }), 60);
	}, [setNodes, setEdges, fitView]);

	const openSaved = async (id: string, asCopy: boolean) => {
		const t = await templates.open(id);
		if (!t) return;
		const draft = templateToDraft(t);
		if (draft.nodes.length < t.nodes.length) notify('Một số khối của quy trình này không còn được hỗ trợ nên đã bị bỏ.', 'error');
		loadIntoCanvas(draft, asCopy ? `${t.name} (bản sao)` : t.name, asCopy ? null : t._id);
	};
	const perform = (a: NonNullable<typeof pendingAction>) => {
		setPendingAction(null);
		if (a.kind === 'new') loadIntoCanvas(emptyDraft(), 'Quy trình mới', null);
		else openSaved(a.id, a.kind === 'duplicate');
	};
	const request = (a: NonNullable<typeof pendingAction>) => (unsaved ? setPendingAction(a) : perform(a));
	const payload = () => ({ name: name.trim() || 'Quy trình', nodes: draftNodes, edges: draftEdges });
	const saveOver = async () => {
		if (!templateId) return;
		setSaving(true);
		if (await templates.update(templateId, payload())) setSavedPrint(print);
		setSaving(false);
	};
	const saveAsNew = async () => {
		setSaving(true);
		const t = await templates.create(payload());
		if (t) { setTemplateId(t._id); setSavedPrint(print); }
		setSaving(false);
	};
	const removeSaved = async (id: string) => {
		if (await templates.remove(id) && id === templateId) { setTemplateId(null); setSavedPrint(''); }
	};

	const zaloBlocks = sendNodes.filter((n) => n.type !== 'email').length;
	const emailBlocks = sendNodes.filter((n) => n.type === 'email').length;
	return (
		<WorkflowUiContext.Provider value={ui}>
			<div className="wf-editor">
				<TemplateBar
					state={state} openedName={templateId ? openedSummary?.name ?? null : null} items={templates.items} loading={templates.loading} user={user}
					canSaveOver={canSaveOver} saving={saving}
					onOpen={(id) => request({ kind: 'open', id })} onDuplicate={(id) => request({ kind: 'duplicate', id })} onDelete={removeSaved}
					onSave={saveOver} onSaveAsNew={saveAsNew} onNew={() => request({ kind: 'new' })}
				/>
				<aside className="wf-palette" aria-label="Các khối">
					<h3>Các khối</h3>
					<p className="wf-hint">Kéo hoặc bấm đúp để thêm khối. Nối khối bằng chấm tròn hoặc dấu +.</p>
					{(['start', 'control', 'action'] as const).map((group) => (
						<div key={group}>
							<p className="wf-palette-group">{GROUP_TITLES[group]}</p>
							{BLOCKS.filter((b) => b.group === group).map((b) => {
								const used = isStartType(b.type) && nodes.some((n) => isStartType(n.data.kind));
								return (
									<div
										key={b.type} className={`wf-palette-item${used ? ' is-used' : ''}`} style={{ '--accent': b.color } as React.CSSProperties}
										draggable={!used}
										onDragStart={(e) => { e.dataTransfer.setData(DRAG_TYPE, b.type); e.dataTransfer.effectAllowed = 'move'; }}
										onDoubleClick={() => !used && addBlock(b.type)}
										title={used ? 'Quy trình đã có khối bắt đầu' : b.hint}
									>
										<span className="wf-palette-icon"><NodeIcon kind={b.type} size={20} /></span>
										<span><strong>{b.title}</strong></span>
									</div>
								);
							})}
						</div>
					))}
				</aside>

				<div className="wf-canvas" onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; }} onDrop={onDrop}>
					<ReactFlow
						nodes={viewNodes} edges={edges} nodeTypes={nodeTypes}
						onNodesChange={onNodesChange} onEdgesChange={onEdgesChange} onConnect={onConnect} onConnectEnd={onConnectEnd}
						isValidConnection={isValidConnection} connectionRadius={44} connectionLineType={ConnectionLineType.Bezier}
						onNodeClick={(_, n) => focusAndOpen(n.id)} onPaneClick={closeAll} onNodeDragStart={() => setMenu(null)}
						snapToGrid snapGrid={[20, 20]} deleteKeyCode={['Backspace', 'Delete']}
						fitView fitViewOptions={{ padding: 0.3, maxZoom: 1 }} proOptions={{ hideAttribution: true }} minZoom={0.3} maxZoom={1.6}
					>
						<Background gap={20} color="#e2e8f0" />
						<Controls showInteractive={false} />
						{!panelId && <MiniMap pannable zoomable nodeStrokeWidth={2} />}
					</ReactFlow>
					{menu && <QuickAddMenu x={menu.x} y={menu.y} onClose={() => setMenu(null)} onPick={(type) => addBlock(type, undefined, { id: menu.fromId, port: menu.port })} />}
				</div>

				<footer className="wf-bar">
					<input className="wf-name" value={name} maxLength={100} onChange={(e) => setName(e.target.value)} aria-label="Tên quy trình" />
					<div className="wf-bar-info">
						{check.ok ? <span>{nodes.length} khối{needsSheet && ready > 0 && <>, <b>{ready}</b> KOC sẽ nhập</>}</span>
							: <span className="wf-bar-problem">{check.problems[0]?.message}{check.problems.length > 1 && ` (và ${check.problems.length - 1} lỗi khác)`}</span>}
						{check.ok && blocker && <span className="wf-bar-problem">{blocker}</span>}
					</div>
					<button type="button" className="tm-btn-submit" disabled={!!blocker || starting} onClick={() => setConfirming(true)}>Bắt đầu</button>
				</footer>

				{pendingAction && (
					<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && setPendingAction(null)}>
						<div className="tiktoday-modal wf-confirm" role="dialog" aria-modal="true" aria-labelledby="wf-pending-title">
							<div className="tm-header"><div><h2 id="wf-pending-title">Bỏ bản đang vẽ?</h2></div></div>
							<div className="tm-body"><p>Bản đang vẽ có thay đổi chưa lưu. Nếu tiếp tục, những thay đổi đó sẽ mất.</p></div>
							<div className="tm-footer">
								<button type="button" className="tm-btn-cancel" onClick={() => setPendingAction(null)}>Quay lại</button>
								<button type="button" className="tm-btn-submit" onClick={() => perform(pendingAction)}>Tiếp tục</button>
							</div>
						</div>
					</div>
				)}

				{confirming && (
					<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && !starting && setConfirming(false)}>
						<div className="tiktoday-modal wf-confirm" role="dialog" aria-modal="true" aria-labelledby="wf-confirm-title">
							<div className="tm-header"><div><h2 id="wf-confirm-title">Bắt đầu quy trình?</h2></div></div>
							<div className="tm-body">
								{needsSheet
									? <p>Máy chủ sẽ nhập <b>{ready}</b> KOC từ file <b>{sheet?.fileName}</b>.</p>
									: <p>Máy chủ sẽ lấy KOC có sẵn trong danh bạ theo khối <b>Lấy KOC có sẵn</b>. Danh sách được lấy ngay bây giờ và không đổi nữa.</p>}
								{sendNodes.length > 0 && (
									<p className="wf-warn">
										Sau đó quy trình sẽ <b>tự nhắn tin cho KOC</b>
										{zaloBlocks > 0 && <> qua Zalo ở {zaloBlocks} bước</>}{zaloBlocks > 0 && emailBlocks > 0 && ' và'}{emailBlocks > 0 && <> bằng email ở {emailBlocks} bước</>},
										chỉ trong khung giờ 08:30 đến 20:30. Quy trình có thể kéo dài nhiều ngày (tối đa 14 ngày). Bạn dừng được bất kỳ lúc nào ở tab này.
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
		</WorkflowUiContext.Provider>
	);
}

/** The canvas where a workflow is drawn: blocks to drag out, wires to connect them, settings in a panel next to the block. */
export default function WorkflowEditor(props: Props) {
	const wide = useWide();
	if (!wide) {
		return <div className="panel-card wf-narrow"><h3 className="section-title">Hãy dùng máy tính để vẽ quy trình</h3><p className="muted-note">Khung vẽ cần màn hình rộng và chuột. Trên điện thoại bạn vẫn xem được tiến độ của quy trình đang chạy.</p></div>;
	}
	return <ReactFlowProvider><EditorInner {...props} /></ReactFlowProvider>;
}
