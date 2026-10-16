import mongoose from 'mongoose';

const kocSchema = new mongoose.Schema({
	tiktokId: { type: String, required: true, unique: true, lowercase: true, trim: true, index: true },
	displayName: { type: String, required: true, trim: true },
	avatarUrl: { type: String, default: '' },
	followers: { type: Number, default: 0, min: 0 },
	gmv: { type: Number, default: 0, min: 0 },
	category: { type: String, default: 'Chưa phân loại', trim: true },
	email: { type: String, default: '', trim: true, lowercase: true },
	phone: { type: String, default: '', trim: true },
	recipient: { type: String, default: '', trim: true },
	shippingAddress: { type: String, default: '', trim: true },
	castFee: { type: Number, default: 0, min: 0 },
	status: { type: String, enum: ['active', 'archived'], default: 'active', index: true },
}, { timestamps: true, versionKey: false });

kocSchema.index({ displayName: 'text', tiktokId: 'text', email: 'text', phone: 'text' });
export default mongoose.model('Koc', kocSchema);