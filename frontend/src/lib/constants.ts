import type {
	ChatMessage,
	CollabStepDef,
	ConversationItem,
	Creator,
	MessageTemplate,
	ReasonOption,
} from '../types/koc';

export const INITIAL_FOLDERS: string[] = ['Làm đẹp', 'Đời sống', 'Thời trang'];

export const COLLAB_STEPS: CollabStepDef[] = [
	{ key: 'cast', title: 'Chốt phí cast', buttonLabel: 'Chốt phí cast' },
	{ key: 'mode', title: 'Chọn hình thức hợp tác', buttonLabel: 'Chọn hình thức hợp tác' },
	{ key: 'sample', title: 'Duyệt & gửi mẫu', buttonLabel: 'Duyệt & gửi mẫu' },
	{ key: 'video', title: 'KOC đăng bài', buttonLabel: 'Cập nhật video' },
	{ key: 'result', title: 'Hiệu quả', buttonLabel: 'Cập nhật hiệu quả' },
];

export const DECLINE_REASONS: ReasonOption[] = [
	{ code: 'R01', label: 'R01 — Không nhận Freecast (Yêu cầu có phí)' },
	{ code: 'R02', label: 'R02 — Giá cast chưa phù hợp ngân sách' },
	{ code: 'R03', label: 'R03 — Hoa hồng Affiliate thấp' },
	{ code: 'R04', label: 'R04 — KOC đang kín lịch đăng bài' },
	{ code: 'R05', label: 'R05 — Sản phẩm không hợp định hướng kênh' },
	{ code: 'R09', label: 'R09 — Lý do khác' },
];

export const POST_RECEIVE_REASONS: ReasonOption[] = [
	{ code: 'P01', label: 'P01 — Không hợp sản phẩm (Kích ứng / Không đúng tông / Không vừa size)' },
	{ code: 'P02', label: 'P02 — Sản phẩm lỗi ngoại quan / Hư hỏng khi vận chuyển' },
	{ code: 'P03', label: 'P03 — KOC đổi định hướng nội dung, xin hoàn trả mẫu' },
];

export const MESSAGE_TEMPLATES: MessageTemplate[] = [
	{
		id: 'invite-1',
		label: 'Mẫu 1: Lời mời hợp tác lần đầu (Gửi mẫu trải nghiệm)',
		build: (koc) =>
			`Chúng mình có theo dõi kênh ${koc?.handle || '@koc'} và nhận thấy định hướng nội dung, hình ảnh của bạn khá phù hợp với thương hiệu bên mình.\nBên mình mong muốn gửi sản phẩm ${koc?.productName || 'mẫu'} để bạn trải nghiệm (nếu không phù hợp bạn có thể gửi hoàn lại). Sau khi nhận mẫu, bạn có thể lên video giới thiệu hoặc dựng bối cảnh phù hợp với phong cách kênh của bạn. Mức hoa hồng đề xuất là ${koc?.commissionRate || 10}% cho đơn tự nhiên.`,
	},
	{
		id: 'remind-2d',
		label: 'Mẫu 2: Nhắc lần 2 sau 48h (+2 ngày)',
		build: (koc) =>
			`Chào ${koc?.name || 'bạn'}, bên mình có gửi lời mời hợp tác trải nghiệm sản phẩm thuộc chiến dịch ${koc?.campaign || 'mới'} cách đây 2 ngày. Bạn xem qua thông tin rồi phản hồi giúp mình nhé!`,
	},
	{
		id: 'remind-4d-exit',
		label: 'Mẫu 3: Nhắc lần 3 sau +4 ngày (Có lối thoát lịch sự)',
		build: (koc) =>
			`Chào ${koc?.name || 'bạn'}, bên mình nhắn ngắn hỏi thăm về lời mời trải nghiệm sản phẩm hôm trước ạ. Nếu đợt này lịch kênh chưa phù hợp hoặc sản phẩm không phù hợp cũng báo giúp bên mình một tiếng nhé!`,
	},
	{
		id: 'ask-address',
		label: 'Mẫu 4: Xin địa chỉ gửi sản phẩm mẫu',
		build: (koc) =>
			`Cảm ơn ${koc?.name || 'bạn'} đã đồng ý hợp tác. Bạn cho mình xin Tên người nhận, Số điện thoại và Địa chỉ chi tiết để bên mình lên đơn gửi sản phẩm mẫu qua bạn trải nghiệm nhé!`,
	},
];

