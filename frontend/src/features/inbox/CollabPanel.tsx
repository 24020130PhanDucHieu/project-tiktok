import { type ReactNode, useCallback, useEffect, useState } from 'react';
import api from '../../lib/axios';
import { formatCurrency, formatNumber } from '../../lib/formatters';
import type { Collab, CollabMode, CollabOutcome, Creator } from '../../types/koc';

const STEPS = ['Chốt phí cast', 'Chọn hình thức hợp tác', 'Duyệt & gửi mẫu', 'KOC đăng bài', 'Hiệu quả'];
const FEE_SUGGESTIONS = [300000, 400000, 500000, 600000];
const REJECT_REASONS = ['Hết hàng mẫu', 'KOC không phù hợp ngành hàng', 'Ngoài ngân sách', 'Khác'];
const OUTCOMES: { key: CollabOutcome; label: string }[] = [
	{ key: 'posted', label: 'KOC đã đăng bài' },
	{ key: 'no_post', label: 'Nhận mẫu rồi không đăng' },
	{ key: 'no_response', label: 'Không nhận mẫu / không phản hồi' },
	{ key: 'no_need', label: 'Hết nhu cầu hợp tác' },
];
const MODES: { key: CollabMode; title: string; sub: string; needsShop: boolean }[] = [
	{ key: 'targeted', title: 'Mời cộng tác mục tiêu', sub: 'Hoa hồng riêng cho KOC này', needsShop: true },
	{ key: 'open', title: 'Tạo link cộng tác mở', sub: 'Hoa hồng theo mức công khai của shop', needsShop: true },
	{ key: 'product', title: 'Dán link sản phẩm', sub: 'TikToday tự theo dõi bài KOC đăng', needsShop: false },
];

const fmtDay = (iso?: string | null) => (iso ? new Date(iso).toLocaleDateString('vi-VN') : '');
const outcomeLabel = (o: string) => OUTCOMES.find((x) => x.key === o)?.label || '';

interface Props {
	koc: Creator | null;
	apiReady: boolean;
	tiktokShopLinked: boolean;
	onLinkShop: () => void;
	onOpenShipping: () => void;
	onTrackShipment: (creator: Creator | null, code?: string, phone4?: string) => void;
	onInsertDraft: (text: string) => void;
	onChanged: () => void;
	showToast: (msg: string, type?: 'success' | 'error') => void;
	children?: ReactNode;
}

