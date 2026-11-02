import { type FormEvent, useEffect, useRef, useState } from 'react';
import type { InboxLabel, InboxTaskKey } from '../../types/koc';

export const TASK_OPTIONS: { key: InboxTaskKey; label: string; color: string }[] = [
	{ key: 'chot-phi-cast', label: 'Chốt phí cast', color: '#2563eb' },
	{ key: 'gui-link', label: 'Gửi link', color: '#0284c7' },
	{ key: 'duyet-mau', label: 'Duyệt mẫu', color: '#4f46e5' },
	{ key: 'gui-mau', label: 'Gửi mẫu', color: '#16a34a' },
	{ key: 'tra-phi-cast', label: 'Trả phí cast', color: '#d97706' },
];

const LABEL_COLORS = ['#ef4444', '#16a34a', '#d97706', '#0284c7', '#7c3aed', '#64748b'];

interface Props {
	labels: InboxLabel[];
	tasks: string[];
	labelIds: string[];
	showHidden: boolean;
	onTasks: (v: string[]) => void;
	onLabelIds: (v: string[]) => void;
	onShowHidden: (v: boolean) => void;
	onCreateLabel: (name: string, color: string) => Promise<void> | void;
	onDeleteLabel: (id: string) => Promise<void> | void;
}

function toggle(list: string[], key: string) {
	return list.includes(key) ? list.filter((x) => x !== key) : [...list, key];
}

export default function ConversationFilterMenu(p: Props) {
	const [open, setOpen] = useState(false);
	const [managing, setManaging] = useState(false);
	const [name, setName] = useState('');
	const rootRef = useRef<HTMLDivElement>(null);
	const activeCount = p.tasks.length + p.labelIds.length + (p.showHidden ? 1 : 0);

	useEffect(() => {
		if (!open) return undefined;
		const onDown = (e: MouseEvent) => {
			if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
		};
		document.addEventListener('mousedown', onDown);
		return () => document.removeEventListener('mousedown', onDown);
	}, [open]);

	async function handleCreate(e: FormEvent) {
		e.preventDefault();
		const trimmed = name.trim();
		if (!trimmed) return;
		await p.onCreateLabel(trimmed, LABEL_COLORS[p.labels.length % LABEL_COLORS.length]);
		setName('');
	}

	return (
		<div className="msg-filter-menu-wrap" ref={rootRef}>
			<button type="button" className={activeCount ? 'msg-sort-label has-filter' : 'msg-sort-label'} onClick={() => setOpen((v) => !v)}>
				Phân loại{activeCount ? ` (${activeCount})` : ''} ⌄
			</button>
			{open && (
				<div className="msg-filter-menu" role="menu">
					<div className="msg-menu-title">Việc của bạn</div>
					{TASK_OPTIONS.map((t) => (
						<label key={t.key} className="msg-menu-check">
							<input type="checkbox" checked={p.tasks.includes(t.key)} onChange={() => p.onTasks(toggle(p.tasks, t.key))} />
							<i style={{ background: t.color }} />
							{t.label}
						</label>
					))}

					<div className="msg-menu-sep" />
					<div className="msg-menu-title">Theo nhãn tự gắn</div>
					{p.labels.map((l) => (
						<label key={l._id} className="msg-menu-check">
							<input type="checkbox" checked={p.labelIds.includes(l._id)} onChange={() => p.onLabelIds(toggle(p.labelIds, l._id))} />
							<i style={{ background: l.color }} />
							<span className="grow">{l.name}</span>
							{managing && (
								<button type="button" className="msg-menu-x" title="Xoá nhãn" onClick={() => p.onDeleteLabel(l._id)}>
									×
								</button>
							)}
						</label>
					))}
					<button type="button" className="msg-menu-link" onClick={() => setManaging((v) => !v)}>
						{managing ? 'Xong' : '+  Thêm / xoá nhãn'}
					</button>
					{managing && (
						<form className="msg-menu-add" onSubmit={handleCreate}>
							<input value={name} onChange={(e) => setName(e.target.value)} maxLength={30} placeholder="Tên nhãn mới" />
							<button type="submit" disabled={!name.trim()}>
								Thêm
							</button>
						</form>
					)}

					<div className="msg-menu-sep" />
					<label className="msg-menu-check">
						<input type="checkbox" checked={p.showHidden} onChange={(e) => p.onShowHidden(e.target.checked)} />
						Đã ẩn
					</label>
					{activeCount > 0 && (
						<button
							type="button"
							className="msg-menu-link"
							onClick={() => {
								p.onTasks([]);
								p.onLabelIds([]);
								p.onShowHidden(false);
							}}
						>
							Xoá bộ lọc
						</button>
					)}
				</div>
			)}
		</div>
	);
}
