import { describe, expect, it } from 'vitest';
import type { BulkRun, Step } from '../../types/campaign';
import {
	type DraftStep,
	MAX_STEPS,
	addStep,
	editStep,
	estimateMinutes,
	formatDuration,
	moveStep,
	previewLine,
	removeStep,
	sameSteps,
	runView,
	sendBlockReason,
	statusLabel,
	stepProblem,
	stepsToSave,
	stepsToSend,
	reasonText,
	startedToday,
	stopText,
	targetText,
	toDraft,
	toggleStep,
} from './campaignSteps';

const text = (t: string): Step => ({ kind: 'text', text: t });
const picture = (id?: string): Step => ({ kind: 'image', imageId: id, caption: 'Ảnh {kenh}' });
const texts = (d: DraftStep[]) => d.map((s) => s.text);

describe('toDraft and toggleStep', () => {
	it('turns every step on, but not a picture that has no image yet', () => {
		const d = toDraft([text('a'), picture(), picture('p1')]);
		expect(d.map((s) => s.on)).toEqual([true, false, true]);
		expect(new Set(d.map((s) => s.key)).size).toBe(3);
	});

	it('does not turn on a picture step without an image', () => {
		const d = toDraft([picture()]);
		expect(toggleStep(d, d[0].key)[0].on).toBe(false);
	});

	it('turns a text step off and on again', () => {
		const d = toDraft([text('a')]);
		const off = toggleStep(d, d[0].key);
		expect(off[0].on).toBe(false);
		expect(toggleStep(off, d[0].key)[0].on).toBe(true);
	});
});

describe('moveStep', () => {
	it('moves a step and keeps the others in order', () => {
		const d = toDraft([text('a'), text('b'), text('c')]);
		expect(texts(moveStep(d, 0, 2))).toEqual(['b', 'c', 'a']);
		expect(texts(moveStep(d, 2, 1))).toEqual(['a', 'c', 'b']);
	});

	it('returns the list as it is for a position outside it', () => {
		const d = toDraft([text('a'), text('b')]);
		expect(moveStep(d, 0, 5)).toBe(d);
		expect(moveStep(d, -1, 0)).toBe(d);
	});
});

describe('editStep, addStep, removeStep', () => {
	it('edits one step by its key', () => {
		const d = toDraft([text('a'), text('b')]);
		expect(texts(editStep(d, d[1].key, { text: 'B' }))).toEqual(['a', 'B']);
	});

	it('adds a text step on, and a picture step off, at the end', () => {
		const d = toDraft([text('a')]);
		const withText = addStep(d, 'text');
		expect(withText).toHaveLength(2);
		expect(withText[1]).toMatchObject({ kind: 'text', text: '', on: true });
		expect(addStep(d, 'image')[1]).toMatchObject({ kind: 'image', on: false });
	});

	it('never goes over the maximum', () => {
		let d = toDraft([text('a')]);
		for (let i = 0; i < 20; i++) d = addStep(d, 'text');
		expect(d).toHaveLength(MAX_STEPS);
	});

	it('removes a step', () => {
		const d = toDraft([text('a'), text('b')]);
		expect(texts(removeStep(d, d[0].key))).toEqual(['b']);
	});
});

describe('stepsToSend', () => {
	it('keeps the steps that are on, in their order, without the editor fields', () => {
		let d = toDraft([text('a'), text('b'), text('c')]);
		d = toggleStep(d, d[1].key);
		d = moveStep(d, 2, 0);
		const out = stepsToSend(d);
		expect(out).toEqual([
			{ kind: 'text', text: 'c' },
			{ kind: 'text', text: 'a' },
		]);
		expect(Object.keys(out[0])).not.toContain('key');
	});

	it('sends the image id and the caption of a picture', () => {
		expect(stepsToSend(toDraft([picture('p1')]))).toEqual([{ kind: 'image', imageId: 'p1', caption: 'Ảnh {kenh}' }]);
	});
});

describe('stepProblem', () => {
	it('asks for at least one step', () => {
		expect(stepProblem([])).toBe('Hãy tích ít nhất một tin.');
		expect(stepProblem(toDraft([text('a')]).map((s) => ({ ...s, on: false })))).toBe('Hãy tích ít nhất một tin.');
	});

	it('names the step that is empty, by its position in the list', () => {
		expect(stepProblem(toDraft([text('a'), text('  ')]))).toBe('Tin số 2 đang trống.');
	});

	it('does not complain about an empty step that is off', () => {
		const d = toDraft([text('a'), text('')]).map((s, i) => (i === 1 ? { ...s, on: false } : s));
		expect(stepProblem(d)).toBeNull();
	});

	it('refuses a text over 2000 characters', () => {
		expect(stepProblem(toDraft([text('x'.repeat(2001))]))).toBe('Tin số 1 dài quá 2000 ký tự.');
	});
});

