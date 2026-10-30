import Koc from '../models/koc.model.js';
import Conversation from '../models/conversation.model.js';
import { sendKocEmail } from '../services/email.service.js';
import {
	advanceFollowUpStep,
	advanceVideoReminderStep,
	buildMorningDigest,
	deliverMessageToKoc,
	fetchRealTikTokVideoMetrics,
	parseKocInboundText,
	runStage2Sweep,
	sendFirstInvite,
	syncKocShipment,
	syncKocTikTokChannel,
} from '../services/outreach.service.js';
import { queryJtTracking } from '../services/jt.service.js';
import { fetchRealTikTokChannel } from '../services/tiktok.service.js';
import ApiError from '../utils/ApiError.js';

function normalizeTikTokId(value = '') {
	return value.trim().replace(/^@/, '').toLowerCase();
}

function normalizePhone(value = '') {
	return value.replace(/\s+/g, '').trim();
}

function resolveCastAndStage(payload, currentStage = 'SOURCED') {
	const castFee = Number(payload.castFee) || 0;
	const castType = castFee > 0 ? 'cast' : 'freecast';
	let stage = payload.stage || currentStage;
	if (stage === 'QUALIFIED' && castFee > 0) {
		stage = 'AWAITING_APPROVAL';
	}
	return { castFee, castType, stage };
}

export async function listKocs(request, response) {
	const { q = '', category, stage, source, page = 1, limit = 50 } = request.query;
	const filter = { status: 'active' };
	if (category && category !== 'Tất cả') filter.category = category;
	if (stage && stage !== 'ALL') filter.stage = stage;
	if (source && source !== 'ALL') filter.source = source;
	if (q) {
		const cleanQ = normalizeTikTokId(q);
		filter.$or = [
			{ displayName: new RegExp(q, 'i') },
			{ tiktokId: new RegExp(cleanQ, 'i') },
			{ phone: new RegExp(q, 'i') },
			{ email: new RegExp(q, 'i') },
		];
	}
	const pageNumber = Math.max(Number(page), 1);
	const pageSize = Math.min(Math.max(Number(limit), 1), 100);
	const [items, total] = await Promise.all([
		Koc.find(filter).sort({ createdAt: -1 }).skip((pageNumber - 1) * pageSize).limit(pageSize).lean(),
		Koc.countDocuments(filter),
	]);
	response.json({
		success: true,
		data: {
			items,
			pagination: { page: pageNumber, limit: pageSize, total, pages: Math.ceil(total / pageSize) },
		},
	});
}

export async function lookupTikTok(request, response) {
	const tiktokId = normalizeTikTokId(request.body.tiktokId);
	const phone = normalizePhone(request.body.phone || '');
	const orConditions = [{ tiktokId }];
	if (phone) orConditions.push({ phone });

	const existing = await Koc.findOne({ $or: orConditions }).lean();
	if (existing) {
		if (existing.daHopTac) {
			return response.json({
				success: true,
				data: {
					source: 'database',
					blocked: true,
					message: 'Chặn liên hệ: KOC này đã từng hợp tác (da_hop_tac = true).',
					profile: existing,
				},
			});
		}
		return response.json({
			success: true,
			data: {
				source: 'database',
				duplicate: existing.status === 'active',
				message: existing.status === 'active' ? 'Kênh hoặc SĐT này đã tồn tại trong danh bạ.' : 'Kênh đã lưu trữ trước đó.',
				profile: existing,
			},
		});
	}

	// Gọi API lấy dữ liệu thực tế từ kênh TikTok
	const liveChannel = await fetchRealTikTokChannel(tiktokId);
	if (liveChannel.found) {
		return response.json({
			success: true,
			data: {
				source: 'tiktok-live',
				duplicate: false,
				blocked: false,
				message: `Đã lấy thông tin kênh thật @${liveChannel.profile.tiktokId} từ TikTok (${new Intl.NumberFormat('vi-VN').format(liveChannel.profile.followers)} follower).`,
				profile: liveChannel.profile,
				videos: liveChannel.videos || [],
			},
		});
	}

	response.status(404).json({
		success: false,
		message: 'Chưa tìm thấy kênh trên TikTok hoặc trong hệ thống. Vẫn có thể thêm thủ công vào danh bạ.',
	});
}

