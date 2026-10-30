import Conversation from '../models/conversation.model.js';
import Koc from '../models/koc.model.js';
import Message from '../models/message.model.js';
import {
	buildChatbotAutoReply,
	parseKocInboundText,
	processInboundWithChatbot,
} from '../services/outreach.service.js';
import { sendZaloTextMessage } from '../services/zalo.service.js';
import { resolveZaloUserByPhone } from '../services/zalo-personal.service.js';

export async function listConversations(request, response) {
	const kocs = await Koc.find({ status: 'active' }).select('displayName tiktokId phone').lean();
	await Promise.all(
		kocs.map((koc) =>
			Conversation.updateOne(
				{ contactHandle: `@${koc.tiktokId}` },
				{
					$setOnInsert: {
						channel: 'internal',
						contactName: koc.displayName,
						contactHandle: `@${koc.tiktokId}`,
						contactPhone: koc.phone,
						lastMessage: 'Đã thêm KOC vào danh bạ',
					},
				},
				{ upsert: true },
			),
		),
	);
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
	if (!conversation.externalContactId && conversation.contactPhone) {
		try {
			const recipient = await resolveZaloUserByPhone(conversation.contactPhone);
			conversation.channel = 'zalo';
			conversation.externalContactId = recipient.userId;
			if (recipient.displayName) conversation.contactName = recipient.displayName;
			await conversation.save();
		} catch (error) {
			return response.status(422).json({ success: false, message: `Không thể tìm tài khoản Zalo theo số điện thoại: ${error.message}` });
		}
	}
	const message = await Message.create({
		conversationId: conversation._id,
		direction: 'outbound',
		text: request.validated.body.text,
		status: 'pending',
	});
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
	response.status(201).json({
		success: true,
		data: message,
		notice: message.status === 'sent' ? 'Đã gửi qua Zalo OA.' : 'Đã lưu tin nhắn nội bộ; cần cấu hình Zalo OA để gửi ra ngoài.',
	});
}

export async function receiveInboundMessage(request, response) {
	const conversation = await Conversation.findById(request.params.conversationId);
	if (!conversation) return response.status(404).json({ success: false, message: 'Không tìm thấy cuộc hội thoại' });

	const text = request.validated.body.text;
	const autoReply = request.body?.autoReply !== false;

	const inboundMessage = await Message.create({
		conversationId: conversation._id,
		direction: 'inbound',
		text,
		status: 'delivered',
	});

	conversation.lastMessage = text;
	conversation.lastMessageAt = inboundMessage.sentAt;
	await conversation.save();

	const { parsed, koc, botMessage } = await processInboundWithChatbot({
		conversation,
		text,
		autoReply,
	});

	response.status(201).json({
		success: true,
		data: {
			inboundMessage,
			botMessage,
			koc,
			parsed,
		},
	});
}

export async function parseChatbotEndpoint(request, response) {
	const { text = '', koc = {} } = request.body || {};
	const parsed = parseKocInboundText(text);
	const botReply = buildChatbotAutoReply({
		koc: {
			displayName: koc.name || koc.displayName || 'bạn',
			productName: koc.productName || 'sản phẩm mẫu',
			commissionRate: koc.commissionRate || 10,
			dnc: Boolean(koc.dnc),
		},
		parsed,
	});
	return response.json({
		success: true,
		data: {
			parsed,
			botReply,
		},
	});
}
