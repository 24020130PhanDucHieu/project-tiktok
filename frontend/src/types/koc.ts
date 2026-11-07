export type KocStage =
	| 'SOURCED'
	| 'QUALIFIED'
	| 'AWAITING_APPROVAL'
	| 'FILTERED_OUT'
	| 'CONTACTED'
	| 'NO_REPLY'
	| 'REPLIED'
	| 'DORMANT'
	| 'AGREED'
	| 'DECLINED'
	| 'SHIPPING'
	| 'DELIVERED'
	| 'VIDEO_POSTED'
	| 'NOT_FIT'
	| 'RETURNING'
	| 'RETURNED'
	| 'STALLED'
	| 'COMPLETED';

export type CollabStepKey = 'cast' | 'mode' | 'sample' | 'video' | 'result';

export interface VideoItem {
	id: string;
	date: string;
	title: string;
	url?: string;
	coverUrl?: string;
	views: string;
	likes: string;
	comments: string;
	shares: string;
	product: string;
}

export interface Creator {
	_id?: string;
	id: string;
	name: string;
	displayName?: string;
	handle: string;
	tiktokId: string;
	contact: string;
	phone?: string;
	email?: string;
	followers: number;
	gmv: number;
	category: string;
	recipient?: string;
	shippingAddress?: string;
	productName?: string;
	campaign?: string;
	commissionRate?: number;
	castFee: number;
	stage: KocStage | string;
	collabStep: number;
	avatar: string;
	avatarUrl?: string;
	videos: VideoItem[];
	channelSyncedAt?: string;
	shippingCarrier?: string;
	trackingCode?: string;
	trackingPhoneLast4?: string;
	shippedAt?: string;
	deliveredAt?: string;
	lastTrackedAt?: string;
	trackingStatusText?: string;
	shippingDelayedAlert?: boolean;
	videoUrl?: string;
	videoPostedAt?: string;
	videoReminderStep?: number;
	kocVideoReply?: string;
	videoViews?: number;
	videoOrders?: number;
	videoGmv?: number;
	stalledFlag?: boolean;
	tungKhongRep?: boolean;
	suggestedDeclineCode?: string;
	declineCode?: string;
	declineNote?: string;
	postReceiveCode?: string;
	postReceiveNote?: string;
	returnTrackingCode?: string;
	returnCondition?: string;
	dnc?: boolean;
	daHopTac?: boolean;
}

export interface ConversationItem {
	_id: string;
	channel?: 'zalo' | 'internal';
	externalContactId?: string;
	contactName: string;
	contactHandle: string;
	contactPhone?: string;
	avatarUrl?: string;
	lastMessage: string;
	lastMessageAt: string;
	unreadCount: number;
	labels?: string[];
	hidden?: boolean;
	chatbotOff?: boolean;
	nextTask?: InboxTaskKey | '';
	status?: 'open' | 'closed';
}

export type InboxTaskKey = 'chot-phi-cast' | 'gui-link' | 'duyet-mau' | 'gui-mau' | 'tra-phi-cast';

export interface InboxLabel {
	_id: string;
	name: string;
	color: string;
}

export interface ChatMessage {
	_id: string;
	kind?: 'system';
	conversationId?: string;
	direction: 'inbound' | 'outbound';
	text: string;
	sentAt: string;
	status: string;
	channel?: 'zalo' | 'email';
	to?: string;
	subject?: string;
}

export interface UserProfile {
	id?: string;
	_id?: string;
	name: string;
	email: string;
	role?: 'admin' | 'member';
}

export interface ToastNotice {
	title: string;
	message: string;
	type: 'success' | 'error';
}

export interface ZaloStatus {
	configured?: boolean;
	connected: boolean;
	state?: string;
	qrImage?: string;
	error?: string;
	bridgeOnline?: boolean;
}

export interface CollabStepDef {
	key: CollabStepKey;
	title: string;
	buttonLabel: string;
}

export interface ReasonOption {
	code: string;
	label: string;
}

export interface MessageTemplate {
	id: string;
	label: string;
	build: (koc?: Creator | null) => string;
}


export type CollabMode = 'targeted' | 'open' | 'product';
export type CollabOutcome = 'posted' | 'no_post' | 'no_response' | 'no_need';

export interface Collab {
	_id: string;
	kocId: string;
	seq: number;
	name: string;
	status: 'active' | 'closed';
	step: number;
	feeSet: boolean;
	castFee: number;
	feeHistory: { fee: number; at: string }[];
	feePaid: boolean;
	mode: CollabMode | '';
	productName: string;
	productId?: string;
	productLink: string;
	productImage?: string;
	targetCollabId?: string;
	commissionRate: number;
	deadline: string;
	freeSample: boolean;
	samples: { id: string; product: string; status: 'pending' | 'approved' | 'rejected'; rejectReason?: string; at: string }[];
	videos: { url: string; title: string; views: number; orders: number; revenue: number; adsCode?: string }[];
	shareOrders: number;
	shareRevenue: number;
	outcome: CollabOutcome | '';
	startedAt: string;
	endedAt: string | null;
	/** real sales read from TikTok Shop for this collaboration; null until it has been synced */
	real?: CollabReal | null;
	perf: {
		revenue: number; commission: number; castFee: number; profit: number; approved: number; videos: number; totalViews: number; costPerMille: number;
		orders: number; source: 'tiktok' | 'manual'; commissionConfirmed: number;
	};
}

export interface CollabReal {
	syncedAt: string;
	orders: number;
	itemsSold: number;
	gmv: number;
	commission: number;
	commissionConfirmed: number;
	excluded: number;
	videos: number;
	views: number;
	viewsKnown: boolean;
	viewsPublic?: boolean;
	byProduct: boolean;
	truncated: boolean;
}

