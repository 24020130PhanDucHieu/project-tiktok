import { type KeyboardEvent, useCallback, useEffect, useRef, useState } from 'react';
import api from '../../lib/axios';
import { formatCurrency } from '../../lib/formatters';
import type { OpenCollabPage, OpenCollabProduct } from '../../types/koc';

// 6 rows are taller than the ~5-row viewport, so the list overflows and the next page is only requested on scroll.
const PAGE_SIZE = 6;
const SKELETON_ROWS = 5;
const AUTO_KEY = 'oc.autoRefreshSec';
const AUTO_OPTIONS = [
	{ sec: 0, label: 'Tắt tự động' },
	{ sec: 30, label: 'Mỗi 30 giây' },
	{ sec: 60, label: 'Mỗi 1 phút' },
	{ sec: 300, label: 'Mỗi 5 phút' },
];

function readAuto() {
	const n = Number(localStorage.getItem(AUTO_KEY));
	return AUTO_OPTIONS.some((o) => o.sec === n) ? n : 0;
}

interface Props {
	saving?: boolean;
	onClose: () => void;
	onConfirm: (product: OpenCollabProduct) => void;
}

const selectable = (p: OpenCollabProduct) => p.status === 'NORMAL' && p.available;

const money = (n: number) => `${new Intl.NumberFormat('vi-VN').format(Math.round(n))}đ`;
const percent = (n: number) => `${n.toFixed(2).replace('.', ',')}%`;

function priceText(p: OpenCollabProduct) {
	if (!p.minPrice) return '';
	const min = formatCurrency(p.minPrice);
	return p.maxPrice && p.maxPrice !== p.minPrice ? `${min} – ${formatCurrency(p.maxPrice)}` : min;
}

/** Commission the seller pays per sale, from the original price (TikTok's own estimate uses the sale price, which this API does not return). */
function estimateText(p: OpenCollabProduct) {
	const min = Number(p.minPrice) * (p.commissionRate / 100);
	const max = Number(p.maxPrice || p.minPrice) * (p.commissionRate / 100);
	if (!min) return '';
	return max && Math.round(max) !== Math.round(min) ? `${money(min)} – ${money(max)}` : money(min);
}