export async function createKoc(request, response) {
	const payload = request.validated.body;
	const tiktokId = normalizeTikTokId(payload.tiktokId);
	let phone = normalizePhone(payload.phone || '');

	const dedupeQuery = [{ tiktokId }];
	if (phone) dedupeQuery.push({ phone });
	const existing = await Koc.findOne({ $or: dedupeQuery });

	if (existing?.daHopTac) {
		throw new ApiError(409, `Chặn liên hệ: KOC @${existing.tiktokId} đã từng hợp tác (da_hop_tac = true)`);
	}
	if (existing?.status === 'active') {
		const reason = existing.tiktokId === tiktokId ? `TikTok ID @${tiktokId}` : `Số điện thoại ${phone}`;
		throw new ApiError(409, `Dedupe tự động: ${reason} đã tồn tại trong danh bạ`);
	}

	// Bổ sung dữ liệu thật từ TikTok nếu người dùng chưa bấm Tìm kênh trước đó
	let liveChannel = null;
	try {
		liveChannel = await fetchRealTikTokChannel(tiktokId);
	} catch {
		// Bỏ qua nếu mạng chậm
	}

	const liveProfile = liveChannel?.profile || {};
	if (!phone && liveProfile.phone) phone = normalizePhone(liveProfile.phone);

	const { castFee, castType, stage } = resolveCastAndStage(payload, payload.stage || 'SOURCED');
	const wasNeverReplied = Boolean(
		existing?.tungKhongRep || ['NO_REPLY', 'DORMANT'].includes(existing?.stage),
	);
	const kocData = {
		...payload,
		tiktokId,
		displayName:
			payload.displayName && !payload.displayName.startsWith('KOC @')
				? payload.displayName
				: liveProfile.displayName || payload.displayName || `KOC @${tiktokId}`,
		avatarUrl: payload.avatarUrl || liveProfile.avatarUrl || '',
		followers: Number(payload.followers) || Number(liveProfile.followers) || 0,
		email: payload.email || liveProfile.email || '',
		phone,
		castFee,
		castType,
		stage,
		tungKhongRep: wasNeverReplied,
		videos: liveChannel?.videos?.length ? liveChannel.videos : [],
		channelSyncedAt: liveChannel?.found ? new Date() : null,
		status: 'active',
	};

	const koc = existing
		? await Koc.findByIdAndUpdate(existing._id, kocData, { new: true, runValidators: true })
		: await Koc.create(kocData);

	await Conversation.updateOne(
		{ contactHandle: `@${koc.tiktokId}` },
		{
			$set: {
				channel: 'internal',
				contactName: koc.displayName,
				contactHandle: `@${koc.tiktokId}`,
				contactPhone: koc.phone,
				status: 'open',
				lastMessage: 'Đã thêm KOC vào danh bạ',
			},
		},
		{ upsert: true },
	);
	response.status(201).json({ success: true, data: koc });
}

export async function bulkImportKocs(request, response) {
	const { items = [], source = 'TikToday' } = request.validated.body;
	const results = {
		imported: [],
		skippedDuplicates: [],
		blockedCollaborated: [],
	};

	for (const raw of items) {
		const tiktokId = normalizeTikTokId(raw.tiktokId || '');
		if (!tiktokId) continue;
		const phone = normalizePhone(raw.phone || '');
		const displayName = (raw.displayName || `KOC @${tiktokId}`).trim();

		const dedupeQuery = [{ tiktokId }];
		if (phone) dedupeQuery.push({ phone });
		const existing = await Koc.findOne({ $or: dedupeQuery });

		if (existing?.daHopTac) {
			results.blockedCollaborated.push({ tiktokId, phone, reason: 'da_hop_tac = true' });
			continue;
		}
		if (existing?.status === 'active') {
			results.skippedDuplicates.push({
				tiktokId,
				phone,
				reason: existing.tiktokId === tiktokId ? 'Trùng ID kênh' : 'Trùng SĐT',
			});
			continue;
		}

		const { castFee, castType, stage } = resolveCastAndStage(raw, 'SOURCED');
		const saved = existing
			? await Koc.findByIdAndUpdate(
					existing._id,
					{ tiktokId, displayName, phone, source: raw.source || source, castFee, castType, stage, status: 'active' },
					{ new: true },
				)
			: await Koc.create({
					tiktokId,
					displayName,
					phone,
					source: raw.source || source,
					category: raw.category || 'Chưa phân loại',
					followers: Number(raw.followers) || 0,
					gmv: Number(raw.gmv) || 0,
					castFee,
					castType,
					stage,
				});

		await Conversation.updateOne(
			{ contactHandle: `@${saved.tiktokId}` },
			{
				$setOnInsert: {
					channel: 'internal',
					contactName: saved.displayName,
					contactHandle: `@${saved.tiktokId}`,
					contactPhone: saved.phone,
					status: 'open',
					lastMessage: 'Nhập hàng loạt từ ' + (raw.source || source),
				},
			},
			{ upsert: true },
		);
		results.imported.push(saved);
	}

	response.status(201).json({
		success: true,
		data: results,
		message: `Đã nhập ${results.imported.length} KOC, bỏ qua ${results.skippedDuplicates.length} trùng lặp, chặn ${results.blockedCollaborated.length} KOC đã hợp tác.`,
	});
}

