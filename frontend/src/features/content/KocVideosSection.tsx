import { useCallback, useEffect, useRef, useState } from 'react';
import Select from '../../components/Select';
import api, { apiMessage } from '../../lib/axios';
import { formatCurrency, formatNumber } from '../../lib/formatters';
import type { KocVideosResult } from '../../types/koc';
import VideoCard from './VideoCard';

interface Props {
	kocId: string;
	username: string;
	/** TikTok product of the collaboration, when known: enables "only this product" */
	productId?: string;
}

const DAYS = [
	{ value: '30', label: '30 ngày gần nhất' },
	{ value: '90', label: '90 ngày gần nhất' },
	{ value: '180', label: '180 ngày gần nhất' },
];

/** The real posts of the KOC of the open chat (read from TikTok Shop), with their sales and a link to each video. */
export default function KocVideosSection({ kocId, username, productId }: Props) {
	const [days, setDays] = useState('90');
	const [onlyProduct, setOnlyProduct] = useState(false);
	const [data, setData] = useState<KocVideosResult | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState('');
	const req = useRef(0);

	const load = useCallback(() => {
		const id = ++req.current;
		setLoading(true);
		setError('');
		api.get<{ data: KocVideosResult }>(`/kocs/${kocId}/videos`, { params: { days, limit: 30, productId: onlyProduct && productId ? productId : undefined } })
			.then(({ data: r }) => id === req.current && setData(r.data))
			.catch((e) => {
				if (id !== req.current) return;
				setData(null);
				setError(apiMessage(e, 'Không tải được bài đăng của KOC.'));
			})
			.finally(() => id === req.current && setLoading(false));
	}, [kocId, days, onlyProduct, productId]);

	useEffect(() => {
		setData(null);
		load();
	}, [load]);

	return (
		<div className="kv-section">
			<div className="kv-bar">
				<small>Bài đăng thật của <b>@{username}</b> trong shop, lấy từ TikTok.</small>
				<div className="kv-bar-actions">
					<Select className="kv-days" value={days} onChange={(e) => setDays(e.target.value)}>
						{DAYS.map((d) => <option key={d.value} value={d.value}>{d.label}</option>)}
					</Select>
					<button type="button" className="cp-link" disabled={loading} onClick={load}>{loading ? 'Đang tải…' : 'Làm mới'}</button>
				</div>
			</div>
			{productId && (
				<label className="kv-check"><input type="checkbox" checked={onlyProduct} onChange={(e) => setOnlyProduct(e.target.checked)} /> Chỉ sản phẩm đang hợp tác</label>
			)}
			{error && <div className="kv-error">{error}</div>}
			{data?.analytics.note && <div className="kv-note">{data.analytics.note}</div>}
			{data?.truncated && <div className="kv-note">Shop có rất nhiều đơn trong khoảng này nên danh sách có thể chưa đủ. Chọn khoảng ngắn hơn để chính xác hơn.</div>}
			{data && data.total > 0 && (
				<div className="kv-totals">
					<span><b>{formatNumber(data.total)}</b> video</span>
					<span><b>{formatNumber(data.totals.orders)}</b> đơn</span>
					<span>Doanh thu <b>{formatCurrency(data.totals.gmv)}</b></span>
					<span>Hoa hồng <b>{formatCurrency(data.totals.commission)}</b></span>
					{data.views != null && <span><b>{formatNumber(data.views)}</b> lượt xem</span>}
				</div>
			)}
			{data && data.totals.excluded > 0 && <small className="kv-muted">{data.totals.excluded} đơn bị loại khỏi số trên vì đã hoàn trả hoặc không đủ điều kiện tính hoa hồng.</small>}
			{loading && !data && <small className="kv-muted">Đang đọc bài đăng từ TikTok…</small>}
			{data && data.total === 0 && !loading && (
				<small className="kv-muted">
					Chưa thấy video nào của @{username} có đơn trong {data.range.days} ngày ({data.range.from} đến {data.range.to}).
					{data.analytics.available ? '' : ' Video chưa có đơn chỉ hiện khi bật quyền Analytics.'}
				</small>
			)}
			{data?.videos.map((v) => <VideoCard key={v.id} video={v} />)}
			{data && data.total > data.videos.length && <small className="kv-muted">Hiện {data.videos.length} / {data.total} video mới nhất. Xem đủ ở tab “KOC đăng bài”.</small>}
		</div>
	);
}