export const initialCreators: Creator[] = [
	{
		id: 'seed-1',
		name: 'Tủ đồ da ngăm',
		handle: '@trlyng.review',
		tiktokId: 'trlyng.review',
		contact: '0327637213',
		phone: '0327637213',
		email: 'trlyng.review@gmail.com',
		followers: 4000,
		gmv: 125500000,
		category: 'Thời trang',
		recipient: 'Trần Lý',
		shippingAddress: 'Quận Cầu Giấy, Hà Nội',
		productName: 'Khăn lạnh thể thao Cooltech',
		campaign: 'Đợt T11 - Cooltech',
		commissionRate: 10,
		castFee: 0,
		stage: 'CONTACTED',
		collabStep: 0,
		avatar: 'TĐ',
		videos: [
			{
				id: 'v1',
				date: '26/10',
				title: 'Một bước nhỏ thêm vào chu trình skincare nhưng có thể...',
				views: '125',
				likes: '2',
				comments: '0',
				shares: '0',
				product: '[HÀNG TẶNG KHÔNG BÁN] Kem dưỡng...',
			},
			{
				id: 'v2',
				date: '23/10',
				title: 'Đồ chạy bộ nhà Coolmate #dochaybo #chaybo...',
				views: '4k',
				likes: '12',
				comments: '0',
				shares: '0',
				product: 'Coolmate Core Tee - Áo Thể Thao...',
			},
			{
				id: 'v3',
				date: '20/10',
				title: 'Một brand đồ thể thao của Việt Nam mà có thể bạn chưa biết...',
				views: '284',
				likes: '10',
				comments: '1',
				shares: '2',
				product: 'Quần tập bike dệt liền Core...',
			},
		],
	},
	{
		id: 'seed-2',
		name: 'Huế Babi',
		handle: '@huebabi.official',
		tiktokId: 'huebabi.official',
		contact: '0866073103',
		phone: '0866073103',
		email: 'huebabi@gmail.com',
		followers: 12700,
		gmv: 45000000,
		category: 'Làm đẹp',
		recipient: 'Huế Babi',
		shippingAddress: 'Quận 7, TP.HCM',
		productName: 'Serum phục hồi B5',
		campaign: 'Đợt T11 - Skincare',
		commissionRate: 15,
		castFee: 500000,
		stage: 'AWAITING_APPROVAL',
		collabStep: 0,
		avatar: 'HB',
		videos: [],
	},
	{
		id: 'seed-3',
		name: 'Gia đình trái cây',
		handle: '@giadinhtraicay1202',
		tiktokId: 'giadinhtraicay1202',
		contact: '0987654321',
		phone: '0987654321',
		email: 'giadinhtraicay@gmail.com',
		followers: 3100,
		gmv: 18500000,
		category: 'Đời sống',
		recipient: 'Gia đình trái cây',
		shippingAddress: 'Quận Thanh Xuân, Hà Nội',
		productName: 'Máy ép chậm mini',
		campaign: 'Đợt T11 - Gia dụng',
		commissionRate: 10,
		castFee: 0,
		stage: 'AGREED',
		collabStep: 2,
		avatar: 'GT',
		videos: [],
	},
];

export const previewConversations: ConversationItem[] = [
	{
		_id: 'preview-1',
		contactName: 'Tủ đồ da ngăm',
		contactHandle: '@trlyng.review',
		contactPhone: '0327637213',
		lastMessage: 'Chúng mình có theo dõi kênh @trlyng.review và nhận thấy định hướng...',
		lastMessageAt: new Date().toISOString(),
		unreadCount: 0,
	},
	{
		_id: 'preview-2',
		contactName: 'Huế Babi',
		contactHandle: '@huebabi.official',
		contactPhone: '0866073103',
		lastMessage: 'Chúng mình có theo dõi kênh và muốn gửi lời mời hợp tác...',
		lastMessageAt: new Date(Date.now() - 86400000).toISOString(),
		unreadCount: 1,
	},
	{
		_id: 'preview-3',
		contactName: 'Gia đình trái cây',
		contactHandle: '@giadinhtraicay1202',
		contactPhone: '0987654321',
		lastMessage: 'Okie cậu nha, mình đã nhận được thông tin rồi ạ',
		lastMessageAt: new Date(Date.now() - 172800000).toISOString(),
		unreadCount: 0,
	},
];

export const previewMessagesMap: Record<string, ChatMessage[]> = {
	'preview-1': [
		{
			_id: 'm1',
			direction: 'outbound',
			text: 'Chào bạn',
			sentAt: new Date(Date.now() - 3600000 * 5).toISOString(),
			status: 'Đã nhận',
		},
		{
			_id: 'm2',
			direction: 'outbound',
			text: 'Chúng mình xin giới thiệu một chút. Chúng mình là thương hiệu đồ thể thao Cooltech. Bên mình là một thương hiệu mới với ưu thế nổi bật ở phần chất liệu: Bên mình tự nghiên cứu và phát triển (R&D) công thức vải, sau đó đặt sản xuất độc quyền, nên kiểm soát được chất lượng từ chất liệu đến thành phẩm.',
			sentAt: new Date(Date.now() - 3600000 * 4).toISOString(),
			status: 'Đã nhận',
		},
		{
			_id: 'm3',
			direction: 'outbound',
			text: 'Chúng mình có theo dõi kênh @trlyng.review và nhận thấy định hướng nội dung, hình ảnh của bạn khá phù hợp với hướng phát triển mà Cooltech đang xây dựng. Vì vậy, chúng mình mong muốn đề xuất hợp tác cùng bạn với sản phẩm khăn lạnh thể thao Cooltech của bên mình.\nSau khi nhận sản phẩm, bạn có thể lên video review/giới thiệu sản phẩm, hoặc đưa vào tuyến nội dung sử dụng sản phẩm thật theo phong cách của bạn thường làm. Mức hoa hồng chúng mình đề xuất là 10% cho đơn tự nhiên và 3% cho đơn từ quảng cáo.',
			sentAt: new Date(Date.now() - 3600000 * 3).toISOString(),
			status: 'Đã nhận',
		},
	],
};