/** Centered window to pick one product the shop opened for open collaboration. Pages load as the list is scrolled. */
export default function OpenCollabModal({ saving, onClose, onConfirm }: Props) {
	const [keyword, setKeyword] = useState('');
	const [items, setItems] = useState<OpenCollabProduct[]>([]);
	const [total, setTotal] = useState(0);
	const [next, setNext] = useState('');
	const [loading, setLoading] = useState(true);
	const [loadingMore, setLoadingMore] = useState(false);
	const [error, setError] = useState('');
	const [selected, setSelected] = useState<OpenCollabProduct | null>(null);
	const [refreshing, setRefreshing] = useState(false);
	const [autoSec, setAutoSec] = useState(readAuto);
	const [updatedAt, setUpdatedAt] = useState<Date | null>(null);

	const listRef = useRef<HTMLDivElement>(null);
	const sentinelRef = useRef<HTMLDivElement>(null);
	const searchRef = useRef<HTMLInputElement>(null);
	const reqRef = useRef(0); // bumped on every new search; replies from an older search are dropped
	const moreRef = useRef(false);
	const kwRef = useRef('');
	const refreshRef = useRef(false);

	const fetchPage = useCallback(async (kw: string, token: string, id: number) => {
		const { data } = await api.get<{ data: OpenCollabPage }>('/shop/open-collaborations', {
			params: { keyword: kw || undefined, pageToken: token || undefined, pageSize: PAGE_SIZE },
		});
		return id === reqRef.current ? data.data : null;
	}, []);

	const search = useCallback(
		async (kw: string) => {
			const id = ++reqRef.current;
			kwRef.current = kw;
			moreRef.current = false;
			setLoading(true);
			setLoadingMore(false);
			setError('');
			setItems([]);
			setNext('');
			setTotal(0);
			listRef.current?.scrollTo({ top: 0 });
			try {
				const page = await fetchPage(kw, '', id);
				if (!page) return;
				setItems(page.items || []);
				setNext(page.nextPageToken || '');
				setTotal(page.total || 0);
			} catch (e: any) {
				if (id === reqRef.current) setError(e?.response?.data?.message || 'Không tải được sản phẩm của shop.');
			} finally {
				if (id === reqRef.current) setLoading(false);
			}
		},
		[fetchPage],
	);

	const loadMore = useCallback(async () => {
		if (!next || moreRef.current || loading) return;
		moreRef.current = true;
		setLoadingMore(true);
		const id = reqRef.current;
		try {
			const page = await fetchPage(kwRef.current, next, id);
			if (!page) return;
			setItems((prev) => {
				const seen = new Set(prev.map((p) => p.productId));
				return [...prev, ...(page.items || []).filter((p) => !seen.has(p.productId))];
			});
			setNext(page.nextPageToken || '');
		} catch (e: any) {
			if (id === reqRef.current) setError(e?.response?.data?.message || 'Không tải thêm được sản phẩm.');
		} finally {
			if (id === reqRef.current) setLoadingMore(false);
			moreRef.current = false;
		}
	}, [fetchPage, next, loading]);

	/** Re-reads the first page from TikTok and merges it in place: loaded rows keep their position and scroll. */
	const refresh = useCallback(async () => {
		if (loading || moreRef.current || refreshRef.current) return;
		refreshRef.current = true;
		setRefreshing(true);
		const id = reqRef.current;
		try {
			const page = await fetchPage(kwRef.current, '', id);
			if (!page) return;
			const fresh = new Map((page.items || []).map((p) => [p.productId, p]));
			setItems((prev) => {
				const known = new Set(prev.map((p) => p.productId));
				const added = (page.items || []).filter((p) => !known.has(p.productId));
				return [...added, ...prev.map((p) => fresh.get(p.productId) || p)];
			});
			setSelected((cur) => (cur ? fresh.get(cur.productId) || cur : cur));
			setTotal(page.total || 0);
			setError('');
			setUpdatedAt(new Date());
		} catch (e: any) {
			if (id === reqRef.current) setError(e?.response?.data?.message || 'Không cập nhật được dữ liệu.');
		} finally {
			refreshRef.current = false;
			setRefreshing(false);
		}
	}, [fetchPage, loading]);

	// keep the interval pointed at the latest refresh without restarting the timer
	const refreshFn = useRef(refresh);
	refreshFn.current = refresh;
	useEffect(() => {
		if (!autoSec) return;
		const t = setInterval(() => {
			if (!document.hidden) refreshFn.current();
		}, autoSec * 1000);
		return () => clearInterval(t);
	}, [autoSec]);

	function changeAuto(sec: number) {
		setAutoSec(sec);
		localStorage.setItem(AUTO_KEY, String(sec));
	}

	// first load is immediate; typing is debounced
	useEffect(() => {
		const timer = setTimeout(() => search(keyword.trim()), keyword ? 350 : 0);
		return () => clearTimeout(timer);
	}, [keyword, search]);

	// next page when the sentinel at the bottom of the list scrolls into view
	useEffect(() => {
		const el = sentinelRef.current;
		if (!el || !next || loading || error) return;
		const io = new IntersectionObserver((e) => e[0].isIntersecting && loadMore(), { root: listRef.current, rootMargin: '40px' });
		io.observe(el);
		return () => io.disconnect();
	}, [next, loading, error, items.length, loadMore]);

	useEffect(() => {
		searchRef.current?.focus();
		const onKey = (e: globalThis.KeyboardEvent) => e.key === 'Escape' && onClose();
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	}, [onClose]);

	function move(dir: 1 | -1) {
		const options = items.filter(selectable);
		if (options.length === 0) return;
		const i = selected ? options.findIndex((p) => p.productId === selected.productId) : -1;
		const target = options[Math.min(options.length - 1, Math.max(0, i + dir))];
		setSelected(target);
		listRef.current?.querySelector(`[data-pid="${target.productId}"]`)?.scrollIntoView({ block: 'nearest' });
	}

	function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
		if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			move(e.key === 'ArrowDown' ? 1 : -1);
		} else if (e.key === 'Enter' && selected && !saving) {
			e.preventDefault();
			onConfirm(selected);
		}
	}

	const empty = !loading && !error && items.length === 0;

	return (
		<div className="cp-backdrop" onClick={onClose}>
			<div className="cp-modal wide cp-oc" role="dialog" aria-modal="true" aria-label="Chọn sản phẩm cộng tác mở" onClick={(e) => e.stopPropagation()} onKeyDown={onKeyDown}>
				<div className="cp-oc-head">
					<div>
						<h4>Chọn sản phẩm cộng tác mở</h4>
						<small>Sản phẩm shop đã mở · hoa hồng công khai</small>
					</div>
					<button type="button" className="cp-oc-x" aria-label="Đóng" onClick={onClose}>×</button>
				</div>

				<label className="cp-oc-search">
					<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg>
					<input ref={searchRef} placeholder="Tìm theo tên hoặc mã sản phẩm" value={keyword} onChange={(e) => setKeyword(e.target.value)} />
				</label>
				<div className="cp-oc-bar">
					<small className="cp-oc-count">{items.length > 0 ? `Đã tải ${items.length}${total > items.length ? ` / ${total}` : ''} sản phẩm` : ' '}</small>
					<span className="cp-oc-sync">
						{updatedAt && <small>Cập nhật lúc {updatedAt.toLocaleTimeString('vi-VN')}</small>}
						<select aria-label="Tự động cập nhật" value={autoSec} onChange={(e) => changeAuto(Number(e.target.value))}>
							{AUTO_OPTIONS.map((o) => <option key={o.sec} value={o.sec}>{o.label}</option>)}
						</select>
						<button type="button" className={`cp-oc-refresh${refreshing ? ' spin' : ''}`} onClick={refresh} disabled={loading || refreshing} title="Lấy dữ liệu mới nhất từ TikTok Shop">
							<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M21 12a9 9 0 1 1-2.64-6.36" /><path d="M21 3v6h-6" /></svg>
							Cập nhật mới
						</button>
					</span>
				</div>

				<div className="cp-oc-cols" aria-hidden="true">
						<span>Thông tin sản phẩm</span>
						<span>Hàng có sẵn</span>
						<span>Hoa hồng tiêu chuẩn</span>
					</div>
					<div className="cp-oc-list" ref={listRef} role="listbox" aria-label="Sản phẩm cộng tác mở">
					{loading && Array.from({ length: SKELETON_ROWS }, (_, i) => (
						<div className="cp-oc-row skeleton" key={i}><span className="cp-oc-img" /><span className="cp-oc-info"><i /><i /></span></div>
					))}
					{!loading && items.map((p) => {
						const ok = selectable(p);
						const on = selected?.productId === p.productId;
						return (
							<button
								type="button"
								role="option"
								aria-selected={on}
								key={p.collaborationId || p.productId}
								data-pid={p.productId}
								className={`cp-oc-row${on ? ' on' : ''}`}
								disabled={!ok}
								onClick={() => setSelected(p)}
								onDoubleClick={() => ok && !saving && onConfirm(p)}
							>
								{p.imageUrl ? <img className="cp-oc-img" src={p.imageUrl} alt="" loading="lazy" /> : <span className="cp-oc-img" />}
								<span className="cp-oc-info">
									<strong title={p.title}>{p.title || p.productId}</strong>
									<small>ID: {p.productId}</small>
									{priceText(p) && <small className="cp-oc-price">{priceText(p)}</small>}
									{!p.available && <em className="cp-oc-bad">Sản phẩm không có sẵn</em>}
								</span>
								<span className={`cp-oc-stock${p.inventory <= 0 || p.productStatus === 'OUT_OF_STOCK' ? ' bad' : ''}`}>
									{(p.inventory <= 0 || p.productStatus === 'OUT_OF_STOCK') && <i aria-hidden="true">×</i>}
									{new Intl.NumberFormat('vi-VN').format(p.inventory)}
								</span>
								<span className="cp-oc-comm">
									<span className={`cp-oc-rate${p.status === 'NORMAL' ? '' : ' warn'}`}>Tỷ lệ {percent(p.commissionRate)}</span>
									{estimateText(p) && <small>Ước tính {estimateText(p)}</small>}
									{p.status !== 'NORMAL' && <small className="cp-oc-warn">Sắp hết hiệu lực</small>}
								</span>
							</button>
						);
					})}
					{!loading && next && !error && <div ref={sentinelRef} className="cp-oc-more">{loadingMore ? 'Đang tải thêm…' : ' '}</div>}
					{empty && <div className="cp-oc-empty">{keyword ? 'Không có sản phẩm khớp.' : 'Shop chưa mở cộng tác cho sản phẩm nào.'}</div>}
					{error && (
						<div className="cp-oc-empty err">
							{error}
							<button type="button" className="cp-link" onClick={() => (items.length ? loadMore() : search(keyword.trim()))}>Thử lại</button>
						</div>
					)}
				</div>

				<div className="cp-oc-foot">
					<span title="Lấy từ TikTok Shop, chỉ đọc">{selected ? <>Hoa hồng công khai: <b>{selected.commissionRate}%</b></> : 'Chọn một sản phẩm'}</span>
					<div className="cp-row">
						<button type="button" className="cp-btn ghost" onClick={onClose}>Huỷ</button>
						<button type="button" className="cp-btn" disabled={!selected || saving} onClick={() => selected && onConfirm(selected)}>{saving ? 'Đang lưu…' : 'Xác nhận'}</button>
					</div>
				</div>
			</div>
		</div>
	);
}
