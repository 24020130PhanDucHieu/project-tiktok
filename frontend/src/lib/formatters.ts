import type { Creator } from '../types/koc';
import { initialCreators } from './constants';

export function stageToCollabStep(stage?: string): number {
	switch (stage) {
		case 'SOURCED':
		case 'AWAITING_APPROVAL':
		case 'QUALIFIED':
		case 'CONTACTED':
		case 'NO_REPLY':
			return 0;
		case 'REPLIED':
			return 1;
		case 'AGREED':
			return 2;
		case 'SHIPPING':
		case 'DELIVERED':
		case 'NOT_FIT':
		case 'RETURNING':
		case 'RETURNED':
		case 'STALLED':
			return 3;
		case 'VIDEO_POSTED':
			return 4;
		case 'COMPLETED':
			return 5;
		default:
			return 0;
	}
}

export function stageBadgeText(stage?: string): string {
	switch (stage) {
		case 'AGREED':
			return 'Chờ gửi mẫu';
		case 'SHIPPING':
			return 'Đang giao J&T';
		case 'DELIVERED':
			return 'Chờ đăng bài';
		case 'VIDEO_POSTED':
			return 'Đã lên bài';
		case 'COMPLETED':
			return 'Hoàn thành';
		case 'NOT_FIT':
		case 'RETURNING':
			return 'Đang hoàn hàng';
		case 'RETURNED':
			return 'Đã nhận hoàn';
		case 'STALLED':
			return 'Cờ đỏ · Bùng';
		case 'REPLIED':
			return 'Đang trao đổi';
		case 'DECLINED':
			return 'Đã từ chối';
		case 'DORMANT':
			return 'Ngủ đông 30N';
		default:
			return 'Chưa có việc';
	}
}

export function mapItemToCreator(item: Partial<Creator> & Record<string, any>): Creator {
	const rawName =
		String(item.displayName || item.name || 'KOC')
			.replace(/[^\p{L}\p{N}\s._-]/gu, '')
			.trim() ||
		String(item.displayName || 'KOC');
	const stage = String(item.stage || 'SOURCED');
	const computedStep = Math.max(Number(item.collabStep) || 0, stageToCollabStep(stage));
	return {
		...item,
		id: String(item._id || item.id || `koc-${Date.now()}`),
		name: rawName,
		tiktokId: String(item.tiktokId || '').replace(/^@/, ''),
		handle: `@${String(item.tiktokId || '').replace(/^@/, '')}`,
		contact: item.phone || '—',
		followers: Number(item.followers) || 0,
		gmv: Number(item.gmv) || 0,
		category: item.category || 'Chưa phân loại',
		castFee: Number(item.castFee) || 0,
		stage,
		collabStep: computedStep,
		avatar: rawName.slice(0, 2).toUpperCase(),
		videos: item.videos?.length ? item.videos : initialCreators[0].videos,
	};
}

export function formatNumber(value?: number | string): string {
	const num = Number(value) || 0;
	if (num >= 1000000) {
		return `${(num / 1000000).toFixed(1).replace('.0', '').replace('.', ',')}tr`;
	}
	if (num >= 1000) {
		return `${(num / 1000).toFixed(1).replace('.0', '').replace('.', ',')}k`;
	}
	return String(num);
}

export function formatCurrency(value?: number | string): string {
	const num = Number(value) || 0;
	if (!num) return '—';
	return `${new Intl.NumberFormat('vi-VN').format(num)}đ`;
}

