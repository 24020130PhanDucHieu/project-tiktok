import type { WorkflowNodeType } from '../../types/workflow';

const SVG = { width: 34, height: 34, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.6, strokeLinecap: 'round', strokeLinejoin: 'round' } as const;

export default function NodeIcon({ kind, size = 34 }: { kind: WorkflowNodeType; size?: number }) {
	const props = { ...SVG, width: size, height: size };
	switch (kind) {
		case 'import':
			return <svg {...props}><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" /><path d="M12 18v-6" /><polyline points="9 15 12 12 15 15" /></svg>;
		case 'pick':
			return <svg {...props}><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" /><path d="M23 21v-2a4 4 0 0 0-3-3.87" /><path d="M16 3.13a4 4 0 0 1 0 7.75" /></svg>;
		case 'wait':
			return <svg {...props}><circle cx="12" cy="12" r="9" /><polyline points="12 7 12 12 15.5 14" /></svg>;
		case 'wait_until':
			return <svg {...props}><rect x="3" y="4" width="18" height="17" rx="2" /><line x1="3" y1="9" x2="21" y2="9" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="16" y1="2" x2="16" y2="6" /><polyline points="12 12 12 15.5 14.5 17" /></svg>;
		case 'send':
			return <svg {...props}><line x1="22" y1="2" x2="11" y2="13" /><polygon points="22 2 15 22 11 13 2 9 22 2" /></svg>;
		case 'send_text':
			return <svg {...props}><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" /><line x1="8" y1="9" x2="16" y2="9" /><line x1="8" y1="13" x2="13" y2="13" /></svg>;
		case 'email':
			return <svg {...props}><path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" /><polyline points="22,6 12,13 2,6" /></svg>;
		case 'label':
			return <svg {...props}><path d="M20.6 13.4l-7.2 7.2a2 2 0 0 1-2.8 0L2 12V2h10l8.6 8.6a2 2 0 0 1 0 2.8z" /><circle cx="7" cy="7" r="1.4" /></svg>;
		case 'stage':
			return <svg {...props}><polyline points="3 17 9 11 13 15 21 7" /><polyline points="15 7 21 7 21 13" /></svg>;
		default:
			return <svg {...props}><path d="M6 3v6a3 3 0 0 0 3 3h6a3 3 0 0 1 3 3v6" /><polyline points="15 18 18 21 21 18" /><circle cx="6" cy="3" r="1.4" /></svg>;
	}
}
