import { describe, expect, it } from 'vitest';
import type { WorkflowNode } from '../../types/workflow';
import { freeSpotRightOf, toRfNode, toWorkflowNode, withoutSettingsOfOtherTypes } from './convert';

describe('toRfNode and toWorkflowNode', () => {
	it('keepEveryHonestSettingOfEveryBlockOnTheRoundTrip', () => {
		const nodes: WorkflowNode[] = [
			{ id: 'a', type: 'import', x: 10, y: 20 },
			{ id: 'b', type: 'pick', x: 1, y: 2, stages: ['SHIPPING', 'DELIVERED'], folder: 'Làm đẹp', limit: 40 },
			{ id: 'c', type: 'wait', x: 3, y: 4, waitMinutes: 90 },
			{ id: 'd', type: 'wait_until', x: 5, y: 6, atMinute: 540 },
			{ id: 'e', type: 'send', x: 7, y: 8, campaignId: 'c1' },
			{ id: 'f', type: 'send_text', x: 9, y: 10, text: 'Chào {ten}' },
			{ id: 'g', type: 'email', x: 11, y: 12, subject: 'Hợp tác', text: 'Xin chào' },
			{ id: 'h', type: 'label', x: 13, y: 14, labelId: '65a1b2c3d4e5f60718293a4b' },
			{ id: 'i', type: 'stage', x: 15, y: 16, stage: 'NO_REPLY' },
			{ id: 'j', type: 'if_replied', x: 17, y: 18 },
		];
		for (const n of nodes) expect(toWorkflowNode(toRfNode(n))).toEqual(n);
	});

	it('putsTheBlockWhereItWasDrawnAndRoundsThePlaceItIsSavedAt', () => {
		const rf = toRfNode({ id: 'a', type: 'wait', x: 12.4, y: 7.6, waitMinutes: 5 });
		expect(rf.position).toEqual({ x: 12.4, y: 7.6 });
		expect(rf.type).toBe('wf');
		const back = toWorkflowNode({ ...rf, position: { x: 12.4, y: 7.6 } });
		expect(back.x).toBe(12);
		expect(back.y).toBe(8);
	});

	it('doesNotLetWhatIsOnlyForTheScreenReachTheServer', () => {
		const rf = toRfNode({ id: 'a', type: 'send', x: 0, y: 0, campaignId: 'c1' });
		rf.data = { ...rf.data, problems: ['x'], campaignName: 'Tên', nowMs: 5, usedPorts: ['out'] };
		const out = toWorkflowNode(rf);
		expect(out).toEqual({ id: 'a', type: 'send', x: 0, y: 0, campaignId: 'c1' });
	});
});

describe('withoutSettingsOfOtherTypes', () => {
	it('dropsTheSettingsTheTypeDoesNotUse', () => {
		const dirty = { id: 'a', type: 'wait', x: 0, y: 0, waitMinutes: 5, text: 'x', labelId: 'l', stage: 'REPLIED', campaignId: 'c' } as WorkflowNode;
		expect(withoutSettingsOfOtherTypes(dirty)).toEqual({ id: 'a', type: 'wait', x: 0, y: 0, waitMinutes: 5 });
	});
});

describe('freeSpotRightOf', () => {
	it('givesASpotToTheRightThatNoBlockStandsOn', () => {
		const taken = [{ x: 400, y: 100 }, { x: 400, y: 260 }];
		const spot = freeSpotRightOf({ x: 100, y: 100 }, taken);
		expect(spot.x).toBeGreaterThan(100);
		for (const t of taken) expect(Math.abs(t.x - spot.x) < 150 && Math.abs(t.y - spot.y) < 150).toBe(false);
	});
	it('putsItNextToTheBlockWhenThereIsRoom', () => {
		expect(freeSpotRightOf({ x: 100, y: 100 }, [])).toEqual({ x: 280, y: 100 });
	});
});

describe('the Zalo account of a send block', () => {
	it('survives the round trip for send and send_text blocks', () => {
		const nodes: WorkflowNode[] = [
			{ id: 's', type: 'send', x: 0, y: 0, campaignId: 'c1', zaloAccountId: 'acc-B' },
			{ id: 't', type: 'send_text', x: 0, y: 0, text: 'Chào', zaloAccountId: 'acc-B' },
		];
		for (const n of nodes) {
			expect(toWorkflowNode(toRfNode(n)).zaloAccountId).toBe('acc-B');
		}
	});

	it('is dropped from every other block, as the server does', () => {
		const dirty = { id: 'w', type: 'wait', x: 0, y: 0, waitMinutes: 5, zaloAccountId: 'acc-B' } as WorkflowNode;
		expect(withoutSettingsOfOtherTypes(dirty).zaloAccountId).toBeUndefined();
		expect(toWorkflowNode(toRfNode(dirty)).zaloAccountId).toBeUndefined();
	});
});
