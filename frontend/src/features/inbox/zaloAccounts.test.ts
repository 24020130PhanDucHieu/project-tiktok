import { describe, expect, it } from 'vitest';
import type { ZaloAccount } from '../../types/zalo';
import {
	accountLabelOf,
	accountOfConversation,
	accountRemoved,
	accountSendProblem,
	accountStatusText,
	accountTone,
	filterByAccount,
	pickSendAccount,
	reconnectTarget,
	showAccountTag,
	switcherSummary,
} from './zaloAccounts';

const acc = (over: Partial<ZaloAccount> & { _id: string }): ZaloAccount => ({
	label: over._id,
	isDefault: false,
	disabled: false,
	state: 'connected',
	connected: true,
	bridgeOnline: true,
	hasCredentials: true,
	needsQr: false,
	...over,
});

describe('accountOfConversation', () => {
	const a = acc({ _id: 'A' });
	const b = acc({ _id: 'B', isDefault: true });

	it('is the account the conversation talks through', () => {
		expect(accountOfConversation([a, b], { zaloAccountId: 'A' })?._id).toBe('A');
	});
	it('is the default account for a conversation that has none yet', () => {
		expect(accountOfConversation([a, b], {})?._id).toBe('B');
		expect(accountOfConversation([a, b], null)?._id).toBe('B');
	});
	it('is the first account when none is marked default (as the server does)', () => {
		expect(accountOfConversation([acc({ _id: 'X' }), acc({ _id: 'Y' })], {})?._id).toBe('X');
	});
	it('is nothing for a conversation whose account was removed: the server does not fall back either, it cannot send', () => {
		expect(accountOfConversation([a, b], { zaloAccountId: 'gone' })).toBeUndefined();
	});
	it('is nothing when there are no accounts', () => {
		expect(accountOfConversation([], { zaloAccountId: 'A' })).toBeUndefined();
	});
});

describe('filterByAccount', () => {
	const list = [{ id: 1, zaloAccountId: 'A' }, { id: 2, zaloAccountId: 'B' }, { id: 3 }];
	it('keeps everything without a filter', () => {
		expect(filterByAccount(list, '')).toHaveLength(3);
	});
	it('keeps only the conversations of the account', () => {
		expect(filterByAccount(list, 'A').map((c) => c.id)).toEqual([1]);
	});
	it('does not mutate its input', () => {
		filterByAccount(list, 'A');
		expect(list).toHaveLength(3);
	});
});

describe('accountStatusText and accountTone', () => {
	it('says connected', () => {
		expect(accountStatusText(acc({ _id: 'A' }))).toBe('Đã kết nối');
		expect(accountTone(acc({ _id: 'A' }))).toBe('ok');
	});
	it('says when the bridge cannot be reached, whatever else is known', () => {
		const a = acc({ _id: 'A', bridgeOnline: false, connected: false, state: 'disconnected' });
		expect(accountStatusText(a)).toBe('Không kết nối được dịch vụ Zalo');
		expect(accountTone(a)).toBe('off');
	});
	it('says an account without a login needs its QR code', () => {
		const a = acc({ _id: 'A', connected: false, state: 'disconnected', hasCredentials: false, needsQr: true });
		expect(accountStatusText(a)).toBe('Cần quét mã QR');
	});
	it('says a login in progress', () => {
		for (const state of ['waiting_qr', 'scanned', 'connecting']) {
			const a = acc({ _id: 'A', connected: false, state, hasCredentials: false, needsQr: true });
			expect(accountStatusText(a)).toMatch(/Đang/);
			expect(accountTone(a)).toBe('warn');
		}
	});
	it('says an account a person disconnected', () => {
		expect(accountStatusText(acc({ _id: 'A', connected: false, state: 'disconnected', disabled: true }))).toBe('Đã ngắt kết nối');
	});
	it('says a refused login needs a new scan', () => {
		const a = acc({ _id: 'A', connected: false, state: 'error', error: 'Đăng nhập thất bại' });
		expect(accountStatusText(a)).toBe('Đăng nhập lỗi, cần quét lại mã QR');
		expect(accountTone(a)).toBe('off');
	});
	it('says lost connection otherwise', () => {
		expect(accountStatusText(acc({ _id: 'A', connected: false, state: 'disconnected' }))).toBe('Mất kết nối');
	});
});

