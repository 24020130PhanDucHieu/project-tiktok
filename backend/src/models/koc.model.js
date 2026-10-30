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
	rejectCode: { type: String, default: '', trim: true },
	rejectNote: { type: String, default: '', trim: true },
	status: { type: String, enum: ['active', 'archived'], default: 'active', index: true },
}, { timestamps: true, versionKey: false });

kocSchema.index({ displayName: 'text', tiktokId: 'text', email: 'text', phone: 'text' });
export default mongoose.model('Koc', kocSchema);
