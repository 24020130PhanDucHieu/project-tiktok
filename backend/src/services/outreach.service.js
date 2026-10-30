import Conversation from '../models/conversation.model.js';
import Koc from '../models/koc.model.js';
import Message from '../models/message.model.js';
import { sendKocEmail } from './email.service.js';
import { queryJtTracking } from './jt.service.js';
import { fetchRealTikTokChannel, fetchRealTikTokVideoMetrics } from './tiktok.service.js';
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

export async function deliverMessageToKoc(koc, text) {
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

/**
 * D1: Chatbot thông minh tự động bóc tách SĐT, địa chỉ nhận hàng hoặc ý định từ chối từ tin nhắn KOC
 */
export function parseKocInboundText(text = '') {
	const raw = String(text || '').trim();
	const lower = raw.toLowerCase();

	const phoneMatch = raw.match(/(?:\+84|0)[35789]\d{8}/);
	const phone = phoneMatch ? phoneMatch[0].replace(/^\+84/, '0') : '';

	const hasAddressKeyword =
		/(địa chỉ|đ\/c|dc:|số nhà|ngõ|hẻm|phường|quận|huyện|tỉnh|thành phố|tp\.|tx\.|thị xã|xã|ấp|thôn|tòa|chung cư|ký túc xá|ktx|đường|phố)/i.test(
			raw,
		);
	const shippingAddress = hasAddressKeyword || (phone && raw.length > 25) ? raw : '';

	const isDecline = /(không nhận|từ chối|chưa phù hợp|bận lịch|không hợp tác|không làm được)/i.test(lower);
	const isNotFitAfterReceive = /(không hợp sản phẩm|bị kích ứng|không lên video được|xin gửi trả|hoàn hàng)/i.test(lower);
	const isWillPost = /(sẽ đăng|đang quay|sắp lên video|mai mình lên|tuần này lên)/i.test(lower);

	const tiktokUrlMatch = raw.match(/https?:\/\/(?:www\.|vt\.|vm\.)?tiktok\.com\/[^\s]+/i);
	const videoUrl = tiktokUrlMatch ? tiktokUrlMatch[0] : '';

	return {
		phone,
		shippingAddress,
		isDecline,
		isNotFitAfterReceive,
		isWillPost,
		videoUrl,
	};
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
		await deliverMessageToKoc(
			koc,
			`[Hệ thống tự động - Sau 4 ngày] Đã gửi Email nhắc lần 2 (${koc.email || 'chưa có email'}).`,
		);
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

/**
 * D3 - D6: Giai đoạn 3 · Tra cứu vận đơn J&T Express thật và cảnh báo > 5 ngày chưa giao
 */
export async function syncKocShipment(koc) {
	if (!koc.trackingCode) return { koc, tracking: null };

	const now = new Date();
	const tracking = await queryJtTracking(koc.trackingCode);

	koc.lastTrackedAt = now;
	koc.trackingStatusText = tracking.statusText;

	const shippedTime = koc.shippedAt ? new Date(koc.shippedAt).getTime() : now.getTime();
	const daysInTransit = (now.getTime() - shippedTime) / (1000 * 60 * 60 * 24);

	if (tracking.delivered) {
		// D4 = Đã giao -> D6: DELIVERED & thông báo KOC đã nhận hàng
		const wasDelivered = koc.stage === 'DELIVERED';
		koc.stage = 'DELIVERED';
		koc.deliveredAt = koc.deliveredAt || now;
		koc.shippingDelayedAlert = false;
		koc.collabStep = Math.max(koc.collabStep || 0, 3);
		if (!wasDelivered) {
			await deliverMessageToKoc(
				koc,
				`[J&T Express #${koc.trackingCode}] Đơn hàng mẫu đã giao thành công! Bạn kiểm tra sản phẩm và chuẩn bị lên video theo brief giúp bên mình nhé.`,
			);
		}
	} else if (daysInTransit > 5) {
		// D4 = > 5 ngày chưa giao -> D5: Cảnh báo Booker kiểm tra đơn J&T
		koc.shippingDelayedAlert = true;
	}

	await koc.save();
	return { koc, tracking };
}

/**
 * E1 - E7: Giai đoạn 4 · Nhắc lên video tự động (5 ngày -> 8 ngày -> 12 ngày -> Quá 3 lần im lặng gắn cờ đỏ STALLED)
 */
export async function advanceVideoReminderStep(koc, targetStep) {
	const now = new Date();
	const product = koc.productName || 'sản phẩm mẫu';

	if (targetStep === 1) {
		// E1 -> E3: Nhắc lần 1 sau 5 ngày kể từ khi DELIVERED
		await deliverMessageToKoc(
			koc,
			`[Nhắc video lần 1 - Sau 5 ngày nhận hàng] Chào ${koc.displayName}, bạn đã trải nghiệm ${product} chưa ạ? Bạn dự kiến ngày nào lên video để bên mình chuẩn bị đẩy traffic nhé!`,
		);
		koc.videoReminderStep = 1;
		koc.lastContactedAt = now;
	} else if (targetStep === 2) {
		// E3: Nhắc lần 2 sau 8 ngày
		await deliverMessageToKoc(
			koc,
			`[Nhắc video lần 2 - Sau 8 ngày nhận hàng] Chào ${koc.displayName}, bên mình nhắc nhẹ lịch đăng video ${product}. Nếu gặp khó khăn gì về sản phẩm bạn báo lại bên mình hỗ trợ ngay nhé!`,
		);
		koc.videoReminderStep = 2;
		koc.lastContactedAt = now;
	} else if (targetStep === 3) {
		// E3: Nhắc lần 3 sau 12 ngày
		await deliverMessageToKoc(
			koc,
			`[Nhắc video lần 3 - Sau 12 ngày nhận hàng] Chào ${koc.displayName}, đơn mẫu ${product} đã giao được 12 ngày. Bạn phản hồi giúp bên mình lịch đăng video hoặc hoàn sản phẩm nếu không phù hợp nhé.`,
		);
		koc.videoReminderStep = 3;
		koc.lastContactedAt = now;
	} else if (targetStep >= 4) {
		// E4 = Im lặng quá 3 lần nhắc -> E7: Đánh dấu STALLED + Cờ đỏ trên dashboard
		await deliverMessageToKoc(
			koc,
			`[CẢNH BÁO CỜ ĐỎ · STALLED] KOC quá 3 lần nhắc lịch video (5N / 8N / 12N) không phản hồi. Đã gắn cờ đỏ trên hệ thống.`,
		);
		koc.stage = 'STALLED';
		koc.stalledFlag = true;
		koc.kocVideoReply = 'SILENT';
	}

	await koc.save();
	return koc;
}

/**
 * E2: Quét kênh TikTok thật của KOC để lấy Follower, danh sách Video và tự động phát hiện video mới đăng
 */
export async function syncKocTikTokChannel(koc, { autoDetectCollabVideo = false } = {}) {
	const channelData = await fetchRealTikTokChannel(koc.tiktokId);
	const now = new Date();

	if (channelData.found && channelData.profile) {
		if (channelData.profile.followers > 0) koc.followers = channelData.profile.followers;
		if (channelData.profile.avatarUrl) koc.avatarUrl = channelData.profile.avatarUrl;
		if (channelData.profile.displayName && (!koc.displayName || koc.displayName.startsWith('KOC @'))) {
			koc.displayName = channelData.profile.displayName;
		}
		if (!koc.phone && channelData.profile.phone) koc.phone = channelData.profile.phone;
		if (!koc.email && channelData.profile.email) koc.email = channelData.profile.email;
	}

	if (channelData.videos?.length > 0) {
		koc.videos = channelData.videos;
	}
	koc.channelSyncedAt = now;

	// E2: Nếu đang ở DELIVERED và bật chế độ tự phát hiện video lên bài
	if (autoDetectCollabVideo && koc.videos?.length > 0 && !koc.videoUrl) {
		const latestVideo = koc.videos[0];
		koc.videoUrl = latestVideo.url;
		koc.videoPostedAt = now;
		const numericViews = parseFloat(String(latestVideo.views).replace(/k/i, '000').replace(/m/i, '000000')) || 0;
		if (numericViews > 0) koc.videoViews = Math.round(numericViews);
		koc.stage = 'VIDEO_POSTED';
		koc.collabStep = Math.max(koc.collabStep || 0, 4);
		koc.stalledFlag = false;
	}

	await koc.save();
	return { koc, channelData };
}

export async function runStage2Sweep({ simulateNextStep = false } = {}) {
	const now = new Date();
	const summary = {
		reopenedDormant: 0,
		zaloReminded: 0,
		emailReminded: 0,
		callTasksCreated: 0,
		shipmentsChecked: 0,
		shipmentsDelivered: 0,
		shippingDelayedAlerts: 0,
		videoReminded: 0,
		stalledFlagged: 0,
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

	// C2 -> C5: Quét các KOC đang CONTACTED hoặc NO_REPLY (Giai đoạn 2)
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

	// D3 -> D6: Tự động kiểm tra vận đơn J&T Express cho các KOC đang SHIPPING (Giai đoạn 3 - 4h/lần)
	const shippingKocs = await Koc.find({
		status: 'active',
		stage: 'SHIPPING',
		trackingCode: { $ne: '' },
	});
	for (const koc of shippingKocs) {
		summary.shipmentsChecked += 1;
		const { tracking } = await syncKocShipment(koc);
		if (tracking?.delivered) summary.shipmentsDelivered += 1;
		if (koc.shippingDelayedAlert) summary.shippingDelayedAlerts += 1;
	}

	// E1 -> E7: Tự động kiểm tra các KOC đang DELIVERED (Giai đoạn 4 - Nhắc video 5N / 8N / 12N / STALLED)
	const deliveredKocs = await Koc.find({
		status: 'active',
		stage: 'DELIVERED',
	});
	for (const koc of deliveredKocs) {
		const deliveredTime = koc.deliveredAt ? new Date(koc.deliveredAt).getTime() : now.getTime();
		const daysSinceDelivered = (now.getTime() - deliveredTime) / (1000 * 60 * 60 * 24);

		if (simulateNextStep) {
			const nextStep = (koc.videoReminderStep || 0) + 1;
			await advanceVideoReminderStep(koc, nextStep);
			if (nextStep >= 4) summary.stalledFlagged += 1;
			else summary.videoReminded += 1;
			continue;
		}

		if (daysSinceDelivered >= 15 && koc.videoReminderStep >= 3 && !koc.videoUrl) {
			await advanceVideoReminderStep(koc, 4);
			summary.stalledFlagged += 1;
		} else if (daysSinceDelivered >= 12 && koc.videoReminderStep < 3) {
			await advanceVideoReminderStep(koc, 3);
			summary.videoReminded += 1;
		} else if (daysSinceDelivered >= 8 && koc.videoReminderStep < 2) {
			await advanceVideoReminderStep(koc, 2);
			summary.videoReminded += 1;
		} else if (daysSinceDelivered >= 5 && koc.videoReminderStep < 1) {
			await advanceVideoReminderStep(koc, 1);
			summary.videoReminded += 1;
		}
	}

	return summary;
}

export { fetchRealTikTokVideoMetrics };

