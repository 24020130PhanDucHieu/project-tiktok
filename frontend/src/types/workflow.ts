/** The blocks of the Quy trình canvas. */
export type WorkflowNodeType =
	| 'import' // starts a workflow: KOCs from an Excel sheet
	| 'pick' // starts a workflow: KOCs that are already in the directory
	| 'wait'
	| 'wait_until'
	| 'send' // a saved campaign on Zalo
	| 'send_text' // one text on Zalo
	| 'email'
	| 'label'
	| 'stage'
	| 'if_replied';

/** The output ports: every block has "out" except if_replied. */
export type WorkflowPort = 'out' | 'replied' | 'unanswered';

export interface WorkflowNode {
	id: string;
	type: WorkflowNodeType;
	x: number;
	y: number;
	/** wait: how long, in minutes */
	waitMinutes?: number;
	/** wait_until: minutes after midnight, Vietnam time */
	atMinute?: number;
	/** send: the saved campaign */
	campaignId?: string;
	/** send: filled by the server when the workflow starts */
	campaignName?: string;
	/** send_text, email: {ten} and {kenh} are filled for each KOC */
	text?: string;
	/** email: empty means the default subject */
	subject?: string;
	/** label: the id of the label */
	labelId?: string;
	/** stage: the stage the KOCs are moved to */
	stage?: string;
	/** pick: the stages to take KOCs from, the folder, and how many at most */
	stages?: string[];
	folder?: string;
	limit?: number;
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
/** One line of the report of a run: at this moment this happened to these KOCs (ids) at this block. */
export interface WorkflowEvent {
	at: string;
	nodeId: string;
	kind: 'in' | 'out' | 'did' | 'skip';
	port?: string;
	kocs: string[];
}

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
	log?: WorkflowEvent[];
	/** KOC id → name, for the KOCs the log speaks of */
	kocNames?: Record<string, string>;
	createdAt: string;
	finishedAt?: string;
}

/** The graph drawn on the canvas, before it is started. */
export interface WorkflowDraft {
	nodes: WorkflowNode[];
	edges: WorkflowEdge[];
}

/** A label of the inbox (what a label block puts on a conversation). */
export interface InboxLabel {
	_id: string;
	name: string;
	color: string;
}

/** A saved workflow in the list. */
export interface TemplateSummary {
	_id: string;
	name: string;
	createdBy: string;
	nodeCount: number;
	updatedAt: string;
}

/** A saved workflow with its blocks and wires (never the steps of a campaign, never a sheet). */
export interface WorkflowTemplate {
	_id: string;
	name: string;
	nodes: WorkflowNode[];
	edges: WorkflowEdge[];
	createdBy: string;
	createdAt: string;
	updatedAt: string;
}
