import type { Node } from '@xyflow/react';
import type { NodeState, WorkflowNode, WorkflowNodeType } from '../../types/workflow';

/**
 * What a block on the canvas carries: its settings (the same fields a WorkflowNode has) and, only for the screen, what is
 * wrong with it, how it is doing while a workflow runs, and which of its outputs already have a wire. A type alias, not an
 * interface: the canvas library needs an index signature.
 */
export type NodeData = {
	kind: WorkflowNodeType;
	waitMinutes?: number;
	atMinute?: number;
	campaignId?: string;
	campaignName?: string;
	text?: string;
	subject?: string;
	labelId?: string;
	stage?: string;
	stages?: string[];
	folder?: string;
	limit?: number;
	source?: string;
	kocIds?: string[];
	kocNames?: Record<string, string>;
	// ---- only for the screen
	problems?: string[];
	state?: NodeState;
	nowMs?: number;
	usedPorts?: string[];
};

export type WfNode = Node<NodeData, 'wf'>;

const SETTINGS = ['waitMinutes', 'atMinute', 'campaignId', 'text', 'subject', 'labelId', 'stage', 'stages', 'folder', 'limit', 'source', 'kocIds', 'kocNames'] as const;

/** The settings a block type uses (the server's SanitizeNode). */
const USED: Record<WorkflowNodeType, readonly (typeof SETTINGS)[number][]> = {
	import: [],
	pick: ['stages', 'folder', 'limit', 'source', 'kocIds', 'kocNames'],
	wait: ['waitMinutes'],
	wait_until: ['atMinute'],
	send: ['campaignId'],
	send_text: ['text'],
	email: ['subject', 'text'],
	label: ['labelId'],
	stage: ['stage'],
	if_replied: [],
};

export function toRfNode(n: WorkflowNode): WfNode {
	const data: NodeData = { kind: n.type };
	for (const key of USED[n.type] ?? []) {
		const value = n[key];
		if (value !== undefined) (data as Record<string, unknown>)[key] = value;
	}
	if (n.campaignName) data.campaignName = n.campaignName; // a running workflow carries the name the server took
	return { id: n.id, type: 'wf', position: { x: n.x, y: n.y }, data };
}

/** The block as the server takes it: the place, the type and the settings of the type, nothing that is only for the screen. */
export function toWorkflowNode(n: WfNode): WorkflowNode {
	const out: WorkflowNode = { id: n.id, type: n.data.kind, x: Math.round(n.position.x), y: Math.round(n.position.y) };
	for (const key of USED[n.data.kind] ?? []) {
		const value = n.data[key];
		if (value !== undefined) (out as unknown as Record<string, unknown>)[key] = value;
	}
	return out;
}

/** A block with only the settings its type uses (what a draft kept by an older version may carry more of). */
export function withoutSettingsOfOtherTypes(n: WorkflowNode): WorkflowNode {
	const out: WorkflowNode = { id: n.id, type: n.type, x: n.x, y: n.y };
	for (const key of USED[n.type] ?? []) {
		const value = n[key];
		if (value !== undefined) (out as unknown as Record<string, unknown>)[key] = value;
	}
	return out;
}

const STEP = 180; // from one block to the next, across
const ROW = 160; // from one row to the next, down

/** A place to the right of a block that no other block is on, going down a row at a time when the places are taken. */
export function freeSpotRightOf(from: { x: number; y: number }, taken: { x: number; y: number }[]): { x: number; y: number } {
	const busy = (x: number, y: number) => taken.some((t) => Math.abs(t.x - x) < 150 && Math.abs(t.y - y) < 130);
	for (let row = 0; row < 12; row++) {
		for (const dy of row === 0 ? [0] : [row * ROW, -row * ROW]) {
			const spot = { x: from.x + STEP, y: from.y + dy };
			if (!busy(spot.x, spot.y)) return spot;
		}
	}
	return { x: from.x + STEP, y: from.y + 12 * ROW };
}
