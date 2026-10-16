import Conversation from '../models/conversation.model.js';
import Message from '../models/message.model.js';

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
	conversation.lastMessage = message.text;
	conversation.lastMessageAt = message.sentAt;
	await conversation.save();
	response.status(201).json({ success: true, data: message, notice: 'Đã tạo tin nhắn. Adapter Zalo sẽ xử lý ở bước tích hợp sau.' });
}