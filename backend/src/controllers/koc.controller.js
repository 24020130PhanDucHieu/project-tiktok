import Koc from '../models/koc.model.js';
import ApiError from '../utils/ApiError.js';

function normalizeTikTokId(value) { return value.trim().replace(/^@/, '').toLowerCase(); }

export async function listKocs(request, response) {
	const { q = '', category, page = 1, limit = 20 } = request.query;
	const filter = { status: 'active' };
	if (category && category !== 'Tất cả') filter.category = category;
	if (q) filter.$or = [{ displayName: new RegExp(q, 'i') }, { tiktokId: new RegExp(normalizeTikTokId(q), 'i') }, { phone: new RegExp(q, 'i') }, { email: new RegExp(q, 'i') }];
	const pageNumber = Math.max(Number(page), 1);
	const pageSize = Math.min(Math.max(Number(limit), 1), 100);
	const [items, total] = await Promise.all([Koc.find(filter).sort({ createdAt: -1 }).skip((pageNumber - 1) * pageSize).limit(pageSize).lean(), Koc.countDocuments(filter)]);
	response.json({ success: true, data: { items, pagination: { page: pageNumber, limit: pageSize, total, pages: Math.ceil(total / pageSize) } } });
}

export async function lookupTikTok(request, response) {
	const tiktokId = normalizeTikTokId(request.body.tiktokId);
	const existing = await Koc.findOne({ tiktokId }).lean();
	if (existing) return response.json({ success: true, data: { source: 'database', profile: existing } });
	response.status(404).json({ success: false, message: 'Chưa tìm thấy kênh trong dữ liệu hệ thống. Có thể kết nối TikTok data provider ở bước tiếp theo.' });
}

export async function createKoc(request, response) {
	const payload = request.validated.body;
	const tiktokId = normalizeTikTokId(payload.tiktokId);
	const exists = await Koc.exists({ tiktokId });
	if (exists) throw new ApiError(409, 'TikTok ID này đã có trong danh bạ');
	const koc = await Koc.create({ ...payload, tiktokId });
	response.status(201).json({ success: true, data: koc });
}

export async function archiveKoc(request, response) {
	const koc = await Koc.findOneAndUpdate({ _id: request.params.id, status: 'active' }, { status: 'archived' }, { new: true }).lean();
	if (!koc) throw new ApiError(404, 'Không tìm thấy KOC đang hoạt động');
	response.json({ success: true, data: koc });
}