import { type KeyboardEvent, type ReactNode, useCallback, useEffect, useRef, useState } from 'react';
import api from '../../lib/axios';
import { formatCurrency, formatNumber } from '../../lib/formatters';
import type { Collab, CollabMode, CollabOutcome, Creator } from '../../types/koc';
import OpenCollabModal from './OpenCollabModal';

const STEPS = ['Chốt phí cast', 'Chọn hình thức hợp tác', 'Duyệt & gửi mẫu', 'KOC đăng bài', 'Hiệu quả'];
const FEE_SUGGESTIONS = [300000, 400000, 500000, 600000];
const REJECT_REASONS = ['Hết hàng mẫu', 'KOC không phù hợp ngành hàng', 'Ngoài ngân sách', 'Khác'];
const OUTCOMES: { key: CollabOutcome; label: string }[] = [
	{ key: 'posted', label: 'KOC đã đăng bài' },
	{ key: 'no_post', label: 'Nhận mẫu rồi không đăng' },
	{ key: 'no_response', label: 'Không nhận mẫu / không phản hồi' },
	{ key: 'no_need', label: 'Hết nhu cầu hợp tác' },
];
const MODES: { key: CollabMode; title: string; sub: string }[] = [
	{ key: 'targeted', title: 'Mời cộng tác mục tiêu', sub: 'Hoa hồng riêng cho KOC này' },
	{ key: 'open', title: 'Tạo link cộng tác mở', sub: 'Hoa hồng theo mức công khai của shop' },
	{ key: 'product', title: 'Dán link sản phẩm', sub: 'TikToday tự theo dõi bài KOC đăng' },
];
// "product" is no longer offered for new collaborations, but older ones may still use it and must keep rendering.
const PICKABLE_MODES = MODES.filter((m) => m.key !== 'product');
// The targeted-invite form stays off for now (plain card); the open-collaboration picker is wired to TikTok Shop.
const MODE_FORMS_ENABLED = false;

const fmtDay = (iso?: string | null) => (iso ? new Date(iso).toLocaleDateString('vi-VN') : '');
const outcomeLabel = (o: string) => OUTCOMES.find((x) => x.key === o)?.label || '';

interface Props {
	koc: Creator | null;
	apiReady: boolean;
	onOpenShipping: () => void;
	onTrackShipment: (creator: Creator | null, code?: string, phone4?: string) => void;
	onInsertDraft: (text: string) => void;
	onChanged: () => void;
	showToast: (msg: string, type?: 'success' | 'error') => void;
	children?: ReactNode;
}

/** "[ DEAL MỞ BÁN ] Áo Polo Cao cấp Cooltech,Chất Liệu Micax… | COOLTECH" -> "Áo Polo Cao cấp Cooltech" (short enough to read in a message). */
function shortProductName(title: string) {
	const parts = title.replace(/^\s*\[[^\]]*\]\s*/, '').split(/,| - | \| /).map((x) => x.trim()).filter(Boolean);
	let t = parts.find((x) => x.length >= 12) || parts[0] || title.trim(); // skip short tags such as "Deal Hot |"
	if (t.length > 60) t = t.slice(0, 60).replace(/\s+\S*$/, '');
	return t;
}

