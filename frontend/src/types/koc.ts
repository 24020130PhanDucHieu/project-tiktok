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
	status?: 'open' | 'closed';
}

export interface ChatMessage {
	_id: string;
	conversationId?: string;
	direction: 'inbound' | 'outbound';
	text: string;
	sentAt: string;
	status: string;
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

