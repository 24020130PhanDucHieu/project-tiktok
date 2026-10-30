import Conversation from '../models/conversation.model.js';
import Koc from '../models/koc.model.js';
import Message from '../models/message.model.js';
import { parseKocInboundText } from '../services/outreach.service.js';

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

	// Tự động bóc tách SĐT / Địa chỉ / Link Video bằng Chatbot D1 & cập nhật trạng thái KOC
	const parsed = parseKocInboundText(text);
	const matchOr = [];
	if (conversation.contactHandle?.startsWith('@')) {
		matchOr.push({ tiktokId: conversation.contactHandle.slice(1).toLowerCase() });
	}
	if (conversation.contactPhone) {
		matchOr.push({ phone: conversation.contactPhone });
	}
	if (parsed.phone) {
		matchOr.push({ phone: parsed.phone });
	}

	if (matchOr.length) {
		const matchedKocs = await Koc.find({ status: 'active', $or: matchOr });
		for (const koc of matchedKocs) {
			koc.lastRepliedAt = new Date();
			koc.callTaskPending = false;
			if (parsed.phone && !koc.phone) koc.phone = parsed.phone;
			if (parsed.shippingAddress) {
				koc.shippingAddress = parsed.shippingAddress;
				if (!koc.recipient) koc.recipient = koc.displayName;
				if (['CONTACTED', 'NO_REPLY', 'REPLIED', 'DORMANT'].includes(koc.stage)) {
					koc.stage = 'AGREED';
					koc.collabStep = Math.max(koc.collabStep || 0, 2);
				}
			} else if (parsed.videoUrl) {
				koc.videoUrl = parsed.videoUrl;
				koc.videoPostedAt = new Date();
				koc.stage = 'VIDEO_POSTED';
				koc.collabStep = Math.max(koc.collabStep || 0, 4);
				koc.stalledFlag = false;
			} else if (parsed.isNotFitAfterReceive && ['DELIVERED', 'SHIPPING'].includes(koc.stage)) {
				koc.stage = 'NOT_FIT';
				koc.kocVideoReply = 'NOT_FIT';
			} else if (parsed.isWillPost && koc.stage === 'DELIVERED') {
				koc.kocVideoReply = 'WILL_POST';
			} else if (['CONTACTED', 'NO_REPLY', 'DORMANT'].includes(koc.stage)) {
				koc.stage = 'REPLIED';
			}
			await koc.save();
		}
	}

	response.json({ success: true });
}
