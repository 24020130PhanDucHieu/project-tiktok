import { describe, expect, it } from 'vitest';
import type { Creator } from '../../types/koc';
import { buildRecipients, filterRecipients } from './bulkMessage';

const NOW = new Date('2026-11-16T03:00:00Z');
const ago = (ms: number) => new Date(NOW.getTime() - ms).toISOString();
const HOUR = 3600 * 1000;
const DAY = 24 * HOUR;

const creator = (over: Record<string, unknown> = {}) => ({ id: 'k', name: 'Nguyễn Thị Lan', phone: '0976451571', ...over }) as unknown as Creator;

describe('buildRecipients', () => {
	const one = (over: Record<string, unknown>) => buildRecipients([creator(over)], NOW)[0];

	it('blocksAKocWhoAskedNotToBeContacted', () => {
		expect(one({ dnc: true })).toMatchObject({ checked: false, blocked: 'Không liên hệ' });
	});
	it('blocksAKocWhoAlreadyCollaborated', () => {
		expect(one({ daHopTac: true })).toMatchObject({ checked: false, blocked: 'Đã hợp tác' });
	});
	it('blocksAKocWithoutAPhone', () => {
		expect(one({ phone: '' })).toMatchObject({ checked: false, blocked: 'Chưa có số điện thoại' });
		expect(one({ phone: undefined })).toMatchObject({ checked: false, blocked: 'Chưa có số điện thoại' });
	});
	it('blocksAKocContactedInTheLast24Hours', () => {
		expect(one({ lastContactedAt: ago(2 * HOUR) })).toMatchObject({ checked: false, blocked: 'Vừa liên hệ trong 24 giờ' });
	});
	it('unticksButDoesNotBlockAKocContactedInTheLast7Days', () => {
		const r = one({ lastContactedAt: ago(3 * DAY + HOUR) });
		expect(r.checked).toBe(false);
		expect(r.blocked).toBeUndefined();
		expect(r.recentDaysAgo).toBe(3);
	});
	it('tickesAKocContactedLongAgoOrNever', () => {
		expect(one({ lastContactedAt: ago(10 * DAY) })).toMatchObject({ checked: true });
		expect(one({ lastContactedAt: null })).toMatchObject({ checked: true });
		expect(one({})).toMatchObject({ checked: true });
	});
});

describe('filterRecipients', () => {
	const list = buildRecipients(
		[creator({ id: '1', name: 'Nguyễn Thị Lan', handle: '@lan.beauty' }), creator({ id: '2', name: 'Minh Review', handle: '@minh.rv', phone: '0912345678' })],
		NOW,
	);
	it('matchesNameHandleOrPhoneWithoutCaringAboutAccentsOrCase', () => {
		expect(filterRecipients(list, 'nguyen').map((r) => r.creator.id)).toEqual(['1']);
		expect(filterRecipients(list, 'MINH.RV').map((r) => r.creator.id)).toEqual(['2']);
		expect(filterRecipients(list, '0912').map((r) => r.creator.id)).toEqual(['2']);
	});
	it('returnsEveryoneForAnEmptyQuery', () => {
		expect(filterRecipients(list, '  ')).toHaveLength(2);
	});
});

