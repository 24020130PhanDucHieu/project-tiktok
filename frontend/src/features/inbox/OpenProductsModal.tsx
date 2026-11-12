import { useCallback, useEffect, useState } from 'react';
import Portal from '../../components/Portal';
import api, { apiMessage } from '../../lib/axios';
import type { OpenProductLink } from '../../types/koc';

interface Props {
	onClose: () => void;
	showToast: (msg: string, type?: 'success' | 'error') => void;
	/** an admin adds, changes and deletes; everyone else only reads and picks */
	isAdmin: boolean;
	/** set: a click on a link chooses it (and its product) for the collaboration that opened this window */
	pick?: { currentProductId?: string; busy?: boolean; onPick: (p: OpenProductLink) => void };
	/** a product with no saved link is chosen from TikTok Shop's own list instead */
	onShopList?: () => void;
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
const rateText = (n: number) => String(n).replace('.', ',');

/**
 * The links of the products in open collaboration, kept on the server. Opened from a collaboration it is a list to pick
 * from: one click chooses the link and its product for the KOC. An admin also adds links here (the product is found from
 * the link itself), sets the Shop Ads commission and deletes; each of those is saved at once.
 */
export default function OpenProductsModal({ onClose, showToast, isAdmin, pick, onShopList }: Props) {
	const [items, setItems] = useState<OpenProductLink[] | null>(null);
	const [error, setError] = useState('');
	const [text, setText] = useState('');
	const [adding, setAdding] = useState(false);
	const [addOpen, setAddOpen] = useState(false);
	const [failed, setFailed] = useState<AddResult[]>([]);
	const [rates, setRates] = useState<Record<string, string>>({});
	const [asking, setAsking] = useState(''); // product whose "Xoá" is waiting for a second click

	const load = useCallback(async () => {
		try {
			const { data } = await api.get<{ data: OpenProductLink[] }>('/open-products');
			const list = data.data || [];
			setItems(list);
			setRates(Object.fromEntries(list.map((p) => [p.productId, p.adsCommissionRate ? rateText(p.adsCommissionRate) : ''])));
			setError('');
		} catch (e) {
			setError(apiMessage(e, 'Không tải được danh sách link.'));
		}
	}, []);

	useEffect(() => {
		load();
	}, [load]);

	// with nothing saved yet, an admin lands on the box to paste links
	useEffect(() => {
		if (isAdmin && items && items.length === 0) setAddOpen(true);
	}, [isAdmin, items]);

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
		if (good > 0) showToast(`Đã lưu ${good} link.`);
		if (good > 0 && bad.length === 0) setAddOpen(false);
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
		} catch (e) {
			showToast(apiMessage(e, 'Không xoá được link.'), 'error');
		}
		setAsking('');
	}

	const lines = parseLines(text).length;
	const picking = !!pick;

	return (
		<Portal>
			<div className="cp-backdrop sw-front" onClick={() => !adding && onClose()}>
				<div className="cp-modal wide opl" role="dialog" aria-modal="true" aria-labelledby="opl-title" onClick={(e) => e.stopPropagation()}>
					<div className="cp-oc-head">
						<div>
							<h4 id="opl-title">{picking ? 'Chọn link sản phẩm' : 'Link sản phẩm cộng tác mở'}</h4>
							<small>{picking ? 'Bấm vào một sản phẩm để gửi link đó cho KOC.' : 'Lưu một lần, các KOC sau chọn sản phẩm là có sẵn link.'}</small>
						</div>
						<button type="button" className="cp-oc-x" aria-label="Đóng" onClick={onClose} disabled={adding}>×</button>
					</div>

					<div className="opl-list" role={picking ? 'listbox' : undefined}>
						{items === null && !error && <div className="cp-oc-empty">Đang tải…</div>}
						{error && (
							<div className="cp-oc-empty err">
								{error}
								<button type="button" className="cp-link" onClick={load}>Thử lại</button>
							</div>
						)}
						{items && items.length === 0 && (
							<div className="cp-oc-empty">{isAdmin ? 'Chưa có link nào. Dán link ở dưới rồi bấm Thêm.' : 'Chưa có link nào. Nhờ quản trị viên thêm link sản phẩm.'}</div>
						)}
						{items?.map((p) => {
							const current = pick?.currentProductId === p.productId;
							const usable = picking && !!p.link && !pick?.busy;
							return (
								<div className={`opl-row${current ? ' on' : ''}`} key={p.productId}>
									<button
										type="button"
										className="opl-main"
										role={picking ? 'option' : undefined}
										aria-selected={picking ? current : undefined}
										disabled={!usable}
										onClick={() => pick?.onPick(p)}
									>
										{p.image ? <img className="cp-oc-img" src={p.image} alt="" loading="lazy" referrerPolicy="no-referrer" /> : <span className="cp-oc-img" />}
										<span className="opl-info">
											<strong title={p.name}>{p.name || `Sản phẩm ${p.productId}`}</strong>
											<small>
												ID {p.productId}
												{current && <b className="opl-now">Đang dùng</b>}
											</small>
											{p.link ? <span className="opl-url">{shortLink(p.link)}</span> : <em className="opl-nolink">Chưa có link</em>}
										</span>
									</button>
									<div className="opl-side">
										{isAdmin ? (
											<>
												<label className="opl-ads" title="Hoa hồng quảng cáo (%)">
													<span>Quảng cáo %</span>
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
												<button type="button" className={`opl-del${asking === p.productId ? ' ask' : ''}`} onClick={() => remove(p)} onBlur={() => setAsking('')} title="Xoá link này khỏi danh sách">
													{asking === p.productId ? 'Xoá thật?' : 'Xoá'}
												</button>
											</>
										) : (
											p.adsCommissionRate > 0 && <span className="opl-rate">Quảng cáo {rateText(p.adsCommissionRate)}%</span>
										)}
									</div>
								</div>
							);
						})}
					</div>

					{isAdmin && (
						<div className="opl-add">
							{!addOpen ? (
								<button type="button" className="opl-add-toggle" onClick={() => setAddOpen(true)}>
									+ Thêm link mới
								</button>
							) : (
								<>
									<textarea
										rows={3}
										value={text}
										onChange={(e) => setText(e.target.value)}
										placeholder={'Dán link vào đây, mỗi dòng một link.\nMuốn kèm hoa hồng quảng cáo thì viết sau link, ví dụ: https://vt.tiktok.com/ZS9h…/ 12'}
										aria-label="Link sản phẩm cần thêm"
										disabled={adding}
										autoFocus={!!items && items.length > 0}
									/>
									<div className="opl-add-bar">
										<small>{lines > 0 ? `${lines} link chờ thêm` : 'Sản phẩm được nhận ra từ chính link.'}</small>
										<span>
											{items && items.length > 0 && (
												<button type="button" className="cp-btn ghost sm" onClick={() => setAddOpen(false)} disabled={adding}>
													Thu gọn
												</button>
											)}
											<button type="button" className="cp-btn sm" disabled={adding || lines === 0} onClick={add}>
												{adding ? 'Đang thêm…' : 'Thêm'}
											</button>
										</span>
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
								</>
							)}
						</div>
					)}

					<div className="cp-oc-foot">
						{onShopList ? (
							<button type="button" className="cp-link" onClick={onShopList}>
								Chọn sản phẩm từ TikTok Shop
							</button>
						) : (
							<span />
						)}
						<button type="button" className="cp-btn ghost" onClick={onClose} disabled={adding}>
							Đóng
						</button>
					</div>
				</div>
			</div>
		</Portal>
	);
}