describe('previewLine', () => {
	const koc = { name: 'Nguyễn Thị Lan', handle: '@lan99' };

	it('fills {ten} with the last word of the name and {kenh} with the channel', () => {
		expect(previewLine(text('Chào {ten}, kênh {kenh}'), koc)).toBe('Chào Lan, kênh @lan99');
	});

	it('greets a KOC without a name as bạn and leaves {kenh} empty without a channel', () => {
		expect(previewLine(text('Chào {ten}{kenh}'), { name: ' ', handle: '' })).toBe('Chào bạn');
	});

	it('shows a picture as Ảnh with its caption', () => {
		expect(previewLine(picture('p1'), koc)).toBe('Ảnh · Ảnh @lan99');
		expect(previewLine({ kind: 'image', imageId: 'p1' }, koc)).toBe('Ảnh');
	});
});

describe('estimateMinutes', () => {
	it('is about 3.5 hours for 30 KOCs of 9 steps, like the server estimate', () => {
		const m = estimateMinutes(30, 9);
		expect(m).toBeGreaterThanOrEqual(200);
		expect(m).toBeLessThanOrEqual(215);
	});
});

describe('sendBlockReason', () => {
	const ok = { bulkOn: true, isAdmin: true, selected: 5, problem: null, running: false, limitLeft: 30 };

	it('is null when everything is ready', () => {
		expect(sendBlockReason(ok)).toBeNull();
	});

	it('tells the reasons in the order the user should fix them', () => {
		expect(sendBlockReason({ ...ok, running: true, bulkOn: false })).toMatch(/đợt gửi/);
		expect(sendBlockReason({ ...ok, bulkOn: false })).toMatch(/Bật công tắc/);
		expect(sendBlockReason({ ...ok, bulkOn: false, isAdmin: false })).toMatch(/quản trị viên/);
		expect(sendBlockReason({ ...ok, selected: 0, problem: 'x' })).toMatch(/chọn ít nhất/);
		expect(sendBlockReason({ ...ok, problem: 'Tin số 2 đang trống.' })).toBe('Tin số 2 đang trống.');
		expect(sendBlockReason({ ...ok, selected: 12, limitLeft: 10 })).toMatch(/còn 10 KOC/);
		expect(sendBlockReason({ ...ok, selected: 51, limitLeft: 100 })).toMatch(/50 KOC/);
	});
});

describe('targetText, statusLabel and stopText', () => {
	it('has a label for every status', () => {
		for (const s of ['queued', 'sending', 'sent', 'replied', 'partial', 'skipped', 'failed', 'unknown', 'not_sent']) {
			expect(statusLabel(s)).not.toBe(s);
		}
	});

	it('adds the reason in words', () => {
		expect(targetText('skipped', 'no_phone')).toBe('Bỏ qua · Chưa có số điện thoại');
		expect(targetText('sent')).toBe('Đã gửi đủ');
		expect(targetText('unknown', 'step_outcome_unknown')).toMatch(/^Không rõ/);
	});

	it('has a sentence for every reason a run can stop for', () => {
		for (const r of ['switch_off', 'outside_0830_2030_window', 'zalo_not_ready', 'zalo_errors', 'daily_koc_limit_reached', 'daily_message_cap_reached', 'internal_error', 'store_unavailable']) {
			expect(stopText(r)).not.toBe(r);
		}
		expect(stopText(undefined)).toBe('');
	});
});

describe('runView', () => {
	const base: BulkRun = {
		_id: 'r1',
		campaignName: 'c',
		steps: [text('a'), text('b'), text('c'), text('d')],
		status: 'running',
		by: 'x',
		createdAt: '2026-11-16T03:00:00Z',
		targets: [
			{ kocId: '1', kocName: 'A', handle: '@a', status: 'sent', stepsSent: 4 },
			{ kocId: '2', kocName: 'B', handle: '@b', status: 'sending', stepsSent: 1 },
			{ kocId: '3', kocName: 'C', handle: '@c', status: 'queued', stepsSent: 0 },
		],
	};

	it('counts the KOCs that are finished and says which step is going out', () => {
		const v = runView(base, new Date('2026-11-16T03:10:00Z'));
		expect(v.done).toBe(1);
		expect(v.total).toBe(3);
		expect(v.current).toEqual({ name: 'B', step: 2, steps: 4 });
		expect(v.counts).toMatchObject({ sent: 1, sending: 1, queued: 1 });
	});

	it('rounds the wait for the next KOC up to whole minutes, and drops a wait that is over', () => {
		const waiting = { ...base, targets: base.targets.map((t) => (t.status === 'sending' ? { ...t, status: 'sent', stepsSent: 4 } : t)), nextAt: '2026-11-16T03:14:20Z' };
		expect(runView(waiting, new Date('2026-11-16T03:10:00Z')).nextInMinutes).toBe(5);
		expect(runView(waiting, new Date('2026-11-16T03:20:00Z')).nextInMinutes).toBeUndefined();
		expect(runView(waiting, new Date('2026-11-16T03:10:00Z')).current).toBeUndefined();
	});
});

