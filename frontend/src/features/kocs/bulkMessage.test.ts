import { describe, expect, it } from 'vitest';
import type { Creator } from '../../types/koc';
import {
	type BatchResponse,
	MSG_BATCH,
	MSG_MAX_BODY,
	MSG_RATE_REASON,
	MSG_STOPPED_REASON,
	MSG_UNKNOWN_REASON,
	buildRecipients,
	filterRecipients,
	needsAttention,
	SUGGESTED_TEMPLATES,
	sendBlockReason,
	reasonText,
	renderPreview,
	runBulkMessage,
	shouldRefresh,
	type SendRow,
} from './bulkMessage';

const NOW = new Date('2026-11-16T03:00:00Z');
const ago = (ms: number) => new Date(NOW.getTime() - ms).toISOString();
const HOUR = 3600 * 1000;
const DAY = 24 * HOUR;

const creator = (over: Record<string, unknown> = {}) => ({ id: 'k', name: 'Nguyễn Thị Lan', phone: '0976451571', ...over }) as unknown as Creator;

describe('renderPreview', () => {
	it('putsTheLastWordOfTheNameWhereTheTagIs', () => {
		expect(renderPreview('Chào {ten}', creator())).toBe('Chào Lan');
	});
	it('greetsAKocWithoutANameAsBan', () => {
		expect(renderPreview('Chào {ten}!', creator({ name: '  ' }))).toBe('Chào bạn!');
	});
});

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

const rows = (n: number): SendRow[] => Array.from({ length: n }, (_, i) => ({ id: `k${i + 1}`, name: `KOC ${i + 1}`, status: 'queued' as const }));
const allSent = (ids: string[]): BatchResponse => ({ results: ids.map((kocId) => ({ kocId, status: 'sent' })), stop: false });
const statusOf = (out: SendRow[], id: string) => out.find((r) => r.id === id)!;

