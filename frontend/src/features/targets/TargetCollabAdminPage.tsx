import { useCallback, useEffect, useState } from 'react';
import Portal from '../../components/Portal';
import Select from '../../components/Select';
import api, { apiMessage } from '../../lib/axios';
import type { TargetConfig, TargetDetail, TargetSummary } from '../../types/koc';
import TargetInviteModal from './TargetInviteModal';
import { lifeState, unixDate, vnd } from './targetUtils';

interface Props {
	isAdmin: boolean;
	showToast: (m: string, t?: 'success' | 'error') => void;
}

const STATUS: { value: string; label: string }[] = [
	{ value: 'VALID', label: 'Đang hiệu lực' },
	{ value: 'ONGOING', label: 'Còn trên 7 ngày' },
	{ value: 'EXPIRING', label: 'Sắp hết hạn' },
	{ value: 'CANCELING', label: 'Đang hủy' },
	{ value: 'COMPLETED', label: 'Đã kết thúc hoặc đã hủy' },
];

const KEYWORD_TYPES: { value: string; label: string }[] = [
	{ value: 'TARGET_COLLABORATION_NAME', label: 'Tên lời mời' },
	{ value: 'TARGET_COLLABORATION_ID', label: 'Mã lời mời' },
	{ value: 'PRODUCT_NAME', label: 'Tên sản phẩm' },
	{ value: 'PRODUCT_ID', label: 'Mã sản phẩm' },
];

const CREATOR_STATUS: Record<string, string> = { NORMAL: 'Bình thường', DELETING: 'Đang gỡ', DELETED: 'Đã gỡ' };

interface ListPayload {
	items: TargetSummary[];
	nextPageToken: string;
	total: number;
}

