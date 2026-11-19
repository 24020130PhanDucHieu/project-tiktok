/** The four blocks of the Quy trình canvas. */
export type WorkflowNodeType = 'import' | 'wait' | 'send' | 'if_replied';

/** The output ports: every block has "out" except if_replied. */
export type WorkflowPort = 'out' | 'replied' | 'unanswered';

export interface WorkflowNode {
	id: string;
	type: WorkflowNodeType;
	x: number;
	y: number;
	/** wait: how long, in minutes */
	waitMinutes?: number;
	/** send: the saved campaign */
	campaignId?: string;
	/** send: filled by the server when the workflow starts */
	campaignName?: string;
}

export interface WorkflowEdge {
	id: string;
	from: string;
	port: WorkflowPort;
	to: string;
}

export type NodeStatus = 'pending' | 'running' | 'waiting' | 'done' | 'stopped';

/** What the screen shows on a block while a workflow runs. A block with no state has not been reached. */
export interface NodeState {
	nodeId: string;
	status: NodeStatus;
	count: number;
	waitUntil?: string;
	reason?: string;
}

export interface WorkflowToken {
	nodeId: string;
	readyAt: string;
}

export type WorkflowStatus = 'running' | 'done' | 'stopped';

/** A workflow as the API answers it. The rows of the sheet never come back. */
export interface Workflow {
	_id: string;
	name: string;
	nodes: WorkflowNode[];
	edges: WorkflowEdge[];
	status: WorkflowStatus;
	stopReason?: string;
	by: string;
	tokens?: WorkflowToken[];
	states?: NodeState[];
	createdAt: string;
	finishedAt?: string;
}

/** The graph drawn on the canvas, before it is started. */
export interface WorkflowDraft {
	nodes: WorkflowNode[];
	edges: WorkflowEdge[];
}
