import type { ChatMessage, ConversationItem, Creator, InboxLabel } from '../types/koc';
import type { ZaloAccount } from '../types/zalo';

/** Every id of the demo data starts with this, so a request that carries one can be recognised and never leaves the browser. */
export const DEMO_ID_PREFIX = 'demo-';
/** The editor only accepts 24-hex ids for labels and KOCs, so the ids the demo diagram refers to look like that (they never go to a server). */
export const demoHexId = (n: number) => `d3e0000000000000000000${String(n).padStart(2, '0')}`;
export const DEMO_BANNER = 'DỮ LIỆU DEMO – không phải dữ liệu thật';

const minutesAgo = (m: number) => new Date(Date.now() - m * 60000).toISOString();

interface Seed {
	n: number;
	name: string;
	handle: string;
	category: string;
	followers: number;
	gmv: number;
	stage: Creator['stage'];
	product: string;
	last: string;
	ago: number;
	unread: number;
	labels: string[];
}

// all names, handles and numbers are made up; the phone numbers are in a range that is not assigned to anybody
const SEEDS: Seed[] = [
	{ n: 1, name: 'Minh Anh Review', handle: '@demo.minhanh', category: 'Làm đẹp', followers: 52000, gmv: 84000000, stage: 'REPLIED', product: 'Serum Vitamin C Demo', last: 'Dạ mình quan tâm ạ, bên bạn gửi mình báo giá nhé', ago: 12, unread: 2, labels: ['d3e000000000000000000011'] },
	{ n: 2, name: 'Thu Hà Daily', handle: '@demo.thuha', category: 'Đời sống', followers: 18500, gmv: 21000000, stage: 'CONTACTED', product: 'Bình giữ nhiệt Demo', last: 'Chào bạn, mình gửi lời mời hợp tác sản phẩm bình giữ nhiệt', ago: 95, unread: 0, labels: [] },
	{ n: 3, name: 'Gia Bảo Style', handle: '@demo.giabao', category: 'Thời trang', followers: 120000, gmv: 256000000, stage: 'AGREED', product: 'Áo khoác gió Demo', last: 'Okie bạn, mình chốt phí cast như đã trao đổi nha', ago: 240, unread: 0, labels: ['d3e000000000000000000011', 'd3e000000000000000000012'] },
	{ n: 4, name: 'Lan Phương Home', handle: '@demo.lanphuong', category: 'Đời sống', followers: 9300, gmv: 4200000, stage: 'SHIPPING', product: 'Bộ hộp bảo quản Demo', last: 'Mình đã nhận được mẫu rồi, cảm ơn shop', ago: 1500, unread: 0, labels: ['d3e000000000000000000013'] },
	{ n: 5, name: 'Đức Huy Tech', handle: '@demo.duchuy', category: 'Công nghệ', followers: 76000, gmv: 98000000, stage: 'NO_REPLY', product: 'Tai nghe Demo X', last: 'Bên mình gửi lại thông tin hợp tác, bạn xem giúp mình nhé', ago: 4300, unread: 0, labels: [] },
];

export const DEMO_CREATORS: Creator[] = SEEDS.map((s) => ({
	id: `${DEMO_ID_PREFIX}koc-${s.n}`,
	name: s.name,
	handle: s.handle,
	tiktokId: s.handle.slice(1),
	contact: `090000000${s.n}`,
	phone: `090000000${s.n}`,
	email: `koc${s.n}@demo.invalid`,
	followers: s.followers,
	gmv: s.gmv,
	category: s.category,
	productName: s.product,
	campaign: 'Đợt DEMO',
	commissionRate: 10,
	castFee: 0,
	stage: s.stage,
	collabStep: 0,
	avatar: s.name
		.split(' ')
		.map((w) => w[0])
		.slice(0, 2)
		.join(''),
	videos: [],
}));

export const DEMO_LABELS: InboxLabel[] = [
	{ _id: 'd3e000000000000000000011', name: 'Ưu tiên', color: '#ef4444' },
	{ _id: 'd3e000000000000000000012', name: 'Đã chốt phí', color: '#22c55e' },
	{ _id: 'd3e000000000000000000013', name: 'Chờ nhận mẫu', color: '#f59e0b' },
];

export const DEMO_ZALO_ACCOUNT: ZaloAccount = {
	_id: 'demo-zalo-1',
	label: 'Zalo demo',
	displayName: 'Tài khoản Zalo (demo)',
	isDefault: true,
	disabled: false,
	state: 'connected',
	connected: true,
	bridgeOnline: true,
	hasCredentials: true,
	needsQr: false,
};

export function buildDemoConversations(): ConversationItem[] {
	return SEEDS.map((s) => ({
		_id: `${DEMO_ID_PREFIX}c${s.n}`,
		channel: 'zalo',
		zaloAccountId: DEMO_ZALO_ACCOUNT._id,
		externalContactId: `demo-zalo-uid-${s.n}`,
		contactName: s.name,
		contactHandle: s.handle,
		contactPhone: `090000000${s.n}`,
		lastMessage: s.last,
		lastMessageAt: minutesAgo(s.ago),
		unreadCount: s.unread,
		labels: s.labels,
		stage: s.stage,
		status: 'open',
	}));
}

export function buildDemoMessages(conversationId: string): ChatMessage[] {
	const seed = SEEDS.find((s) => `${DEMO_ID_PREFIX}c${s.n}` === conversationId);
	if (!seed) return [];
	const m = (i: number, direction: ChatMessage['direction'], text: string, ago: number): ChatMessage => ({
		_id: `${conversationId}-m${i}`,
		conversationId,
		direction,
		text,
		sentAt: minutesAgo(ago),
		status: direction === 'outbound' ? 'Đã nhận' : '',
		channel: 'zalo',
		canReact: direction === 'inbound',
	});
	return [
		m(1, 'outbound', `Chào ${seed.name}, bên mình có theo dõi kênh và muốn mời bạn hợp tác sản phẩm ${seed.product}.`, seed.ago + 180),
		m(2, 'inbound', 'Chào shop, bạn cho mình xin thêm thông tin sản phẩm với ạ.', seed.ago + 120),
		m(3, 'outbound', 'Dạ bên mình gửi bạn bảng giá và chính sách hoa hồng 10% nhé.', seed.ago + 60),
		m(4, 'inbound', seed.last, seed.ago),
	];
}
