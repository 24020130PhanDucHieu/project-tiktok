import { describe, expect, it } from 'vitest';
import type { WorkflowEdge, WorkflowNode, WorkflowTemplate } from '../../types/workflow';
import { agoText, canChange, fingerprint, loadDraftMeta, saveDraftMeta, saveState, templateToDraft } from './templates';

const n = (id: string, over: Partial<WorkflowNode> = {}): WorkflowNode => ({ id, type: 'wait', x: 0, y: 0, waitMinutes: 60, ...over });
const e = (from: string, to: string): WorkflowEdge => ({ id: `${from}>${to}`, from, port: 'out', to });

describe('fingerprint', () => {
	it('isTheSameForTheSameDrawingWhateverTheOrder', () => {
		const a = fingerprint([n('a'), n('b')], [e('a', 'b')]);
		const b = fingerprint([n('b'), n('a')], [e('a', 'b')]);
		expect(a).toBe(b);
	});

	it('changesWhenABlockMovesOrASettingChangesOrAWireIsAddedOrRemoved', () => {
		const base = fingerprint([n('a'), n('b')], [e('a', 'b')]);
		expect(fingerprint([n('a', { x: 5 }), n('b')], [e('a', 'b')])).not.toBe(base);
		expect(fingerprint([n('a', { waitMinutes: 61 }), n('b')], [e('a', 'b')])).not.toBe(base);
		expect(fingerprint([n('a'), n('b')], [])).not.toBe(base);
		expect(fingerprint([n('a'), n('b'), n('c')], [e('a', 'b')])).not.toBe(base);
	});

	it('ignoresWhatIsOnlyForTheScreenOrWasNeverSaved', () => {
		const withName = n('a', { campaignName: 'Tên do máy chủ' });
		expect(fingerprint([withName], [])).toBe(fingerprint([n('a')], []));
	});
});

describe('saveState', () => {
	it('saysWhetherTheDrawingIsSavedChangedOrNeverSaved', () => {
		const print = fingerprint([n('a')], []);
		expect(saveState(null, '', print)).toBe('draft');
		expect(saveState('t1', print, print)).toBe('saved');
		expect(saveState('t1', print, fingerprint([n('a', { x: 9 })], []))).toBe('changed');
	});
});

describe('canChange', () => {
	it('letsTheOwnerAndAnAdminChangeATemplate', () => {
		expect(canChange({ createdBy: 'a@x.vn' }, { email: 'a@x.vn', role: 'member' })).toBe(true);
		expect(canChange({ createdBy: 'a@x.vn' }, { email: 'b@x.vn', role: 'member' })).toBe(false);
		expect(canChange({ createdBy: 'a@x.vn' }, { email: 'b@x.vn', role: 'admin' })).toBe(true);
		expect(canChange({ createdBy: '' }, { email: '', role: 'member' })).toBe(false);
	});
});

describe('agoText', () => {
	const now = Date.parse('2026-11-20T10:00:00Z');
	const at = (seconds: number) => new Date(now - seconds * 1000).toISOString();
	it('writesHowLongAgoInTheLargestUnit', () => {
		expect(agoText(at(10), now)).toBe('vừa xong');
		expect(agoText(at(5 * 60), now)).toBe('5 phút trước');
		expect(agoText(at(2 * 3600), now)).toBe('2 giờ trước');
		expect(agoText(at(3 * 86400), now)).toBe('3 ngày trước');
		expect(agoText(at(40 * 86400), now)).toMatch(/\d{2}\/\d{2}\/\d{4}/);
	});
});

describe('templateToDraft', () => {
	it('opensATemplateAsADraftWithoutSettingsOfOtherTypes', () => {
		const tpl = {
			_id: 't1', name: 'Mẫu', createdBy: 'a@x.vn', createdAt: '', updatedAt: '',
			nodes: [{ id: 'a', type: 'wait', x: 1, y: 2, waitMinutes: 30, text: 'lạ' }, { id: 'b', type: 'import', x: 3, y: 4 }],
			edges: [e('b', 'a')],
		} as unknown as WorkflowTemplate;
		const d = templateToDraft(tpl);
		expect(d.nodes[0]).toEqual({ id: 'a', type: 'wait', x: 1, y: 2, waitMinutes: 30 });
		expect(d.edges).toHaveLength(1);
	});

	it('dropsABlockOfATypeThisVersionDoesNotKnowAndTheWiresOfIt', () => {
		const tpl = {
			_id: 't1', name: 'Mẫu', createdBy: 'a', createdAt: '', updatedAt: '',
			nodes: [{ id: 'a', type: 'import', x: 0, y: 0 }, { id: 'z', type: 'telegram', x: 0, y: 0 }],
			edges: [e('a', 'z')],
		} as unknown as WorkflowTemplate;
		const d = templateToDraft(tpl);
		expect(d.nodes.map((x) => x.id)).toEqual(['a']);
		expect(d.edges).toEqual([]);
	});
});

describe('draft meta', () => {
	it('survivesAStorageThatThrowsAndGarbage', () => {
		const original = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
		Object.defineProperty(globalThis, 'localStorage', { configurable: true, get() { throw new Error('blocked'); } });
		try {
			expect(loadDraftMeta()).toEqual({ templateId: null, savedPrint: '', name: 'Quy trình mới' });
			expect(() => saveDraftMeta({ templateId: 't', savedPrint: 'p', name: 'n' })).not.toThrow();
		} finally {
			if (original) Object.defineProperty(globalThis, 'localStorage', original);
			else delete (globalThis as { localStorage?: unknown }).localStorage;
		}
		const store: Record<string, string> = { 'workflow-draft-meta': '{not json' };
		Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; } } });
		try {
			expect(loadDraftMeta().templateId).toBeNull();
			saveDraftMeta({ templateId: 't1', savedPrint: 'abc', name: 'Nhắc KOC' });
			expect(loadDraftMeta()).toEqual({ templateId: 't1', savedPrint: 'abc', name: 'Nhắc KOC' });
		} finally {
			delete (globalThis as { localStorage?: unknown }).localStorage;
		}
	});
});
