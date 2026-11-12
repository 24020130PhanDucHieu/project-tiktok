import type { TargetDetail, TargetInviteInput } from '../../types/koc';

export const runes = (s: string) => Array.from(s).length;

/** Same text the server sends to creators: the owner's message plus a footer (TikTok's API has no field for contact info). */
export function composeMessage(f: Pick<TargetInviteInput, 'message' | 'zalo' | 'facebook' | 'email' | 'contentPref'>): string {
	let msg = f.message.trim();
	const foot: string[] = [];
	if (f.zalo.trim()) foot.push(`Zalo: ${f.zalo.trim()}`);
	if (f.facebook.trim()) foot.push(`Facebook: ${f.facebook.trim()}`);
	// the email is sent through TikTok's own contact field; repeating it here gets the invitation refused
	if (foot.length) msg += `\n\nLiên hệ — ${foot.join(' · ')}`;
	if (f.contentPref === 'video') msg += '\nƯu tiên: video giới thiệu sản phẩm';
	if (f.contentPref === 'live') msg += '\nƯu tiên: LIVE giới thiệu sản phẩm';
	return msg;
}

export function newRequestKey(): string {
	if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
	return `${Date.now().toString(16)}-${Math.random().toString(16).slice(2)}-${Math.random().toString(16).slice(2)}`;
}

/** YYYY-MM-DD of tomorrow in the browser's calendar (the earliest end date TikTok accepts). */
export function tomorrow(): string {
	const d = new Date();
	d.setDate(d.getDate() + 1);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

export const toNumber = (v: string): number => {
	const n = parseFloat(v.replace(',', '.'));
	return Number.isFinite(n) ? n : 0;
};

export const vnd = (n: number | string) => `${new Intl.NumberFormat('vi-VN').format(Math.round(Number(n) || 0))}đ`;

export const unixDate = (sec: number) => (sec ? new Date(sec * 1000).toLocaleDateString('vi-VN') : '—');

/** "Đang diễn ra" / "Sắp hết hạn" (under 7 days left) / "Đã kết thúc" from the invitation's end time. */
export function lifeState(endSec: number, localStatus?: string): { label: string; tone: 'ok' | 'warn' | 'off' } {
	if (localStatus === 'cancelled') return { label: 'Đã hủy', tone: 'off' };
	const left = endSec * 1000 - Date.now();
	if (left <= 0) return { label: 'Đã kết thúc', tone: 'off' };
	if (left < 7 * 24 * 3600 * 1000) return { label: 'Sắp hết hạn', tone: 'warn' };
	return { label: 'Đang diễn ra', tone: 'ok' };
}

/** "[ DEAL MỞ BÁN ] Áo Polo Cao cấp Cooltech,Chất Liệu Micax… | COOLTECH" -> "Áo Polo Cao cấp Cooltech" (short enough to read in a message). */
export function shortTitle(title: string) {
	const parts = title.replace(/^\s*\[[^\]]*\]\s*/, '').split(/,| - | \| /).map((x) => x.trim()).filter(Boolean);
	let t = parts.find((x) => x.length >= 12) || parts[0] || title.trim(); // skip short tags such as "Deal Hot |"
	if (t.length > 60) t = t.slice(0, 60).replace(/\s+\S*$/, '');
	return t;
}

/**
 * The Zalo message that goes with a sent invitation. Built from what TikTok reports for it (products, commissions,
 * samples, end date), never from the form, so it states exactly what the creator will see. The owner reviews and sends it.
 */
export function targetChatText(d: Pick<TargetDetail, 'products' | 'endTime' | 'hasFreeSample' | 'sampleApprovalExempt'>, link: string): string {
	// plain sentences, the way a person writes on Zalo: no bullets, no brackets, no quotation marks
	const lines = ['Chào bạn, bên mình vừa gửi cho bạn một lời mời cộng tác riêng trên TikTok Shop.'];
	const shown = d.products.slice(0, 3);
	for (const p of shown) {
		const ads = p.adsCommissionPct ? `, đơn từ quảng cáo ${p.adsCommissionPct}%` : '';
		lines.push(`${shortTitle(p.title)}, hoa hồng ${p.commissionPct}%${ads}.`);
	}
	if (d.products.length > shown.length) lines.push(`Ngoài ra còn ${d.products.length - shown.length} sản phẩm khác nữa.`);
	if (d.hasFreeSample) lines.push(d.sampleApprovalExempt ? 'Bên mình tặng kèm hàng mẫu miễn phí và duyệt mẫu tự động.' : 'Bên mình tặng kèm hàng mẫu miễn phí.');
	if (d.endTime) lines.push(`Lời mời có hiệu lực đến ${unixDate(d.endTime)}.`);
	lines.push('', `Bạn bấm vào link này để xem chi tiết và chấp nhận nhé: ${link}`, 'Có thắc mắc gì bạn cứ nhắn lại cho bên mình nhé!');
	return plainMessage(lines.join('\n'));
}

/**
 * A message for a creator reads like a person wrote it: no quotation marks, no bullets and no tags in square brackets
 * (product titles often start with one, such as a deal tag). Every prepared message goes through here before it lands
 * in the chat box.
 */
export function plainMessage(text: string): string {
	return text
		.replace(/[ \t]*\[[^\]\n]*\][ \t]*/g, ' ')
		.replace(/["“”„‟«»]/g, '')
		.replace(/^[ \t]*[•·▪●◦][ \t]*/gm, '')
		.replace(/[ \t]{2,}/g, ' ')
		.replace(/^[ \t]+|[ \t]+$/gm, '')
		.trim();
}
