import type { WorkflowDraft, WorkflowEdge, WorkflowNode, WorkflowTemplate } from '../../types/workflow';
import { BLOCK_BY_TYPE } from './blocks';
import { withoutSettingsOfOtherTypes } from './convert';

/**
 * A text that is the same for the same drawing and different when a block moves, a setting changes or a wire is added or
 * removed. What the server fills in (the name of a campaign) is not part of a drawing and is left out.
 */
export function fingerprint(nodes: WorkflowNode[], edges: WorkflowEdge[]): string {
	const ns = nodes.map(withoutSettingsOfOtherTypes).map((x) => ({ ...x, x: Math.round(x.x), y: Math.round(x.y) }));
	ns.sort((a, b) => a.id.localeCompare(b.id));
	const es = edges.map((x) => [x.from, x.port, x.to]).sort((a, b) => a.join('>').localeCompare(b.join('>')));
	return JSON.stringify({ n: ns, e: es });
}

export type SaveState = 'draft' | 'saved' | 'changed';

/** "draft": never saved; "saved": the same as the saved template; "changed": a saved template with changes that are not saved. */
export function saveState(templateId: string | null, savedPrint: string, current: string): SaveState {
	if (!templateId) return 'draft';
	return savedPrint === current ? 'saved' : 'changed';
}

/** The server's rule: the person who made a template, or an admin, changes and deletes it. */
export function canChange(template: { createdBy: string }, user: { email: string; role?: string }): boolean {
	return user.role === 'admin' || (template.createdBy !== '' && template.createdBy === user.email);
}

/** "5 phút trước": when something was last changed. */
export function agoText(iso: string, nowMs: number): string {
	const seconds = Math.max(0, Math.floor((nowMs - Date.parse(iso)) / 1000));
	if (seconds < 60) return 'vừa xong';
	if (seconds < 3600) return `${Math.floor(seconds / 60)} phút trước`;
	if (seconds < 86400) return `${Math.floor(seconds / 3600)} giờ trước`;
	if (seconds < 30 * 86400) return `${Math.floor(seconds / 86400)} ngày trước`;
	return new Date(iso).toLocaleDateString('vi-VN', { day: '2-digit', month: '2-digit', year: 'numeric' });
}

/** A saved template as a draft: the blocks of a type this version does not know are left out, with their wires. */
export function templateToDraft(t: WorkflowTemplate): WorkflowDraft {
	const nodes = t.nodes.filter((n) => BLOCK_BY_TYPE[n.type]).map(withoutSettingsOfOtherTypes);
	const ids = new Set(nodes.map((n) => n.id));
	return { nodes, edges: t.edges.filter((e) => ids.has(e.from) && ids.has(e.to)) };
}

const META_KEY = 'workflow-draft-meta';

/** Which saved template the drawing came from (and how it was when it was saved), kept in this browser with the draft. */
export interface DraftMeta {
	templateId: string | null;
	savedPrint: string;
	name: string;
}

export function loadDraftMeta(): DraftMeta {
	try {
		const raw = localStorage.getItem(META_KEY);
		if (raw) {
			const m = JSON.parse(raw) as Partial<DraftMeta>;
			return { templateId: typeof m.templateId === 'string' ? m.templateId : null, savedPrint: typeof m.savedPrint === 'string' ? m.savedPrint : '', name: typeof m.name === 'string' && m.name ? m.name : 'Quy trình mới' };
		}
	} catch {
		// blocked storage or garbage: a new draft
	}
	return { templateId: null, savedPrint: '', name: 'Quy trình mới' };
}

export function saveDraftMeta(meta: DraftMeta): void {
	try {
		localStorage.setItem(META_KEY, JSON.stringify(meta));
	} catch {
		// a convenience: nothing is lost if it cannot be kept
	}
}
