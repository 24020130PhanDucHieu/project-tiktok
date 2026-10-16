import mongoose from 'mongoose';

const conversationSchema = new mongoose.Schema({
	channel: { type: String, enum: ['zalo', 'internal'], default: 'zalo' },
	externalContactId: { type: String, trim: true },
	contactName: { type: String, required: true, trim: true },
	contactHandle: { type: String, default: '' },
	contactPhone: { type: String, default: '', trim: true },
	avatarUrl: { type: String, default: '' },
	lastMessage: { type: String, default: '' },
	lastMessageAt: { type: Date, default: Date.now },
	unreadCount: { type: Number, default: 0, min: 0 },
	status: { type: String, enum: ['open', 'closed'], default: 'open' },
}, { timestamps: true, versionKey: false });

conversationSchema.index({ lastMessageAt: -1 });
export default mongoose.model('Conversation', conversationSchema);