describe('runBulkMessage', () => {
	it('sendsInOrderInBatchesOfFiveOneAfterTheOther', async () => {
		const calls: string[][] = [];
		let inFlight = 0;
		let maxInFlight = 0;
		const progress: number[] = [];
		const out = await runBulkMessage(
			rows(12),
			async (ids) => {
				inFlight++;
				maxInFlight = Math.max(maxInFlight, inFlight);
				calls.push(ids);
				await Promise.resolve();
				inFlight--;
				return allSent(ids);
			},
			{ shouldStop: () => false, onProgress: (done) => progress.push(done) },
		);
		expect(MSG_BATCH).toBe(5);
		expect(calls.map((c) => c.length)).toEqual([5, 5, 2]);
		expect(calls[0]).toEqual(['k1', 'k2', 'k3', 'k4', 'k5']);
		expect(maxInFlight).toBe(1);
		expect(out.every((r) => r.status === 'sent')).toBe(true);
		expect(progress[progress.length - 1]).toBe(12);
	});

	it('mergesTheServersResultsByKocId', async () => {
		const out = await runBulkMessage(
			rows(2),
			async () => ({ results: [{ kocId: 'k1', status: 'sent' }, { kocId: 'k2', status: 'skipped', reason: 'no_phone' }], stop: false }),
			{ shouldStop: () => false, onProgress: () => {} },
		);
		expect(statusOf(out, 'k1').status).toBe('sent');
		expect(statusOf(out, 'k2')).toMatchObject({ status: 'skipped', reason: 'Chưa có số điện thoại' });
	});

	it('stopsCallingTheServerWhenItSaysStop', async () => {
		let calls = 0;
		const out = await runBulkMessage(
			rows(12),
			async (ids) => {
				calls++;
				return { results: ids.map((kocId, i) => ({ kocId, status: i === 0 ? 'sent' : 'not_sent', reason: i === 0 ? undefined : 'daily_quota_reached' })), stop: true, stopReason: 'daily_quota_reached' };
			},
			{ shouldStop: () => false, onProgress: () => {} },
		);
		expect(calls).toBe(1);
		expect(statusOf(out, 'k1').status).toBe('sent');
		expect(statusOf(out, 'k6')).toMatchObject({ status: 'not_sent', reason: reasonText('daily_quota_reached') });
		expect(statusOf(out, 'k12').status).toBe('not_sent');
	});

	it('stopsWhenTheUserPressesStop', async () => {
		let stop = false;
		let calls = 0;
		const out = await runBulkMessage(
			rows(12),
			async (ids) => {
				calls++;
				stop = true; // pressed during the first batch
				return allSent(ids);
			},
			{ shouldStop: () => stop, onProgress: () => {} },
		);
		expect(calls).toBe(1);
		expect(statusOf(out, 'k5').status).toBe('sent');
		expect(statusOf(out, 'k6')).toMatchObject({ status: 'not_sent', reason: MSG_STOPPED_REASON });
	});

	it('marksTheBatchUnknownAndStopsOnANetworkError', async () => {
		let calls = 0;
		const out = await runBulkMessage(
			rows(12),
			async () => {
				calls++;
				throw new Error('Network Error');
			},
			{ shouldStop: () => false, onProgress: () => {} },
		);
		expect(calls).toBe(1);
		expect(statusOf(out, 'k1')).toMatchObject({ status: 'unknown', reason: MSG_UNKNOWN_REASON });
		expect(statusOf(out, 'k5').status).toBe('unknown');
		expect(statusOf(out, 'k6').status).toBe('not_sent');
	});

	it('treatsAServerErrorAnswerAsUnknownToo', async () => {
		const out = await runBulkMessage(
			rows(3),
			async () => {
				throw { response: { status: 502, data: { message: 'Bad Gateway' } } };
			},
			{ shouldStop: () => false, onProgress: () => {} },
		);
		expect(statusOf(out, 'k1').status).toBe('unknown');
	});

	it('marksTheBatchNotSentWithTheServersMessageOnA409AndStops', async () => {
		let calls = 0;
		const out = await runBulkMessage(
			rows(12),
			async () => {
				calls++;
				throw { response: { status: 409, data: { message: 'Công tắc gửi tin tự động đang tắt.' } } };
			},
			{ shouldStop: () => false, onProgress: () => {} },
		);
		expect(calls).toBe(1);
		expect(statusOf(out, 'k1')).toMatchObject({ status: 'not_sent', reason: 'Công tắc gửi tin tự động đang tắt.' });
		expect(statusOf(out, 'k12').status).toBe('not_sent');
	});

	it('explainsA429AndStops', async () => {
		const out = await runBulkMessage(
			rows(7),
			async () => {
				throw { response: { status: 429, data: { message: 'x' } } };
			},
			{ shouldStop: () => false, onProgress: () => {} },
		);
		expect(statusOf(out, 'k1')).toMatchObject({ status: 'not_sent', reason: MSG_RATE_REASON });
		expect(statusOf(out, 'k7').status).toBe('not_sent');
	});

	it('marksAnIdTheResponseLeftOutAsUnknown', async () => {
		const out = await runBulkMessage(rows(2), async () => ({ results: [{ kocId: 'k1', status: 'sent' }], stop: false }), {
			shouldStop: () => false,
			onProgress: () => {},
		});
		expect(statusOf(out, 'k1').status).toBe('sent');
		expect(statusOf(out, 'k2')).toMatchObject({ status: 'unknown', reason: MSG_UNKNOWN_REASON });
	});
});

describe('a message that went out but was not recorded', () => {
	it('givesTheRestAStopSentenceOfTheirOwnNotTheSentOne', async () => {
		const out = await runBulkMessage(
			rows(3),
			async () => ({
				results: [
					{ kocId: 'k1', status: 'sent', reason: 'message_not_recorded' },
					{ kocId: 'k2', status: 'not_sent', reason: 'message_not_recorded' },
					{ kocId: 'k3', status: 'not_sent', reason: 'message_not_recorded' },
				],
				stop: true,
				stopReason: 'message_not_recorded',
			}),
			{ shouldStop: () => false, onProgress: () => {} },
		);
		expect(statusOf(out, 'k1').reason).toBe('Đã gửi nhưng chưa ghi lại được tin trong hội thoại');
		expect(statusOf(out, 'k3').reason).toBe('Chưa gửi, đã dừng vì tin trước chưa được ghi lại');
	});
});

