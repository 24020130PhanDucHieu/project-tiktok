import { useEffect, useRef, useState } from 'react';
import type { TemplateSummary } from '../../types/workflow';
import { type SaveState, agoText, canChange } from './templates';

interface Props {
	state: SaveState;
	/** the name of the saved workflow the drawing came from, if any */
	openedName: string | null;
	items: TemplateSummary[];
	loading: boolean;
	user: { email: string; role?: string };
	canSaveOver: boolean;
	saving: boolean;
	onOpen: (id: string) => void;
	onDuplicate: (id: string) => void;
	onDelete: (id: string) => void;
	onSave: () => void;
	onSaveAsNew: () => void;
	onNew: () => void;
}

const STATE_TEXT: Record<SaveState, string> = { draft: 'Bản nháp, chưa lưu', saved: 'Đã lưu', changed: 'Có thay đổi chưa lưu' };

/** The bar above the canvas: where the drawing came from, whether it is saved, and the buttons to open, save and start anew. */
export default function TemplateBar({ state, openedName, items, loading, user, canSaveOver, saving, onOpen, onDuplicate, onDelete, onSave, onSaveAsNew, onNew }: Props) {
	const [listOpen, setListOpen] = useState(false);
	const [deleting, setDeleting] = useState<string | null>(null);
	const box = useRef<HTMLDivElement>(null);
	useEffect(() => {
		if (!listOpen) return;
		const away = (e: MouseEvent) => !box.current?.contains(e.target as Node) && setListOpen(false);
		const key = (e: KeyboardEvent) => e.key === 'Escape' && setListOpen(false);
		document.addEventListener('mousedown', away);
		document.addEventListener('keydown', key);
		return () => { document.removeEventListener('mousedown', away); document.removeEventListener('keydown', key); };
	}, [listOpen]);

	return (
		<div className="wf-tbar">
			<div className="wf-tstate">
				<span className={`wf-tdot wf-tdot-${state}`} />
				<span>{STATE_TEXT[state]}</span>
				{openedName && <span className="wf-tname">· {openedName}</span>}
			</div>
			<span className="wf-tspacer" />
			<button type="button" className="tm-btn-cancel" onClick={onNew}>Mới</button>
			<div className="wf-topen" ref={box}>
				<button type="button" className="tm-btn-cancel" aria-expanded={listOpen} onClick={() => setListOpen((v) => !v)}>Mở ▾</button>
				{listOpen && (
					<div className="wf-tlist" role="menu" aria-label="Quy trình đã lưu">
						<p className="wf-tlist-title">Quy trình đã lưu {!loading && <small>({items.length}/50)</small>}</p>
						{loading && <p className="wf-tlist-empty">Đang tải…</p>}
						{!loading && items.length === 0 && <p className="wf-tlist-empty">Chưa có quy trình nào được lưu. Vẽ một quy trình rồi bấm Lưu.</p>}
						{items.map((t) => (
							<div key={t._id} className="wf-trow">
								<button type="button" className="wf-trow-main" onClick={() => { setListOpen(false); onOpen(t._id); }}>
									<strong>{t.name}</strong>
									<small>{t.nodeCount} khối · {t.createdBy || 'không rõ'} · {agoText(t.updatedAt, Date.now())}</small>
								</button>
								{deleting === t._id ? (
									<span className="wf-trow-actions">
										<button type="button" className="danger" onClick={() => { setDeleting(null); onDelete(t._id); }}>Xóa</button>
										<button type="button" onClick={() => setDeleting(null)}>Hủy</button>
									</span>
								) : (
									<span className="wf-trow-actions">
										<button type="button" onClick={() => { setListOpen(false); onDuplicate(t._id); }} title="Mở thành một bản mới">Nhân bản</button>
										{canChange(t, user) && <button type="button" className="danger" onClick={() => setDeleting(t._id)}>Xóa</button>}
									</span>
								)}
							</div>
						))}
					</div>
				)}
			</div>
			<button type="button" className="tm-btn-cancel" disabled={saving} onClick={onSaveAsNew}>Lưu thành bản mới</button>
			<button type="button" className="tm-btn-submit" disabled={saving || (state === 'saved' && canSaveOver)} onClick={canSaveOver ? onSave : onSaveAsNew}>
				{saving ? 'Đang lưu…' : 'Lưu'}
			</button>
		</div>
	);
}
