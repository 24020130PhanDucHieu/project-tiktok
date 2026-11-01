export type Role = 'admin' | 'member';
export type AccountStatus = 'active' | 'suspended';

export interface AdminUser {
	id: string;
	name: string;
	email: string;
	role: Role;
	status: AccountStatus;
	lastLoginAt?: string;
	createdAt: string;
	lockedUntil?: string;
}

export interface Pagination {
	page: number;
	limit: number;
	total: number;
	pages: number;
}

export interface AuditEntry {
	id: string;
	actorEmail: string;
	action: string;
	targetType?: string;
	targetId?: string;
	detail?: string;
	ip?: string;
	createdAt: string;
}

export interface TaskKoc {
	id: string;
	tiktokId: string;
	displayName: string;
	avatarUrl: string;
	phone: string;
	stage: string;
	trackingCode: string;
}

export interface BatchFunnel {
	campaign: string;
	pipeline: number;
	contacted: number;
	replied: number;
	agreed: number;
	delivered: number;
	posted: number;
	gmv: number;
}

export interface DashboardData {
	total: number;
	byStage: Record<string, number>;
	videoGmv: number;
	funnels: BatchFunnel[];
	tasks: Record<string, TaskKoc[]>;
}

export interface CollectionInfo {
	name: string;
	count: number;
	sizeMB: number;
	storageMB: number;
	avgObjBytes: number;
	indexes: number;
	indexSizeMB: number;
}

export interface JobRun {
	name: string;
	interval: string;
	lastStart: string;
	durationMs: number;
	ok: boolean;
	error?: string;
	runs: number;
	failures: number;
}

export interface SystemInfo {
	version: string;
	goVersion: string;
	host: {
		os: string;
		hostname: string;
		cpus: number;
		load1: number;
		load5: number;
		load15: number;
		memTotalMB: number;
		memAvailableMB: number;
		swapTotalMB: number;
		swapFreeMB: number;
		processRssMB: number;
		uptimeSeconds: number;
		diskTotalGB: number;
		diskFreeGB: number;
	};
	runtime: { goroutines: number; cpus: number; heapAllocMB: number; sysMB: number; gcRuns: number; lastGcPauseMs: number };
	http: {
		uptimeSeconds: number;
		requests: number;
		success: number;
		clientErrors: number;
		serverErrors: number;
		inFlight: number;
		avgMs: number;
		p95Ms: number;
		slowestMs: number;
	};
	database: {
		ok: boolean;
		error?: string;
		pingMs: number;
		name: string;
		serverVersion: string;
		uptimeSeconds: number;
		dataSizeMB: number;
		storageSizeMB: number;
		indexSizeMB: number;
		connections: Record<string, number>;
		opcounters: Record<string, number>;
		memory: Record<string, number>;
		cache: Record<string, number>;
		network: Record<string, number>;
		collections: CollectionInfo[];
	};
	jobs: JobRun[];
	services: {
		smtpConfigured: boolean;
		zaloPersonal: { state: string; connected: boolean; bridgeOnline?: boolean; error?: string };
	};
	users: { total: number; admins: number; suspended: number; activeLast7d?: number };
}
