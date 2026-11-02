import { useEffect, useMemo, useState } from 'react';
import { formatCurrency, formatNumber } from '../../lib/formatters';
import type { Creator, VideoItem } from '../../types/koc';

const PAGE = 12;

function parseCount(s?: string): number {
	const m = String(s ?? '').trim().toLowerCase().replace(/,/g, '.').match(/^([\d.]+)\s*([kmb]?)/);
	if (!m) return 0;
	const mult = { '': 1, k: 1e3, m: 1e6, b: 1e9 }[m[2] as '' | 'k' | 'm' | 'b'];
	return (parseFloat(m[1]) || 0) * mult;
}

function embedUrl(v: VideoItem): string {
	const id = v.id || (v.url || '').match(/video\/(\d+)/)?.[1] || '';
	return /^\d+$/.test(id) ? `https://www.tiktok.com/embed/v2/${id}` : '';
}

interface Props {
	koc: Creator | null;
	syncing: boolean;
	setSyncing: (v: boolean) => void;
	onSyncChannel: (creator: Creator | null, opts?: { autoDetectCollabVideo?: boolean }) => void;
}

export default function ChannelTab({ koc, syncing, setSyncing, onSyncChannel }: Props) {
	const [type, setType] = useState<'ALL' | 'SELL' | 'NONSELL'>('ALL');
	const [sort, setSort] = useState<'NEWEST' | 'POPULAR'>('NEWEST');
	const [shown, setShown] = useState(PAGE);
	const [viewIdx, setViewIdx] = useState<number | null>(null);
	const [requested, setRequested] = useState(false);

	useEffect(() => {
		setType('ALL');
		setSort('NEWEST');
		setShown(PAGE);
		setViewIdx(null);
		setRequested(false);
	}, [koc?.id]);

	const videos = koc?.videos || [];
	const list = useMemo(() => {
		const out = videos.filter((v) => (type === 'SELL' ? !!v.product : type === 'NONSELL' ? !v.product : true));
		if (sort === 'POPULAR') out.sort((a, b) => parseCount(b.views) - parseCount(a.views));
		return out;
	}, [videos, type, sort]);

	if (!koc) return <div className="cp-empty">Chọn một KOC để xem số liệu kênh.</div>;

	const hasData = !!koc.channelSyncedAt || videos.length > 0;
	const handle = (koc.handle || koc.tiktokId || '').replace(/^@/, '');

	function collect() {
		if (!handle) return;
		setRequested(true);
		setSyncing(true);
		Promise.resolve(onSyncChannel(koc)).finally(() => setTimeout(() => setSyncing(false), 500));
	}

	const current = viewIdx !== null ? list[viewIdx] : null;
	const embed = current ? embedUrl(current) : '';

	return (
		<div className="ch">
			{!hasData && (
				<div className="cp-card ch-empty">
					<strong>Chưa có kênh này trong kho dữ liệu</strong>
					{handle ? (
						<>
							<small>Bấm Thu thập dữ liệu, rồi mở lại hội thoại sau vài phút.</small>
							<button type="button" className="cp-btn" disabled={requested || syncing} onClick={collect}>
								{requested ? 'Đã gửi yêu cầu thu thập' : syncing ? 'Đang gửi…' : 'Thu thập dữ liệu'}
							</button>
						</>
					) : (
						<small>KOC này chưa có @handle nên chưa thu thập được.</small>
					)}
				</div>
			)}

			<div className="channel-stat-cards">
				<div className="ch-stat-box">
					<span>Follower</span>
					<strong>{koc.followers ? formatNumber(koc.followers) : <em className="ch-na">chưa tra được</em>}</strong>
				</div>
				<div className="ch-stat-box">
					<span>GMV 30 ngày</span>
					<strong>{koc.gmv ? formatCurrency(koc.gmv) : <em className="ch-na">chưa tra được</em>}</strong>
				</div>
			</div>

			{hasData && (
				<>
					<div className="ch-video-header">
						<span>VIDEO · {videos.length} GẦN NHẤT</span>
						<small>
							Quét {koc.channelSyncedAt ? new Date(koc.channelSyncedAt).toLocaleDateString('vi-VN') : '—'}
							<button type="button" className="btn-refresh-channel" disabled={syncing || !handle} onClick={collect}>
								{syncing ? 'Đang làm mới…' : 'Làm mới'}
							</button>
						</small>
					</div>

					<div className="ch-select-row">
						<label className="ch-filter">
							<span>Loại video</span>
							<select value={type} onChange={(e) => { setType(e.target.value as typeof type); setShown(PAGE); }}>
								<option value="ALL">Tất cả</option>
								<option value="SELL">Video bán hàng</option>
								<option value="NONSELL">Không bán hàng</option>
							</select>
						</label>
						<label className="ch-filter">
							<span>Sắp xếp</span>
							<select value={sort} onChange={(e) => setSort(e.target.value as typeof sort)}>
								<option value="NEWEST">Mới nhất</option>
								<option value="POPULAR">Phổ biến nhất</option>
							</select>
						</label>
					</div>

					{list.length === 0 && <div className="cp-empty">Không có video phù hợp bộ lọc.</div>}
					<div className="ch-video-list">
						{list.slice(0, shown).map((v, i) => (
							<button key={v.id || v.url || i} type="button" className="ch-video-item" onClick={() => setViewIdx(i)}>
								<div
									className="ch-video-thumb"
									style={v.coverUrl ? { backgroundImage: `linear-gradient(rgba(15,23,42,0.12), rgba(15,23,42,0.45)), url(${v.coverUrl})`, backgroundSize: 'cover', backgroundPosition: 'center', color: '#fff' } : undefined}
								>
									{!v.coverUrl && <span className="play-triangle">▷</span>}
									{!v.coverUrl && <small>{v.date}</small>}
								</div>
								<div className="ch-video-info">
									<strong>{v.title || 'Video không có tiêu đề'}</strong>
									<small className="ch-video-metrics">
										<span>{v.views || '—'} xem</span>
										<span>{v.likes || '—'} thích</span>
										<span>{v.comments || '—'} bình luận</span>
										<span>{v.shares || '—'} chia sẻ</span>
									</small>
									{v.product && <small className="ch-video-prod">🛍 {v.product}</small>}
								</div>
							</button>
						))}
					</div>
					{shown < list.length && (
						<button type="button" className="cp-btn ghost wide" onClick={() => setShown((n) => n + PAGE)}>
							Tải thêm ({list.length - shown})
						</button>
					)}
				</>
			)}

			{current && (
				<div className="cp-backdrop" onClick={() => setViewIdx(null)}>
					<div className="ch-viewer" onClick={(e) => e.stopPropagation()}>
						<div className="ch-viewer-head">
							<strong>{current.title || 'Video'}</strong>
							<button type="button" className="cp-link" onClick={() => setViewIdx(null)}>Đóng ✕</button>
						</div>
						{embed ? (
							<iframe title="Video TikTok" src={embed} allow="encrypted-media; fullscreen" className="ch-viewer-frame" />
						) : (
							<div className="cp-empty">Không nhúng được video này. Hãy mở trên TikTok.</div>
						)}
						<div className="ch-viewer-nav">
							<button type="button" className="cp-btn ghost" disabled={viewIdx === 0} onClick={() => setViewIdx((i) => (i ?? 1) - 1)}>← Trước</button>
							<a className="cp-btn" href={current.url || `https://www.tiktok.com/@${handle}`} target="_blank" rel="noreferrer">Mở trên TikTok</a>
							<button type="button" className="cp-btn ghost" disabled={viewIdx !== null && viewIdx >= list.length - 1} onClick={() => setViewIdx((i) => (i ?? 0) + 1)}>Sau →</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
}
