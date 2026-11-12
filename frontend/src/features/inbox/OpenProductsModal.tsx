import { useCallback, useEffect, useState } from 'react';
import Portal from '../../components/Portal';
import api, { apiMessage } from '../../lib/axios';
import type { OpenProductLink } from '../../types/koc';

interface Props {
	onClose: () => void;
	/** called after a link was added, changed or removed, so the list that opened this window can refresh */
	onChanged: () => void;
	showToast: (msg: string, type?: 'success' | 'error') => void;
}

interface AddResult {
	link: string;
	productId?: string;
	name?: string;
	ok: boolean;
	error?: string;
}

const CHUNK = 50; // what the server takes in one request

/** "link", or "link 12,5" / "link<tab>12.5%": one link per line, the Shop Ads commission after it is optional. */
export function parseLines(text: string): { link: string; adsCommissionRate?: number }[] {
	return text
		.split(/\r?\n/)
		.map((l) => l.trim())
		.filter(Boolean)
		.map((line) => {
			const [link, rate] = line.split(/\s+/);
			const n = rate ? Number(rate.replace('%', '').replace(',', '.')) : NaN;
			return Number.isFinite(n) ? { link, adsCommissionRate: n } : { link };
		});
}

const shortLink = (l: string) => l.replace(/^https?:\/\//, '');

/**
 * The links of the products in open collaboration, kept on the server: they are listed here, new ones are pasted in
 * (the product is found from the link itself) and everything saves as soon as it is done, so the next collaboration
 * on that product already has its link.
 */
export default function OpenProductsModal({ onClose, onChanged, showToast }: Props) {
	const [items, setItems] = useState<OpenProductLink[] | null>(null);
	const [error, setError] = useState('');
	const [text, setText] = useState('');
	const [adding, setAdding] = useState(false);
	const [failed, setFailed] = useState<AddResult[]>([]);
	const [rates, setRates] = useState<Record<string, string>>({});
	const [asking, setAsking] = useState(''); // product whose "Xoá" is waiting for a second click

	const load = useCallback(async () => {
		try {
			const { data } = await api.get<{ data: OpenProductLink[] }>('/open-products');
			setItems(data.data || []);
			setRates(Object.fromEntries((data.data || []).map((p) => [p.productId, p.adsCommissionRate ? String(p.adsCommissionRate).replace('.', ',') : ''])));
			setError('');
		} catch (e) {
			setError(apiMessage(e, 'Không tải được danh sách link.'));
		}
	}, []);

	useEffect(() => {
		load();
	}, [load]);

	useEffect(() => {
		const onKey = (e: KeyboardEvent) => e.key === 'Escape' && !adding && onClose();
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	}, [onClose, adding]);

	async function add() {
		const rows = parseLines(text);
		if (rows.length === 0 || adding) return;
		setAdding(true);
		setFailed([]);
		const results: AddResult[] = [];
		try {
			for (let i = 0; i < rows.length; i += CHUNK) {
				const { data } = await api.post<{ data: AddResult[] }>('/open-products', { rows: rows.slice(i, i + CHUNK) });
				results.push(...(data.data || []));
			}
		} catch (e) {
			showToast(apiMessage(e, 'Không thêm được link.'), 'error');
		}
		const bad = results.filter((r) => !r.ok);
		const good = results.length - bad.length;
		setFailed(bad);
		// what could not be added stays in the box so it can be fixed; what was added leaves it
		setText(bad.map((r) => r.link).join('\n'));
		if (good > 0) {
			showToast(`Đã lưu ${good} link.`);
			onChanged();
		}
		await load();
		setAdding(false);
	}

	async function saveRate(p: OpenProductLink) {
		const raw = (rates[p.productId] ?? '').trim();
		const n = raw === '' ? 0 : Number(raw.replace(',', '.'));
		if (!Number.isFinite(n) || n === p.adsCommissionRate) return;
		try {
			await api.put(`/open-products/${p.productId}`, { adsCommissionRate: n });
			setItems((cur) => cur && cur.map((x) => (x.productId === p.productId ? { ...x, adsCommissionRate: n } : x)));
			onChanged();
		} catch (e) {
			showToast(apiMessage(e, 'Không lưu được hoa hồng quảng cáo.'), 'error');
			load();
		}
	}

	async function remove(p: OpenProductLink) {
		if (asking !== p.productId) {
			setAsking(p.productId);
			return;
		}
		try {
			await api.delete(`/open-products/${p.productId}`);
			setItems((cur) => cur && cur.filter((x) => x.productId !== p.productId));
			onChanged();
		} catch (e) {
			showToast(apiMessage(e, 'Không xoá được link.'), 'error');
		}
		setAsking('');
	}

	function copy(link: string) {
		navigator.clipboard.writeText(link).then(
			() => showToast('Đã sao chép link.'),
			() => showToast('Không sao chép được, hãy bôi đen link và copy thủ công.', 'error'),
		);
	}

	const lines = parseLines(text).length;

	return (
		<Portal>
			<div className="cp-backdrop sw-front" onClick={() => !adding && onClose()}>
				<div className="cp-modal wide opl" role="dialog" aria-modal="true" aria-labelledby="opl-title" onClick={(e) => e.stopPropagation()}>
					<div className="cp-oc-head">
						<div>
							<h4 id="opl-title">Link sản phẩm cộng tác mở</h4>
							<small>Lưu một lần. Chọn sản phẩm cho KOC là có sẵn link và hoa hồng quảng cáo.</small>
						</div>
						<button type="button" className="cp-oc-x" aria-label="Đóng" onClick={onClose} disabled={adding}>×</button>
					</div>

					<div className="opl-add">
						<textarea
							rows={3}
							value={text}
							onChange={(e) => setText(e.target.value)}
							placeholder={'Dán link vào đây, mỗi dòng một link.\nMuốn kèm hoa hồng quảng cáo thì viết sau link: https://vt.tiktok.com/ZS9h…/ 12'}
							aria-label="Link sản phẩm cần thêm"
							disabled={adding}
						/>
						<div className="opl-add-bar">
							<small>{lines > 0 ? `${lines} link chờ thêm` : 'Sản phẩm được nhận ra từ chính link.'}</small>
							<button type="button" className="cp-btn" disabled={adding || lines === 0} onClick={add}>
								{adding ? 'Đang thêm…' : 'Thêm'}
							</button>
						</div>
						{failed.length > 0 && (
							<ul className="opl-failed" role="alert">
								{failed.map((r, i) => (
									<li key={i}>
										<code>{shortLink(r.link)}</code>
										<span>{r.error}</span>
									</li>
								))}
							</ul>
						)}
					</div>

					<div className="opl-title-row">
						<strong>Đã lưu{items ? ` (${items.length})` : ''}</strong>
						<small>Hoa hồng quảng cáo sửa trực tiếp, tự lưu khi bấm ra ngoài.</small>
					</div>
					<div className="opl-list">
						{items === null && !error && <div className="cp-oc-empty">Đang tải…</div>}
						{error && (
							<div className="cp-oc-empty err">
								{error}
								<button type="button" className="cp-link" onClick={load}>Thử lại</button>
							</div>
						)}
						{items && items.length === 0 && <div className="cp-oc-empty">Chưa có link nào. Dán link ở khung trên rồi bấm Thêm.</div>}
						{items?.map((p) => (
							<div className="opl-row" key={p.productId}>
								{p.image ? <img className="cp-oc-img" src={p.image} alt="" loading="lazy" referrerPolicy="no-referrer" /> : <span className="cp-oc-img" />}
								<span className="opl-info">
									<strong title={p.name}>{p.name || `Sản phẩm ${p.productId}`}</strong>
									<small>ID: {p.productId}</small>
									{p.link ? (
										<a href={p.link} target="_blank" rel="noreferrer" title={p.link}>{shortLink(p.link)}</a>
									) : (
										<em className="opl-nolink">Chưa có link</em>
									)}
								</span>
								<label className="opl-ads">
									<span>QC %</span>
									<input
										inputMode="decimal"
										placeholder="—"
										aria-label={`Hoa hồng quảng cáo của ${p.name || p.productId}`}
										value={rates[p.productId] ?? ''}
										onChange={(e) => setRates({ ...rates, [p.productId]: e.target.value.replace(/[^\d.,]/g, '') })}
										onBlur={() => saveRate(p)}
										onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLInputElement).blur()}
									/>
								</label>
								<span className="opl-actions">
									{p.link && <button type="button" className="cp-btn ghost sm" onClick={() => copy(p.link)}>Sao chép</button>}
									<button type="button" className={`cp-btn ghost sm${asking === p.productId ? ' danger' : ''}`} onClick={() => remove(p)} onBlur={() => setAsking('')}>
										{asking === p.productId ? 'Xoá thật?' : 'Xoá'}
									</button>
								</span>
							</div>
						))}
					</div>

					<div className="cp-oc-foot">
						<small>Link thuộc sản phẩm nào do TikTok quyết định, không cần gõ mã sản phẩm.</small>
						<button type="button" className="cp-btn ghost" onClick={onClose} disabled={adding}>Đóng</button>
					</div>
				</div>
			</div>
		</Portal>
	);
}
