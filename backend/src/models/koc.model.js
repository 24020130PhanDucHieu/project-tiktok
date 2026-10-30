import mongoose from 'mongoose';

export const KOC_STAGES = [
	'SOURCED',
	'QUALIFIED',
	'AWAITING_APPROVAL',
	'FILTERED_OUT',
	'CONTACTED',
	'NO_REPLY',
	'REPLIED',
	'DORMANT',
	'AGREED',
	'DECLINED',
	'SHIPPING',
	'DELIVERED',
	'VIDEO_POSTED',
	'NOT_FIT',
	'RETURNING',
	'RETURNED',
	'STALLED',
	'COMPLETED',
];

const videoItemSchema = new mongoose.Schema({
	id: { type: String, default: '' },
	date: { type: String, default: '' },
	title: { type: String, default: '' },
	url: { type: String, default: '' },
	coverUrl: { type: String, default: '' },
	views: { type: String, default: '0' },
	likes: { type: String, default: '0' },
	comments: { type: String, default: '0' },
	shares: { type: String, default: '0' },
	product: { type: String, default: '' },
}, { _id: false });

const kocSchema = new mongoose.Schema({
	tiktokId: { type: String, required: true, unique: true, lowercase: true, trim: true, index: true },
	displayName: { type: String, required: true, trim: true },
	avatarUrl: { type: String, default: '' },
	followers: { type: Number, default: 0, min: 0 },
	gmv: { type: Number, default: 0, min: 0 },
	category: { type: String, default: 'Chưa phân loại', trim: true },
	contentPillar: { type: String, default: '', trim: true },
	targetAudience: { type: String, default: '', trim: true },
	source: { type: String, enum: ['TikToday', 'TikTok Liên kết', 'TikTok'], default: 'TikToday' },
	email: { type: String, default: '', trim: true, lowercase: true },
	phone: { type: String, default: '', trim: true, index: true },
	recipient: { type: String, default: '', trim: true },
	shippingAddress: { type: String, default: '', trim: true },
	daHopTac: { type: Boolean, default: false, index: true },
	campaign: { type: String, default: '', trim: true },
	productName: { type: String, default: '', trim: true },
	commissionRate: { type: Number, default: 10, min: 0, max: 100 },
	castType: { type: String, enum: ['freecast', 'cast'], default: 'freecast' },
	castFee: { type: Number, default: 0, min: 0 },
	stage: { type: String, enum: KOC_STAGES, default: 'SOURCED', index: true },
	collabStep: { type: Number, default: 0, min: 0, max: 5 },
	rejectCode: { type: String, default: '', trim: true },
	rejectNote: { type: String, default: '', trim: true },

	// Giai đoạn 2 · Liên hệ & Bám đuổi
	invitedAt: { type: Date, default: null },
	lastContactedAt: { type: Date, default: null },
	lastRepliedAt: { type: Date, default: null },
	reminderStep: { type: Number, default: 0, min: 0, max: 3 },
	callTaskPending: { type: Boolean, default: false, index: true },
	dormantUntil: { type: Date, default: null },
	declineCode: { type: String, default: '', trim: true },
	declineNote: { type: String, default: '', trim: true },
	inviteBrief: { type: String, default: '', trim: true },

	// Giai đoạn 3 · Giao hàng (J&T Express / Tự vận chuyển)
	shippingCarrier: { type: String, enum: ['J&T Express', 'Tự vận chuyển', ''], default: 'J&T Express' },
	trackingCode: { type: String, default: '', trim: true, index: true },
	shippedAt: { type: Date, default: null },
	deliveredAt: { type: Date, default: null },
	lastTrackedAt: { type: Date, default: null },
	trackingStatusText: { type: String, default: '', trim: true },
	shippingDelayedAlert: { type: Boolean, default: false },

	// Giai đoạn 4 · Lên video, GMV & Hoàn hàng (P01-P03, STALLED)
	videoUrl: { type: String, default: '', trim: true },
	videoPostedAt: { type: Date, default: null },
	videoReminderStep: { type: Number, default: 0, min: 0, max: 3 }, // 1: sau 5 ngày, 2: sau 8 ngày, 3: sau 12 ngày
	kocVideoReply: { type: String, enum: ['', 'WILL_POST', 'NOT_FIT', 'SILENT'], default: '' },
	videoViews: { type: Number, default: 0, min: 0 },
	videoOrders: { type: Number, default: 0, min: 0 },
	videoGmv: { type: Number, default: 0, min: 0 },
	stalledFlag: { type: Boolean, default: false, index: true }, // Cờ đỏ trên dashboard (E7)
	postReceiveCode: { type: String, default: '', trim: true }, // P01 - P03 (F1)
	postReceiveNote: { type: String, default: '', trim: true },
	returnTrackingCode: { type: String, default: '', trim: true }, // Mã vận đơn trả (F2)
	returnCondition: { type: String, default: '', trim: true }, // Tình trạng hàng trả (F2)
	returnedAt: { type: Date, default: null },

	// Danh sách video và thời điểm đồng bộ kênh TikTok thật
	videos: { type: [videoItemSchema], default: [] },
	channelSyncedAt: { type: Date, default: null },

	status: { type: String, enum: ['active', 'archived'], default: 'active', index: true },
}, { timestamps: true, versionKey: false });

kocSchema.index({ displayName: 'text', tiktokId: 'text', email: 'text', phone: 'text' });
export default mongoose.model('Koc', kocSchema);