describe('reconnectTarget', () => {
	const a = acc({ _id: 'A', connected: false, state: 'disconnected' });
	const b = acc({ _id: 'B' });
	it('is the account of the open conversation when that one is down', () => {
		expect(reconnectTarget([a, b], { zaloAccountId: 'A' })?._id).toBe('A');
	});
	it('is the first account that is down when the open conversation is fine', () => {
		expect(reconnectTarget([b, a], { zaloAccountId: 'B' })?._id).toBe('A');
	});
	it('is nothing when every account is connected', () => {
		expect(reconnectTarget([b], { zaloAccountId: 'B' })).toBeUndefined();
	});
});

describe('accountRemoved', () => {
	const a = acc({ _id: 'A' });
	it('is true for a conversation of an account that no longer exists', () => {
		expect(accountRemoved([a], { zaloAccountId: 'gone' })).toBe(true);
	});
	it('is false for a conversation of an account that exists, and for one with no account yet', () => {
		expect(accountRemoved([a], { zaloAccountId: 'A' })).toBe(false);
		expect(accountRemoved([a], {})).toBe(false);
		expect(accountRemoved([a], null)).toBe(false);
	});
	it('is false while the accounts are not loaded yet: not knowing is not the same as removed', () => {
		expect(accountRemoved([], { zaloAccountId: 'A' })).toBe(false);
	});
});

describe('the chip of the one account that has never been logged in', () => {
	it('says it needs its QR code, keeping the capitals of QR', () => {
		const fresh = acc({ _id: 'A', label: 'Tài khoản chính', connected: false, state: 'disconnected', hasCredentials: false, needsQr: true });
		expect(switcherSummary([fresh])).toEqual({ title: 'Tài khoản chính', sub: 'Zalo · cần quét mã QR', tone: 'off' });
	});
});

describe('showAccountTag, accountLabelOf, switcherSummary', () => {
	it('tags a conversation with its account only when there are several', () => {
		expect(showAccountTag([acc({ _id: 'A' })])).toBe(false);
		expect(showAccountTag([acc({ _id: 'A' }), acc({ _id: 'B' })])).toBe(true);
	});
	it('names an account, or says nothing for an unknown one', () => {
		expect(accountLabelOf([acc({ _id: 'A', label: 'Chính' })], 'A')).toBe('Chính');
		expect(accountLabelOf([], 'A')).toBe('');
		expect(accountLabelOf([acc({ _id: 'A' })], undefined)).toBe('');
	});
	it('sums up the accounts for the header', () => {
		expect(switcherSummary([acc({ _id: 'A', label: 'Chính' })])).toEqual({ title: 'Chính', sub: 'Zalo · đã kết nối', tone: 'ok' });
		expect(switcherSummary([acc({ _id: 'A', label: 'Chính', connected: false, state: 'disconnected' })]).tone).toBe('off');
		const two = switcherSummary([acc({ _id: 'A' }), acc({ _id: 'B', connected: false, state: 'disconnected' })]);
		expect(two).toEqual({ title: '2 tài khoản Zalo', sub: '1/2 đang kết nối', tone: 'warn' });
		expect(switcherSummary([acc({ _id: 'A' }), acc({ _id: 'B' })]).tone).toBe('ok');
		expect(switcherSummary([acc({ _id: 'A', connected: false }), acc({ _id: 'B', connected: false })]).tone).toBe('off');
	});
});

describe('pickSendAccount', () => {
	const a = acc({ _id: 'A' });
	const b = acc({ _id: 'B', isDefault: true });
	it('is the account asked for', () => {
		expect(pickSendAccount([a, b], 'A')?._id).toBe('A');
	});
	it('is the default account when none is asked for, or the one asked for is gone', () => {
		expect(pickSendAccount([a, b], '')?._id).toBe('B');
		expect(pickSendAccount([a, b], 'gone')?._id).toBe('B');
	});
	it('is the first account when none is marked default', () => {
		expect(pickSendAccount([acc({ _id: 'X' }), acc({ _id: 'Y' })], '')?._id).toBe('X');
	});
	it('is nothing without accounts', () => {
		expect(pickSendAccount([], 'A')).toBeUndefined();
	});
});

describe('accountSendProblem', () => {
	it('is empty for a connected account, and when there is no account to speak of', () => {
		expect(accountSendProblem(acc({ _id: 'A' }))).toBe('');
		expect(accountSendProblem(undefined)).toBe('');
	});
	it('asks to connect an account that is down before sending', () => {
		const p = accountSendProblem(acc({ _id: 'A', label: 'Zalo phụ', connected: false, state: 'disconnected' }));
		expect(p).toBe('Tài khoản Zalo "Zalo phụ" chưa kết nối. Hãy kết nối lại trước khi gửi.');
	});
});