export default function TargetCollabAdminPage({ isAdmin, showToast }: Props) {
	const [status, setStatus] = useState('VALID');
	const [accept, setAccept] = useState('ALL');
	const [sample, setSample] = useState('ALL');
	const [keywordType, setKeywordType] = useState('TARGET_COLLABORATION_NAME');
	const [keyword, setKeyword] = useState('');
	const [items, setItems] = useState<TargetSummary[]>([]);
	const [total, setTotal] = useState(0);
	const [next, setNext] = useState('');
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState('');
	const [creating, setCreating] = useState(false);
	const [detail, setDetail] = useState<{ d: TargetDetail; sentBy?: string } | null>(null);
	const [detailLoading, setDetailLoading] = useState(false);
	const [menu, setMenu] = useState<string | null>(null);
	const [cancelling, setCancelling] = useState<TargetSummary | null>(null);
	const [cancelText, setCancelText] = useState('');
	const [busy, setBusy] = useState('');
	const [cfg, setCfg] = useState<TargetConfig | null>(null);
	const [askOn, setAskOn] = useState(false); // confirmation window before turning sending on
	const [understood, setUnderstood] = useState(false);
	const [switching, setSwitching] = useState(false);

	const load = useCallback(
		async (token = '') => {
			setLoading(true);
			setError('');
			try {
				const { data } = await api.get<{ data: ListPayload }>('/target-collabs', {
					params: { status, acceptStatus: accept, freeSample: sample, keyword: keyword.trim() || undefined, keywordType: keyword.trim() ? keywordType : undefined, pageToken: token || undefined },
				});
				setItems((prev) => (token ? [...prev, ...data.data.items] : data.data.items));
				setNext(data.data.nextPageToken || '');
				setTotal(data.data.total);
			} catch (e) {
				if (!token) setItems([]);
				setError(apiMessage(e, 'Không tải được danh sách lời mời.'));
			} finally {
				setLoading(false);
			}
		},
		[status, accept, sample, keyword, keywordType],
	);

	useEffect(() => {
		const t = window.setTimeout(() => load(), keyword ? 450 : 0);
		return () => window.clearTimeout(t);
	}, [load, keyword]);

	useEffect(() => {
		api.get<{ data: TargetConfig }>('/target-collabs/config').then(({ data }) => setCfg(data.data)).catch(() => undefined);
	}, []);

	async function setSending(enabled: boolean) {
		setSwitching(true);
		try {
			await api.put('/target-collabs/switch', { enabled, confirm: enabled });
			setCfg((c) => (c ? { ...c, enabled } : c));
			setAskOn(false);
			setUnderstood(false);
			showToast(enabled ? 'Đã BẬT gửi lời mời thật. Hãy cẩn thận với từng lần gửi.' : 'Đã tắt gửi lời mời thật.');
		} catch (e) {
			showToast(apiMessage(e, 'Không đổi được công tắc.'), 'error');
		} finally {
			setSwitching(false);
		}
	}

	useEffect(() => {
		if (!menu) return undefined;
		const close = () => setMenu(null);
		window.addEventListener('click', close);
		return () => window.removeEventListener('click', close);
	}, [menu]);

	async function openDetail(id: string) {
		setDetailLoading(true);
		setDetail(null);
		try {
			const { data } = await api.get<{ data: { detail: TargetDetail; local: { sentByEmail?: string } | null } }>(`/target-collabs/${id}`);
			setDetail({ d: data.data.detail, sentBy: data.data.local?.sentByEmail });
		} catch (e) {
			showToast(apiMessage(e, 'Không tải được chi tiết lời mời.'), 'error');
		} finally {
			setDetailLoading(false);
		}
	}

	async function copyLink(it: TargetSummary) {
		setBusy(it.id);
		try {
			const { data } = await api.post<{ data: { link: string } }>(`/target-collabs/${it.id}/link`, {});
			await navigator.clipboard.writeText(data.data.link);
			showToast('Đã sao chép link lời mời. Gửi link này cho nhà sáng tạo để họ xem và chấp nhận.');
		} catch (e) {
			showToast(apiMessage(e, 'Không tạo được link lời mời.'), 'error');
		} finally {
			setBusy('');
		}
	}

	async function confirmCancel() {
		if (!cancelling) return;
		setBusy(cancelling.id);
		try {
			await api.post(`/target-collabs/${cancelling.id}/cancel`, { confirm: true });
			showToast('Đã hủy lời mời.');
			setCancelling(null);
			setCancelText('');
			load();
		} catch (e) {
			showToast(apiMessage(e, 'Không hủy được lời mời.'), 'error');
		} finally {
			setBusy('');
		}
	}

	return (
		<div className="page-shell tc-page">
			<header className="page-head">
				<div>
					<h1>Cộng tác mục tiêu</h1>
					<p>Quản lý các lời mời cộng tác riêng đã gửi cho nhà sáng tạo trên TikTok Shop. Dữ liệu lấy trực tiếp từ TikTok.</p>
				</div>
				<div className="tc-head-actions">
					<button className="tm-btn-cancel" onClick={() => load()} disabled={loading}>{loading ? 'Đang tải...' : 'Làm mới'}</button>
					<button className="cp-btn" onClick={() => setCreating(true)}>Mời cộng tác</button>
				</div>
			</header>

			<div className={`tc-switch-card${cfg?.enabled ? ' on' : ''}`}>
				<div>
					<b>Cho phép gửi lời mời thật</b>
					<small>
						{cfg == null
							? 'Đang tải trạng thái…'
							: cfg.enabled
								? 'ĐANG BẬT: quản trị viên có thể gửi lời mời tới nhà sáng tạo thật trên TikTok. Tắt lại bất cứ lúc nào.'
								: 'Đang tắt (mặc định): chỉ xem, tìm và kiểm tra lời mời, chưa gửi được. Gạt sang bật khi bạn sẵn sàng gửi.'}
					</small>
				</div>
				<label className={`ti-switch${!isAdmin || switching || cfg == null ? ' disabled' : ''}`} title={isAdmin ? '' : 'Chỉ quản trị viên'}>
					<input type="checkbox" checked={!!cfg?.enabled} disabled={!isAdmin || switching || cfg == null} onChange={(e) => (e.target.checked ? setAskOn(true) : setSending(false))} />
					<span className="ti-track"><i /></span>
					<b>{cfg?.enabled ? 'Bật' : 'Tắt'}</b>
				</label>
			</div>

			<div className="tc-toolbar">
				<Select className="adm-filter" value={status} onChange={(e) => setStatus(e.target.value)}>
					{STATUS.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
				</Select>
				<Select className="adm-filter" value={accept} onChange={(e) => setAccept(e.target.value)}>
					<option value="ALL">Mọi nhà sáng tạo</option>
					<option value="ACCEPT">Đã thêm vào showcase</option>
				</Select>
				<Select className="adm-filter" value={sample} onChange={(e) => setSample(e.target.value)}>
					<option value="ALL">Mọi cài đặt hàng mẫu</option>
					<option value="WITH_FREE_SAMPLE">Có hàng mẫu miễn phí</option>
				</Select>
				<Select className="adm-filter" value={keywordType} onChange={(e) => setKeywordType(e.target.value)}>
					{KEYWORD_TYPES.map((k) => <option key={k.value} value={k.value}>{k.label}</option>)}
				</Select>
				<input className="adm-input tc-search" placeholder="Tìm kiếm…" value={keyword} onChange={(e) => setKeyword(e.target.value)} />
			</div>

			{error && <div className="tc-error">{error}</div>}

			<section className="panel-card flush">
				<div className="table-scroll">
					<table className="adm-table tc-table">
						<thead>
							<tr><th>Lời mời</th><th>Trạng thái</th><th className="r">Sản phẩm</th><th className="r">Đã mời</th><th className="r">Đã chấp nhận</th><th className="r">Đã quảng bá</th><th>Hết hạn</th><th /></tr>
						</thead>
						<tbody>
							{items.map((it) => {
								const life = lifeState(it.endTime, it.localStatus);
								return (
									<tr key={it.id}>
										<td>
											<div className="tc-name">
												<b>{it.name}</b>
												<small>
													Cập nhật {unixDate(it.updateTime)}
													{it.type === 'TOP_CREATOR_PROGRAM' ? ' · Chương trình Top Creator' : ''}
													{it.sentBy ? ` · Gửi bởi ${it.sentBy}` : ' · Tạo ngoài ứng dụng'}
												</small>
												{it.creatorNames && it.creatorNames.length > 0 && <small className="tc-people">{it.creatorNames.join(', ')}</small>}
											</div>
										</td>
										<td><span className={`tc-state ${life.tone}`}><i />{life.label}</span></td>
										<td className="r">{it.productCount}</td>
										<td className="r">{it.invitedCount}</td>
										<td className="r">{it.showcaseCreatorCount}</td>
										<td className="r">{it.contentCreatorCount}</td>
										<td className="nowrap">{unixDate(it.endTime)}</td>
										<td className="tc-actions-cell">
											<div className="tc-actions">
											<button type="button" className="cp-btn ghost" onClick={() => openDetail(it.id)}>Xem</button>
											<div className="tc-menu-wrap">
												<button type="button" className="tc-dots" aria-label="Thao tác khác" onClick={(e) => { e.stopPropagation(); setMenu(menu === it.id ? null : it.id); }}>···</button>
												{menu === it.id && (
													<div className="tc-menu" onClick={(e) => e.stopPropagation()}>
														<button type="button" disabled={!isAdmin || busy === it.id} onClick={() => { setMenu(null); copyLink(it); }}>Sao chép link lời mời</button>
														<button type="button" className="danger" disabled={!isAdmin || life.tone === 'off' || it.type !== 'STANDARD'} title={it.type !== 'STANDARD' ? 'Lời mời của chương trình Top Creator không hủy ở đây' : ''} onClick={() => { setMenu(null); setCancelling(it); setCancelText(''); }}>Hủy lời mời</button>
														{!isAdmin && <small>Chỉ quản trị viên</small>}
													</div>
												)}
											</div>
											</div>
										</td>
									</tr>
								);
							})}
							{!loading && items.length === 0 && !error && <tr><td colSpan={8} className="dash-empty">Không có lời mời nào khớp bộ lọc.</td></tr>}
						</tbody>
					</table>
				</div>
				<div className="tc-foot">
					<small>{items.length}{total > items.length ? ` / ${total}` : ''} lời mời</small>
					{next && <button type="button" className="cp-btn ghost" disabled={loading} onClick={() => load(next)}>Tải thêm</button>}
				</div>
			</section>

			{(detail || detailLoading) && (
				<Portal>
				<div className="tc-drawer-backdrop" onClick={() => { setDetail(null); setDetailLoading(false); }}>
					<aside className="tc-drawer" onClick={(e) => e.stopPropagation()}>
						<button type="button" className="cp-oc-x tc-drawer-x" aria-label="Đóng" onClick={() => { setDetail(null); setDetailLoading(false); }}>×</button>
						{detailLoading || !detail ? (
							<div className="ti-empty">Đang tải…</div>
						) : (
							<>
								<h3>{detail.d.name}</h3>
								<p className="tc-sub">Hết hạn {unixDate(detail.d.endTime)} · {detail.d.sampleApprovalExempt ? 'Tự động duyệt hàng mẫu' : detail.d.hasFreeSample ? 'Có hàng mẫu, cần bạn duyệt' : 'Không có hàng mẫu'}{detail.sentBy ? ` · Gửi bởi ${detail.sentBy}` : ''}</p>
								<h5>Lời nhắn</h5>
								<div className="tc-msg">{detail.d.message || '—'}</div>
								{detail.d.email && <p className="tc-sub">Liên hệ: {detail.d.email}</p>}
								<h5>Nhà sáng tạo ({detail.d.creators.length})</h5>
								<div className="tc-list">
									{detail.d.creators.map((c) => (
										<div key={c.openId} className="tc-item">
											{c.avatarUrl ? <img src={c.avatarUrl} alt="" loading="lazy" /> : <span className="ti-avatar">{c.username.slice(0, 1)}</span>}
											<span><b>@{c.username}</b><small>{c.nickname} · {CREATOR_STATUS[c.status] || c.status}</small></span>
											<em>{c.showcaseProductCount} showcase · {c.contentProductCount} nội dung</em>
										</div>
									))}
								</div>
								<h5>Sản phẩm ({detail.d.products.length})</h5>
								<div className="tc-list">
									{detail.d.products.map((p) => (
										<div key={p.id} className="tc-item">
											{p.imageUrl ? <img src={p.imageUrl} alt="" loading="lazy" /> : <span className="ti-avatar sq" />}
											<span><b title={p.title}>{p.title}</b><small>{p.minPrice ? `${vnd(p.minPrice)}${p.maxPrice !== p.minPrice ? ` – ${vnd(p.maxPrice)}` : ''}` : ''}</small></span>
											<em>Hoa hồng {p.commissionPct}%{p.adsCommissionPct ? ` · QC ${p.adsCommissionPct}%` : ''}</em>
										</div>
									))}
								</div>
							</>
						)}
					</aside>
				</div>
				</Portal>
			)}

			{askOn && (
				<Portal>
					<div className="cp-backdrop" onClick={() => !switching && setAskOn(false)}>
						<div className="cp-modal" role="alertdialog" aria-modal="true" aria-labelledby="tc-on-title" onClick={(e) => e.stopPropagation()}>
							<h4 id="tc-on-title">Bật gửi lời mời thật?</h4>
							<p className="cp-muted cp-dialog-text">Khi bật, nút <b>Gửi lời mời</b> sẽ tạo lời mời thật trên TikTok Shop và nhà sáng tạo được chọn sẽ nhận được nó. Mỗi lần gửi vẫn cần kiểm tra và xác nhận riêng, và chỉ quản trị viên gửi được.</p>
							<label className="ti-confirm"><input type="checkbox" checked={understood} onChange={(e) => setUnderstood(e.target.checked)} /><span>Tôi hiểu lời mời được gửi tới nhà sáng tạo thật và ảnh hưởng tới shop.</span></label>
							<div className="cp-row end">
								<button type="button" className="cp-btn ghost" onClick={() => { setAskOn(false); setUnderstood(false); }} disabled={switching}>Giữ tắt</button>
								<button type="button" className="cp-btn" disabled={!understood || switching} onClick={() => setSending(true)}>{switching ? 'Đang bật…' : 'Bật gửi lời mời'}</button>
							</div>
						</div>
					</div>
				</Portal>
			)}

			{cancelling && (
				<Portal>
				<div className="cp-backdrop" onClick={() => setCancelling(null)}>
					<div className="cp-modal" role="alertdialog" aria-modal="true" aria-labelledby="tc-cancel-title" onClick={(e) => e.stopPropagation()}>
						<h4 id="tc-cancel-title">Hủy lời mời này?</h4>
						<p className="cp-muted cp-dialog-text">Nhà sáng tạo sẽ không còn nhận được lời mời <b>{cancelling.name}</b>. Gõ đúng tên lời mời để xác nhận.</p>
						<input autoFocus placeholder={cancelling.name} value={cancelText} onChange={(e) => setCancelText(e.target.value)} />
						<div className="cp-row end">
							<button type="button" className="cp-btn ghost" onClick={() => setCancelling(null)}>Giữ lại</button>
							<button type="button" className="cp-btn tc-danger" disabled={cancelText.trim().toLowerCase() !== cancelling.name.trim().toLowerCase() || busy === cancelling.id} onClick={confirmCancel}>{busy === cancelling.id ? 'Đang hủy…' : 'Hủy lời mời'}</button>
						</div>
					</div>
				</div>
				</Portal>
			)}

			{creating && <TargetInviteModal isAdmin={isAdmin} showToast={showToast} onClose={() => setCreating(false)} onSent={() => load()} />}
		</div>
	);
}
