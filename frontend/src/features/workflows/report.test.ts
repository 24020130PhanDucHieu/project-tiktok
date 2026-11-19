import { describe, expect, it } from 'vitest';
import type { Workflow, WorkflowEvent } from '../../types/workflow';
import { buildReport } from './report';

const ev = (at: string, nodeId: string, kind: WorkflowEvent['kind'], kocs: string[], port?: string): WorkflowEvent => ({ at: `2026-11-19T10:${at}Z`, nodeId, kind, kocs, ...(port ? { port } : {}) });

function wf(over: Partial<Workflow>): Workflow {
	return {
		_id: 'w', name: 'T', status: 'done', by: 'a', createdAt: '', nodes: [], edges: [],
		kocNames: { a: 'Lan Beauty', b: 'Minh Review', c: '@thu.shop' }, ...over,
	};
}

describe('buildReport', () => {
	it('writesOneLineForEveryThingThatHappenedInTheOrderItHappenedWithNamesNotIds', () => {
		const w = wf({
			nodes: [{ id: 'p', type: 'pick', x: 0, y: 0 }, { id: 's', type: 'stage', x: 0, y: 0, stage: 'DORMANT' }],
			log: [ev('00:03', 's', 'did', ['a']), ev('00:01', 'p', 'out', ['a', 'b'], 'out'), ev('00:02', 's', 'in', ['a', 'b'])],
		});
		const lines = buildReport(w);
		expect(lines.map((l) => `${l.title}|${l.text}|${l.names.join(',')}`)).toEqual([
			'Lấy KOC có sẵn|Lấy được 2 KOC|Lan Beauty,Minh Review',
			'Đổi giai đoạn|Nhận 2 KOC|Lan Beauty,Minh Review',
			'Đổi giai đoạn|Đã đổi sang Ngủ đông: 1 KOC|Lan Beauty',
		]);
	});

	it('saysWhoWasSkippedAndWhichBranchTheReplyCheckSent', () => {
		const w = wf({
			nodes: [{ id: 'r', type: 'if_replied', x: 0, y: 0 }],
			log: [ev('00:01', 'r', 'out', ['a'], 'replied'), ev('00:01', 'r', 'out', ['b'], 'unanswered'), ev('00:01', 'r', 'skip', ['c'])],
		});
		const texts = buildReport(w).map((l) => `${l.text}|${l.names.join(',')}|${l.tone}`);
		expect(texts).toEqual(['Đã trả lời, đi nhánh "đã trả lời": 1 KOC|Lan Beauty|ok', 'Chưa trả lời, đi nhánh "chưa trả lời": 1 KOC|Minh Review|ok', 'Bỏ qua: 1 KOC|@thu.shop|skip']);
	});

	it('namesAKocWhoseNameIsUnknownInsteadOfShowingAnId', () => {
		const w = wf({ nodes: [{ id: 'p', type: 'pick', x: 0, y: 0 }], log: [ev('00:01', 'p', 'out', ['zzz'], 'out')] });
		expect(buildReport(w)[0].names).toEqual(['KOC không còn trong danh sách']);
	});

	it('isEmptyWhenNothingHasHappenedYetAndSkipsBlocksThatAreNotInTheGraph', () => {
		expect(buildReport(wf({}))).toEqual([]);
		expect(buildReport(wf({ nodes: [], log: [ev('00:01', 'gone', 'in', ['a'])] }))).toEqual([]);
	});
});
