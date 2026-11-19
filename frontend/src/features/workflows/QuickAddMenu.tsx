import { useEffect, useRef } from 'react';
import type { WorkflowNodeType } from '../../types/workflow';
import { BLOCKS, GROUP_TITLES } from './blocks';
import NodeIcon from './NodeIcon';

interface Props {
	/** where the menu opens, in screen coordinates */
	x: number;
	y: number;
	onPick: (type: WorkflowNodeType) => void;
	onClose: () => void;
}

/** The menu that adds a block right after an output: a list of the blocks that can follow, grouped like the palette. */
export default function QuickAddMenu({ x, y, onPick, onClose }: Props) {
	const box = useRef<HTMLDivElement>(null);
	useEffect(() => {
		const away = (e: MouseEvent) => !box.current?.contains(e.target as Node) && onClose();
		const key = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
		// a frame later: the click that opened the menu must not close it
		const id = window.setTimeout(() => document.addEventListener('mousedown', away), 0);
		document.addEventListener('keydown', key);
		return () => { window.clearTimeout(id); document.removeEventListener('mousedown', away); document.removeEventListener('keydown', key); };
	}, [onClose]);

	// keep the menu on the screen
	const left = Math.min(x + 8, window.innerWidth - 270);
	const top = Math.min(y - 12, window.innerHeight - 440);
	const followers = BLOCKS.filter((b) => b.group !== 'start');
	return (
		<div ref={box} className="wf-menu" style={{ left: Math.max(8, left), top: Math.max(8, top) }} role="menu" aria-label="Thêm khối tiếp theo">
			<p className="wf-menu-title">Thêm khối tiếp theo</p>
			{(['control', 'action'] as const).map((group) => (
				<div key={group}>
					<p className="wf-menu-group">{GROUP_TITLES[group]}</p>
					{followers.filter((b) => b.group === group).map((b) => (
						<button key={b.type} type="button" role="menuitem" className="wf-menu-item" style={{ '--accent': b.color } as React.CSSProperties} onClick={() => onPick(b.type)}>
							<span className="wf-menu-icon"><NodeIcon kind={b.type} size={18} /></span>
							<span><strong>{b.title}</strong><small>{b.hint}</small></span>
						</button>
					))}
				</div>
			))}
		</div>
	);
}
