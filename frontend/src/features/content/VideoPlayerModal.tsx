import { useEffect } from 'react';
import Portal from '../../components/Portal';
import type { CreatorVideo } from '../../types/koc';

/**
 * Plays a TikTok video inside this web app with TikTok's official embedded player. TikTok's normal video page often
 * will not open on a computer; the embedded player does, as long as the creator allows embedding.
 */
export default function VideoPlayerModal({ video, onClose }: { video: Pick<CreatorVideo, 'id' | 'username' | 'title' | 'url'>; onClose: () => void }) {
	useEffect(() => {
		const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	}, [onClose]);

	return (
		<Portal>
			<div className="kv-modal-backdrop" onClick={onClose}>
				<div className="kv-modal" role="dialog" aria-label="Xem video" onClick={(e) => e.stopPropagation()}>
					<div className="kv-modal-head">
						<div>
							<b>@{video.username}</b>
							<small title={video.title}>{video.title || `Video ${video.id.slice(-6)}`}</small>
						</div>
						<button type="button" className="cp-oc-x" aria-label="Đóng" onClick={onClose}>×</button>
					</div>
					<iframe
						className="kv-modal-player"
						title="Video TikTok"
						src={`https://www.tiktok.com/player/v1/${video.id}?music_info=1&description=1`}
						allow="autoplay; fullscreen; picture-in-picture"
						allowFullScreen
					/>
					<small className="kv-modal-note">
						Video không hiện? Nhà sáng tạo có thể đã tắt cho phép nhúng, hoặc TikTok đang bận. <a href={video.url} target="_blank" rel="noreferrer">Mở trên TikTok</a> (thường mở được trên điện thoại).
					</small>
				</div>
			</div>
		</Portal>
	);
}
