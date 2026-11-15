import { describe, expect, it } from 'vitest';
import { formatLastContacted } from './formatters';

const NOW = new Date('2026-11-16T03:00:00Z');
const HOUR = 3600 * 1000;
const DAY = 24 * HOUR;
const ago = (ms: number) => new Date(NOW.getTime() - ms).toISOString();

describe('formatLastContacted', () => {
	it('saysNeverWhenThereIsNoDateOrTheDateIsNotOne', () => {
		expect(formatLastContacted(undefined, NOW)).toBe('Chưa liên hệ');
		expect(formatLastContacted(null, NOW)).toBe('Chưa liên hệ');
		expect(formatLastContacted('không phải ngày', NOW)).toBe('Chưa liên hệ');
	});
	it('saysTodayWithinTheLast24Hours', () => {
		expect(formatLastContacted(ago(2 * HOUR), NOW)).toBe('Hôm nay');
	});
	it('countsWholeDaysUpToTwentyNine', () => {
		expect(formatLastContacted(ago(3 * DAY + HOUR), NOW)).toBe('3 ngày trước');
		expect(formatLastContacted(ago(29 * DAY + HOUR), NOW)).toBe('29 ngày trước');
	});
	it('showsTheVietnamDateFromThirtyDays', () => {
		// 45 days before 2026-11-16 10:00 (Vietnam) is 2026-10-02 10:00 (Vietnam)
		expect(formatLastContacted(ago(45 * DAY), NOW)).toBe('02/10/2026');
	});
});
