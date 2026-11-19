import { useEffect, useState } from 'react';
import api from '../../lib/axios';
import { MAX_PICK } from './graph';

interface Hit {
	id: string;
	name: string;
	sub: string;
}

interface Props {
	ids: string[];
	names: Record<string, string>;
	onChange: (ids: string[], names: Record<string, string>) => void;
}

/** Chooses KOCs by name: type to search the directory, bấm a result to add it; the chosen ones are chips that can be removed. */
export default function KocChooser({ ids, names, onChange }: Props) {
	const [q, setQ] = useState('');
	const [hits, setHits] = useState<Hit[]>([]);
	const [busy, setBusy] = useState(false);

	useEffect(() => {
		const text = q.trim();
		if (text.length < 2) {
			setHits([]);
			return;
		}
		let alive = true;
		setBusy(true);
		const timer = setTimeout(() => {
			api.get('/kocs', { params: { q: text, limit: 8 } })
				.then(({ data }) => {
					if (!alive) return;
					const items = (data.data?.items ?? []) as { _id?: string; id?: string; displayName?: string; tiktokId?: string; stage?: string }[];
					setHits(items.map((it) => ({
						id: String(it._id ?? it.id),
						name: it.displayName || `@${String(it.tiktokId ?? '').replace(/^@/, '')}`,
						sub: it.tiktokId ? `@${String(it.tiktokId).replace(/^@/, '')}` : '',
					})));
				})
				.catch(() => alive && setHits([]))
				.finally(() => alive && setBusy(false));
		}, 300);
		return () => { alive = false; clearTimeout(timer); };
	}, [q]);

	const add = (h: Hit) => {
		if (ids.includes(h.id) || ids.length >= MAX_PICK) return;
		onChange([...ids, h.id], { ...names, [h.id]: h.name });
		setQ('');
		setHits([]);
	};
	const remove = (id: string) => {
		const rest = { ...names };
		delete rest[id];
		onChange(ids.filter((x) => x !== id), rest);
	};

	return (
		<div className="wfp-koc">
			<input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Gõ tên hoặc TikTok ID của KOC…" aria-label="Tìm KOC theo tên" />
			{q.trim().length >= 2 && (
				<ul className="wfp-koc-hits" role="listbox" aria-label="Kết quả tìm KOC">
					{busy && hits.length === 0 && <li className="wfp-koc-note">Đang tìm…</li>}
					{!busy && hits.length === 0 && <li className="wfp-koc-note">Không thấy KOC nào.</li>}
					{hits.map((h) => (
						<li key={h.id}>
							<button type="button" role="option" aria-selected={ids.includes(h.id)} disabled={ids.includes(h.id)} onClick={() => add(h)}>
								<b>{h.name}</b> <small>{h.sub}</small>{ids.includes(h.id) && <small> · đã chọn</small>}
							</button>
						</li>
					))}
				</ul>
			)}
			{ids.length > 0 && (
				<div className="wfp-chips">
					{ids.map((id) => (
						<button key={id} type="button" className="wfp-chip on" onClick={() => remove(id)} title="Bấm để bỏ KOC này">
							{names[id] ?? 'KOC đã chọn'} ×
						</button>
					))}
				</div>
			)}
		</div>
	);
}
