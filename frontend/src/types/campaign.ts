// Campaigns of the bulk message (a series of texts and pictures) and the runs that send them.

export type StepKind = 'text' | 'image';

/** One message of a campaign: a text, or a picture with an optional caption. {ten} and {kenh} are filled per KOC. */
export interface Step {
	kind: StepKind;
	text?: string;
	imageId?: string;
	caption?: string;
}

export interface Campaign {
	_id: string;
	name: string;
	steps: Step[];
	updatedAt: string;
}

export type RunStatus = 'running' | 'done' | 'stopped';

/** Status of one KOC inside a run (the server's domain.Target* values). */
export type TargetStatus = 'queued' | 'sending' | 'sent' | 'replied' | 'partial' | 'skipped' | 'failed' | 'unknown' | 'not_sent';

export interface RunTarget {
	kocId: string;
	kocName: string;
	handle: string;
	status: TargetStatus | string;
	reason?: string;
	stepsSent: number;
	startedAt?: string;
}

export interface BulkRun {
	_id: string;
	campaignName: string;
	steps: Step[];
	targets: RunTarget[];
	status: RunStatus;
	stopReason?: string;
	by: string;
	/** the Zalo account the run sends through, and whose daily limits it counts against (missing on runs made before there were several) */
	zaloAccountId?: string;
	createdAt: string;
	nextAt?: string;
	finishedAt?: string;
}
