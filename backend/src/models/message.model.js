import mongoose from 'mongoose';

const messageSchema = new mongoose.Schema({
	conversationId: { type: mongoose.Schema.Types.ObjectId, ref: 'Conversation', required: true, index: true },
	direction: { type: String, enum: ['inbound', 'outbound'], required: true },
	text: { type: String, required: true, trim: true, maxlength: 4000 },
	status: { type: String, enum: ['pending', 'sent', 'delivered', 'failed'], default: 'sent' },
	sentAt: { type: Date, default: Date.now },
}, { timestamps: true, versionKey: false });

export default mongoose.model('Message', messageSchema);