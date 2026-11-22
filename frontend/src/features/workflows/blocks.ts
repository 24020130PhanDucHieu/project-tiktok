import type { Campaign } from '../../types/campaign';
import type { InboxLabel, WorkflowNode, WorkflowNodeType } from '../../types/workflow';
import type { ZaloAccount } from '../../types/zalo';
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
	/** what comes in from the block before it (shown on the left when the block is selected) */
	input: string;
	/** and what goes out to the block after it (on the right) */
	output: string;
}

export const GROUP_TITLES: Record<BlockGroup, string> = { start: 'Bắt đầu', control: 'Điều khiển', action: 'Hành động' };

const b = (type: WorkflowNodeType, title: string, hint: string, group: BlockGroup, color: string, input: string, output: string): BlockMeta =>
	({ type, title, hint, group, color, sends: isSendingType(type), input, output });

/** Every block, in the order the palette shows them. */
export const BLOCKS: BlockMeta[] = [
	b('import', 'Nhập Excel', 'Thêm KOC từ file Excel. Khối bắt đầu.', 'start', '#0284c7',
		'Không có: đây là khối bắt đầu.', 'Các KOC đã được thêm từ file.'),
	b('pick', 'Lấy KOC có sẵn', 'Lấy KOC đã có trong danh bạ theo giai đoạn, thư mục, nguồn, hoặc chọn từng KOC theo tên. Khối bắt đầu, không cần Excel.', 'start', '#0d9488',
		'Không có: đây là khối bắt đầu.', 'Các KOC khớp giai đoạn hoặc thư mục, lấy một lần lúc bắt đầu.'),
	b('wait', 'Chờ', 'Chờ một khoảng thời gian rồi đi tiếp.', 'control', '#d97706',
		'Danh sách KOC từ khối trước.', 'Cùng danh sách đó, sau khi chờ xong.'),
	b('wait_until', 'Chờ đến giờ', 'Chờ đến một giờ cố định trong ngày (giờ Việt Nam).', 'control', '#ca8a04',
		'Danh sách KOC từ khối trước.', 'Cùng danh sách đó, đúng giờ đã chọn.'),
	b('if_replied', 'KOC đã trả lời chưa?', 'Tách KOC đã trả lời và chưa trả lời, mỗi nhóm đi một nhánh.', 'control', '#7c3aed',
		'KOC đã được gửi tin ở khối trước.', 'Hai nhánh: KOC đã trả lời và KOC chưa trả lời.'),
	b('send', 'Gửi chiến dịch', 'Gửi một chiến dịch Zalo đã lưu cho các KOC đi qua khối này.', 'action', '#1d4ed8',
		'KOC cần gửi tin.', 'KOC đã được gửi tin (kèm thời điểm gửi).'),
	b('send_text', 'Gửi tin Zalo', 'Gửi một tin Zalo tự viết cho các KOC đi qua khối này.', 'action', '#2563eb',
		'KOC cần gửi tin.', 'KOC đã được gửi tin (kèm thời điểm gửi).'),
	b('email', 'Gửi email', 'Gửi email cho từng KOC, cách nhau vài giây, tối đa 100 email mỗi khối.', 'action', '#db2777',
		'KOC cần gửi email.', 'KOC đã nhận email (kèm thời điểm gửi).'),
	b('label', 'Gắn nhãn', 'Gắn một nhãn vào hội thoại của từng KOC.', 'action', '#16a34a',
		'KOC cần gắn nhãn.', 'Cùng danh sách đó (khối chỉ gắn nhãn, không đổi danh sách).'),
	b('stage', 'Đổi giai đoạn', 'Chuyển từng KOC sang một giai đoạn.', 'action', '#ea580c',
		'KOC cần đổi giai đoạn.', 'Cùng danh sách đó (khối chỉ đổi giai đoạn, không đổi danh sách).'),
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
	/** the Zalo accounts, to name the one a send block sends through; not loaded (or none) means nothing is said about it */
	zaloAccounts?: ZaloAccount[];
	/** the rows of the Excel file that was chosen for the import block */
	rows: number;
}

const cut = (text: string, max = 30) => {
	const chars = Array.from(text.replace(/\s+/g, ' ').trim());
	return chars.length > max ? `${chars.slice(0, max).join('')}…` : chars.join('');
};

export interface DetailContext {
	labels: InboxLabel[];
	zaloAccounts?: ZaloAccount[];
}

const sendsOnZalo = (n: WorkflowNode) => n.type === 'send' || n.type === 'send_text';

/** What is wrong with the Zalo account a send block names, or '' (it was removed since the block was drawn). */
export function blockAccountProblem(n: WorkflowNode, accounts: ZaloAccount[]): string {
	if (!sendsOnZalo(n) || !n.zaloAccountId || accounts.length === 0) return '';
	return accounts.some((a) => a._id === n.zaloAccountId) ? '' : 'Tài khoản Zalo của khối này đã bị xóa. Hãy chọn lại.';
}

/** " · Zalo phụ": the account a send block uses, said only when there is a choice to make (several accounts). */
function accountSuffix(n: WorkflowNode, accounts?: ZaloAccount[]): string {
	if (!sendsOnZalo(n) || !n.zaloAccountId || !accounts || accounts.length < 2) return '';
	const found = accounts.find((a) => a._id === n.zaloAccountId);
	return found ? ` · ${found.label}` : '';
}

