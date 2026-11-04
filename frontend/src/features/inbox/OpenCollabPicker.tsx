import { useEffect, useState } from 'react';
import api from '../../lib/axios';
import type { OpenCollabProduct } from '../../types/koc';

interface Props {
	disabled?: boolean;
	onConfirm: (product: OpenCollabProduct) => void;
}

const price = (p: OpenCollabProduct) => {
	if (!p.minPrice) return '';
	const fmt = (v: string) => Number(v).toLocaleString('vi-VN');
	const range = p.maxPrice && p.maxPrice !== p.minPrice ? `${fmt(p.minPrice)} – ${fmt(p.maxPrice)}` : fmt(p.minPrice);
	return `${range} ${p.currency === 'VND' ? 'đ' : p.currency}`;
};

/** Lets the user pick one product the shop opened for open collaboration; the commission shown is the shop's public rate. */
export default function OpenCollabPicker({ disabled, onConfirm }: Props) {
	const [keyword, setKeyword] = useState('');
	const [items, setItems] = useState<OpenCollabProduct[]>([]);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState('');
	const [selected, setSelected] = useState<OpenCollabProduct | null>(null);

	useEffect(() => {
		let cancelled = false;
		setLoading(true);
		setError('');
		const timer = setTimeout(() => {
			api.get('/shop/open-collaborations', { params: { keyword: keyword.trim() || undefined, pageSize: 20 } })
				.then(({ data }) => {
					if (!cancelled) setItems(data.data.items || []);
				})
				.catch((e) => {
					if (!cancelled) {
						setItems([]);
						setError(e?.response?.data?.message || 'Không tải được sản phẩm cộng tác mở của shop.');
					}
				})
				.finally(() => {
					if (!cancelled) setLoading(false);
				});
		}, keyword ? 350 : 0);
		return () => {
			cancelled = true;
			clearTimeout(timer);
		};
	}, [keyword]);

	return (
		<div className="cp-form">
			<input placeholder="Tìm sản phẩm theo tên hoặc mã" value={keyword} onChange={(e) => setKeyword(e.target.value)} />
			<div className="cp-oc-list">
				{loading && <small className="cp-muted">Đang tải sản phẩm của shop…</small>}
				{!loading && error && <small className="cp-oc-error">{error}</small>}
				{!loading && !error && items.length === 0 && (
					<small className="cp-muted">{keyword ? 'Không có sản phẩm khớp.' : 'Shop chưa mở cộng tác cho sản phẩm nào.'}</small>
				)}
				{!loading &&
					items.map((p) => {
						const ending = p.status !== 'NORMAL';
						return (
							<button
								type="button"
								key={p.collaborationId || p.productId}
								className={`cp-oc-item${selected?.productId === p.productId ? ' selected' : ''}`}
								disabled={ending}
								onClick={() => setSelected(p)}
							>
								{p.imageUrl ? <img src={p.imageUrl} alt="" loading="lazy" /> : <span className="cp-oc-noimg" />}
								<span className="cp-oc-info">
									<strong>{p.title || p.productId}</strong>
									<small>{[price(p), `Còn ${p.inventory}`].filter(Boolean).join(' · ')}</small>
								</span>
								<span className="cp-oc-rate">{ending ? 'Sắp hết hạn' : `${p.commissionRate}%`}</span>
							</button>
						);
					})}
			</div>
			{selected && (
				<small className="cp-muted">
					Hoa hồng công khai của shop: <b>{selected.commissionRate}%</b> (chỉ đọc, lấy từ TikTok Shop).
				</small>
			)}
			<div className="cp-row">
				<button type="button" className="cp-btn" disabled={disabled || !selected} onClick={() => selected && onConfirm(selected)}>
					Xác nhận
				</button>
			</div>
		</div>
	);
}