describe('needsAttention', () => {
	it('keepsEveryRowThatIsNotSentAndASentRowThatCarriesAReason', () => {
		const list: SendRow[] = [
			{ id: 'a', name: 'A', status: 'sent' },
			{ id: 'b', name: 'B', status: 'sent', reason: 'Đã gửi nhưng chưa ghi lại được tin trong hội thoại' },
			{ id: 'c', name: 'C', status: 'skipped', reason: 'Chưa có số điện thoại' },
			{ id: 'd', name: 'D', status: 'unknown' },
		];
		expect(needsAttention(list).map((r) => r.id)).toEqual(['b', 'c', 'd']);
	});
});

describe('shouldRefresh', () => {
	it('isTrueWhenAnythingMayHaveGoneOutEvenIfTheOutcomeIsUnknown', () => {
		expect(shouldRefresh([{ id: 'a', name: 'A', status: 'unknown' }])).toBe(true);
		expect(shouldRefresh([{ id: 'a', name: 'A', status: 'pending' }])).toBe(true);
		expect(shouldRefresh([{ id: 'a', name: 'A', status: 'sent' }])).toBe(true);
	});
	it('isFalseWhenNothingWentOut', () => {
		expect(shouldRefresh([])).toBe(false);
		expect(shouldRefresh([{ id: 'a', name: 'A', status: 'not_sent' }, { id: 'b', name: 'B', status: 'skipped' }, { id: 'c', name: 'C', status: 'failed' }])).toBe(false);
	});
});

describe('SUGGESTED_TEMPLATES', () => {
	it('offersAFewTemplatesEachWithTheNameTag', () => {
		expect(SUGGESTED_TEMPLATES.length).toBeGreaterThanOrEqual(4);
		SUGGESTED_TEMPLATES.forEach((t) => {
			expect(t.name.trim()).not.toBe('');
			expect(t.body).toContain('{ten}');
			expect(t.body.length).toBeLessThanOrEqual(MSG_MAX_BODY);
		});
		expect(new Set(SUGGESTED_TEMPLATES.map((t) => t.id)).size).toBe(SUGGESTED_TEMPLATES.length);
	});
	it('followsTheProjectsWritingPolicyNeverGiftNeverPraiseRealUse', () => {
		SUGGESTED_TEMPLATES.forEach((t) => {
			expect(t.body.toLowerCase()).not.toMatch(/tặng/);
			expect(t.body.toLowerCase()).not.toMatch(/dùng thật/);
			expect(t.body).not.toMatch(/["“”]/); // plain sentences, no quotation marks
		});
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

describe('sendBlockReason', () => {
	const ok = { armed: true, switchOff: false, selected: 2, body: 'Chào {ten}' };
	it('isNullWhenEverythingIsReady', () => {
		expect(sendBlockReason(ok)).toBeNull();
	});
	it('saysWhyWhenSomethingIsMissingInThisOrder', () => {
		expect(sendBlockReason({ ...ok, body: '   ' })).toBe('Hãy soạn nội dung tin nhắn.');
		expect(sendBlockReason({ ...ok, selected: 0 })).toBe('Hãy chọn ít nhất một KOC.');
		expect(sendBlockReason({ ...ok, body: 'x'.repeat(MSG_MAX_BODY + 1) })).toBe('Nội dung quá dài.');
		expect(sendBlockReason({ ...ok, switchOff: true })).toBe('Công tắc gửi tự động đang tắt, nhờ quản trị viên bật.');
		expect(sendBlockReason({ ...ok, armed: false })).toBe('Bật công tắc "Cho phép gửi" để gửi.');
	});
});
