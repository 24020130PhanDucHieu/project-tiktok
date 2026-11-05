import { useCallback, useEffect, useRef, useState } from 'react';
import Portal from '../../components/Portal';
import Select from '../../components/Select';
import api, { apiMessage } from '../../lib/axios';
import { formatCurrency, formatNumber } from '../../lib/formatters';
import type { CreatorVideo, KocVideosResult } from '../../types/koc';
import VideoCard, { VideoCover, postedLabel } from './VideoCard';
import VideoPlayerModal from './VideoPlayerModal';

interface Props {
	showToast: (m: string, t?: 'success' | 'error') => void;
}

const DAYS = [
	{ value: '30', label: '30 ngày gần nhất' },
	{ value: '90', label: '90 ngày gần nhất' },
	{ value: '180', label: '180 ngày gần nhất' },
	{ value: '365', label: '365 ngày gần nhất' },
];

const dash = (v: number | null) => (v == null ? '—' : formatNumber(v));

/** Management tab: every post of the KOCs saved in the app, read from TikTok Shop, with real views and sales. */
export default function KocContentPage({ showToast }: Props) {
	const [days, setDays] = useState('90');
	const [keyword, setKeyword] = useState('');
	const [q, setQ] = useState('');
	const [all, setAll] = useState(false);
	const [data, setData] = useState<KocVideosResult | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState('');
	const [detail, setDetail] = useState<CreatorVideo | null>(null);
	const [watching, setWatching] = useState<CreatorVideo | null>(null);
	const req = useRef(0);

	useEffect(() => {
		const t = window.setTimeout(() => setQ(keyword.trim()), 500);
		return () => window.clearTimeout(t);
	}, [keyword]);

	const load = useCallback(() => {
		const id = ++req.current;
		setLoading(true);
		setError('');
		api.get<{ data: KocVideosResult }>('/content/videos', { params: { days, q: q || undefined, all: all ? 1 : undefined, limit: 100 } })
			.then(({ data: r }) => id === req.current && setData(r.data))
			.catch((e) => {
				if (id !== req.current) return;
				setError(apiMessage(e, 'Không tải được bài đăng của KOC.'));
				showToast('Không tải được bài đăng của KOC.', 'error');
			})
			.finally(() => id === req.current && setLoading(false));
	}, [days, q, all, showToast]);

	useEffect(() => {
		load();
	}, [load]);

	const t = data?.totals;
	return (
		<div className="page-shell tc-page">
			<header className="page-head">
				<div>
					<h1>KOC đăng bài</h1>
					<p>Các video KOC đã đăng cho shop, lấy trực tiếp từ TikTok Shop: số đơn, doanh thu, hoa hồng và link xem video.</p>
				</div>
				<div className="tc-head-actions">
					<button className="tm-btn-cancel" onClick={load} disabled={loading}>{loading ? 'Đang tải...' : 'Làm mới'}</button>
				</div>
			</header>

			{data?.analytics.note && <div className="kv-note wide">{data.analytics.note}</div>}
			{data?.truncated && <div className="kv-note wide">Shop có rất nhiều đơn trong khoảng này nên danh sách có thể chưa đủ. Chọn khoảng ngắn hơn để chính xác hơn.</div>}

			<div className="kv-kpis">
				<div><span>Video</span><strong>{data ? formatNumber(data.total) : '—'}</strong></div>
				<div><span>Đơn</span><strong>{t ? formatNumber(t.orders) : '—'}</strong></div>
				<div><span>Doanh thu</span><strong>{t ? formatCurrency(t.gmv) : '—'}</strong></div>
				<div><span>Hoa hồng</span><strong>{t ? formatCurrency(t.commission) : '—'}</strong></div>
				<div><span>Lượt xem{data?.viewsPublic ? ' (công khai)' : ''}</span><strong>{data?.views != null ? formatNumber(data.views) : '—'}</strong></div>
			</div>

			<div className="tc-toolbar">
				<Select className="adm-filter" value={days} onChange={(e) => setDays(e.target.value)}>
					{DAYS.map((d) => <option key={d.value} value={d.value}>{d.label}</option>)}
				</Select>
				<input className="adm-input tc-search" placeholder="Tìm theo KOC, tiêu đề, mã video…" value={keyword} onChange={(e) => setKeyword(e.target.value)} />
				<label className="kv-check"><input type="checkbox" checked={all} onChange={(e) => setAll(e.target.checked)} /> Cả nhà sáng tạo chưa lưu trong app</label>
			</div>

			{error && <div className="tc-error">{error}</div>}

			<section className="panel-card flush">
				<div className="table-scroll">
					<table className="adm-table tc-table kv-table">
						<thead>
							<tr><th>Video</th><th>KOC</th><th>Ngày đăng</th><th className="r">Lượt xem</th><th className="r">Thích</th><th className="r">Đơn</th><th className="r">Doanh thu</th><th className="r">Hoa hồng</th><th /></tr>
						</thead>
						<tbody>
							{data?.videos.map((v) => (
								<tr key={v.id} className="kv-row" onClick={() => setDetail(v)}>
									<td>
										<div className="kv-cell">
											<VideoCover video={v} className="sm" />
											<span className="kv-cell-title" title={v.title}>{v.title || `Video ${v.id.slice(-6)}`}</span>
										</div>
									</td>
									<td className="nowrap"><div className="tc-name"><b>@{v.username}</b>{v.kocName && v.kocName !== v.username && <small>{v.kocName}</small>}{!v.inApp && <small>Chưa lưu trong app</small>}</div></td>
									<td className="nowrap">{postedLabel(v.postedAt)}</td>
									<td className="r">{dash(v.views)}</td>
									<td className="r">{dash(v.likes)}</td>
									<td className="r">{formatNumber(v.orders)}</td>
									<td className="r nowrap">{formatCurrency(v.gmv)}</td>
									<td className="r nowrap">{formatCurrency(v.commission)}</td>
									<td className="tc-actions-cell" onClick={(e) => e.stopPropagation()}>
										<button type="button" className="cp-btn ghost" onClick={() => setWatching(v)}>Xem</button>
									</td>
								</tr>
							))}
							{!loading && data && data.videos.length === 0 && !error && (
								<tr><td colSpan={9} className="dash-empty">Chưa thấy video nào có đơn trong khoảng này. Thử khoảng ngày dài hơn{all ? '' : ' hoặc bật “Cả nhà sáng tạo chưa lưu trong app”'}.</td></tr>
							)}
							{loading && !data && <tr><td colSpan={9} className="dash-empty">Đang đọc từ TikTok…</td></tr>}
						</tbody>
					</table>
				</div>
				<div className="tc-foot">
					<small>{data ? `${data.videos.length}${data.total > data.videos.length ? ` / ${data.total}` : ''} video · ${data.range.from} đến ${data.range.to}` : ''}</small>
					<small>{data ? `Cập nhật ${new Date(data.asOf).toLocaleTimeString('vi-VN')}` : ''}</small>
				</div>
			</section>

			{watching && <VideoPlayerModal video={watching} onClose={() => setWatching(null)} />}

			{detail && (
				<Portal>
					<div className="tc-drawer-backdrop" onClick={() => setDetail(null)}>
						<aside className="tc-drawer" onClick={(e) => e.stopPropagation()}>
							<button type="button" className="cp-oc-x tc-drawer-x" aria-label="Đóng" onClick={() => setDetail(null)}>×</button>
							<h3>Chi tiết nội dung</h3>
							<p className="tc-sub">@{detail.username}{detail.kocName && detail.kocName !== detail.username ? ` · ${detail.kocName}` : ''}</p>
							<VideoCard video={detail} showKoc />
							<h5>Số liệu bán hàng</h5>
							<div className="kv-detail-grid">
								<div><span>Đơn</span><strong>{formatNumber(detail.orders)}</strong></div>
								<div><span>Sản phẩm bán ra</span><strong>{formatNumber(detail.items)}</strong></div>
								<div><span>Doanh thu</span><strong>{formatCurrency(detail.gmv)}</strong></div>
								<div><span>Hoa hồng</span><strong>{formatCurrency(detail.commission)}</strong></div>
								<div><span>Đã đối soát</span><strong>{formatCurrency(detail.commissionConfirmed)}</strong></div>
								<div><span>Chia sẻ</span><strong>{dash(detail.shares)}</strong></div>
							</div>
							<small className="tc-sub">Doanh thu đã trừ đơn hoàn trả. Hoa hồng gồm hoa hồng cho KOC và hoa hồng quảng cáo cửa hàng; phần chưa đối soát là số ước tính của TikTok.</small>
							{detail.productIds.length > 0 && <small className="tc-sub">Mã sản phẩm: {detail.productIds.join(', ')}</small>}
						</aside>
					</div>
				</Portal>
			)}
		</div>
	);
}