export async function updateKocStage(request, response) {
	const { id } = request.params;
	const payload = request.validated.body;
	const koc = await Koc.findOne({ _id: id, status: 'active' });
	if (!koc) throw new ApiError(404, 'Không tìm thấy KOC đang hoạt động');

	if (koc.daHopTac && payload.action !== 'toggle_collaborated') {
		throw new ApiError(409, 'KOC này đang bị chặn liên hệ (da_hop_tac = true)');
	}

	switch (payload.action) {
		// === GIAI ĐOẠN 1 & BƯỚC 1-2 HỢP TÁC ===
		case 'qualify': {
			const castFee = payload.castFee !== undefined ? Number(payload.castFee) : koc.castFee;
			koc.contentPillar = payload.contentPillar ?? koc.contentPillar;
			koc.targetAudience = payload.targetAudience ?? koc.targetAudience;
			koc.campaign = payload.campaign ?? koc.campaign;
			koc.productName = payload.productName ?? koc.productName;
			if (payload.commissionRate !== undefined) koc.commissionRate = Number(payload.commissionRate);
			koc.castFee = castFee;
			koc.castType = castFee > 0 ? 'cast' : 'freecast';
			koc.stage = castFee > 0 ? 'AWAITING_APPROVAL' : 'QUALIFIED';
			koc.collabStep = Math.max(koc.collabStep || 0, 2);
			koc.rejectCode = '';
			koc.rejectNote = '';
			break;
		}
		case 'approve_cast': {
			koc.stage = 'QUALIFIED';
			if (payload.castFee !== undefined) {
				koc.castFee = Number(payload.castFee);
				koc.castType = koc.castFee > 0 ? 'cast' : 'freecast';
			}
			koc.collabStep = Math.max(koc.collabStep || 0, 1);
			break;
		}
		case 'filter_out': {
			koc.stage = 'FILTERED_OUT';
			koc.rejectCode = payload.rejectCode || 'F01';
			koc.rejectNote = payload.rejectNote || '';
			break;
		}
		case 'toggle_collaborated': {
			koc.daHopTac = Boolean(payload.daHopTac);
			break;
		}
		case 'toggle_dnc': {
			koc.dnc = Boolean(payload.dnc);
			break;
		}

		// === GIAI ĐOẠN 2 · LIÊN HỆ & BÁM ĐUỔI ===
		case 'send_invite': {
			const updated = await sendFirstInvite(koc, payload.inviteBrief);
			return response.json({ success: true, data: updated });
		}
		case 'remind_step': {
			const step = Number(payload.reminderStep) || Math.min((koc.reminderStep || 0) + 1, 3);
			const updated = await advanceFollowUpStep(koc, step);
			return response.json({ success: true, data: updated });
		}
		case 'mark_replied': {
			koc.stage = 'REPLIED';
			koc.lastRepliedAt = new Date();
			koc.callTaskPending = false;
			if (payload.inboundText) {
				const parsed = parseKocInboundText(payload.inboundText);
				if (parsed.phone && !koc.phone) koc.phone = parsed.phone;
				if (parsed.shippingAddress) {
					koc.shippingAddress = parsed.shippingAddress;
					koc.stage = 'AGREED';
					koc.collabStep = Math.max(koc.collabStep || 0, 2);
				}
			}
			break;
		}
		case 'mark_dormant': {
			const reopenDate = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000);
			koc.stage = 'DORMANT';
			koc.callTaskPending = false;
			koc.dormantUntil = reopenDate;
			break;
		}
		case 'reopen_dormant': {
			koc.stage = 'QUALIFIED';
			koc.reminderStep = 0;
			koc.callTaskPending = false;
			koc.dormantUntil = null;
			break;
		}
		case 'agree': {
			koc.stage = 'AGREED';
			koc.callTaskPending = false;
			koc.declineCode = '';
			koc.declineNote = '';
			if (payload.shippingAddress !== undefined) koc.shippingAddress = payload.shippingAddress;
			if (payload.recipient !== undefined) koc.recipient = payload.recipient;
			if (payload.phone !== undefined) koc.phone = normalizePhone(payload.phone);
			koc.collabStep = Math.max(koc.collabStep || 0, 2);
			break;
		}
		case 'decline': {
			koc.stage = 'DECLINED';
			koc.callTaskPending = false;
			koc.declineCode = payload.declineCode || 'R01';
			koc.declineNote = payload.declineNote || '';
			break;
		}

		// === GIAI ĐOẠN 3 · GIAO HÀNG J&T EXPRESS (BƯỚC 3: DUYỆT & GỬI MẪU) ===
		case 'ship_sample': {
			// D1 -> D3: Booker duyệt & tạo vận đơn J&T Express / Tự vận chuyển
			if (payload.recipient !== undefined) koc.recipient = payload.recipient;
			if (payload.phone !== undefined) koc.phone = normalizePhone(payload.phone);
			if (payload.shippingAddress !== undefined) koc.shippingAddress = payload.shippingAddress;
			koc.shippingCarrier = payload.shippingCarrier || 'J&T Express';
			koc.trackingCode = (payload.trackingCode || '').trim();
			koc.shippedAt = payload.shippedAt ? new Date(payload.shippedAt) : new Date();
			koc.stage = 'SHIPPING';
			koc.collabStep = Math.max(koc.collabStep || 0, 3);
			koc.shippingDelayedAlert = false;

			if (koc.shippingCarrier === 'J&T Express' && koc.trackingCode) {
				await syncKocShipment(koc);
			} else {
				koc.trackingStatusText = `Đang vận chuyển (${koc.shippingCarrier})`;
			}

			await deliverMessageToKoc(
				koc,
				`[Gửi mẫu thành công] Bên mình đã gửi sản phẩm mẫu tới địa chỉ "${koc.shippingAddress || 'của bạn'}" qua ${koc.shippingCarrier}${koc.trackingCode ? ` (Mã vận đơn: #${koc.trackingCode})` : ''}. Bạn để ý điện thoại nhận hàng giúp mình nhé!`,
			);
			break;
		}
		case 'mark_delivered': {
			// D6: Xác nhận KOC đã nhận hàng (DELIVERED)
			koc.stage = 'DELIVERED';
			koc.deliveredAt = new Date();
			koc.shippingDelayedAlert = false;
			koc.trackingStatusText = 'Đã giao hàng thành công (DELIVERED)';
			koc.collabStep = Math.max(koc.collabStep || 0, 3);
			await deliverMessageToKoc(
				koc,
				`[Đã giao mẫu] Đơn hàng mẫu đã giao thành công! Bạn trải nghiệm sản phẩm và lên video theo brief giúp nhãn hàng nhé.`,
			);
			break;
		}

		// === GIAI ĐOẠN 4 · LÊN VIDEO, HOÀN HÀNG & HIỆU QUẢ (BƯỚC 4 & BƯỚC 5) ===
		case 'post_video': {
			// E2 -> E5: KOC đăng bài (VIDEO_POSTED)
			const videoUrl = (payload.videoUrl || '').trim();
			if (videoUrl) {
				koc.videoUrl = videoUrl;
				const meta = await fetchRealTikTokVideoMetrics(videoUrl);
				if (meta.postedAt) koc.videoPostedAt = meta.postedAt;
			}
			if (payload.videoViews !== undefined) koc.videoViews = Number(payload.videoViews) || 0;
			if (payload.videoOrders !== undefined) koc.videoOrders = Number(payload.videoOrders) || 0;
			if (payload.videoGmv !== undefined) koc.videoGmv = Number(payload.videoGmv) || 0;
			koc.stage = 'VIDEO_POSTED';
			koc.kocVideoReply = 'WILL_POST';
			koc.stalledFlag = false;
			koc.collabStep = Math.max(koc.collabStep || 0, 4);
			break;
		}
		case 'remind_video': {
			// E1 -> E3 -> E7: Nhắc lên video lần 1 (5N), lần 2 (8N), lần 3 (12N) hoặc gắn cờ đỏ STALLED (4)
			const nextStep = Number(payload.videoReminderStep) || (koc.videoReminderStep || 0) + 1;
			const updated = await advanceVideoReminderStep(koc, nextStep);
			return response.json({ success: true, data: updated });
		}
		case 'video_reply': {
			// E4: KOC phản hồi sau khi nhận hàng (Sẽ đăng / Không hợp P01-P03 / Im lặng quá 3 lần -> STALLED)
			const replyType = payload.kocVideoReply || 'WILL_POST';
			koc.kocVideoReply = replyType;
			if (replyType === 'WILL_POST') {
				koc.stalledFlag = false;
			} else if (replyType === 'NOT_FIT') {
				// F1: Chọn mã lý do sau nhận hàng (P01-P03) -> Yêu cầu hoàn hàng
				koc.stage = 'NOT_FIT';
				koc.postReceiveCode = payload.postReceiveCode || 'P01';
				koc.postReceiveNote = payload.postReceiveNote || '';
			} else if (replyType === 'SILENT') {
				// E7: Gắn cờ đỏ STALLED
				koc.stage = 'STALLED';
				koc.stalledFlag = true;
			}
			break;
		}
		case 'return_product': {
			// F2: Theo dõi hoàn hàng (RETURNING) - mã vận đơn trả + tình trạng hàng
			koc.stage = 'RETURNING';
			if (payload.postReceiveCode) koc.postReceiveCode = payload.postReceiveCode;
			if (payload.postReceiveNote !== undefined) koc.postReceiveNote = payload.postReceiveNote;
			if (payload.returnTrackingCode !== undefined) koc.returnTrackingCode = payload.returnTrackingCode;
			if (payload.returnCondition !== undefined) koc.returnCondition = payload.returnCondition;
			break;
		}
		case 'complete_returned': {
			// F3: Đã nhận hàng hoàn (RETURNED)
			koc.stage = 'RETURNED';
			koc.returnedAt = new Date();
			if (payload.returnCondition !== undefined) koc.returnCondition = payload.returnCondition;
			break;
		}
		case 'complete_collab': {
			// E5 -> E6: Theo dõi views, đơn hàng, GMV -> COMPLETED & tự động bật da_hop_tac = true
			if (payload.videoViews !== undefined) koc.videoViews = Number(payload.videoViews) || 0;
			if (payload.videoOrders !== undefined) koc.videoOrders = Number(payload.videoOrders) || 0;
			if (payload.videoGmv !== undefined) {
				koc.videoGmv = Number(payload.videoGmv) || 0;
				if (koc.videoGmv > 0) koc.gmv = Math.max(koc.gmv || 0, koc.videoGmv);
			}
			koc.stage = 'COMPLETED';
			koc.collabStep = 5;
			koc.stalledFlag = false;
			if (payload.markDaHopTac !== false) {
				koc.daHopTac = true;
			}
			break;
		}
		case 'set_stage': {
			if (payload.stage) koc.stage = payload.stage;
			if (payload.rejectCode !== undefined) koc.rejectCode = payload.rejectCode;
			if (payload.rejectNote !== undefined) koc.rejectNote = payload.rejectNote;
			break;
		}
		default:
			throw new ApiError(400, 'Hành động cập nhật không hợp lệ');
	}

	await koc.save();
	return response.json({ success: true, data: koc });
}

