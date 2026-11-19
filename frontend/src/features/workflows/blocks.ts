import type { Campaign } from '../../types/campaign';
import type { InboxLabel, WorkflowNode, WorkflowNodeType } from '../../types/workflow';
import { STAGE_TARGET_VALUES, isSendingType, isStartType, waitLabel } from './graph';

export { isSendingType, isStartType };

export type BlockGroup = 'start' | 'control' | 'action';

export interface BlockMeta {
	type: WorkflowNodeType;
	title: string;
	hint: string;
	group: BlockGroup;
	/** the colour of its icon and its border */
	color: string;
	/** true when the block messages people by itself (Zalo or email): it needs an admin and a confirmation */
	sends: boolean;
}

export const GROUP_TITLES: Record<BlockGroup, string> = { start: 'Bắt đầu', control: 'Điều khiển', action: 'Hành động' };

const b = (type: WorkflowNodeType, title: string, hint: string, group: BlockGroup, color: string): BlockMeta => ({ type, title, hint, group, color, sends: isSendingType(type) });

/** Every block, in the order the palette shows them. */
export const BLOCKS: BlockMeta[] = [
	b('import', 'Nhập Excel', 'Thêm KOC từ file Excel. Khối bắt đầu.', 'start', '#0284c7'),
	b('pick', 'Lấy KOC có sẵn', 'Lấy KOC đã có trong danh bạ theo giai đoạn hoặc thư mục. Khối bắt đầu, không cần Excel.', 'start', '#0d9488'),
	b('wait', 'Chờ', 'Chờ một khoảng thời gian rồi đi tiếp.', 'control', '#d97706'),
	b('wait_until', 'Chờ đến giờ', 'Chờ đến một giờ cố định trong ngày (giờ Việt Nam).', 'control', '#ca8a04'),
	b('if_replied', 'KOC đã trả lời chưa?', 'Tách KOC đã trả lời và chưa trả lời, mỗi nhóm đi một nhánh.', 'control', '#7c3aed'),
	b('send', 'Gửi chiến dịch', 'Gửi một chiến dịch Zalo đã lưu cho các KOC đi qua khối này.', 'action', '#1d4ed8'),
	b('send_text', 'Gửi tin Zalo', 'Gửi một tin Zalo tự viết cho các KOC đi qua khối này.', 'action', '#2563eb'),
	b('email', 'Gửi email', 'Gửi email cho từng KOC, cách nhau vài giây, tối đa 100 email mỗi khối.', 'action', '#db2777'),
	b('label', 'Gắn nhãn', 'Gắn một nhãn vào hội thoại của từng KOC.', 'action', '#16a34a'),
	b('stage', 'Đổi giai đoạn', 'Chuyển từng KOC sang một giai đoạn.', 'action', '#ea580c'),
];

export const BLOCK_BY_TYPE = Object.fromEntries(BLOCKS.map((x) => [x.type, x])) as Record<WorkflowNodeType, BlockMeta>;

/** The name of every stage a KOC can be in, as the rest of the app calls it. */
export const STAGE_LABELS: Record<string, string> = {
	SOURCED: 'Mới tìm',
	QUALIFIED: 'Đạt phễu',
	AWAITING_APPROVAL: 'Chờ duyệt',
	FILTERED_OUT: 'Loại',
	CONTACTED: 'Đã liên hệ',
	NO_REPLY: 'Chưa phản hồi',
	REPLIED: 'Đã phản hồi',
	DORMANT: 'Ngủ đông',
	AGREED: 'Đồng ý',
	DECLINED: 'Từ chối',
	SHIPPING: 'Đang giao',
	DELIVERED: 'Đã nhận mẫu',
	VIDEO_POSTED: 'Đã lên video',
	NOT_FIT: 'Không hợp',
	RETURNING: 'Đang hoàn',
	RETURNED: 'Đã hoàn',
	STALLED: 'Đình trệ',
	COMPLETED: 'Hoàn tất',
};

/** The stages a stage block offers. */
export const STAGE_TARGETS = STAGE_TARGET_VALUES.map((value) => ({ value, label: STAGE_LABELS[value] }));

/** "09:00": a time of day given as minutes after midnight. */
export function formatClock(atMinute: number): string {
	const h = Math.floor(atMinute / 60);
	const m = atMinute % 60;
	return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
}

export interface SummaryContext {
	campaigns: Campaign[];
	labels: InboxLabel[];
	/** the rows of the Excel file that was chosen for the import block */
	rows: number;
}

const cut = (text: string, max = 30) => {
	const chars = Array.from(text.replace(/\s+/g, ' ').trim());
	return chars.length > max ? `${chars.slice(0, max).join('')}…` : chars.join('');
};

/** The line under the name of a block: what it is set to, or what is still missing. */
export function nodeSummary(n: WorkflowNode, ctx: SummaryContext): string {
	switch (n.type) {
		case 'import':
			return ctx.rows > 0 ? `${ctx.rows} dòng sẽ nhập` : 'Chưa chọn file Excel';
		case 'pick': {
			const stages = (n.stages ?? []).map((s) => STAGE_LABELS[s] ?? s).join(', ');
			const where = [stages, n.folder ? `thư mục ${n.folder}` : ''].filter(Boolean).join(', ');
			if (!where) return 'Chưa chọn giai đoạn';
			return n.limit ? `${where} (tối đa ${n.limit})` : where;
		}
		case 'wait':
			return n.waitMinutes ? `Chờ ${waitLabel(n.waitMinutes)}` : 'Chưa đặt thời gian';
		case 'wait_until':
			return Number.isInteger(n.atMinute) ? `Chờ đến ${formatClock(n.atMinute as number)}` : 'Chưa đặt giờ';
		case 'send':
			return ctx.campaigns.find((c) => c._id === n.campaignId)?.name ?? n.campaignName ?? 'Chưa chọn chiến dịch';
		case 'send_text':
			return n.text ? cut(n.text) : 'Chưa có nội dung';
		case 'email':
			if (!n.text) return 'Chưa có nội dung';
			return n.subject ? cut(n.subject) : 'Email, không chủ đề';
		case 'label':
			return ctx.labels.find((l) => l._id === n.labelId)?.name ?? 'Chưa chọn nhãn';
		case 'stage':
			return n.stage ? STAGE_LABELS[n.stage] ?? n.stage : 'Chưa chọn giai đoạn';
		default:
			return 'Chia theo việc đã trả lời';
	}
}
