import Conversation from '../models/conversation.model.js';
import Koc from '../models/koc.model.js';
import Message from '../models/message.model.js';
import { sendKocEmail } from './email.service.js';
import { resolveZaloUserByPhone } from './zalo-personal.service.js';
import { sendZaloTextMessage } from './zalo.service.js';

export function buildInviteMessage(koc, customBrief = '') {
	const product = koc.productName || 'sản phẩm chủ lực của shop';
	const campaign = koc.campaign ? `[${koc.campaign}] ` : '';
	const offer =
		koc.castFee > 0
			? `phí cast ${new Intl.NumberFormat('vi-VN').format(koc.castFee)}đ kèm hoa hồng ${koc.commissionRate || 10}%`
			: `tặng mẫu miễn phí (Freecast) kèm hoa hồng Affiliate ${koc.commissionRate || 10}%`;
	const briefPart = customBrief?.trim()
		? `\nBrief tóm tắt: ${customBrief.trim()}`
		: `\nBrief: Lên 1 video trải nghiệm thực tế gắn giỏ hàng ${product}.`;

	return `Chào ${koc.displayName}, bên mình đang triển khai đợt booking ${campaign}cho dòng ${product} với chính sách ${offer}.${briefPart}\nBạn phản hồi giúp mình để bên mình gửi mẫu nhé!`;
}

async function deliverMessageToKoc(koc, text) {
	const conversation = await Conversation.findOneAndUpdate(
		{ contactHandle: `@${koc.tiktokId}` },
		{
			$set: {
				contactName: koc.displayName,
				contactHandle: `@${koc.tiktokId}`,
				contactPhone: koc.phone || '',
				status: 'open',
				lastMessage: text,
				lastMessageAt: new Date(),
			},
			$setOnInsert: { channel: 'internal' },
		},
		{ new: true, upsert: true, setDefaultsOnInsert: true },
	);

	if (!conversation.externalContactId && conversation.contactPhone) {
		try {
			const recipient = await resolveZaloUserByPhone(conversation.contactPhone);
			if (recipient?.userId) {
				conversation.channel = 'zalo';
				conversation.externalContactId = recipient.userId;
				await conversation.save();
			}
		} catch {
			// Giữ kênh internal nếu chưa kết nối được Zalo
		}
	}

	let deliveryStatus = 'sent';
	if (conversation.channel === 'zalo' && conversation.externalContactId) {
		try {
			await sendZaloTextMessage({ userId: conversation.externalContactId, text });
			deliveryStatus = 'sent';
		} catch {
			deliveryStatus = 'pending';
		}
	}

	const message = await Message.create({
		conversationId: conversation._id,
		direction: 'outbound',
		text,
		status: deliveryStatus,
	});

	return { conversation, message, deliveryStatus };
}

export async function sendFirstInvite(koc, customBrief = '') {
	const text = buildInviteMessage(koc, customBrief || koc.inviteBrief);
	await deliverMessageToKoc(koc, text);

	const now = new Date();
	koc.stage = 'CONTACTED';
	koc.invitedAt = now;
	koc.lastContactedAt = now;
	koc.reminderStep = 0;
	koc.callTaskPending = false;
	koc.dormantUntil = null;
	if (customBrief) koc.inviteBrief = customBrief;
	await koc.save();
	return koc;
}