export async function syncChannelEndpoint(request, response) {
	const koc = await Koc.findOne({ _id: request.params.id, status: 'active' });
	if (!koc) throw new ApiError(404, 'Không tìm thấy KOC đang hoạt động');

	const autoDetectCollabVideo = Boolean(request.body?.autoDetectCollabVideo);
	const { koc: updated, channelData } = await syncKocTikTokChannel(koc, { autoDetectCollabVideo });
	return response.json({
		success: true,
		data: updated,
		channelData,
		message: channelData.found
			? `Đã đồng bộ kênh thật @${updated.tiktokId}: ${new Intl.NumberFormat('vi-VN').format(updated.followers)} follower · ${updated.videos?.length || 0} video mới nhất.`
			: `Không lấy được video công khai từ kênh @${updated.tiktokId}.`,
	});
}

export async function trackShipmentEndpoint(request, response) {
	const koc = await Koc.findOne({ _id: request.params.id, status: 'active' });
	if (!koc) throw new ApiError(404, 'Không tìm thấy KOC đang hoạt động');
	if (!koc.trackingCode && !request.body?.trackingCode) {
		throw new ApiError(400, 'KOC chưa có mã vận đơn J&T Express để tra cứu');
	}
	if (request.body?.trackingCode) {
		koc.trackingCode = String(request.body.trackingCode).trim();
	}
	const { koc: updated, tracking } = await syncKocShipment(koc);
	return response.json({
		success: true,
		data: updated,
		tracking,
		message: tracking?.statusText || 'Đã cập nhật trạng thái vận đơn J&T Express',
	});
}

