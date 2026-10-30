import Conversation from '../models/conversation.model.js';
import Koc from '../models/koc.model.js';
import Message from '../models/message.model.js';

export function verifyZaloWebhook(request, response) {
	response.json({ success: true, service: 'zalo-webhook' });
}

export async function receiveZaloWebhook(request, response) {
	const payload = request.body || {};
	const eventName = payload.event_name || payload.event;
	const userId = payload.sender?.id || payload.user_id;
	const text = payload.message?.text || payload.text;
	if (eventName && eventName !== 'user_send_text') return response.json({ success: true, ignored: true });
	if (!userId || !text) return response.status(400).json({ success: false, message: 'Webhook thiếu user_id hoặc nội dung tin nhắn' });

	const conversation = await Conversation.findOneAndUpdate(
		{ channel: 'zalo', externalContactId: userId },
		{
			$set: {
				contactName: payload.sender?.display_name || `Zalo ${userId}`,
				contactHandle: `zalo:${userId}`,
				lastMessage: text,
				lastMessageAt: new Date(),
				status: 'open',
			},
			$inc: { unreadCount: 1 },
		},
		{ new: true, upsert: true, setDefaultsOnInsert: true },
	);
	await Message.create({ conversationId: conversation._id, direction: 'inbound', text, status: 'delivered' });

	// Tự động chuyển trạng thái KOC sang REPLIED nếu đang ở Giai đoạn 2 (CONTACTED / NO_REPLY / DORMANT)
	const matchOr = [];
	if (conversation.contactHandle?.startsWith('@')) {
		matchOr.push({ tiktokId: conversation.contactHandle.slice(1).toLowerCase() });
	}
	if (conversation.contactPhone) {
		matchOr.push({ phone: conversation.contactPhone });
	}
	if (matchOr.length) {
		await Koc.updateMany(
			{
				status: 'active',
				stage: { $in: ['CONTACTED', 'NO_REPLY', 'DORMANT'] },
				$or: matchOr,
			},
			{
				$set: {
					stage: 'REPLIED',
					lastRepliedAt: new Date(),
					callTaskPending: false,
				},
			},
		);
	}

	response.json({ success: true });
}
