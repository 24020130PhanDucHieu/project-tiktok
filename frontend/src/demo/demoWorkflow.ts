import type { WorkflowDraft } from '../types/workflow';
import { DEMO_CREATORS, DEMO_LABELS, demoHexId } from './inboxData';

/** Ids of the made-up KOCs the demo run works on (the same ones the demo inbox shows). */
export const DEMO_KOC_IDS = DEMO_CREATORS.map((_, i) => demoHexId(i + 1));
export const DEMO_KOC_NAMES: Record<string, string> = Object.fromEntries(DEMO_CREATORS.map((c, i) => [DEMO_KOC_IDS[i], c.name]));
/** How many of the demo KOCs "answer" at the if_replied block; the others take the "unanswered" branch. */
export const DEMO_REPLIED = 3;

export const DEMO_WORKFLOW_NAME = 'Quy trình mẫu: mời hợp tác và nhắc người chưa trả lời';

/**
 * A ready-made diagram: take KOCs → text them on Zalo → wait a day → split by who answered →
 * answered get a label, the others get a reminder email. It is only ever drawn and simulated in the browser.
 */
export const DEMO_WORKFLOW: WorkflowDraft = {
	nodes: [
		{ id: 'pick-1', type: 'pick', x: 0, y: 120, stages: ['CONTACTED'], limit: DEMO_KOC_IDS.length, kocIds: DEMO_KOC_IDS, kocNames: DEMO_KOC_NAMES },
		{ id: 'send_text-1', type: 'send_text', x: 300, y: 120, text: 'Chào {ten}, bên mình có theo dõi kênh {kenh} và muốn mời bạn hợp tác. Bạn xem giúp mình nhé!' },
		{ id: 'wait-1', type: 'wait', x: 600, y: 120, waitMinutes: 1440 },
		{ id: 'if_replied-1', type: 'if_replied', x: 900, y: 120 },
		{ id: 'label-1', type: 'label', x: 1200, y: 20, labelId: DEMO_LABELS[0]._id },
		{ id: 'email-1', type: 'email', x: 1200, y: 240, subject: 'Nhắc nhẹ: lời mời hợp tác', text: 'Chào {ten}, mình nhắn lại về lời mời hợp tác hôm qua. Bạn có thể phản hồi giúp mình khi tiện nhé.' },
	],
	edges: [
		{ id: 'e1', from: 'pick-1', port: 'out', to: 'send_text-1' },
		{ id: 'e2', from: 'send_text-1', port: 'out', to: 'wait-1' },
		{ id: 'e3', from: 'wait-1', port: 'out', to: 'if_replied-1' },
		{ id: 'e4', from: 'if_replied-1', port: 'replied', to: 'label-1' },
		{ id: 'e5', from: 'if_replied-1', port: 'unanswered', to: 'email-1' },
	],
};
