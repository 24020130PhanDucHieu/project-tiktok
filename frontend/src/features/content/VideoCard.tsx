import { useState } from 'react';
import { formatCurrency, formatNumber } from '../../lib/formatters';
import type { CreatorVideo } from '../../types/koc';
import VideoPlayerModal from './VideoPlayerModal';

const dash = (v: number | null) => (v == null ? '—' : formatNumber(v));

export const postedLabel = (iso: string | null) => (iso ? new Date(iso).toLocaleDateString('vi-VN') : '—');

/** Cover picture, or a plain tile while TikTok has not given one. */
export function VideoCover({ video, className = '' }: { video: CreatorVideo; className?: string }) {
	return video.coverUrl ? (
		<img className={`kv-cover ${className}`} src={video.coverUrl} alt="" loading="lazy" referrerPolicy="no-referrer" />
	) : (
		<span className={`kv-cover empty ${className}`} aria-hidden="true">▶</span>
	);
}

interface Props {
	video: CreatorVideo;
	/** show the KOC the video belongs to (lists that mix several KOCs) */
	showKoc?: boolean;
	onOpen?: (v: CreatorVideo) => void;
}

/** One post of a KOC with its real numbers and the link to watch it on TikTok. */
export default function VideoCard({ video: v, showKoc, onOpen }: Props) {
	const title = v.title || `Video ${v.id.slice(-6)}`;
	const [playing, setPlaying] = useState(false);
	return (
		<article className="kv-card">
			<button type="button" className="kv-cover-btn" aria-label="Xem video" onClick={() => setPlaying(true)}>
				<VideoCover video={v} />
				<span className="kv-playmark" aria-hidden="true">▶</span>
			</button>
			<div className="kv-main">
				<b className="kv-title" title={title}>{title}</b>
				<small className="kv-sub">
					{showKoc && <>@{v.username}{v.kocName && v.kocName !== v.username ? ` · ${v.kocName}` : ''} · </>}
					Đăng {postedLabel(v.postedAt)}
				</small>
				<div className="kv-stats" title={v.statsSource === 'public' ? 'Số công khai hiển thị trên trang TikTok của video' : v.statsSource === 'analytics' ? 'Số từ TikTok Shop Analytics' : ''}>
					<span title="Lượt xem">👁 {dash(v.views)}</span>
					<span title="Lượt thích">♥ {dash(v.likes)}</span>
					<span title="Bình luận">💬 {dash(v.comments)}</span>
				</div>
				<div className="kv-sales">
					<span><b>{formatNumber(v.orders)}</b> đơn</span>
					<span>Doanh thu <b>{formatCurrency(v.gmv)}</b></span>
					<span title={v.commissionConfirmed >= v.commission && v.commission > 0 ? 'TikTok đã đối soát' : 'Một phần còn là ước tính, TikTok chưa đối soát'}>
						Hoa hồng <b>{formatCurrency(v.commission)}</b>
					</span>
				</div>
				<div className="kv-actions">
					<button type="button" className="kv-link" onClick={() => setPlaying(true)}>Xem video</button>
					<a className="kv-more" href={v.url} target="_blank" rel="noreferrer">Mở trên TikTok</a>
					{onOpen && <button type="button" className="kv-more" onClick={() => onOpen(v)}>Chi tiết</button>}
				</div>
			</div>
			{playing && <VideoPlayerModal video={v} onClose={() => setPlaying(false)} />}
		</article>
	);
}
