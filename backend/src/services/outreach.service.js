import Conversation from '../models/conversation.model.js';
import Koc from '../models/koc.model.js';
import Message from '../models/message.model.js';
import { sendKocEmail } from './email.service.js';
import { queryJtTracking } from './jt.service.js';
import { fetchRealTikTokChannel, fetchRealTikTokVideoMetrics } from './tiktok.service.js';
import { resolveZaloUserByPhone } from './zalo-personal.service.js';
import { sendZaloTextMessage } from './zalo.service.js';

// Bộ đếm chống spam / chống khóa nick theo ngày
let dailySentTracker = {
	dateKey: new Date().toISOString().slice(0, 10),
	count: 0,
	maxPerDay: 80,
};

function resetDailyTrackerIfNeeded() {
	const today = new Date().toISOString().slice(0, 10);
	if (dailySentTracker.dateKey !== today) {
		dailySentTracker = { dateKey: today, count: 0, maxPerDay: 80 };
	}
}

/**
 * Kiểm tra khung giờ gửi tin chủ động: Chỉ gửi từ 08:30 đến 20:30 theo giờ Việt Nam (UTC+7)
 */
export function isWithinAllowedSendWindow(date = new Date()) {
	const vnTimeStr = date.toLocaleTimeString('en-GB', {
		timeZone: 'Asia/Ho_Chi_Minh',
		hour12: false,
		hour: '2-digit',
		minute: '2-digit',
	});
	const [h, m] = vnTimeStr.split(':').map(Number);
	const totalMinutes = h * 60 + m;
	return totalMinutes >= 8 * 60 + 30 && totalMinutes <= 20 * 60 + 30;
}

/**
 * Kiểm tra điều kiện chống spam cho 1 KOC:
 * - Không gửi nếu KOC có cờ dnc = true hoặc daHopTac = true
 * - Một KOC không nhận quá 1 tin chủ động mỗi 24 giờ
 * - Không vượt hạn mức mỗi ngày của kênh
 */
export function canSendProactiveMessage(koc, { bypassWindow = false } = {}) {
	if (!koc || koc.dnc || koc.daHopTac) return { allowed: false, reason: 'dnc_or_blocked' };
	resetDailyTrackerIfNeeded();
	if (dailySentTracker.count >= dailySentTracker.maxPerDay) {
		return { allowed: false, reason: 'daily_quota_reached' };
	}
	if (!bypassWindow && !isWithinAllowedSendWindow()) {
		return { allowed: false, reason: 'outside_0830_2030_window' };
	}
	if (!bypassWindow && koc.lastContactedAt) {
		const hoursSinceLast = (Date.now() - new Date(koc.lastContactedAt).getTime()) / (1000 * 60 * 60);
		if (hoursSinceLast < 24) {
			return { allowed: false, reason: 'already_sent_within_24h' };
		}
	}
	return { allowed: true };
}

/**
 * Mục #2 + Bảng Chống Spam & Bảng Cấm Kỵ:
 * - Template có nhiều biến thể câu để tránh gửi nguyên văn hàng loạt
 * - TUYỆT ĐỐI KHÔNG viết "tặng sản phẩm" (chính sách là gửi sản phẩm mẫu để trải nghiệm, không hợp thì gửi lại)
 * - TUYỆT ĐỐI KHÔNG khen kiểu "dùng thật" (nhiều KOC làm video dựng bối cảnh)
 */
export function buildInviteMessage(koc, customBrief = '') {
	const product = koc.productName || 'sản phẩm mẫu của nhãn hàng';
	const campaign = koc.campaign ? `[${koc.campaign}] ` : '';
	const offer =
		koc.castFee > 0
			? `phí cast đã duyệt ${new Intl.NumberFormat('vi-VN').format(koc.castFee)}đ kèm hoa hồng Affiliate ${koc.commissionRate || 10}%`
			: `gửi sản phẩm mẫu để trải nghiệm (không hợp bạn có thể gửi hoàn lại) kèm hoa hồng Affiliate ${koc.commissionRate || 10}%`;

	const briefPart = customBrief?.trim()
		? `\nBrief: ${customBrief.trim()}`
		: `\nBrief: Quay 1 video giới thiệu hoặc dựng bối cảnh phù hợp với phong cách kênh của bạn và gắn giỏ hàng ${product}.`;

	const variants = [
		`Chào ${koc.displayName}, bên mình có theo dõi kênh @${koc.tiktokId} và thấy định hướng nội dung của bạn rất hợp với chiến dịch ${campaign}cho dòng ${product}. Chính sách đợt này là ${offer}.${briefPart}\nBạn xem qua và phản hồi giúp bên mình nhé!`,
		`Xin chào ${koc.displayName}, nhãn hàng bên mình đang triển khai đợt hợp tác ${campaign}với sản phẩm ${product}. Bên mình đề xuất ${offer}.${briefPart}\nNếu phù hợp với lịch của kênh, bạn nhắn lại để bên mình gửi sản phẩm mẫu qua nhé!`,
		`Chào bạn ${koc.displayName}, mình liên hệ từ đội ngũ Booking chiến dịch ${campaign}${product}. Bên mình mong muốn hợp tác cùng kênh @${koc.tiktokId} với chính sách ${offer}.${briefPart}\nBạn phản hồi lại giúp mình để bên mình chuẩn bị mẫu gửi bạn trải nghiệm nhé!`,
	];

	const idx = Math.abs((koc.tiktokId || '').length + new Date().getDate()) % variants.length;
	return variants[idx];
}

