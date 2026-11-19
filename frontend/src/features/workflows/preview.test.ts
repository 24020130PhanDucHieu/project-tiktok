import { describe, expect, it } from 'vitest';
import type { WorkflowEdge, WorkflowNode } from '../../types/workflow';
import { previewEffects } from './preview';

const n = (id: string, type: WorkflowNode['type'], over: Partial<WorkflowNode> = {}): WorkflowNode => ({ id, type, x: 0, y: 0, ...over });
const e = (from: string, to: string, port: WorkflowEdge['port'] = 'out'): WorkflowEdge => ({ id: `${from}>${to}`, from, port, to });

describe('previewEffects', () => {
	it('tellsInOrderWhatEveryBlockAfterTheStartWillDoToHowManyKocs', () => {
		const nodes = [n('p', 'pick'), n('w', 'wait', { waitMinutes: 60 }), n('s', 'stage', { stage: 'SOURCED' }), n('m', 'email', { subject: 'Hi', text: 'x' })];
		const edges = [e('p', 'w'), e('w', 's'), e('s', 'm')];
		const fx = previewEffects(nodes, edges, 3);
		expect(fx.lines).toEqual([
			'Chờ 1 giờ',
			'Đổi giai đoạn sang Mới tìm cho 3 KOC',
			'Gửi email cho tối đa 3 KOC (người đã được liên hệ trong 24 giờ qua sẽ bị bỏ qua)',
		]);
		expect(fx.sends).toBe(true);
	});

	it('saysSoWhenNothingChangesAnyone', () => {
		const fx = previewEffects([n('p', 'pick'), n('w', 'wait', { waitMinutes: 5 })], [e('p', 'w')], 2);
		expect(fx.sends).toBe(false);
		expect(fx.lines).toEqual(['Chờ 5 phút']);
	});

	it('countsAtMostAfterTheReplyCheckBecauseWhoAnswersIsNotKnownYet', () => {
		const nodes = [n('p', 'pick'), n('r', 'if_replied'), n('t', 'send_text', { text: 'a' })];
		const fx = previewEffects(nodes, [e('p', 'r'), e('r', 't', 'unanswered')], 4);
		expect(fx.lines).toEqual(['Tách KOC đã trả lời và chưa trả lời', 'Gửi tin Zalo cho tối đa 4 KOC (ở nhánh "chưa trả lời")']);
	});
});