export default function CollabPanel({ koc, apiReady, tiktokShopLinked, onLinkShop, onOpenShipping, onTrackShipment, onInsertDraft, onChanged, showToast, children }: Props) {
	const [items, setItems] = useState<Collab[]>([]);
	const [owed, setOwed] = useState(0);
	const [loading, setLoading] = useState(false);
	const [viewId, setViewId] = useState<string | null>(null);
	const [openStep, setOpenStep] = useState<number | null>(null);
	const [dialog, setDialog] = useState<'fee' | 'close' | 'sample-reject' | null>(null);
	const [feeInput, setFeeInput] = useState('');
	const [showHistory, setShowHistory] = useState(false);
	const [naming, setNaming] = useState(false);
	const [nameInput, setNameInput] = useState('');
	const [closeOutcome, setCloseOutcome] = useState<CollabOutcome>('posted');
	const [closePaid, setClosePaid] = useState(false);
	const [rejectSampleId, setRejectSampleId] = useState('');
	const [rejectReason, setRejectReason] = useState(REJECT_REASONS[0]);
	const [form, setForm] = useState({ product: '', rate: '', deadline: '', freeSample: true, link: '', sampleProduct: '', vUrl: '', vViews: '', vOrders: '', vRevenue: '', shareOrders: '', shareRevenue: '' });
	const set = (k: keyof typeof form, v: string | boolean) => setForm((f) => ({ ...f, [k]: v }));

	const live = !!koc && apiReady && !String(koc.id).startsWith('seed-') && !String(koc.id).startsWith('local-');
	const active = items.find((c) => c.status === 'active') || null;
	const shown = (viewId && items.find((c) => c._id === viewId)) || active || null;
	const closedView = !!shown && shown.status === 'closed';
	const closedItems = items.filter((c) => c.status === 'closed');

	const load = useCallback(() => {
		if (!live || !koc) return;
		setLoading(true);
		api.get(`/kocs/${koc.id}/collabs`)
			.then(({ data }) => {
				setItems(data.data.items || []);
				setOwed(data.data.owed || 0);
			})
			.catch(() => showToast('Không tải được các lần hợp tác.', 'error'))
			.finally(() => setLoading(false));
	}, [live, koc?.id]);

	useEffect(() => {
		setItems([]);
		setViewId(null);
		setOpenStep(null);
		load();
	}, [load]);

	async function run(req: () => Promise<{ data: { data: Collab } }>, ok?: string) {
		try {
			const { data } = await req();
			const c = data.data;
			setItems((prev) => (prev.some((x) => x._id === c._id) ? prev.map((x) => (x._id === c._id ? c : x)) : [c, ...prev]));
			if (ok) showToast(ok);
			onChanged();
			if (c.status === 'closed' || c.feePaid) load();
			return c;
		} catch (e: any) {
			showToast(e?.response?.data?.message || 'Thao tác không thành công.', 'error');
			return null;
		}
	}

	const patch = (c: Collab, body: Record<string, unknown>, ok?: string) => run(() => api.patch(`/collabs/${c._id}`, body), ok);

	async function saveFee(free = false) {
		if (!koc) return;
		const fee = free ? 0 : Number(feeInput.replace(/\D/g, ''));
		if (!free && !fee) return showToast('Nhập số tiền phí cast hoặc chọn Free cast.', 'error');
		const c = active
			? await patch(active, { castFee: fee }, 'Đã đổi phí cast.')
			: await run(() => api.post(`/kocs/${koc.id}/collabs`, { castFee: fee }), 'Đã chốt phí cast.');
		if (c) {
			setDialog(null);
			setFeeInput('');
		}
	}

	if (!koc) return <div className="cp-empty">Chọn một KOC để xem lần hợp tác.</div>;
	if (!live) return <div className="cp-empty">Kết nối API và mở KOC thật để dùng Lần hợp tác.</div>;
	if (loading && items.length === 0) return <div className="cp-empty">Đang tải…</div>;

	const stepState = (i: number) => (!shown ? 'future' : shown.status === 'closed' || i < shown.step ? 'done' : i === shown.step ? 'current' : 'future');
	const isOpen = (i: number) => (openStep === null ? stepState(i) === 'current' : openStep === i);
	const noShopHint = (
		<span className="cp-lock">
			🔒{' '}
			<button type="button" onClick={onLinkShop}>
				Liên kết shop để dùng tính năng này
			</button>
		</span>
	);

	function renderStepBody(c: Collab, i: number) {
		const locked = c.status === 'closed';
		if (i === 0) {
			return (
				<div className="cp-body">
					<p className="cp-big">{c.castFee ? formatCurrency(c.castFee) : 'Free cast'}</p>
					{!locked && (
						<div className="cp-row">
							<button type="button" className="cp-link" onClick={() => { setFeeInput(String(c.castFee || '')); setDialog('fee'); }}>Đổi giá</button>
							{c.feeHistory.length > 1 && (
								<button type="button" className="cp-link" onClick={() => setShowHistory((v) => !v)}>Đã đổi {c.feeHistory.length - 1} lần ›</button>
							)}
							{c.castFee > 0 && !c.feePaid && (
								<button type="button" className="cp-link" onClick={() => patch(c, { feePaid: true }, 'Đã đánh dấu trả phí.')}>Đánh dấu đã trả phí</button>
							)}
						</div>
					)}
					{c.feePaid && c.castFee > 0 && <small className="cp-ok">✓ Đã trả phí cast</small>}
					{showHistory && (
						<ul className="cp-history">
							{c.feeHistory.map((f, k) => (
								<li key={k}>{fmtDay(f.at)} · {f.fee ? formatCurrency(f.fee) : 'Free cast'}</li>
							))}
						</ul>
					)}
				</div>
			);
		}
		if (i === 1) {
			if (c.mode) {
				const m = MODES.find((x) => x.key === c.mode)!;
				return (
					<div className="cp-body">
						<div className="cp-card selected"><strong>{m.title}</strong><small>{c.productName || c.productLink || m.sub}</small></div>
						<small className="cp-muted">Không đổi được hình thức. Muốn đổi, kết thúc lần này và mở lần mới.</small>
					</div>
				);
			}
			return (
				<div className="cp-body">
					{MODES.map((m) => {
						const blocked = m.needsShop && !tiktokShopLinked;
						return (
							<div key={m.key} className={blocked ? 'cp-card locked' : 'cp-card'}>
								<strong>{m.title}</strong>
								<small>{m.sub}</small>
								{blocked && noShopHint}
								{!blocked && m.key === 'targeted' && (
									<div className="cp-form">
										<input placeholder="Sản phẩm" value={form.product} onChange={(e) => set('product', e.target.value)} />
										<div className="cp-grid2">
											<input placeholder="Hoa hồng %" inputMode="numeric" value={form.rate} onChange={(e) => set('rate', e.target.value.replace(/[^\d.]/g, ''))} />
											<input type="date" title="Hạn cộng tác" value={form.deadline} onChange={(e) => set('deadline', e.target.value)} />
										</div>
										<label className="cp-check"><input type="checkbox" checked={form.freeSample} onChange={(e) => set('freeSample', e.target.checked)} /> Tặng mẫu miễn phí</label>
										<div className="cp-row">
											<button type="button" className="cp-btn" disabled={!form.product.trim()} onClick={() => patch(c, { mode: 'targeted', productName: form.product, commissionRate: Number(form.rate) || 0, deadline: form.deadline, freeSample: form.freeSample }, 'Đã chọn Mời cộng tác mục tiêu.')}>Gửi lời mời</button>
											<button type="button" className="cp-link" disabled={!form.product.trim()} onClick={() => onInsertDraft(`Bên mình mời bạn cộng tác sản phẩm ${form.product}${form.rate ? ` với hoa hồng ${form.rate}%` : ''}${form.deadline ? `, hạn ${fmtDay(form.deadline)}` : ''}. Bạn xem lời mời trong ứng dụng TikTok nhé.`)}>Chèn lời mời</button>
										</div>
									</div>
								)}
								{!blocked && m.key === 'open' && (
									<div className="cp-form">
										<input placeholder="Sản phẩm" value={form.product} onChange={(e) => set('product', e.target.value)} />
										<input placeholder="Dán link xin mẫu của sản phẩm" value={form.link} onChange={(e) => set('link', e.target.value)} />
										<div className="cp-row">
											<button type="button" className="cp-btn" disabled={!form.product.trim()} onClick={() => patch(c, { mode: 'open', productName: form.product, productLink: form.link }, 'Đã chọn Link cộng tác mở.')}>Xác nhận</button>
											<button type="button" className="cp-link" disabled={!form.link.trim()} onClick={() => onInsertDraft(`Link xin mẫu ${form.product}: ${form.link}`)}>Chèn link xin mẫu</button>
										</div>
									</div>
								)}
								{m.key === 'product' && (
									<div className="cp-form">
										<input placeholder="Dán link sản phẩm TikTok Shop" value={form.link} onChange={(e) => set('link', e.target.value)} />
										<div className="cp-row">
											<button type="button" className="cp-btn" disabled={!/^https?:\/\//.test(form.link.trim())} onClick={() => patch(c, { mode: 'product', productLink: form.link }, 'Đã thêm link sản phẩm.')}>Thêm</button>
											<button type="button" className="cp-link" disabled={!form.link.trim()} onClick={() => onInsertDraft(form.link.trim())}>Chèn link</button>
										</div>
									</div>
								)}
							</div>
						);
					})}
				</div>
			);
		}
		if (i === 2) {
			return (
				<div className="cp-body">
					{c.samples.length === 0 && <small className="cp-muted">Chưa có yêu cầu mẫu từ KOC.</small>}
					{c.samples.map((s) => (
						<div key={s.id} className="cp-card">
							<strong>{s.product}</strong>
							<small>{fmtDay(s.at)} · {s.status === 'pending' ? 'Chờ duyệt' : s.status === 'approved' ? 'Đã duyệt' : `Đã từ chối (${s.rejectReason})`}</small>
							{s.status === 'pending' && !locked && (
								<div className="cp-row">
									<button type="button" className="cp-btn" onClick={() => run(() => api.post(`/collabs/${c._id}/samples/${s.id}/approve`, {}), 'Đã duyệt mẫu. Hãy gửi hàng trước hạn.')}>Duyệt</button>
									<button type="button" className="cp-btn ghost" onClick={() => { setRejectSampleId(s.id); setDialog('sample-reject'); }}>Từ chối</button>
								</div>
							)}
						</div>
					))}
					{!locked && (
						<div className="cp-form">
							<div className="cp-row">
								<input placeholder="Thêm yêu cầu mẫu (tên sản phẩm)" value={form.sampleProduct} onChange={(e) => set('sampleProduct', e.target.value)} />
								<button type="button" className="cp-btn" disabled={!form.sampleProduct.trim()} onClick={async () => { if (await run(() => api.post(`/collabs/${c._id}/samples`, { product: form.sampleProduct.trim() }))) set('sampleProduct', ''); }}>Thêm</button>
							</div>
						</div>
					)}
					<div className="cp-card">
						<strong>Vận chuyển mẫu</strong>
						<small>{koc?.trackingCode ? `J&T #${koc.trackingCode} · ${koc.trackingStatusText || 'Chưa có trạng thái'}` : 'Chưa có mã vận đơn'}</small>
						<div className="cp-row">
							<button type="button" className="cp-btn ghost" onClick={onOpenShipping}>{koc?.trackingCode ? 'Sửa vận đơn' : 'Nhập mã vận đơn'}</button>
							{koc?.trackingCode && <button type="button" className="cp-link" onClick={() => onTrackShipment(koc, koc.trackingCode)}>Tra cứu J&T</button>}
						</div>
					</div>
				</div>
			);
		}
		if (i === 3) {
			return (
				<div className="cp-body">
					{c.videos.length === 0 && <small className="cp-muted">KOC chưa đăng video cho lần này.</small>}
					{c.videos.map((v, idx) => (
						<div key={v.url} className="cp-card">
							<a href={v.url} target="_blank" rel="noreferrer">{v.title || v.url}</a>
							<small>{formatNumber(v.views)} lượt xem · {formatNumber(v.orders)} đơn · Doanh thu {formatCurrency(v.revenue)}</small>
							{v.adsCode ? <small className="cp-ok">Code ads: {v.adsCode}</small> : !locked && (
								<button type="button" className="cp-link" onClick={() => { const code = window.prompt('Nhập code ads của video'); if (code) run(() => api.put(`/collabs/${c._id}/videos/${idx}/ads`, { code })); }}>+ Thêm code ads</button>
							)}
						</div>
					))}
					{!locked && (
						<div className="cp-form">
							<input placeholder="Link video TikTok" value={form.vUrl} onChange={(e) => set('vUrl', e.target.value)} />
							<div className="cp-grid3">
								<input placeholder="Lượt xem" inputMode="numeric" value={form.vViews} onChange={(e) => set('vViews', e.target.value.replace(/\D/g, ''))} />
								<input placeholder="Đơn" inputMode="numeric" value={form.vOrders} onChange={(e) => set('vOrders', e.target.value.replace(/\D/g, ''))} />
								<input placeholder="Doanh thu" inputMode="numeric" value={form.vRevenue} onChange={(e) => set('vRevenue', e.target.value.replace(/\D/g, ''))} />
							</div>
							<button type="button" className="cp-btn" disabled={!/^https?:\/\//.test(form.vUrl.trim())} onClick={async () => {
								const ok = await run(() => api.post(`/collabs/${c._id}/videos`, { url: form.vUrl.trim(), views: Number(form.vViews) || 0, orders: Number(form.vOrders) || 0, revenue: Number(form.vRevenue) || 0 }), 'Đã thêm video.');
								if (ok) setForm((f) => ({ ...f, vUrl: '', vViews: '', vOrders: '', vRevenue: '' }));
							}}>Thêm video</button>
						</div>
					)}
				</div>
			);
		}
		const p = c.perf;
		const loss = p.profit < 0;
		return (
			<div className="cp-body">
				{c.mode === 'product' ? (
					<div className="cp-perf">
						<div><span>Số video</span><strong>{p.videos}</strong></div>
						<div><span>Tổng lượt xem</span><strong>{formatNumber(p.totalViews)}</strong></div>
						<div><span>Giá mỗi 1.000 lượt xem</span><strong>{p.totalViews ? formatCurrency(Math.round(p.costPerMille)) : '—'}</strong></div>
					</div>
				) : (
					<>
						<div className={loss ? 'cp-result loss' : 'cp-result profit'}>
							<small>{loss ? 'Lỗ lần này' : 'Lãi lần này'}</small>
							<strong>{loss ? '-' : '+'}{formatCurrency(Math.abs(p.profit))}</strong>
						</div>
						<div className="cp-perf">
							<div><span>Doanh thu</span><strong>{formatCurrency(p.revenue)}</strong></div>
							<div><span>Hoa hồng</span><strong>{formatCurrency(p.commission)}</strong></div>
							<div><span>Phí cast</span><strong>{formatCurrency(p.castFee)}</strong></div>
							<div><span>Đã duyệt</span><strong>{p.approved} mẫu</strong></div>
						</div>
					</>
				)}
				<div className="cp-card">
					<strong>Đơn từ link chia sẻ</strong>
					<small>Người mua bấm link KOC chia sẻ, không qua bài nào. Không nằm trong số của các thẻ bài.</small>
					<div className="cp-grid2">
						<input placeholder="Số đơn" inputMode="numeric" disabled={locked} value={form.shareOrders || String(c.shareOrders || '')} onChange={(e) => set('shareOrders', e.target.value.replace(/\D/g, ''))} onBlur={() => form.shareOrders !== '' && patch(c, { shareOrders: Number(form.shareOrders) })} />
						<input placeholder="Doanh thu" inputMode="numeric" disabled={locked} value={form.shareRevenue || String(c.shareRevenue || '')} onChange={(e) => set('shareRevenue', e.target.value.replace(/\D/g, ''))} onBlur={() => form.shareRevenue !== '' && patch(c, { shareRevenue: Number(form.shareRevenue) })} />
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="cp">
			{shown ? (
				<div className="cp-head">
					<div className="cp-head-top">
						<strong>Lần hợp tác thứ {shown.seq}</strong>
						<span className={closedView ? 'cp-pill closed' : 'cp-pill'}>{closedView ? 'Đã kết thúc' : '● Đang chạy'}</span>
					</div>
					{naming && !closedView ? (
						<input autoFocus className="cp-name-input" value={nameInput} maxLength={80} onChange={(e) => setNameInput(e.target.value)}
							onKeyDown={(e) => { if (e.key === 'Enter') { patch(shown, { name: nameInput }); setNaming(false); } if (e.key === 'Escape') setNaming(false); }}
							onBlur={() => setNaming(false)} />
					) : (
						<button type="button" className="cp-name" disabled={closedView} onClick={() => { setNameInput(shown.name); setNaming(true); }}>
							✎ {shown.name || 'Đặt tên cho lần này'}
						</button>
					)}
					<small>{fmtDay(shown.startedAt)} → {shown.endedAt ? fmtDay(shown.endedAt) : 'nay'}{closedView && shown.outcome ? ` · ${outcomeLabel(shown.outcome)}` : ''}</small>
					{closedView && (
						<div className="cp-row">
							<select value={shown.outcome} onChange={(e) => run(() => api.patch(`/collabs/${shown._id}/outcome`, { outcome: e.target.value }), 'Đã sửa kết cục.')}>
								{OUTCOMES.map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
							</select>
							<small className="cp-muted">Sửa kết cục</small>
						</div>
					)}
				</div>
			) : (
				<div className="cp-head">
					<strong>Chưa có lần hợp tác đang chạy</strong>
					<small>Chốt phí cast để mở lần hợp tác mới với KOC này.</small>
				</div>
			)}

			{owed > 0 && (
				<div className="cp-owed">
					Còn nợ {formatCurrency(owed)}
					<button type="button" className="cp-link" onClick={() => { const c = closedItems.find((x) => x.castFee > 0 && !x.feePaid && x.outcome === 'posted'); if (c) patch(c, { feePaid: true }, 'Đã đánh dấu trả phí.'); }}>Đánh dấu đã trả phí</button>
				</div>
			)}

			{closedItems.length > 0 && (
				<div className="cp-row cp-prev">
					{active && viewId && <button type="button" className="cp-link" onClick={() => setViewId(null)}>‹ Lần hiện tại</button>}
					{(!viewId || viewId === active?._id) && <button type="button" className="cp-link" onClick={() => setViewId(closedItems[0]._id)}>Lần trước ›</button>}
					{viewId && closedItems.length > 1 && (
						<select value={viewId} onChange={(e) => setViewId(e.target.value)}>
							{closedItems.map((c) => <option key={c._id} value={c._id}>Lần {c.seq}{c.name ? ` · ${c.name}` : ''}</option>)}
						</select>
					)}
				</div>
			)}

			{children}

			{!shown || (!active && !viewId) ? (
				<button type="button" className="cp-btn wide" onClick={() => { setFeeInput(''); setDialog('fee'); }}>Chốt phí cast</button>
			) : (
				<ol className="cp-steps">
					{STEPS.map((label, i) => {
						const st = stepState(i);
						return (
							<li key={label} className={`cp-step ${st}`}>
								<span className="cp-dot" />
								<button type="button" className="cp-step-title" disabled={st === 'future'} onClick={() => setOpenStep(isOpen(i) ? -1 : i)}>{label}</button>
								{st !== 'future' && isOpen(i) && renderStepBody(shown, i)}
							</li>
						);
					})}
				</ol>
			)}

			{active && !viewId && (
				<button type="button" className="cp-btn ghost wide cp-end" onClick={() => { setClosePaid(false); setDialog('close'); }}>Kết thúc lần hợp tác</button>
			)}

			{dialog === 'fee' && (
				<div className="cp-backdrop" onClick={() => setDialog(null)}>
					<div className="cp-modal" onClick={(e) => e.stopPropagation()}>
						<h4>Phí cast</h4>
						<div className="cp-chips">
							{FEE_SUGGESTIONS.map((f) => <button key={f} type="button" className="cp-chip" onClick={() => setFeeInput(String(f))}>{formatCurrency(f)}</button>)}
						</div>
						<input autoFocus inputMode="numeric" placeholder="Nhập số tiền (đ)" value={feeInput ? Number(feeInput.replace(/\D/g, '')).toLocaleString('vi-VN') : ''} onChange={(e) => setFeeInput(e.target.value.replace(/\D/g, ''))} onKeyDown={(e) => e.key === 'Enter' && saveFee()} />
						<div className="cp-row end">
							<button type="button" className="cp-btn ghost" onClick={() => saveFee(true)}>Free cast</button>
							<button type="button" className="cp-btn" onClick={() => saveFee()}>Lưu phí</button>
						</div>
					</div>
				</div>
			)}

			{dialog === 'close' && active && (
				<div className="cp-backdrop" onClick={() => setDialog(null)}>
					<div className="cp-modal" onClick={(e) => e.stopPropagation()}>
						<h4>Kết thúc lần hợp tác</h4>
						{OUTCOMES.map((o) => (
							<label key={o.key} className="cp-check"><input type="radio" name="outcome" checked={closeOutcome === o.key} onChange={() => setCloseOutcome(o.key)} /> {o.label}</label>
						))}
						{closeOutcome === 'posted' && active.castFee > 0 && !active.feePaid && (
							<label className="cp-check"><input type="checkbox" checked={closePaid} onChange={(e) => setClosePaid(e.target.checked)} /> Đã trả phí cast</label>
						)}
						<div className="cp-row end">
							<button type="button" className="cp-btn ghost" onClick={() => setDialog(null)}>Huỷ</button>
							<button type="button" className="cp-btn" onClick={async () => { if (await run(() => api.post(`/collabs/${active._id}/close`, { outcome: closeOutcome, paid: closePaid }), 'Đã kết thúc lần hợp tác.')) setDialog(null); }}>Xác nhận</button>
						</div>
					</div>
				</div>
			)}

			{dialog === 'sample-reject' && active && (
				<div className="cp-backdrop" onClick={() => setDialog(null)}>
					<div className="cp-modal" onClick={(e) => e.stopPropagation()}>
						<h4>Từ chối yêu cầu mẫu</h4>
						{REJECT_REASONS.map((r) => (
							<label key={r} className="cp-check"><input type="radio" name="reject" checked={rejectReason === r} onChange={() => setRejectReason(r)} /> {r}</label>
						))}
						<div className="cp-row end">
							<button type="button" className="cp-btn ghost" onClick={() => setDialog(null)}>Huỷ</button>
							<button type="button" className="cp-btn" onClick={async () => { if (await run(() => api.post(`/collabs/${active._id}/samples/${rejectSampleId}/reject`, { reason: rejectReason }), 'Đã từ chối yêu cầu mẫu.')) setDialog(null); }}>Từ chối</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
}
