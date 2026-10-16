import Conversation from '../models/conversation.model.js';
import Message from '../models/message.model.js';
import { sendZaloTextMessage } from '../services/zalo.service.js';

export async function listConversations(request, response) {
	const items = await Conversation.find({ status: 'open' }).sort({ lastMessageAt: -1 }).lean();
	response.json({ success: true, data: items });
}

export async function listMessages(request, response) {
	const items = await Message.find({ conversationId: request.params.conversationId }).sort({ sentAt: 1 }).lean();
	response.json({ success: true, data: items });
}

export async function sendMessage(request, response) {
	const conversation = await Conversation.findById(request.params.conversationId);
	if (!conversation) return response.status(404).json({ success: false, message: 'Không tìm thấy cuộc hội thoại' });
	const message = await Message.create({ conversationId: conversation._id, direction: 'outbound', text: request.validated.body.text, status: 'pending' });
	if (conversation.channel === 'zalo' && conversation.externalContactId) {
		try {
			await sendZaloTextMessage({ userId: conversation.externalContactId, text: message.text });
			message.status = 'sent';
		} catch (error) {
			message.status = 'failed';
			await message.save();
			return response.status(502).json({ success: false, message: `Zalo không nhận được tin nhắn: ${error.message}` });
		}
		await message.save();
	}
	conversation.lastMessage = message.text;
	conversation.lastMessageAt = message.sentAt;
	await conversation.save();
	response.status(201).json({ success: true, data: message, notice: message.status === 'sent' ? 'Đã gửi qua Zalo OA.' : 'Đã lưu tin nhắn nội bộ; cần cấu hình Zalo OA để gửi ra ngoài.' });
}