function accountLine(n: WorkflowNode, accounts?: ZaloAccount[]): [string, string][] {
	if (!n.zaloAccountId || !accounts?.length) return [];
	return [['Tài khoản Zalo', accounts.find((a) => a._id === n.zaloAccountId)?.label ?? 'Đã bị xóa']];
}

/** What a block was set to, as label and value pairs, for the read-only view of a workflow that runs. */
export function nodeDetailLines(n: WorkflowNode, ctx: DetailContext): [string, string][] {
	switch (n.type) {
		case 'import':
			return [['Nguồn', 'File Excel']];
		case 'pick': {
			const lines: [string, string][] = [];
			if (n.kocIds?.length) {
				lines.push(['KOC đã chọn', n.kocIds.map((id) => n.kocNames?.[id] ?? 'KOC đã chọn').join(', ')]);
				return lines;
			}
			if (n.source) lines.push(['Nguồn', n.source]);
			if (n.stages?.length) lines.push(['Giai đoạn', n.stages.map((s) => STAGE_LABELS[s] ?? s).join(', ')]);
			if (n.folder) lines.push(['Thư mục', n.folder]);
			if (n.limit) lines.push(['Tối đa', `${n.limit} KOC`]);
			return lines;
		}
		case 'wait':
			return [['Chờ', waitLabel(n.waitMinutes ?? 0)]];
		case 'wait_until':
			return [['Chờ đến', `${formatClock(n.atMinute ?? 0)} (giờ Việt Nam)`]];
		case 'send':
			return [['Chiến dịch', n.campaignName ?? 'Chiến dịch đã chọn'], ...accountLine(n, ctx.zaloAccounts)];
		case 'send_text':
			return [['Nội dung', n.text ?? ''], ...accountLine(n, ctx.zaloAccounts)];
		case 'email':
			return [['Chủ đề', n.subject || 'Mặc định'], ['Nội dung', n.text ?? '']];
		case 'label':
			return [['Nhãn', ctx.labels.find((l) => l._id === n.labelId)?.name ?? 'Nhãn đã chọn']];
		case 'stage':
			return [['Giai đoạn', n.stage ? STAGE_LABELS[n.stage] ?? n.stage : '']];
		default:
			return [];
	}
}

/** The line under the name of a block: what it is set to, or what is still missing. */
export function nodeSummary(n: WorkflowNode, ctx: SummaryContext): string {
	switch (n.type) {
		case 'import':
			return ctx.rows > 0 ? `${ctx.rows} dòng sẽ nhập` : 'Chưa chọn file Excel';
		case 'pick': {
			if (n.kocIds?.length) return `${n.kocIds.length} KOC đã chọn`;
			const stages = (n.stages ?? []).map((s) => STAGE_LABELS[s] ?? s).join(', ');
			const where = [stages, n.folder ? `thư mục ${n.folder}` : '', n.source ? `nguồn ${n.source}` : ''].filter(Boolean).join(', ');
			if (!where) return 'Chưa chọn giai đoạn';
			return n.limit ? `${where} (tối đa ${n.limit})` : where;
		}
		case 'wait':
			return n.waitMinutes ? `Chờ ${waitLabel(n.waitMinutes)}` : 'Chưa đặt thời gian';
		case 'wait_until':
			return Number.isInteger(n.atMinute) ? `Chờ đến ${formatClock(n.atMinute as number)}` : 'Chưa đặt giờ';
		case 'send': {
			if (!n.campaignId) return 'Chưa chọn chiến dịch';
			const found = ctx.campaigns.find((c) => c._id === n.campaignId);
			if (!found) return ctx.campaigns.length > 0 ? 'Chọn lại chiến dịch (đã bị xóa)' : n.campaignName ?? 'Chiến dịch đã chọn';
			if (blockAccountProblem(n, ctx.zaloAccounts ?? [])) return 'Chọn lại tài khoản Zalo (đã bị xóa)';
			return found.name + accountSuffix(n, ctx.zaloAccounts);
		}
		case 'send_text':
			if (!n.text) return 'Chưa có nội dung';
			if (blockAccountProblem(n, ctx.zaloAccounts ?? [])) return 'Chọn lại tài khoản Zalo (đã bị xóa)';
			return cut(n.text) + accountSuffix(n, ctx.zaloAccounts);
		case 'email':
			if (!n.text) return 'Chưa có nội dung';
			return n.subject ? cut(n.subject) : 'Email, không chủ đề';
		case 'label': {
			if (!n.labelId) return 'Chưa chọn nhãn';
			const found = ctx.labels.find((l) => l._id === n.labelId);
			if (found) return found.name;
			return ctx.labels.length > 0 ? 'Chọn lại nhãn (đã bị xóa)' : 'Nhãn đã chọn';
		}
		case 'stage':
			return n.stage ? STAGE_LABELS[n.stage] ?? n.stage : 'Chưa chọn giai đoạn';
		default:
			return 'Chia theo việc đã trả lời';
	}
}
