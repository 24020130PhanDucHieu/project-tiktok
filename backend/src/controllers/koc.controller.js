import Koc from '../models/koc.model.js';
import Conversation from '../models/conversation.model.js';
import { sendKocEmail } from '../services/email.service.js';
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
	const { q = '', category, stage, source, page = 1, limit = 20 } = request.query;
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
	response.status(404).json({
		success: false,
		message: 'Chưa tìm thấy kênh trong dữ liệu hệ thống. Có thể thêm mới vào phễu SOURCED.',
	});
}

export async function createKoc(request, response) {
	const payload = request.validated.body;
	const tiktokId = normalizeTikTokId(payload.tiktokId);
	const phone = normalizePhone(payload.phone || '');

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

	const { castFee, castType, stage } = resolveCastAndStage(payload, payload.stage || 'SOURCED');
	const kocData = {
		...payload,
		tiktokId,
		phone,
		castFee,
		castType,
		stage,
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
		case 'qualify': {
			// A6 -> A7 (Có) -> A9 (Tạo bản ghi hợp tác) -> A10 (Có phí cast?)
			const castFee = payload.castFee !== undefined ? Number(payload.castFee) : koc.castFee;
			koc.contentPillar = payload.contentPillar ?? koc.contentPillar;
			koc.targetAudience = payload.targetAudience ?? koc.targetAudience;
			koc.campaign = payload.campaign ?? koc.campaign;
			koc.productName = payload.productName ?? koc.productName;
			if (payload.commissionRate !== undefined) koc.commissionRate = Number(payload.commissionRate);
			koc.castFee = castFee;
			koc.castType = castFee > 0 ? 'cast' : 'freecast';
			// Nếu có phí cast -> chuyển sang AWAITING_APPROVAL (Chủ shop duyệt), nếu Freecast -> QUALIFIED
			koc.stage = castFee > 0 ? 'AWAITING_APPROVAL' : 'QUALIFIED';
			koc.rejectCode = '';
			koc.rejectNote = '';
			break;
		}
		case 'approve_cast': {
			// A11: Chủ shop duyệt giá cast -> QUALIFIED (Sẵn sàng gửi tin mời GĐ 2)
			koc.stage = 'QUALIFIED';
			if (payload.castFee !== undefined) {
				koc.castFee = Number(payload.castFee);
				koc.castType = koc.castFee > 0 ? 'cast' : 'freecast';
			}
			break;
		}
		case 'filter_out': {
			// A8 hoặc A11 (Không duyệt): Loại + ghi mã lý do lọc
			koc.stage = 'FILTERED_OUT';
			koc.rejectCode = payload.rejectCode || 'F01';
			koc.rejectNote = payload.rejectNote || '';
			break;
		}
		case 'toggle_collaborated': {
			// A4 -> A5: Đánh dấu hoặc gỡ cờ da_hop_tac
			koc.daHopTac = Boolean(payload.daHopTac);
			break;
		}
		case 'set_stage': {
			if (payload.stage) koc.stage = payload.stage;
			if (payload.rejectCode !== undefined) koc.rejectCode = payload.rejectCode;
			if (payload.rejectNote !== undefined) koc.rejectNote = payload.rejectNote;
			break;
		}
		default:
			throw new ApiError(400, 'Hành động cập nhật phễu không hợp lệ');
	}

	await koc.save();
	response.json({ success: true, data: koc });
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
