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
	perf: { revenue: number; commission: number; castFee: number; profit: number; approved: number; videos: number; totalViews: number; costPerMille: number };
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
