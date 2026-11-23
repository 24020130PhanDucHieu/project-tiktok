import type { ZaloAccount } from '../../types/zalo';

/** What these helpers need to know of a conversation: the account it talks through, if it has one yet. */
interface HasAccount {
	zaloAccountId?: string;
}

export type AccountTone = 'ok' | 'warn' | 'off';

const LOGGING_IN = ['waiting_qr', 'scanned', 'connecting'];

/**
 * The account a conversation is answered through: its own; the default one (marked default, otherwise the first) when
 * it has none yet. A conversation whose account was removed has none: the server does not move it to another account
 * either, it cannot send until somebody does.
 */
export function accountOfConversation(accounts: ZaloAccount[], conv: HasAccount | null | undefined): ZaloAccount | undefined {
	if (conv?.zaloAccountId) return accounts.find((a) => a._id === conv.zaloAccountId);
	return accounts.find((a) => a.isDefault) ?? accounts[0];
}

/** True for a conversation of an account that was removed. Before the accounts are loaded nothing is known to be removed. */
export function accountRemoved(accounts: ZaloAccount[], conv: HasAccount | null | undefined): boolean {
	return accounts.length > 0 && !!conv?.zaloAccountId && !accounts.some((a) => a._id === conv.zaloAccountId);
}

/** The conversations of one account; an empty id keeps them all. */
export function filterByAccount<T extends HasAccount>(convs: T[], accountId: string): T[] {
	return accountId ? convs.filter((c) => c.zaloAccountId === accountId) : convs;
}

export function accountTone(a: ZaloAccount): AccountTone {
	if (a.connected) return 'ok';
	if (a.bridgeOnline && LOGGING_IN.includes(a.state)) return 'warn';
	return 'off';
}

/** One line for a person: what is going on with the account. */
export function accountStatusText(a: ZaloAccount): string {
	if (!a.bridgeOnline) return 'Không kết nối được dịch vụ Zalo';
	if (a.connected) return 'Đã kết nối';
	if (LOGGING_IN.includes(a.state)) return a.state === 'connecting' ? 'Đang kết nối' : 'Đang đăng nhập';
	if (a.disabled) return 'Đã ngắt kết nối';
	if (a.needsQr) return 'Cần quét mã QR';
	if (a.state === 'error') return 'Đăng nhập lỗi, cần quét lại mã QR';
	return 'Mất kết nối';
}

/** The account to log in again when somebody asks to reconnect: the open conversation's if it is down, else the first that is. */
export function reconnectTarget(accounts: ZaloAccount[], conv: HasAccount | null | undefined): ZaloAccount | undefined {
	const own = accountOfConversation(accounts, conv);
	if (own && !own.connected) return own;
	return accounts.find((a) => !a.connected);
}

/** A conversation row names its account only when there is a choice to make. */
export function showAccountTag(accounts: ZaloAccount[]): boolean {
	return accounts.length > 1;
}

export function accountLabelOf(accounts: ZaloAccount[], id: string | undefined): string {
	return (id && accounts.find((a) => a._id === id)?.label) || '';
}

/** "Cần quét mã QR" → "cần quét mã QR": only the first letter goes down, so QR stays QR. */
const lowerFirst = (text: string) => text.charAt(0).toLowerCase() + text.slice(1);

/** What the header chip says: one account by its name, several by how many are logged in. */
export function switcherSummary(accounts: ZaloAccount[]): { title: string; sub: string; tone: AccountTone } {
	if (accounts.length === 1) {
		const a = accounts[0];
		return { title: a.label, sub: a.connected ? 'Zalo · đã kết nối' : `Zalo · ${lowerFirst(accountStatusText(a))}`, tone: accountTone(a) };
	}
	const up = accounts.filter((a) => a.connected).length;
	return {
		title: `${accounts.length} tài khoản Zalo`,
		sub: `${up}/${accounts.length} đang kết nối`,
		tone: up === accounts.length ? 'ok' : up === 0 ? 'off' : 'warn',
	};
}

/** The account a bulk send goes through: the one chosen, else the default one (the server's rule), else the first. */
export function pickSendAccount(accounts: ZaloAccount[], chosenId: string): ZaloAccount | undefined {
	return accounts.find((a) => a._id === chosenId) ?? accounts.find((a) => a.isDefault) ?? accounts[0];
}

/** Why nothing can be sent through an account right now, or '' when it can. */
export function accountSendProblem(a: ZaloAccount | undefined): string {
	if (!a || a.connected) return '';
	return `Tài khoản Zalo "${a.label}" chưa kết nối. Hãy kết nối lại trước khi gửi.`;
}