export async function advanceFollowUpStep(koc, targetStep) {
	const now = new Date();
	const product = koc.productName || 'sản phẩm mẫu';

	if (targetStep === 1) {
		// C2 -> C3: Quá 48h (2 ngày) -> Nhắc tự động qua Zalo
		const reminderText = `[Nhắc lịch Zalo - Sau 2 ngày] Chào ${koc.displayName}, bên mình có gửi lời mời hợp tác sản phẩm ${product} cách đây 2 ngày. Bạn xem qua brief và phản hồi giúp mình nhé!`;
		await deliverMessageToKoc(koc, reminderText);
		koc.stage = 'NO_REPLY';
		koc.reminderStep = 1;
		koc.lastContactedAt = now;
	} else if (targetStep === 2) {
		// C3: Sau 4 ngày -> Nhắc tự động qua Gmail
		const emailText = `Chào ${koc.displayName}, bên mình đã nhắn qua Zalo về lời mời hợp tác ${product} nhưng chưa thấy bạn phản hồi. Bạn kiểm tra thông tin và phản hồi lại giúp nhãn hàng nhé.`;
		if (koc.email) {
			try {
				await sendKocEmail({
					to: koc.email,
					displayName: koc.displayName,
					tiktokId: koc.tiktokId,
					text: emailText,
				});
			} catch {
				// Bỏ qua lỗi SMTP khi môi trường local chưa cấu hình
			}
		}
		await deliverMessageToKoc(koc, `[Hệ thống tự động - Sau 4 ngày] Đã gửi Email nhắc lần 2 (${koc.email || 'chưa có email'}).`);
		koc.stage = 'NO_REPLY';
		koc.reminderStep = 2;
		koc.lastContactedAt = now;
	} else if (targetStep === 3) {
		// C5: Sau 7 ngày -> Tạo Task gọi điện cho Booker
		await deliverMessageToKoc(
			koc,
			`[Task Booker - Sau 7 ngày] KOC chưa phản hồi sau chuỗi nhắc Zalo & Gmail. Booker cần gọi điện trực tiếp tới SĐT ${koc.phone || 'chưa cập nhật'}.`,
		);
		koc.stage = 'NO_REPLY';
		koc.reminderStep = 3;
		koc.callTaskPending = true;
		koc.lastContactedAt = now;
	}

	await koc.save();
	return koc;
}

export async function runStage2Sweep({ simulateNextStep = false } = {}) {
	const now = new Date();
	const summary = {
		reopenedDormant: 0,
		zaloReminded: 0,
		emailReminded: 0,
		callTasksCreated: 0,
	};

	// C7: Tự động mở lại các KOC DORMANT đã đủ 30 ngày
	const dormantKocs = await Koc.find({
		status: 'active',
		stage: 'DORMANT',
		dormantUntil: { $ne: null, $lte: now },
	});
	for (const koc of dormantKocs) {
		koc.stage = 'QUALIFIED';
		koc.reminderStep = 0;
		koc.callTaskPending = false;
		koc.dormantUntil = null;
		await koc.save();
		summary.reopenedDormant += 1;
	}

	// C2 -> C5: Quét các KOC đang CONTACTED hoặc NO_REPLY
	const activeOutreach = await Koc.find({
		status: 'active',
		daHopTac: false,
		stage: { $in: ['CONTACTED', 'NO_REPLY'] },
	});

	for (const koc of activeOutreach) {
		const invitedTime = koc.invitedAt ? new Date(koc.invitedAt).getTime() : now.getTime();
		const hoursElapsed = (now.getTime() - invitedTime) / (1000 * 60 * 60);

		if (simulateNextStep) {
			if (koc.reminderStep === 0) {
				await advanceFollowUpStep(koc, 1);
				summary.zaloReminded += 1;
			} else if (koc.reminderStep === 1) {
				await advanceFollowUpStep(koc, 2);
				summary.emailReminded += 1;
			} else if (koc.reminderStep === 2) {
				await advanceFollowUpStep(koc, 3);
				summary.callTasksCreated += 1;
			}
			continue;
		}

		if (hoursElapsed >= 24 * 7 && koc.reminderStep < 3) {
			await advanceFollowUpStep(koc, 3);
			summary.callTasksCreated += 1;
		} else if (hoursElapsed >= 24 * 4 && koc.reminderStep < 2) {
			await advanceFollowUpStep(koc, 2);
			summary.emailReminded += 1;
		} else if (hoursElapsed >= 48 && koc.reminderStep < 1) {
			await advanceFollowUpStep(koc, 1);
			summary.zaloReminded += 1;
		}
	}

	return summary;
}