export default function CollabPanel({ koc, apiReady, onOpenShipping, onTrackShipment, onInsertDraft, onChanged, showToast, children }: Props) {
	const [items, setItems] = useState<Collab[]>([]);
	const [owed, setOwed] = useState(0);
	const [loading, setLoading] = useState(false);
	const [viewId, setViewId] = useState<string | null>(null);
	const [openStep, setOpenStep] = useState<number | null>(null);
	const [dialog, setDialog] = useState<'fee' | 'close' | 'sample-reject' | 'open-products' | 'change-mode' | 'ads-code' | null>(null);
	const [pickSaving, setPickSaving] = useState(false);
	const [linkBusy, setLinkBusy] = useState(false);
	const [adsIdx, setAdsIdx] = useState<number | null>(null); // video that gets the ads code
	const [adsInput, setAdsInput] = useState('');
	const [feeInput, setFeeInput] = useState('');
	const [showHistory, setShowHistory] = useState(false);
	const [naming, setNaming] = useState(false);
	const [nameInput, setNameInput] = useState('');
	const reqRef = useRef(0); // bumped per load: a slow reply for a KOC you already left must not show under the new one
	const nameDone = useRef(false);
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
		const id = ++reqRef.current;
		setLoading(true);
		api.get(`/kocs/${koc.id}/collabs`)
			.then(({ data }) => {
				if (id !== reqRef.current) return;
				setItems(data.data.items || []);
				setOwed(data.data.owed || 0);
			})
			.catch(() => id === reqRef.current && showToast('Không tải được các lần hợp tác.', 'error'))
			.finally(() => id === reqRef.current && setLoading(false));
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

	// Asks TikTok Shop for the affiliate link of the chosen open-collaboration product (a creator who opens it adds the product).
	async function makeLink(c: Collab) {
		setLinkBusy(true);
		await run(() => api.post(`/collabs/${c._id}/open-link`, {}), 'Đã tạo link cộng tác mở.');
		setLinkBusy(false);
	}

	function copyLink(link: string) {
		navigator.clipboard.writeText(link).then(
			() => showToast('Đã sao chép link.'),
			() => showToast('Không sao chép được, hãy bôi đen link và copy thủ công.', 'error'),
		);
	}

	// The usual outreach message, with the real product, link and public commission. The Shop Ads rate is not available
	// from TikTok's seller API, so that sentence is left for the user to add by hand.
	const inviteText = (c: Collab) =>
		`Chúng mình có theo dõi kênh ${koc?.handle || 'của bạn'} và nhận thấy định hướng nội dung, hình ảnh của bạn khá phù hợp với hướng phát triển mà Cooltech đang xây dựng. Vì vậy, chúng mình mong muốn đề xuất hợp tác cùng bạn với sản phẩm ${shortProductName(c.productName)} của bên mình:
${c.productLink}
Sau khi nhận sản phẩm, bạn có thể lên video review/giới thiệu sản phẩm, hoặc đưa vào tuyến nội dung sử dụng sản phẩm thật theo phong cách của bạn thường làm. Mức hoa hồng chúng mình đề xuất là ${c.commissionRate}% cho đơn tự nhiên.`;

	// Go back to choosing the collaboration form; the chosen product and its link are cleared on the server.
	function changeMode() {
		setDialog('change-mode');
	}

	// Save once even though Enter also blurs the field; an unchanged name is not re-sent.
	function commitName(c: Collab, save: boolean) {
		if (nameDone.current) return;
		nameDone.current = true;
		setNaming(false);
		if (save && nameInput.trim() !== (c.name || '').trim()) patch(c, { name: nameInput.trim() });
	}

	if (!koc) return <div className="cp-empty">Chọn một KOC để xem lần hợp tác.</div>;
	if (!live) return <div className="cp-empty">Kết nối API và mở KOC thật để dùng Lần hợp tác.</div>;
	if (loading && items.length === 0) return <div className="cp-empty">Đang tải…</div>;

	const stepState = (i: number) => (!shown ? 'future' : shown.status === 'closed' || i < shown.step ? 'done' : i === shown.step ? 'current' : 'future');
	const isOpen = (i: number) => (openStep === null ? stepState(i) === 'current' : openStep === i);

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
						<div className="cp-card selected cp-picked">
							<div className="cp-picked-head">
								{c.productImage ? <img className="cp-picked-img" src={c.productImage} alt="" loading="lazy" /> : <span className="cp-picked-img" />}
								<div className="cp-picked-info">
									<small className="cp-picked-mode">{m.title}</small>
									<strong title={c.productName}>{c.productName || m.sub}</strong>
									{c.mode === 'open' && c.commissionRate > 0 && <span className="cp-oc-rate">Hoa hồng {c.commissionRate}%</span>}
								</div>
							</div>
							{!locked && (
								<div className="cp-picked-actions">
									{c.mode === 'open' && <button type="button" className="cp-btn ghost" onClick={() => setDialog('open-products')}>Đổi sản phẩm</button>}
									<button type="button" className="cp-btn ghost" onClick={changeMode}>Đổi hình thức</button>
								</div>
							)}
						</div>
						{c.mode === 'open' && c.productLink && (
							<div className="cp-card cp-link-card">
								<small className="cp-muted">Link gửi cho KOC</small>
								<a className="cp-link-url" href={c.productLink} target="_blank" rel="noreferrer">{c.productLink}</a>
								<button type="button" className="cp-btn wide" onClick={() => { onInsertDraft(inviteText(c)); showToast('Đã chèn lời mời vào khung chat.'); }}>Chèn lời mời vào chat</button>
								<div className="cp-row cp-link-actions">
									<button type="button" className="cp-btn ghost" onClick={() => copyLink(c.productLink)}>Sao chép link</button>
									{!locked && <button type="button" className="cp-btn ghost" disabled={linkBusy} onClick={() => makeLink(c)}>{linkBusy ? 'Đang tạo…' : 'Tạo lại link'}</button>}
								</div>
							</div>
						)}
						{c.mode === 'open' && !c.productLink && !locked && (
							<div className="cp-card cp-link-card">
								<small className="cp-muted">Chưa có link để gửi cho KOC.</small>
								<button type="button" className="cp-btn wide" disabled={linkBusy} onClick={() => makeLink(c)}>{linkBusy ? 'Đang tạo…' : 'Tạo link cộng tác mở'}</button>
							</div>
						)}
					</div>
				);
			}
			return (
				<div className="cp-body">
					{PICKABLE_MODES.map((m) => {
						return (
							<div
								key={m.key}
								className={`cp-card cp-mode${m.key === 'open' ? ' clickable' : ''}`}
								{...(m.key === 'open' ? { role: 'button', tabIndex: 0, onClick: () => setDialog('open-products'), onKeyDown: (e: KeyboardEvent<HTMLDivElement>) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setDialog('open-products'); } } } : {})}
							>
								<strong>{m.title}</strong>
								<small>{m.sub}</small>
								{MODE_FORMS_ENABLED && m.key === 'targeted' && (
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
								<button type="button" className="cp-link" onClick={() => { setAdsInput(''); setAdsIdx(idx); setDialog('ads-code'); }}>+ Thêm code ads</button>
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
						<div className="cp-name-edit">
							<input autoFocus className="cp-name-input" value={nameInput} maxLength={80} placeholder="Tên cho lần hợp tác này" onChange={(e) => setNameInput(e.target.value)}
								onKeyDown={(e) => { if (e.key === 'Enter') commitName(shown, true); if (e.key === 'Escape') commitName(shown, false); }}
								onBlur={() => commitName(shown, true)} />
							<small>Enter để lưu · Esc để hủy</small>
						</div>
					) : (
						<button type="button" className={`cp-name${shown.name ? ' named' : ''}`} disabled={closedView} onClick={() => { setNameInput(shown.name); nameDone.current = false; setNaming(true); }}>
							<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M12 20h9" /><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" /></svg>
							<span>{shown.name || 'Đặt tên cho lần này'}</span>
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
						const open = st !== 'future' && isOpen(i);
						return (
							<li key={label} className={`cp-step ${st}${open ? ' open' : ''}`}>
								<span className="cp-dot" />
								<button type="button" className="cp-step-title" aria-expanded={open} aria-controls={`cp-step-${i}`} disabled={st === 'future'} onClick={() => setOpenStep(open ? -1 : i)}>{label}</button>
								<div id={`cp-step-${i}`} className={`cp-collapse${open ? ' open' : ''}`} aria-hidden={!open}>
									<div className="cp-collapse-inner">{st !== 'future' && renderStepBody(shown, i)}</div>
								</div>
							</li>
						);
					})}
				</ol>
			)}

			{active && !viewId && (
				<button type="button" className="cp-btn ghost wide cp-end" onClick={() => { setClosePaid(false); setDialog('close'); }}>Kết thúc lần hợp tác</button>
			)}

			{dialog === 'open-products' && active && (
				<OpenCollabModal
					saving={pickSaving}
					onClose={() => setDialog(null)}
					onConfirm={async (p) => {
						setPickSaving(true);
						const ok = await patch(active, { mode: 'open', productId: p.productId }, 'Đã chọn Link cộng tác mở.');
						setPickSaving(false);
						if (ok) {
							setDialog(null);
							makeLink(ok);
						}
					}}
				/>
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

			{dialog === 'change-mode' && active && (
				<div className="cp-backdrop" onClick={() => setDialog(null)}>
					<div className="cp-modal" role="alertdialog" aria-modal="true" aria-labelledby="cp-cm-title" onClick={(e) => e.stopPropagation()}>
						<h4 id="cp-cm-title">Chọn lại hình thức hợp tác?</h4>
						<p className="cp-muted cp-dialog-text">Sản phẩm và link đã chọn sẽ bị xóa. Phí cast và các bước khác của lần hợp tác được giữ nguyên.</p>
						<div className="cp-row end">
							<button type="button" className="cp-btn ghost" onClick={() => setDialog(null)}>Huỷ</button>
							<button type="button" className="cp-btn" onClick={async () => { if (await patch(active, { resetMode: true }, 'Đã mở lại bước chọn hình thức.')) setDialog(null); }}>Chọn lại</button>
						</div>
					</div>
				</div>
			)}

			{dialog === 'ads-code' && active && adsIdx !== null && (
				<div className="cp-backdrop" onClick={() => setDialog(null)}>
					<div className="cp-modal" role="dialog" aria-modal="true" aria-labelledby="cp-ads-title" onClick={(e) => e.stopPropagation()}>
						<h4 id="cp-ads-title">Code ads của video</h4>
						<p className="cp-muted cp-dialog-text">Nhập code ads TikTok của video này để theo dõi đơn từ quảng cáo.</p>
						<input
							autoFocus
							placeholder="Nhập code ads"
							value={adsInput}
							onChange={(e) => setAdsInput(e.target.value)}
							onKeyDown={async (e) => { if (e.key === 'Enter' && adsInput.trim()) { e.preventDefault(); if (await run(() => api.put(`/collabs/${active._id}/videos/${adsIdx}/ads`, { code: adsInput.trim() }), 'Đã lưu code ads.')) setDialog(null); } }}
						/>
						<div className="cp-row end">
							<button type="button" className="cp-btn ghost" onClick={() => setDialog(null)}>Huỷ</button>
							<button type="button" className="cp-btn" disabled={!adsInput.trim()} onClick={async () => { if (await run(() => api.put(`/collabs/${active._id}/videos/${adsIdx}/ads`, { code: adsInput.trim() }), 'Đã lưu code ads.')) setDialog(null); }}>Lưu</button>
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