export async function deliverMessageToKoc(koc, text, { isProactive = true } = {}) {
	if (koc?.dnc) {
		return { conversation: null, message: null, deliveryStatus: 'blocked_dnc' };
	}

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

	if (isProactive) {
		resetDailyTrackerIfNeeded();
		dailySentTracker.count += 1;
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
 * D1 + Bảng Quy Tắc Chatbot:
 * Phân loại ý định (đồng ý, từ chối + đề xuất mã lý do, hỏi chính sách, đòi tăng giá cast/hoa hồng, xác nhận đã nhận hàng tự vận chuyển, gửi link video, báo không hợp)
 */
export function parseKocInboundText(text = '') {
	const raw = String(text || '').trim();
	const lower = raw.toLowerCase();

	const phoneMatch = raw.match(/(?:\+84|0)[35789]\d{8}/);
	const phone = phoneMatch ? phoneMatch[0].replace(/^\+84/, '0') : '';

	const hasAddressKeyword =
		/(địa chỉ|đ\/c|dc:|số nhà|ngõ|hẻm|phường|quận|huyện|tỉnh|thành phố|tp\.|tx\.|thị xã|xã|ấp|thôn|tòa|chung cư|ký túc xá|ktx|đường|phố|cầu giấy|đống đa|thanh xuân|hà nội|hcm|sài gòn|đà nẵng)/i.test(
			raw,
		);
	const shippingAddress = hasAddressKeyword || (phone && raw.length > 22) ? raw : '';

	const isDncRequest = /(đừng nhắn nữa|ngừng làm phiền|stop|hủy nhận tin|dnc)/i.test(lower);
	const isNegotiateCast =
		/(tăng cast|thêm phí cast|tăng hoa hồng|giá cast của mình là|bên mình nhận cast|có phí cast mới nhận|hoa hồng thấp quá)/i.test(
			lower,
		);
	const isDecline =
		/(không nhận|từ chối|chưa phù hợp|không phù hợp|bận lịch|kín lịch|không hợp tác|không làm được|thôi shop|mình xin phép không)/i.test(
			lower,
		);
	const isNotFitAfterReceive =
		/(không hợp sản phẩm|bị kích ứng|không lên video được|xin gửi trả|hoàn hàng|trả lại mẫu)/i.test(lower);
	const isConfirmedReceived = /(nhận được rồi|đã nhận hàng|mình nhận rồi|nhận mẫu rồi|rồi shop ơi)/i.test(lower);
	const isWillPost = /(sẽ đăng|đang quay|sắp lên video|mai mình lên|tuần này lên|đang dựng video)/i.test(lower);
	const isAgreeIntent = /(đồng ý|ok shop|okie|nhận nhé|mình nhận|gửi mẫu giúp|hợp tác nhé|chốt nhé|ok bạn)/i.test(lower);
	const isAskPolicy = /(hoa hồng|phí cast|chính sách|quyền lợi|brief như nào|thời gian nhận hàng|quy trình)/i.test(lower);

	const tiktokUrlMatch = raw.match(/https?:\/\/(?:www\.|vt\.|vm\.)?tiktok\.com\/[^\s]+/i);
	const videoUrl = tiktokUrlMatch ? tiktokUrlMatch[0] : '';

	// Đề xuất mã lý do từ chối để người xác nhận
	let suggestedDeclineCode = '';
	if (isDecline || isNegotiateCast) {
		if (/freecast|có phí/i.test(lower)) suggestedDeclineCode = 'R01';
		else if (/giá|ngân sách|cast/i.test(lower)) suggestedDeclineCode = 'R02';
		else if (/hoa hồng/i.test(lower)) suggestedDeclineCode = 'R03';
		else if (/bận|kín lịch/i.test(lower)) suggestedDeclineCode = 'R04';
		else if (/không hợp|định hướng/i.test(lower)) suggestedDeclineCode = 'R05';
		else suggestedDeclineCode = 'R09';
	}

	let intent = 'GENERAL_REPLY';
	if (isDncRequest) intent = 'SET_DNC';
	else if (videoUrl) intent = 'SUBMITTED_VIDEO';
	else if (shippingAddress) intent = 'PROVIDED_ADDRESS';
	else if (isConfirmedReceived) intent = 'CONFIRMED_RECEIVED';
	else if (isNotFitAfterReceive) intent = 'NOT_FIT_RETURN';
	else if (isNegotiateCast) intent = 'NEGOTIATE_CAST';
	else if (isDecline) intent = 'DECLINED';
	else if (isAgreeIntent) intent = 'AGREE_NO_ADDRESS';
	else if (isAskPolicy) intent = 'ASK_POLICY';
	else if (isWillPost) intent = 'WILL_POST';

	return {
		intent,
		phone,
		shippingAddress,
		suggestedDeclineCode,
		isDecline,
		isNegotiateCast,
		isNotFitAfterReceive,
		isConfirmedReceived,
		isWillPost,
		isAgreeIntent,
		isAskPolicy,
		videoUrl,
	};
}

/**
 * Thực thi Chatbot tự động tuân thủ nghiêm ngặt 2 bảng "Chatbot làm gì" & "Chatbot không được làm":
 * - Không trả lời nếu KOC đã đặt cờ dnc = true
 * - Không tự ý báo giá cast hay đổi hoa hồng ngoài bảng đã duyệt -> Chuyển sang người/chủ shop duyệt
 * - Không viết "tặng" sản phẩm -> Ghi rõ là gửi mẫu để trải nghiệm, không hợp thì gửi lại
 * - Đề xuất mã lý do từ chối để Booker xác nhận
 */
export async function processInboundWithChatbot({ conversation, text, autoReply = true }) {
	const parsed = parseKocInboundText(text);
	const matchOr = [];

	if (conversation?.contactHandle?.startsWith('@')) {
		matchOr.push({ tiktokId: conversation.contactHandle.slice(1).toLowerCase() });
	}
	if (conversation?.contactPhone) {
		matchOr.push({ phone: conversation.contactPhone });
	}
	if (parsed.phone) {
		matchOr.push({ phone: parsed.phone });
	}

	const koc = matchOr.length ? await Koc.findOne({ status: 'active', $or: matchOr }) : null;

	// Cấm kỵ #4: Không trả lời khi KOC đã đặt cờ dnc
	if (koc?.dnc) {
		return { parsed, koc, botMessage: null };
	}

	let botReplyText = '';

	if (koc) {
		koc.lastRepliedAt = new Date();
		koc.callTaskPending = false;
		koc.tungKhongRep = false;
		if (parsed.phone && !koc.phone) koc.phone = parsed.phone;

		if (parsed.intent === 'SET_DNC') {
			koc.dnc = true;
			await koc.save();
			return { parsed, koc, botMessage: null };
		}

		if (parsed.intent === 'PROVIDED_ADDRESS') {
			// Mục #7: Thu địa chỉ và SĐT nhận hàng, ghi thẳng vào bảng giao hàng
			koc.shippingAddress = parsed.shippingAddress;
			if (!koc.recipient) koc.recipient = koc.displayName;
			koc.stage = 'AGREED';
			koc.collabStep = Math.max(koc.collabStep || 0, 2);
			botReplyText = `[Chatbot tự động] Cảm ơn ${koc.displayName}! Hệ thống đã lưu thông tin nhận mẫu trải nghiệm ("${parsed.shippingAddress}")${parsed.phone ? ` · SĐT ${parsed.phone}` : ''}. Thời gian giao hàng dự kiến 2–4 ngày qua J&T Express. Nếu trải nghiệm không hợp bạn có thể gửi hoàn lại giúp shop nhé!`;
		} else if (parsed.intent === 'AGREE_NO_ADDRESS') {
			// Mục #7: KOC đồng ý -> Chatbot hỏi tên, SĐT, địa chỉ nhận
			koc.stage = 'REPLIED';
			koc.collabStep = Math.max(koc.collabStep || 0, 1);
			botReplyText = `[Chatbot tự động] Cảm ơn ${koc.displayName} đã đồng ý hợp tác! Bạn cho mình xin Tên người nhận + Số điện thoại + Địa chỉ nhận hàng để bên mình gửi sản phẩm mẫu qua bạn trải nghiệm nhé!`;
		} else if (parsed.intent === 'NEGOTIATE_CAST') {
			// Cấm kỵ #1: Không tự báo giá cast hay đổi hoa hồng ngoài bảng đã duyệt -> Chuyển sang người, chờ chủ shop duyệt
			koc.stage = 'AWAITING_APPROVAL';
			koc.suggestedDeclineCode = parsed.suggestedDeclineCode || 'R02';
			koc.collabStep = Math.max(koc.collabStep || 0, 0);
			botReplyText = `[Chatbot tự động] Mình đã ghi nhận đề xuất về mức phí cast / hoa hồng của ${koc.displayName} và chuyển ngay cho Booker & Chủ shop phê duyệt. Bên mình sẽ phản hồi lại bạn sớm nhất nhé!`;
		} else if (parsed.intent === 'ASK_POLICY') {
			// Chatbot trả lời câu hỏi lặp lại: hoa hồng, quy trình, thời gian nhận hàng (trong bảng đã duyệt)
			koc.stage = 'REPLIED';
			koc.collabStep = Math.max(koc.collabStep || 0, 1);
			const feeNote =
				koc.castFee > 0
					? `phí cast đã duyệt ${new Intl.NumberFormat('vi-VN').format(koc.castFee)}đ`
					: 'gửi sản phẩm mẫu để trải nghiệm (nếu không hợp bạn gửi hoàn lại giúp shop)';
			botReplyText = `[Chatbot tự động] Chào ${koc.displayName}, quy trình bên mình gồm: ${feeNote}, hoa hồng Affiliate ${koc.commissionRate || 10}% trên đơn hàng, thời gian giao mẫu từ 2–4 ngày và lên video trong 5–7 ngày sau khi nhận mẫu. Bạn gửi giúp mình SĐT + Địa chỉ nhận hàng nhé!`;
		} else if (parsed.intent === 'CONFIRMED_RECEIVED') {
			// Mục #9: Tự vận chuyển, KOC xác nhận "rồi" -> ghi delivered_at
			koc.stage = 'DELIVERED';
			koc.deliveredAt = new Date();
			koc.trackingStatusText = 'KOC đã xác nhận nhận được hàng';
			koc.collabStep = Math.max(koc.collabStep || 0, 3);
			botReplyText = `[Chatbot tự động] Cảm ơn ${koc.displayName} đã xác nhận nhận hàng! Bạn trải nghiệm sản phẩm và chuẩn bị lên video theo brief giúp bên mình nhé.`;
		} else if (parsed.intent === 'SUBMITTED_VIDEO') {
			// Mục #13: Có video -> Đổi status = VIDEO_POSTED, hủy mọi nhắc
			koc.videoUrl = parsed.videoUrl;
			koc.videoPostedAt = new Date();
			koc.stage = 'VIDEO_POSTED';
			koc.collabStep = Math.max(koc.collabStep || 0, 4);
			koc.stalledFlag = false;
			botReplyText = `[Chatbot tự động] Hệ thống đã ghi nhận video TikTok của ${koc.displayName} (${parsed.videoUrl}) và dừng chuỗi nhắc lịch. Cảm ơn bạn đã đồng hành cùng nhãn hàng!`;
		} else if (parsed.intent === 'NOT_FIT_RETURN') {
			// Mục #15: KOC báo "không hợp" -> status = NOT_FIT; Chatbot hướng dẫn gửi lại SP
			koc.stage = 'NOT_FIT';
			koc.kocVideoReply = 'NOT_FIT';
			koc.postReceiveCode = koc.postReceiveCode || 'P01';
			botReplyText = `[Chatbot tự động] Bên mình đã ghi nhận sản phẩm chưa phù hợp với bạn (Mã P01). Theo chính sách gửi mẫu trải nghiệm, bạn vui lòng đóng gói và gửi hoàn lại sản phẩm về kho giúp shop trong vòng 7 ngày và nhắn lại mã vận đơn hoàn vào đây nhé!`;
		} else if (parsed.intent === 'DECLINED') {
			// Đề xuất mã lý do từ chối để người xác nhận
			koc.stage = 'REPLIED';
			koc.suggestedDeclineCode = parsed.suggestedDeclineCode || 'R04';
			botReplyText = `[Chatbot tự động] Cảm ơn ${koc.displayName} đã phản hồi. Mình đã ghi nhận thông tin (Đề xuất mã: ${koc.suggestedDeclineCode}) để Booker cập nhật hồ sơ. Hẹn hợp tác cùng bạn ở đợt tiếp theo nhé!`;
		} else if (parsed.intent === 'WILL_POST') {
			koc.kocVideoReply = 'WILL_POST';
			botReplyText = `[Chatbot tự động] Cảm ơn ${koc.displayName}! Khi nào đăng video xong bạn gửi link vào đây để hệ thống tự động ghi nhận nhé.`;
		} else if (['CONTACTED', 'NO_REPLY', 'DORMANT'].includes(koc.stage)) {
			koc.stage = 'REPLIED';
			koc.collabStep = Math.max(koc.collabStep || 0, 1);
		}

		await koc.save();
	}

	let botMessage = null;
	if (autoReply && botReplyText && conversation) {
		botMessage = await Message.create({
			conversationId: conversation._id,
			direction: 'outbound',
			text: botReplyText,
			status: 'Chatbot đã trả lời',
		});
		conversation.lastMessage = botReplyText;
		conversation.lastMessageAt = botMessage.sentAt;
		await conversation.save();
	}

	return { parsed, koc, botMessage };
}

export async function sendFirstInvite(koc, customBrief = '', { bypassWindow = true } = {}) {
	const check = canSendProactiveMessage(koc, { bypassWindow });
	if (!check.allowed) {
		throw new Error(`Chặn gửi tin chủ động (${check.reason})`);
	}

	const text = buildInviteMessage(koc, customBrief || koc.inviteBrief);
	await deliverMessageToKoc(koc, text, { isProactive: true });

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

/**
 * Mục #3, #4, #5, #6:
 * - #3: 48 giờ sau tin mời (+2 ngày), chưa rep -> Nhắc lần 2 (đổi sang Email nếu có email, không có thì Zalo)
 * - #4: 4 ngày sau tin mời (+4 ngày), chưa rep -> Nhắc lần 3 qua Zalo, tin ngắn, có lối thoát "không phù hợp cũng báo giúp"
 * - #5: 7 ngày sau tin mời (+7 ngày), chưa rep -> Tạo task gọi điện vào danh sách việc của Booker
 * - #6: Hết chuỗi nhắc -> status = DORMANT, đặt recontact_after (+30 ngày mở lại) & gắn nhãn tungKhongRep = true
 */
export async function advanceFollowUpStep(koc, targetStep) {
	if (koc.dnc) return koc;
	const now = new Date();
	const product = koc.productName || 'sản phẩm mẫu';

	if (targetStep === 1) {
		// Mục #3: 48 giờ (+2 ngày) -> Nhắc lần 2 (đổi sang Email nếu có email, hoặc Zalo)
		const msgText = `Chào ${koc.displayName}, cách đây 2 ngày bên mình có gửi lời mời hợp tác trải nghiệm sản phẩm ${product}. Bạn kiểm tra thông tin và phản hồi giúp nhãn hàng nhé!`;
		if (koc.email) {
			try {
				await sendKocEmail({
					to: koc.email,
					displayName: koc.displayName,
					tiktokId: koc.tiktokId,
					text: msgText,
				});
			} catch {
				// Ignore SMTP error in local dev
			}
			await deliverMessageToKoc(
				koc,
				`[Nhắc lần 2 · +2 ngày qua Gmail (${koc.email})] ${msgText}`,
				{ isProactive: true },
			);
		} else {
			await deliverMessageToKoc(koc, `[Nhắc lần 2 · +2 ngày qua Zalo] ${msgText}`, { isProactive: true });
		}
		koc.stage = 'NO_REPLY';
		koc.reminderStep = 1;
		koc.lastContactedAt = now;
	} else if (targetStep === 2) {
		// Mục #4: 4 ngày (+4 ngày) -> Nhắc lần 3 qua Zalo, tin ngắn, có lối thoát "không phù hợp cũng báo giúp"
		const shortExitText = `Chào ${koc.displayName}, bên mình nhắn ngắn hỏi thăm về lời mời trải nghiệm ${product} hôm trước ạ. Nếu đợt này lịch kênh chưa phù hợp hoặc sản phẩm không phù hợp cũng báo giúp bên mình một tiếng để bên mình ghi nhận nhé!`;
		await deliverMessageToKoc(koc, shortExitText, { isProactive: true });
		koc.stage = 'NO_REPLY';
		koc.reminderStep = 2;
		koc.lastContactedAt = now;
	} else if (targetStep === 3) {
		// Mục #5: 7 ngày (+7 ngày) -> Tạo task gọi điện vào danh sách việc của Booker
		await deliverMessageToKoc(
			koc,
			`[Task gọi điện Booker · +7 ngày] KOC chưa phản hồi sau 3 lần nhắn. Booker cần gọi điện trực tiếp tới SĐT ${koc.phone || 'chưa cập nhật'}.`,
			{ isProactive: false },
		);
		koc.stage = 'NO_REPLY';
		koc.reminderStep = 3;
		koc.callTaskPending = true;
		koc.tungKhongRep = true;
	} else if (targetStep >= 4) {
		// Mục #6: Hết chuỗi nhắc -> status = DORMANT, đặt dormantUntil (+30 ngày mở lại)
		koc.stage = 'DORMANT';
		koc.callTaskPending = false;
		koc.tungKhongRep = true;
		koc.dormantUntil = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000);
	}

	await koc.save();
	return koc;
}

/**
 * Mục #8 & #10:
 * - #8: Có mã vận đơn -> Kiểm tra trạng thái giao định kỳ 4 giờ một lần (J&T); cập nhật delivery_status
 * - #10: Quá 5 ngày chưa giao -> Cảnh báo Booker và Chủ shop
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
		const wasDelivered = koc.stage === 'DELIVERED';
		koc.stage = 'DELIVERED';
		koc.deliveredAt = koc.deliveredAt || now;
		koc.shippingDelayedAlert = false;
		koc.collabStep = Math.max(koc.collabStep || 0, 3);
		if (!wasDelivered) {
			await deliverMessageToKoc(
				koc,
				`[J&T Express #${koc.trackingCode}] Đơn hàng mẫu đã giao thành công! Bạn kiểm tra sản phẩm và chuẩn bị lên video theo brief giúp bên mình nhé.`,
				{ isProactive: true },
			);
		}
	} else if (daysInTransit > 5) {
		// Mục #10: Quá 5 ngày chưa giao -> Cảnh báo Booker và Chủ shop
		koc.shippingDelayedAlert = true;
	}

	await koc.save();
	return { koc, tracking };
}

/**
 * Mục #11, #12, #14:
 * - #11: delivered_at + 5 ngày, chưa có video -> Nhắc lên video lần 1, gợi ý hướng nội dung phù hợp
 * - #12: +8 và +12 ngày, chưa có video -> Nhắc lần 2 và 3, mỗi lần một nội dung khác
 * - #14: Hết 3 lần nhắc video -> status = STALLED, đưa lên dashboard; không nhắc thêm tự động
 */
export async function advanceVideoReminderStep(koc, targetStep) {
	if (koc.dnc || koc.stage === 'VIDEO_POSTED' || koc.stage === 'COMPLETED') return koc;
	const now = new Date();
	const product = koc.productName || 'sản phẩm mẫu';

	if (targetStep === 1) {
		// Mục #11: +5 ngày -> Nhắc lên video lần 1, gợi ý hướng nội dung phù hợp
		await deliverMessageToKoc(
			koc,
			`[Nhắc video lần 1 · +5 ngày] Chào ${koc.displayName}, bạn nhận được ${product} được 5 ngày rồi ạ. Gợi ý nhỏ: Với tệp kênh @${koc.tiktokId}, bạn có thể dựng bối cảnh phối đồ / unbox nhanh 15–30s gắn giỏ hàng sẽ rất dễ cắn đề xuất. Bạn dự kiến hôm nào lên bài để bên mình chuẩn bị đẩy traffic nhé!`,
			{ isProactive: true },
		);
		koc.videoReminderStep = 1;
		koc.lastContactedAt = now;
	} else if (targetStep === 2) {
		// Mục #12: +8 ngày -> Nhắc lần 2 (nội dung khác)
		await deliverMessageToKoc(
			koc,
			`[Nhắc video lần 2 · +8 ngày] Chào ${koc.displayName}, bên mình hỏi thăm tiến độ quay dựng video cho ${product} ạ. Nếu bạn cần thêm thông tin USP sản phẩm hoặc gặp vấn đề gì khi trải nghiệm thì nhắn lại bên mình hỗ trợ ngay nhé!`,
			{ isProactive: true },
		);
		koc.videoReminderStep = 2;
		koc.lastContactedAt = now;
	} else if (targetStep === 3) {
		// Mục #12: +12 ngày -> Nhắc lần 3 (nội dung khác)
		await deliverMessageToKoc(
			koc,
			`[Nhắc video lần 3 · +12 ngày] Chào ${koc.displayName}, đơn mẫu ${product} đã giao được 12 ngày nhưng hệ thống chưa thấy video trên kênh. Bạn phản hồi giúp bên mình lịch đăng bài cụ thể, hoặc nếu sản phẩm không hợp thì báo lại để bên mình hướng dẫn gửi hoàn mẫu nhé!`,
			{ isProactive: true },
		);
		koc.videoReminderStep = 3;
		koc.lastContactedAt = now;
	} else if (targetStep >= 4) {
		// Mục #14: Hết 3 lần nhắc video -> status = STALLED, đưa lên dashboard; không nhắc thêm tự động
		koc.stage = 'STALLED';
		koc.stalledFlag = true;
		koc.kocVideoReply = 'SILENT';
	}

	await koc.save();
	return koc;
}

/**
 * Mục #13: Mỗi ngày quét TikTok theo ID kênh và SP; khớp thì ghi video, đổi status = VIDEO_POSTED, hủy mọi nhắc
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

	// Mục #13: Kiểm tra video trên kênh có khớp sản phẩm hoặc mới đăng sau khi nhận mẫu
	if (autoDetectCollabVideo && koc.videos?.length > 0 && !koc.videoUrl) {
		const productKeywords = (koc.productName || '')
			.toLowerCase()
			.split(/\s+/)
			.filter((w) => w.length >= 3);

		const matchedVideo =
			koc.videos.find((v) => {
				const titleLower = (v.title || '').toLowerCase();
				return productKeywords.some((kw) => titleLower.includes(kw));
			}) || koc.videos[0];

		if (matchedVideo) {
			koc.videoUrl = matchedVideo.url;
			koc.videoPostedAt = now;
			const numericViews = parseFloat(String(matchedVideo.views).replace(/k/i, '000').replace(/m/i, '000000')) || 0;
			if (numericViews > 0) koc.videoViews = Math.round(numericViews);
			koc.stage = 'VIDEO_POSTED';
			koc.collabStep = Math.max(koc.collabStep || 0, 4);
			koc.stalledFlag = false;
			koc.callTaskPending = false;
		}
	}

	await koc.save();
	return { koc, channelData };
}

/**
 * Mục #16: 8:00 mỗi sáng -> Bản tóm tắt: việc cần làm hôm nay, KOC sắp quá hạn, STALLED mới
 */
export async function buildMorningDigest() {
	const activeKocs = await Koc.find({ status: 'active' }).lean();
	const needCall = activeKocs.filter((k) => k.callTaskPending);
	const needShip = activeKocs.filter((k) => k.stage === 'AGREED');
	const delayedShipments = activeKocs.filter((k) => k.shippingDelayedAlert);
	const stalledKocs = activeKocs.filter((k) => k.stage === 'STALLED' || k.stalledFlag);
	const pendingApproval = activeKocs.filter((k) => k.stage === 'AWAITING_APPROVAL');
	const pendingReturn = activeKocs.filter((k) => ['NOT_FIT', 'RETURNING'].includes(k.stage));

	const text = [
		`[BẢN TÓM TẮT 08:00 SÁNG · VIỆC CẦN LÀM HÔM NAY]`,
		`• Cần duyệt phí cast / đàm phán: ${pendingApproval.length} KOC`,
		`• Cần gọi điện (quá 7 ngày chưa rep): ${needCall.length} KOC`,
		`• Chờ gửi mẫu (đã có địa chỉ): ${needShip.length} KOC`,
		`• Đơn J&T quá 5 ngày chưa giao: ${delayedShipments.length} đơn`,
		`• Cờ đỏ STALLED (quá 3 lần nhắc video): ${stalledKocs.length} KOC`,
		`• Đang chờ thu hồi hoàn mẫu (NOT_FIT): ${pendingReturn.length} KOC`,
	].join('\n');

	return {
		text,
		stats: {
			pendingApproval: pendingApproval.length,
			needCall: needCall.length,
			needShip: needShip.length,
			delayedShipments: delayedShipments.length,
			stalledKocs: stalledKocs.length,
			pendingReturn: pendingReturn.length,
		},
	};
}

export async function runStage2Sweep({ simulateNextStep = false } = {}) {
	const now = new Date();
	const summary = {
		reopenedDormant: 0,
		zaloReminded: 0,
		emailReminded: 0,
		callTasksCreated: 0,
		selfShipAsked: 0,
		shipmentsChecked: 0,
		shipmentsDelivered: 0,
		shippingDelayedAlerts: 0,
		tiktokDailyScanned: 0,
		videoReminded: 0,
		stalledFlagged: 0,
		returnReminded: 0,
	};

	// Mục #6: Tự động mở lại các KOC DORMANT đã đủ 30 ngày
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

	// Mục #3, #4, #5: Quét các KOC đang CONTACTED hoặc NO_REPLY
	const activeOutreach = await Koc.find({
		status: 'active',
		daHopTac: false,
		dnc: false,
		stage: { $in: ['CONTACTED', 'NO_REPLY'] },
	});

	for (const koc of activeOutreach) {
		const invitedTime = koc.invitedAt ? new Date(koc.invitedAt).getTime() : now.getTime();
		const hoursElapsed = (now.getTime() - invitedTime) / (1000 * 60 * 60);

		if (simulateNextStep) {
			if (koc.reminderStep === 0) {
				await advanceFollowUpStep(koc, 1);
				summary.emailReminded += 1;
			} else if (koc.reminderStep === 1) {
				await advanceFollowUpStep(koc, 2);
				summary.zaloReminded += 1;
			} else if (koc.reminderStep === 2) {
				await advanceFollowUpStep(koc, 3);
				summary.callTasksCreated += 1;
			}
			continue;
		}

		if (!canSendProactiveMessage(koc).allowed) continue;

		if (hoursElapsed >= 24 * 7 && koc.reminderStep < 3) {
			await advanceFollowUpStep(koc, 3);
			summary.callTasksCreated += 1;
		} else if (hoursElapsed >= 24 * 4 && koc.reminderStep < 2) {
			await advanceFollowUpStep(koc, 2);
			summary.zaloReminded += 1;
		} else if (hoursElapsed >= 48 && koc.reminderStep < 1) {
			await advanceFollowUpStep(koc, 1);
			summary.emailReminded += 1;
		}
	}

	// Mục #8, #9, #10: Quét các KOC đang SHIPPING
	const shippingKocs = await Koc.find({
		status: 'active',
		stage: 'SHIPPING',
	});
	for (const koc of shippingKocs) {
		const shippedTime = koc.shippedAt ? new Date(koc.shippedAt).getTime() : now.getTime();
		const daysSinceShipped = (now.getTime() - shippedTime) / (1000 * 60 * 60 * 24);

		if (koc.shippingCarrier === 'Tự vận chuyển' || !koc.trackingCode) {
			// Mục #9: Tự vận chuyển, 3 ngày sau ngày gửi -> Hỏi KOC đã nhận hàng chưa
			if ((daysSinceShipped >= 3 || simulateNextStep) && !koc.selfShipAskedAt) {
				await deliverMessageToKoc(
					koc,
					`Chào ${koc.displayName}, sản phẩm mẫu bên mình tự vận chuyển gửi đi được 3 ngày rồi ạ. Bạn đã nhận được hàng chưa? Nếu nhận được rồi bạn nhắn chữ "rồi" giúp mình nhé!`,
					{ isProactive: true },
				);
				koc.selfShipAskedAt = now;
				await koc.save();
				summary.selfShipAsked += 1;
			}
			if (daysSinceShipped > 5) {
				koc.shippingDelayedAlert = true;
				await koc.save();
				summary.shippingDelayedAlerts += 1;
			}
		} else {
			// Mục #8: Có mã vận đơn J&T -> Kiểm tra 4 giờ 1 lần
			summary.shipmentsChecked += 1;
			const { tracking } = await syncKocShipment(koc);
			if (tracking?.delivered) summary.shipmentsDelivered += 1;
			if (koc.shippingDelayedAlert) summary.shippingDelayedAlerts += 1;
		}
	}

	// Mục #11, #12, #13, #14: Quét các KOC đang DELIVERED
	const deliveredKocs = await Koc.find({
		status: 'active',
		dnc: false,
		stage: 'DELIVERED',
	});
	for (const koc of deliveredKocs) {
		// Mục #13: Mỗi ngày quét TikTok theo ID kênh và SP; khớp thì đổi status = VIDEO_POSTED, hủy mọi nhắc
		try {
			await syncKocTikTokChannel(koc, { autoDetectCollabVideo: true });
			summary.tiktokDailyScanned += 1;
		} catch {
			// Ignore scan error
		}
		if (koc.stage === 'VIDEO_POSTED') continue;

		const deliveredTime = koc.deliveredAt ? new Date(koc.deliveredAt).getTime() : now.getTime();
		const daysSinceDelivered = (now.getTime() - deliveredTime) / (1000 * 60 * 60 * 24);

		if (simulateNextStep) {
			const nextStep = (koc.videoReminderStep || 0) + 1;
			await advanceVideoReminderStep(koc, nextStep);
			if (nextStep >= 4) summary.stalledFlagged += 1;
			else summary.videoReminded += 1;
			continue;
		}

		if (daysSinceDelivered > 12 && koc.videoReminderStep >= 3 && !koc.videoUrl) {
			// Mục #14: Hết 3 lần nhắc video (Sau +12 ngày) -> STALLED
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

	// Mục #15: KOC báo "không hợp" (NOT_FIT) quá +7 ngày chưa gửi lại -> Nhắc hoàn hàng
	const notFitKocs = await Koc.find({
		status: 'active',
		dnc: false,
		stage: 'NOT_FIT',
		returnTrackingCode: '',
	});
	for (const koc of notFitKocs) {
		const updatedTime = koc.updatedAt ? new Date(koc.updatedAt).getTime() : now.getTime();
		const daysSinceNotFit = (now.getTime() - updatedTime) / (1000 * 60 * 60 * 24);
		if ((daysSinceNotFit >= 7 || simulateNextStep) && !koc.returnReminderSentAt) {
			await deliverMessageToKoc(
				koc,
				`[Nhắc gửi hoàn sản phẩm · +7 ngày] Chào ${koc.displayName}, sản phẩm mẫu không phù hợp bạn đã gửi lại giúp shop chưa ạ? Bạn cho mình xin mã vận đơn gửi hoàn nhé!`,
				{ isProactive: true },
			);
			koc.returnReminderSentAt = now;
			await koc.save();
			summary.returnReminded += 1;
		}
	}

	return summary;
}

export { fetchRealTikTokVideoMetrics };
