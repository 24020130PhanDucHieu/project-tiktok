import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Portal from '../../components/Portal';
import Select from '../../components/Select';
import SendingSwitch from './SendingSwitch';
import api, { apiMessage } from '../../lib/axios';
import type { ShopProduct, TargetConfig, TargetCreator, TargetInviteInput, TargetInviteRecord, TargetPreview } from '../../types/koc';
import { composeMessage, newRequestKey, runes, toNumber, tomorrow, vnd } from './targetUtils';

interface SelectedProduct {
	id: string;
	title: string;
	commission: string; // percent as typed
	ads: string;
}

export interface SentInvite {
	record: TargetInviteRecord;
	name: string;
	products: { id: string; title: string; commissionPct: number }[];
}

interface Props {
	isAdmin: boolean;
	onClose: () => void;
	onSent?: (sent: SentInvite) => void;
	/** the switch was flipped inside this window: tell the page behind it */
	onSendingChange?: (enabled: boolean) => void;
	showToast: (m: string, t?: 'success' | 'error') => void;
	/** pre-filled KOC (username as on TikTok) */
	initialCreator?: { username: string };
	initialProduct?: { id: string; title: string; commissionPct?: number };
	kocId?: string;
	collabId?: string;
}

function Section({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
	return (
		<section className="ti-section">
			<h3><span className="ti-num">{n}</span>{title}</h3>
			<div className="ti-section-body">{children}</div>
		</section>
	);
}

function Row({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
	return (
		<div className="ti-row">
			<div className="ti-row-side"><b>{label}</b>{hint && <small>{hint}</small>}</div>
			<div className="ti-row-main">{children}</div>
		</div>
	);
}

/** Centered window to create a target collaboration invitation: the same form as TikTok Shop's "Tạo lời mời". */
export default function TargetInviteModal({ isAdmin, onClose, onSent, onSendingChange, showToast, initialCreator, initialProduct, kocId, collabId }: Props) {
	const [cfg, setCfg] = useState<TargetConfig | null>(null);
	const [name, setName] = useState('');
	const [endDate, setEndDate] = useState('');
	const [email, setEmail] = useState('');
	const [zalo, setZalo] = useState('');
	const [facebook, setFacebook] = useState('');
	const [contentPref, setContentPref] = useState<'' | 'video' | 'live'>('video');
	const [message, setMessage] = useState('');
	const [hasFreeSample, setHasFreeSample] = useState(true);
	const [sampleExempt, setSampleExempt] = useState(false);
	const [products, setProducts] = useState<SelectedProduct[]>(
		initialProduct ? [{ id: initialProduct.id, title: initialProduct.title, commission: initialProduct.commissionPct && initialProduct.commissionPct >= 10 ? String(initialProduct.commissionPct) : '', ads: '' }] : [],
	);
	const [creators, setCreators] = useState<TargetCreator[]>([]);

	const [catalog, setCatalog] = useState<ShopProduct[]>([]);
	const [catalogNext, setCatalogNext] = useState('');
	const [catalogOpen, setCatalogOpen] = useState(false);
	const [catalogLoading, setCatalogLoading] = useState(false);
	const [catalogFilter, setCatalogFilter] = useState('');

	const [query, setQuery] = useState('');
	const [found, setFound] = useState<TargetCreator[]>([]);
	const [searching, setSearching] = useState(false);
	const [searchNote, setSearchNote] = useState('');

	const [preview, setPreview] = useState<TargetPreview | null>(null);
	const [previewKey, setPreviewKey] = useState('');
	const [checking, setChecking] = useState(false);
	const [confirmed, setConfirmed] = useState(false);
	const [sending, setSending] = useState(false);
	const [error, setError] = useState('');
	const [blocked, setBlocked] = useState(false); // an earlier attempt may have reached TikTok: do not send again from here
	const requestKey = useRef(newRequestKey());
	const inFlight = useRef(false); // a second click in the same tick must not start a second request

	useEffect(() => {
		api.get<{ data: TargetConfig }>('/target-collabs/config').then(({ data }) => setCfg(data.data)).catch(() => setError('Không tải được cấu hình lời mời.'));
		const onKey = (e: KeyboardEvent) => e.key === 'Escape' && !sending && onClose();
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	}, [onClose, sending]);

	const input = useMemo<TargetInviteInput>(
		() => ({
			name, endDate, email, zalo, facebook, contentPref, message, hasFreeSample, sampleApprovalExempt: sampleExempt,
			products: products.map((p) => ({ id: p.id, title: p.title, commissionPct: toNumber(p.commission), adsCommissionPct: toNumber(p.ads) })),
			creators: creators.map((c) => ({ openId: c.openId, username: c.username })),
		}),
		[name, endDate, email, zalo, facebook, contentPref, message, hasFreeSample, sampleExempt, products, creators],
	);
	const formKey = useMemo(() => JSON.stringify(input), [input]);
	const ready = !!preview?.ready && previewKey === formKey;
	const stale = !!preview && previewKey !== formKey;

	// any edit invalidates the check and the confirmation
	useEffect(() => {
		setConfirmed(false);
	}, [formKey]);

	const composed = composeMessage(input);

	// ---- creators ----
	const searchCreators = useCallback(async (kw: string) => {
		setSearching(true);
		setSearchNote('');
		try {
			const { data } = await api.get<{ data: { items: TargetCreator[] } }>('/target-collabs/creators', { params: { keyword: kw } });
			setFound(data.data.items);
			if (data.data.items.length === 0) setSearchNote('Không có nhà sáng tạo nào khớp trong Creator Marketplace.');
			return data.data.items;
		} catch (e) {
			setFound([]);
			setSearchNote(apiMessage(e, 'Không tìm được nhà sáng tạo.'));
			return [];
		} finally {
			setSearching(false);
		}
	}, []);

	useEffect(() => {
		const kw = query.trim().replace(/^@/, '');
		if (kw.length < 2) {
			setFound([]);
			setSearchNote('');
			return undefined;
		}
		const t = window.setTimeout(() => searchCreators(kw), 700);
		return () => window.clearTimeout(t);
	}, [query, searchCreators]);

	// the KOC we came from: added only when TikTok has a creator with exactly that username
	useEffect(() => {
		const want = initialCreator?.username?.replace(/^@/, '').trim();
		if (!want) return;
		setQuery(want);
		searchCreators(want).then((items) => {
			const exact = items.find((c) => c.username.toLowerCase() === want.toLowerCase());
			if (exact) setCreators((prev) => (prev.some((c) => c.openId === exact.openId) ? prev : [...prev, exact]));
			else setSearchNote(`Không tìm thấy @${want} trong Creator Marketplace, nên chưa thể mời bằng cộng tác mục tiêu.`);
		});
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, []);

	const addCreator = (c: TargetCreator) => {
		if (creators.some((x) => x.openId === c.openId)) return;
		if (cfg && creators.length >= cfg.maxCreators) {
			showToast(`Mỗi lời mời gửi tối đa ${cfg.maxCreators} nhà sáng tạo.`, 'error');
			return;
		}
		setCreators((prev) => [...prev, c]);
	};

	// ---- products ----
	const loadCatalog = useCallback(async (token = '') => {
		setCatalogLoading(true);
		try {
			const { data } = await api.get<{ data: { items: ShopProduct[]; nextPageToken: string } }>('/target-collabs/products', { params: { pageToken: token || undefined } });
			setCatalog((prev) => (token ? [...prev, ...data.data.items] : data.data.items));
			setCatalogNext(data.data.nextPageToken || '');
		} catch (e) {
			showToast(apiMessage(e, 'Không tải được danh sách sản phẩm.'), 'error');
		} finally {
			setCatalogLoading(false);
		}
	}, [showToast]);

	const openCatalog = () => {
		setCatalogOpen(true);
		if (catalog.length === 0) loadCatalog();
	};

	const addProduct = (p: ShopProduct) => {
		if (products.some((x) => x.id === p.id)) return;
		if (cfg && products.length >= cfg.maxProducts) {
			showToast(`Mỗi lời mời tối đa ${cfg.maxProducts} sản phẩm.`, 'error');
			return;
		}
		setProducts((prev) => [...prev, { id: p.id, title: p.title, commission: '', ads: '' }]);
	};

	const patchProduct = (id: string, patch: Partial<SelectedProduct>) => setProducts((prev) => prev.map((p) => (p.id === id ? { ...p, ...patch } : p)));

	const shownCatalog = catalog.filter((p) => !catalogFilter.trim() || p.title.toLowerCase().includes(catalogFilter.trim().toLowerCase()) || p.id.includes(catalogFilter.trim()));

	// ---- check and send ----
	async function check() {
		setChecking(true);
		setError('');
		try {
			const { data } = await api.post<{ data: { preview: TargetPreview } }>('/target-collabs/preview', input);
			setPreview(data.data.preview);
			setPreviewKey(formKey);
		} catch (e) {
			setPreview(null);
			setError(apiMessage(e, 'Không kiểm tra được lời mời.'));
		} finally {
			setChecking(false);
		}
	}

	async function send() {
		if (!ready || !confirmed || sending || blocked || inFlight.current) return;
		inFlight.current = true;
		setSending(true);
		setError('');
		try {
			const { data } = await api.post<{ data: { invite: TargetInviteRecord; duplicate?: boolean } }>('/target-collabs', { requestKey: requestKey.current, confirm: true, kocId, collabId, invite: input });
			const record = data.data.invite;
			showToast(data.data.duplicate ? 'Lời mời này đã được gửi trước đó.' : 'Đã gửi lời mời cho nhà sáng tạo.');
			onSent?.({ record, name: name.trim(), products: input.products.map((p) => ({ id: p.id, title: p.title, commissionPct: p.commissionPct })) });
			onClose();
		} catch (e: any) {
			const msg = apiMessage(e, 'Không gửi được lời mời.');
			setError(msg);
			if (msg.startsWith('Chưa rõ') || msg.startsWith('Yêu cầu này đã')) setBlocked(true); // possibly sent: never resend from this window
			else requestKey.current = newRequestKey(); // TikTok refused: a corrected form is a new request
			setConfirmed(false);
		} finally {
			setSending(false);
			inFlight.current = false;
		}
	}

	const canSend = ready && confirmed && !sending && !blocked && isAdmin && !!cfg?.enabled;
	const first = products[0];
	const nameLeft = cfg ? cfg.maxName - runes(name) : 30 - runes(name);
	const msgLen = runes(composed);
	const maxMsg = cfg?.maxMessage ?? 500;

	return (
		<Portal>
		<div className="cp-backdrop" onClick={() => !sending && onClose()}>
			<div className="ti-modal" role="dialog" aria-modal="true" aria-label="Mời cộng tác mục tiêu" onClick={(e) => e.stopPropagation()}>
				<div className="ti-head">
					<div>
						<h4>Mời cộng tác mục tiêu</h4>
						<small>Gửi lời mời riêng tới nhà sáng tạo, với hoa hồng riêng cho từng sản phẩm. Lời mời được tạo thật trên TikTok Shop.</small>
					</div>
					<button type="button" className="cp-oc-x" aria-label="Đóng" onClick={onClose} disabled={sending}>×</button>
				</div>

				{cfg && (
					<div className={`ti-banner switch${cfg.enabled ? '' : ' warn'}`}>
						<span>
							{cfg.enabled
								? <>Gửi lời mời thật đang <b>bật</b>: bấm "Gửi lời mời" sẽ tạo lời mời thật trên TikTok. Tắt lại bất cứ lúc nào.</>
								: <>Gửi lời mời đang <b>tắt</b> (mặc định, để an toàn). Bạn vẫn soạn và bấm "Kiểm tra" được. Gạt công tắc để bật gửi.</>}
						</span>
						<span className="ti-banner-switch"><b>Cho phép gửi lời mời thật</b><SendingSwitch enabled={cfg.enabled} isAdmin={isAdmin} showToast={showToast} onChanged={(on) => { setCfg((c) => (c ? { ...c, enabled: on } : c)); onSendingChange?.(on); }} /></span>
					</div>
				)}
				{!isAdmin && <div className="ti-banner">Chỉ quản trị viên được bấm Gửi lời mời. Bạn có thể soạn và kiểm tra.</div>}

				<div className="ti-body">
					<div className="ti-main">
						<Section n={1} title="Tạo lời mời">
							<Row label="Lưu ý" hint="Đặt tên dễ nhận ra (mục đích, sự kiện…). Nhà sáng tạo không nhìn thấy tên này. Hết hạn rồi thì họ không nhận được lời mời nữa.">
								<div className="ti-note">Thời hạn tối đa 365 ngày, tính đến hết ngày bạn chọn (giờ Việt Nam).</div>
							</Row>
							<Row label="Tên lời mời *">
								<div className="ti-field">
									<input value={name} maxLength={cfg?.maxName ?? 30} onChange={(e) => setName(e.target.value)} placeholder="Tên lời mời" />
									<span className="ti-count">{runes(name)}/{cfg?.maxName ?? 30}</span>
								</div>
								{nameLeft < 0 && <small className="ti-err">Quá {-nameLeft} ký tự.</small>}
							</Row>
							<Row label="Có hiệu lực đến *">
								<input type="date" className="ti-input" min={tomorrow()} value={endDate} onChange={(e) => setEndDate(e.target.value)} />
							</Row>
							<Row label="Thông tin liên hệ" hint="Chia sẻ để nhà sáng tạo được mời có thể liên hệ và đặt câu hỏi. Zalo và Facebook được ghi vào cuối lời nhắn.">
								<input type="email" className="ti-input" placeholder="Email liên hệ *" value={email} onChange={(e) => setEmail(e.target.value)} />
								<input className="ti-input" placeholder="Tài khoản Zalo (số điện thoại)" value={zalo} onChange={(e) => setZalo(e.target.value)} />
								<input className="ti-input" placeholder="Tài khoản Facebook" value={facebook} onChange={(e) => setFacebook(e.target.value)} />
							</Row>
							<Row label="Nội dung lời mời *" hint="Giới thiệu bản thân và chia sẻ lý do bạn mong muốn cộng tác.">
								<div className="ti-field area">
									<textarea rows={6} value={message} onChange={(e) => setMessage(e.target.value)} placeholder="Chào bạn! Bên mình mong muốn được cùng bạn hợp tác quảng bá sản phẩm…" />
									<span className={`ti-count${msgLen > maxMsg ? ' bad' : ''}`}>{msgLen}/{maxMsg}</span>
								</div>
								<small className="ti-hint">Con số đã tính cả phần liên hệ và ưu tiên nối ở cuối.</small>
							</Row>
							<Row label="Loại nội dung ưu tiên" hint="Cho nhà sáng tạo biết bạn ưu tiên video có link bán hàng hay LIVE.">
								<Select className="ti-select" value={contentPref} onChange={(e) => setContentPref(e.target.value as '' | 'video' | 'live')}>
									<option value="video">Video có link bán hàng</option>
									<option value="live">LIVE bán hàng</option>
									<option value="">Không ưu tiên</option>
								</Select>
							</Row>
						</Section>

						<Section n={2} title="Chọn sản phẩm">
							{products.length === 0 ? (
								<div className="ti-empty">Chọn và thêm sản phẩm vào lời mời, rồi đặt tỷ lệ hoa hồng.</div>
							) : (
								<div className="ti-table-wrap">
									<table className="ti-table">
										<thead>
											<tr><th>Sản phẩm</th><th>Hoa hồng tiêu chuẩn %</th><th>Hoa hồng quảng cáo cửa hàng %</th><th /></tr>
										</thead>
										<tbody>
											{products.map((p) => (
												<tr key={p.id}>
													<td><b className="ti-ptitle" title={p.title}>{p.title}</b><small>ID: {p.id}</small></td>
													<td><input className="ti-rate" inputMode="decimal" placeholder={`${cfg?.minRatePct ?? 10}–${cfg?.maxRatePct ?? 80}`} value={p.commission} onChange={(e) => patchProduct(p.id, { commission: e.target.value })} /></td>
													<td><input className="ti-rate" inputMode="decimal" placeholder="Để trống nếu không đặt" value={p.ads} onChange={(e) => patchProduct(p.id, { ads: e.target.value })} /></td>
													<td><button type="button" className="ti-x" aria-label="Bỏ sản phẩm" onClick={() => setProducts((prev) => prev.filter((x) => x.id !== p.id))}>×</button></td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
							)}
							<small className="ti-hint">Hoa hồng tiêu chuẩn từ {cfg?.minRatePct ?? 10}% đến {cfg?.maxRatePct ?? 80}%. Hoa hồng quảng cáo từ {cfg?.minAdsRatePct ?? 1}% đến {cfg?.maxRatePct ?? 80}% (chỉ áp dụng cho đơn từ quảng cáo). Tối đa {cfg?.maxProducts ?? 20} sản phẩm.</small>
							<div className="ti-actions"><button type="button" className="cp-btn ghost" onClick={openCatalog}>+ Thêm sản phẩm</button></div>
							{catalogOpen && (
								<div className="ti-picker">
									<div className="ti-picker-head">
										<input className="ti-input" placeholder="Lọc theo tên hoặc mã sản phẩm" value={catalogFilter} onChange={(e) => setCatalogFilter(e.target.value)} />
										<button type="button" className="ti-x" aria-label="Đóng" onClick={() => setCatalogOpen(false)}>×</button>
									</div>
									<div className="ti-picker-list">
										{shownCatalog.map((p) => {
											const added = products.some((x) => x.id === p.id);
											return (
												<div key={p.id} className="ti-picker-row">
													<span><b title={p.title}>{p.title}</b><small>{p.minPrice ? `${vnd(p.minPrice)}${p.maxPrice !== p.minPrice ? ` – ${vnd(p.maxPrice)}` : ''}` : 'Chưa có giá'} · Còn {p.inventory}</small></span>
													<button type="button" className="cp-btn ghost" disabled={added} onClick={() => addProduct(p)}>{added ? 'Đã thêm' : 'Thêm'}</button>
												</div>
											);
										})}
										{catalogLoading && <div className="ti-empty">Đang tải…</div>}
										{!catalogLoading && shownCatalog.length === 0 && <div className="ti-empty">Không có sản phẩm khớp.</div>}
										{catalogNext && !catalogLoading && <button type="button" className="cp-link" onClick={() => loadCatalog(catalogNext)}>Tải thêm sản phẩm</button>}
									</div>
								</div>
							)}
						</Section>

						<Section n={3} title="Cài đặt hàng mẫu miễn phí">
							<label className="ti-switch">
								<input type="checkbox" checked={hasFreeSample} onChange={(e) => setHasFreeSample(e.target.checked)} />
								<span className="ti-track"><i /></span>
								<b>Cung cấp hàng mẫu miễn phí</b>
							</label>
							{hasFreeSample && (
								<div className="ti-radios">
									<label className={`ti-radio${!sampleExempt ? ' on' : ''}`}>
										<input type="radio" name="ti-sample" checked={!sampleExempt} onChange={() => setSampleExempt(false)} />
										<span><b>Dùng cài đặt hàng mẫu cấp sản phẩm</b><small>Quy tắc duyệt và giới hạn bạn đã đặt trong Cài đặt hàng mẫu sẽ áp dụng. Bạn cần xem xét thủ công các yêu cầu với sản phẩm chưa đặt quy tắc.</small></span>
									</label>
									<label className={`ti-radio${sampleExempt ? ' on' : ''}`}>
										<input type="radio" name="ti-sample" checked={sampleExempt} onChange={() => setSampleExempt(true)} />
										<span><b>Phê duyệt mọi người trong lời mời này</b><small>Tất cả nhà sáng tạo trong lời mời này tự động được duyệt nhận hàng mẫu miễn phí, bất kể cài đặt cấp sản phẩm.</small></span>
									</label>
								</div>
							)}
						</Section>

						<Section n={4} title="Chọn nhà sáng tạo">
							<div className="ti-search">
								<input className="ti-input" placeholder="Tìm nhà sáng tạo theo tên người dùng (username)" value={query} onChange={(e) => setQuery(e.target.value)} />
								{searching && <span className="ti-spin" aria-label="Đang tìm" />}
							</div>
							{searchNote && <small className="ti-hint warn">{searchNote}</small>}
							{found.length > 0 && (
								<div className="ti-found">
									{found.map((c) => {
										const added = creators.some((x) => x.openId === c.openId);
										return (
											<div key={c.openId} className="ti-creator">
												{c.avatarUrl ? <img src={c.avatarUrl} alt="" loading="lazy" /> : <span className="ti-avatar">{c.username.slice(0, 1)}</span>}
												<span><b>@{c.username}</b><small>{c.nickname} · {new Intl.NumberFormat('vi-VN').format(c.followers)} người theo dõi</small></span>
												<button type="button" className="cp-btn ghost" disabled={added} onClick={() => addCreator(c)}>{added ? 'Đã thêm' : 'Thêm'}</button>
											</div>
										);
									})}
								</div>
							)}
							<div className="ti-selected-head"><b>Đã chọn ({creators.length}/{cfg?.maxCreators ?? 10})</b><small>Mỗi lời mời gửi tối đa {cfg?.maxCreators ?? 10} nhà sáng tạo để tránh gửi nhầm.</small></div>
							{creators.length === 0 ? (
								<div className="ti-empty">Chưa chọn nhà sáng tạo nào.</div>
							) : (
								<div className="ti-found">
									{creators.map((c) => (
										<div key={c.openId} className="ti-creator on">
											{c.avatarUrl ? <img src={c.avatarUrl} alt="" loading="lazy" /> : <span className="ti-avatar">{c.username.slice(0, 1)}</span>}
											<span><b>@{c.username}</b><small>{c.nickname}</small></span>
											<button type="button" className="ti-x" aria-label="Bỏ" onClick={() => setCreators((prev) => prev.filter((x) => x.openId !== c.openId))}>×</button>
										</div>
									))}
								</div>
							)}
						</Section>
					</div>

					<aside className="ti-aside">
						<h4>Xem trước</h4>
						<div className="ti-phone">
							<div className="ti-phone-bar">Lời mời từ cửa hàng</div>
							<div className="ti-phone-tabs"><span className="on">Tổng quan</span><span>Sản phẩm ({products.length})</span></div>
							<div className="ti-phone-block"><b>Về cửa hàng này</b><span className="ti-shop">Cửa hàng của bạn</span></div>
							<div className="ti-phone-block"><b>Tin nhắn</b><p>{composed || 'Nội dung lời mời sẽ hiện ở đây.'}</p></div>
							<div className="ti-phone-block"><b>Loại nội dung ưu tiên</b><span>{contentPref === 'video' ? 'Video có link bán hàng' : contentPref === 'live' ? 'LIVE bán hàng' : 'Không ưu tiên'}</span></div>
							<div className="ti-phone-block">
								<b>Đặc quyền</b>
								<span className="ti-kv"><i>Mẫu miễn phí</i><em>{!hasFreeSample ? 'Không có' : sampleExempt ? 'Tự động phê duyệt' : 'Cần người bán phê duyệt'}</em></span>
								<span className="ti-kv"><i>Tỷ lệ hoa hồng</i><em>{first && toNumber(first.commission) ? `${toNumber(first.commission)}%` : '0,00 %'}</em></span>
								<span className="ti-kv"><i>Hoa hồng Quảng cáo cửa hàng</i><em>{first && toNumber(first.ads) ? `${toNumber(first.ads)}%` : '0,00 %'}</em></span>
							</div>
							<div className="ti-phone-foot"><span>Từ chối</span><span className="hot">Thêm tất cả</span></div>
						</div>
						{preview?.payload && (
							<details className="ti-json" open={ready}>
								<summary>Nội dung API sẽ gửi cho TikTok</summary>
								<pre>{JSON.stringify(preview.payload, null, 2)}</pre>
							</details>
						)}
					</aside>
				</div>

				<div className="ti-foot">
					<div className="ti-status">
						{error && <div className="ti-err-box">{error}</div>}
						{preview && preview.problems.length > 0 && (
							<ul className="ti-problems">{preview.problems.map((p) => <li key={p}>{p}</li>)}</ul>
						)}
						{stale && <small className="ti-hint warn">Bạn đã sửa lời mời sau lần kiểm tra. Hãy kiểm tra lại trước khi gửi.</small>}
						{ready && (
							<label className="ti-confirm">
								<input type="checkbox" checked={confirmed} onChange={(e) => setConfirmed(e.target.checked)} />
								<span>
									Tôi xác nhận gửi lời mời <b>thật</b> tới <b>{preview?.creators.length} nhà sáng tạo</b> ({preview?.creators.map((c) => `@${c.username}`).join(', ')}) với <b>{preview?.products.length} sản phẩm</b>. Việc này không thể thu hồi ngay, chỉ có thể hủy lời mời sau.
								</span>
							</label>
						)}
						{blocked && <div className="ti-err-box">Đã khóa gửi trong cửa sổ này để tránh gửi trùng. Hãy đóng cửa sổ và kiểm tra tab Cộng tác mục tiêu.</div>}
					</div>
					<div className="ti-buttons">
						<button type="button" className="cp-btn ghost" onClick={onClose} disabled={sending}>Hủy</button>
						<button type="button" className="cp-btn ghost" onClick={check} disabled={checking || sending}>{checking ? 'Đang kiểm tra…' : 'Kiểm tra lời mời'}</button>
						<button type="button" className="cp-btn" onClick={send} disabled={!canSend} title={!canSend ? 'Cần kiểm tra, tick xác nhận và có quyền gửi' : ''}>{sending ? 'Đang gửi…' : 'Gửi lời mời'}</button>
					</div>
				</div>
			</div>
		</div>
		</Portal>
	);
}