export async function lookupJtTrackingEndpoint(request, response) {
	const trackingCode = String(request.body?.trackingCode || '').trim();
	if (!trackingCode) throw new ApiError(400, 'Vui lòng nhập mã vận đơn J&T Express');
	const tracking = await queryJtTracking(trackingCode);
	return response.json({
		success: true,
		data: tracking,
		message: tracking?.statusText || `Đã tra cứu vận đơn J&T #${trackingCode}`,
	});
}

export async function lookupTikTokVideoEndpoint(request, response) {
	const videoUrl = String(request.body?.videoUrl || '').trim();
	if (!videoUrl) throw new ApiError(400, 'Vui lòng nhập đường dẫn video TikTok');
	const metrics = await fetchRealTikTokVideoMetrics(videoUrl);
	return response.json({
		success: true,
		data: metrics,
		message: metrics?.videoId
			? `Đã xác thực video TikTok thật (ID: ${metrics.videoId}) của @${metrics.authorUniqueId || 'koc'}`
			: 'Đã ghi nhận đường dẫn video TikTok',
	});
}

export async function runOutreachAutomation(request, response) {
	const { mode, simulateNextStep = false, inviteBrief = '' } = request.validated.body;

	if (mode === 'invite_all_qualified') {
		const qualifiedKocs = await Koc.find({
			status: 'active',
			daHopTac: false,
			stage: 'QUALIFIED',
		});
		const updatedItems = [];
		for (const koc of qualifiedKocs) {
			const updated = await sendFirstInvite(koc, inviteBrief);
			updatedItems.push(updated);
		}
		return response.json({
			success: true,
			data: { count: updatedItems.length, items: updatedItems },
			message: `Đã gửi lời mời lần 1 (Zalo + Brief) cho ${updatedItems.length} KOC đạt phễu.`,
		});
	}

	const summary = await runStage2Sweep({ simulateNextStep });
	const digest = await buildMorningDigest();
	const items = await Koc.find({ status: 'active' }).sort({ createdAt: -1 }).limit(100).lean();
	return response.json({
		success: true,
		data: { summary, digest, items },
		message: `Quét tự động hoàn tất: Nhắc Email (+2N): ${summary.emailReminded} · Nhắc Zalo lối thoát (+4N): ${summary.zaloReminded} · Task gọi (+7N): ${summary.callTasksCreated} · Vận đơn J&T: ${summary.shipmentsChecked} · Quét TikTok: ${summary.tiktokDailyScanned} · Nhắc video: ${summary.videoReminded} · Cờ đỏ STALLED: ${summary.stalledFlagged}.`,
	});
}

export async function getMorningDigestEndpoint(request, response) {
	const digest = await buildMorningDigest();
	return response.json({ success: true, data: digest });
}

export async function archiveKoc(request, response) {
	const koc = await Koc.findOneAndUpdate({ _id: request.params.id, status: 'active' }, { status: 'archived' }, { new: true }).lean();
	if (!koc) throw new ApiError(404, 'Không tìm thấy KOC đang hoạt động');
	response.json({ success: true, data: koc });
}

export async function emailKoc(request, response) {
	const koc = await Koc.findOne({ _id: request.params.id, status: 'active' }).lean();
	if (!koc) throw new ApiError(404, 'Không tìm thấy KOC đang hoạt động');
	if (!koc.email) throw new ApiError(422, 'KOC chưa có email để gửi');
	await sendKocEmail({ to: koc.email, displayName: koc.displayName, tiktokId: koc.tiktokId, text: request.validated.body.text });
	response.json({ success: true, message: 'Đã gửi email cho KOC' });
}
