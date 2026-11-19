import { describe, expect, it } from 'vitest';
import { edgeFlow, progressOf } from './flow';

describe('edgeFlow', () => {
	it('isDoneWhenTheBlockAtTheEndIsDone', () => {
		expect(edgeFlow('done', 'done', true)).toBe('done');
	});
	it('isActiveWhileTheBlockAtTheEndWorksOrWaitsOrIsAboutToGetTheKocs', () => {
		expect(edgeFlow('done', 'running', true)).toBe('active');
		expect(edgeFlow('done', 'waiting', true)).toBe('active');
		expect(edgeFlow('done', undefined, true)).toBe('active');
		expect(edgeFlow('running', undefined, true)).toBe('active');
	});
	it('isIdleWhenNothingCameThroughOrTheRunIsOver', () => {
		expect(edgeFlow(undefined, undefined, true)).toBe('idle');
		expect(edgeFlow('done', undefined, false)).toBe('idle');
		expect(edgeFlow('done', 'stopped', false)).toBe('idle');
	});
});

describe('progressOf', () => {
	it('countsTheBlocksThatAreDone', () => {
		const p = progressOf([{ id: 'a' }, { id: 'b' }, { id: 'c' }], [{ nodeId: 'a', status: 'done' }, { nodeId: 'b', status: 'running' }]);
		expect(p).toEqual({ done: 1, total: 3, percent: 33 });
	});
	it('isZeroWithoutBlocks', () => {
		expect(progressOf([], [])).toEqual({ done: 0, total: 0, percent: 0 });
	});
});
