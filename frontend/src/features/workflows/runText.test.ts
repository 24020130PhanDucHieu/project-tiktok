import { describe, expect, it } from 'vitest';
import type { Workflow } from '../../types/workflow';
import { SWITCH_HINT, finishText, markWorkflowSeen, seenWorkflowId, shouldShowFinished, workflowStopText } from './runText';

const base = (over: Partial<Workflow> = {}): Workflow => ({
	_id: 'w1', name: 'Mời rồi nhắc', nodes: [], edges: [], status: 'done', by: 'a@x.vn', createdAt: '2026-11-20T01:00:00Z', ...over,
});

describe('workflowStopText', () => {
	it('hasASentenceForEveryReasonTheServerGives', () => {
		const reasons = [
			'stopped_by_user', 'expired', 'import_busy', 'send_busy', 'import_failed', 'send_failed', 'internal_error',
			'import_stopped:rate_limited', 'import_stopped:tiktok_not_connected', 'import_stopped:stopped_by_user',
			'send_stopped:switch_off', 'send_stopped:zalo_errors', 'send_stopped:zalo_not_ready', 'send_stopped:store_unavailable', 'send_stopped:internal_error',
		];
		for (const r of reasons) {
			const text = workflowStopText(r);
			expect(text, r).not.toBe('');
			expect(text, r).not.toContain('_'); // a sentence, not the code
		}
	});

	it('explainsTheImportReasonInsideTheImportSentence', () => {
		expect(workflowStopText('import_stopped:rate_limited')).toContain('TikTok');
		expect(workflowStopText('send_stopped:switch_off')).toContain('công tắc');
	});

	it('givesAGeneralSentenceForAReasonItDoesNotKnowAndNothingForNoReason', () => {
		expect(workflowStopText('something_new')).toBe('Quy trình đã dừng (something_new).');
		expect(workflowStopText(undefined)).toBe('');
		expect(workflowStopText('')).toBe('');
	});
});

describe('SWITCH_HINT', () => {
	it('tellsThatTheSwitchIsNotTheStopButtonOfAWorkflow', () => {
		// the switch of "Gửi hàng loạt" turns itself off after every run, so between two runs of a workflow it says nothing:
		// the only way to stop the next run is the stop button of the workflow
		expect(SWITCH_HINT).toContain('Gửi hàng loạt');
		expect(SWITCH_HINT).toContain('Dừng quy trình');
	});
});

describe('finishText', () => {
	it('summarisesADoneWorkflowFromTheCountsOnItsBlocks', () => {
		const wf = base({
			nodes: [{ id: 'i', type: 'import', x: 0, y: 0 }, { id: 's', type: 'send', x: 0, y: 0 }, { id: 's2', type: 'send', x: 0, y: 0 }],
			states: [
				{ nodeId: 'i', status: 'done', count: 30 },
				{ nodeId: 's', status: 'done', count: 28 },
				{ nodeId: 's2', status: 'done', count: 12 },
			],
		});
		const r = finishText(wf);
		expect(r.type).toBe('success');
		expect(r.message).toContain('30');
		expect(r.message).toContain('28');
		expect(r.message).toContain('12');
	});

	it('isAnErrorWithTheReasonForAStoppedWorkflow', () => {
		const r = finishText(base({ status: 'stopped', stopReason: 'expired' }));
		expect(r.type).toBe('error');
		expect(r.message).toContain('14 ngày');
	});
});

describe('shouldShowFinished', () => {
	const now = new Date('2026-11-21T01:00:00Z');
	it('offersAnEndedWorkflowOnceUntilItIsSeen', () => {
		const wf = base({ finishedAt: '2026-11-20T23:00:00Z' });
		expect(shouldShowFinished(wf, '', now)).toBe(true);
		expect(shouldShowFinished(wf, 'w1', now)).toBe(false);
	});
	it('neverOffersARunningOneAsFinishedAndForgetsOldOnes', () => {
		expect(shouldShowFinished(base({ status: 'running' }), '', now)).toBe(false);
		expect(shouldShowFinished(base({ finishedAt: '2026-11-01T00:00:00Z' }), '', now)).toBe(false);
	});
});

describe('seen workflow id', () => {
	it('survivesAStorageThatThrows', () => {
		const original = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
		Object.defineProperty(globalThis, 'localStorage', { configurable: true, get() { throw new Error('blocked'); } });
		try {
			expect(seenWorkflowId()).toBe('');
			expect(() => markWorkflowSeen('w1')).not.toThrow();
		} finally {
			if (original) Object.defineProperty(globalThis, 'localStorage', original);
			else delete (globalThis as { localStorage?: unknown }).localStorage;
		}
	});
	it('keepsTheIdThatWasMarked', () => {
		const store: Record<string, string> = {};
		Object.defineProperty(globalThis, 'localStorage', {
			configurable: true,
			value: { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; } },
		});
		try {
			markWorkflowSeen('w9');
			expect(seenWorkflowId()).toBe('w9');
		} finally {
			delete (globalThis as { localStorage?: unknown }).localStorage;
		}
	});
});