/** One video of a creator with its real sales (views and the like are null while TikTok's Analytics scope is missing). */
export interface CreatorVideo {
	id: string;
	username: string;
	title: string;
	url: string;
	coverUrl: string;
	postedAt: string | null;
	productIds: string[];
	views: number | null;
	likes: number | null;
	comments: number | null;
	shares: number | null;
	/** analytics = TikTok Shop API; public = counters shown on the video's TikTok page */
	statsSource?: 'analytics' | 'public' | '';
	orders: number;
	items: number;
	gmv: number;
	commission: number;
	commissionConfirmed: number;
	kocId?: string;
	kocName?: string;
	kocAvatar?: string;
	inApp: boolean;
}

export interface VideoTotals {
	orders: number;
	items: number;
	gmv: number;
	commission: number;
	commissionConfirmed: number;
	excluded: number;
	videos: number;
	nonVideoOrders: number;
}

export interface KocVideosResult {
	videos: CreatorVideo[];
	/** videos that match, over all pages */
	total: number;
	/** 1-based page of `videos` and how many videos a page holds */
	page: number;
	pageSize: number;
	totals: VideoTotals;
	views: number | null;
	viewsPublic?: boolean;
	analytics: { available: boolean; note?: string };
	range: { from: string; to: string; days: number };
	truncated: boolean;
	asOf: string;
	koc?: { id: string; name: string; username: string; avatar: string };
	note?: string;
}

export interface OpenCollabProduct {
	collaborationId: string;
	productId: string;
	title: string;
	imageUrl: string;
	currency: string;
	minPrice: string;
	maxPrice: string;
	salePriceMin?: string; // price buyers pay now (ongoing deal/flash sale); absent = no discount
	salePriceMax?: string;
	inventory: number;
	commissionRate: number; // percent, the shop's public rate
	status: 'NORMAL' | 'TERMINATING' | string;
	productStatus: string;
	available: boolean; // false: TikTok shows "Sản phẩm không có sẵn"
	creatorCount: number;
	showcaseCreatorCount: number;
}

export interface OpenCollabPage {
	items: OpenCollabProduct[];
	nextPageToken: string;
	total: number;
}

export interface KocContact {
	id: string;
	type: 'phone' | 'email';
	value: string;
	source: string;
	unreachable: boolean;
	updatedAt: string;
}

export interface GmailStatus {
	configured: boolean;
	connected: boolean;
	broken: boolean;
	email: string;
}

export interface SavedTemplate {
	_id: string;
	name: string;
	body: string;
}

export type ChatChannel = 'zalo' | 'email';

export interface CollabTotals {
	collabs: number;
	active: number;
	closed: number;
	posted: number;
	videos: number;
	views: number;
	orders: number;
	revenue: number;
	commission: number;
	castFee: number;
	profit: number;
	owed: number;
}

export interface CollabBucket extends CollabTotals {
	key: string;
	label: string;
	start: string;
}

export type CollabGranularity = 'day' | 'week' | 'month' | 'year';

export interface CollabOverview {
	granularity: CollabGranularity;
	from: string;
	to: string;
	totals: CollabTotals;
	series: CollabBucket[];
	outcomes: Record<string, number>;
	modes: Record<string, number>;
}

export interface CollabRow extends Collab {
	koc: { id: string; name: string; handle: string; avatarUrl: string };
}

// ---- Target collaboration ("Cộng tác mục tiêu") ----

export interface TargetConfig {
	enabled: boolean; // false: the server blocks sending
	maxCreators: number;
	maxProducts: number;
	minRatePct: number;
	maxRatePct: number;
	minAdsRatePct: number;
	maxName: number;
	maxMessage: number;
	/** the shop's own contact, remembered from the last invitation sent */
	contact?: { email: string; zalo: string; facebook: string };
}

export interface TargetCreator {
	openId: string;
	username: string;
	nickname: string;
	avatarUrl: string;
	followers: number;
}

export interface ShopProduct {
	id: string;
	title: string;
	status: string;
	currency: string;
	minPrice: string;
	maxPrice: string;
	inventory: number;
}

export interface TargetInviteInput {
	name: string;
	endDate: string;
	email: string;
	zalo: string;
	facebook: string;
	contentPref: '' | 'video' | 'live';
	message: string;
	hasFreeSample: boolean;
	sampleApprovalExempt: boolean;
	products: { id: string; title: string; commissionPct: number; adsCommissionPct: number }[];
	creators: { openId: string; username: string }[];
}

export interface TargetPreview {
	payload: Record<string, unknown> | null;
	problems: string[];
	creators: TargetCreator[];
	products: ShopProduct[];
	ready: boolean;
}

export interface TargetSummary {
	id: string;
	name: string;
	type: string;
	message: string;
	startTime: number;
	endTime: number;
	updateTime: number;
	productCount: number;
	invitedCount: number;
	showcaseCreatorCount: number;
	contentCreatorCount: number;
	hasFreeSample: boolean;
	sampleApprovalExempt: boolean;
	sentBy?: string;
	creatorNames?: string[];
	localStatus?: string;
}

export interface TargetDetail extends TargetSummary {
	email: string;
	creators: { openId: string; username: string; nickname: string; avatarUrl: string; status: string; showcaseProductCount: number; contentProductCount: number }[];
	products: { id: string; title: string; imageUrl: string; status: string; currency: string; minPrice: string; maxPrice: string; commissionPct: number; adsCommissionPct: number }[];
}

export interface TargetInviteRecord {
	id: string;
	status: 'sending' | 'sent' | 'failed' | 'unknown' | 'cancelled';
	targetId: string;
	name: string;
}