describe('stepsToSave', () => {
	it('keeps every step in order, the ones that are off included, without the editor fields', () => {
		let d = toDraft([text('a'), text('b')]);
		d = toggleStep(d, d[0].key);
		expect(stepsToSave(d)).toEqual([
			{ kind: 'text', text: 'a' },
			{ kind: 'text', text: 'b' },
		]);
	});
});

describe('startedToday', () => {
	const run = (startedAt?: string): BulkRun => ({
		_id: 'r',
		campaignName: 'c',
		steps: [],
		status: 'done',
		by: 'x',
		createdAt: '2026-11-16T03:00:00Z',
		targets: [{ kocId: '1', kocName: 'A', handle: '@a', status: 'sent', stepsSent: 1, startedAt }],
	});

	it('counts the KOCs started on today\'s Vietnam date and not the ones of yesterday or never started', () => {
		const now = new Date('2026-11-16T03:00:00Z'); // 10:00 in Vietnam
		expect(startedToday([run('2026-11-16T02:00:00Z'), run('2026-11-15T10:00:00Z'), run(undefined)], now)).toBe(1);
	});

	it('goes by the Vietnam day, which starts at 17:00 UTC', () => {
		const now = new Date('2026-11-16T18:00:00Z'); // 01:00 on the 17th in Vietnam
		expect(startedToday([run('2026-11-16T16:00:00Z'), run('2026-11-16T17:30:00Z')], now)).toBe(1);
	});
});

describe('formatDuration', () => {
	it('writes minutes and hours for people', () => {
		expect(formatDuration(45)).toBe('45 phút');
		expect(formatDuration(208)).toBe('3 giờ 28 phút');
		expect(formatDuration(120)).toBe('2 giờ');
		expect(formatDuration(0)).toBe('0 phút');
	});
});

describe('sameSteps', () => {
	it('compares the steps in order and treats a missing caption as an empty one', () => {
		expect(sameSteps([text('a'), { kind: 'image', imageId: 'p' }], [text('a'), { kind: 'image', imageId: 'p', caption: '' }])).toBe(true);
		expect(sameSteps([text('a'), text('b')], [text('b'), text('a')])).toBe(false);
		expect(sameSteps([text('a')], [text('a'), text('b')])).toBe(false);
	});
});

describe('startedToday for one Zalo account', () => {
	const run = (zaloAccountId: string | undefined, startedAt: string): BulkRun => ({
		_id: 'r',
		campaignName: 'c',
		steps: [],
		status: 'done',
		by: 'x',
		createdAt: '2026-11-16T03:00:00Z',
		zaloAccountId,
		targets: [{ kocId: '1', kocName: 'A', handle: '@a', status: 'sent', stepsSent: 1, startedAt }],
	});
	const now = new Date('2026-11-16T03:00:00Z');
	const today = '2026-11-16T02:00:00Z';

	it('counts only the runs of that account, each account has its own limit', () => {
		const runs = [run('A', today), run('A', today), run('B', today)];
		expect(startedToday(runs, now, 'A')).toBe(2);
		expect(startedToday(runs, now, 'B')).toBe(1);
		expect(startedToday(runs, now, 'C')).toBe(0);
	});

	it('counts every run when no account is asked for', () => {
		expect(startedToday([run('A', today), run('B', today)], now)).toBe(2);
	});

	it('files a run made before there were accounts under the default account', () => {
		const runs = [run(undefined, today), run('B', today)];
		expect(startedToday(runs, now, 'A', 'A')).toBe(1); // A is the default: the old run is its
		expect(startedToday(runs, now, 'B', 'A')).toBe(1);
	});

	it('does not file an old run under anyone when the default is not known', () => {
		expect(startedToday([run(undefined, today)], now, 'A')).toBe(0);
	});
});

describe('the reason a KOC was left out because of another account', () => {
	it('says it in words', () => {
		expect(reasonText('on_other_account')).toBe('Đang nói chuyện qua tài khoản Zalo khác');
	});
});
