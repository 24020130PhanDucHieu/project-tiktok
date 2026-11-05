import { useState } from 'react';
import Portal from '../../components/Portal';
import api, { apiMessage } from '../../lib/axios';

interface Props {
	enabled: boolean;
	/** false until the setting has been read: the switch stays locked */
	loaded?: boolean;
	isAdmin: boolean;
	onChanged: (enabled: boolean) => void;
	showToast: (m: string, t?: 'success' | 'error') => void;
}

/**
 * The on/off switch for sending real invitations (off by default, admin only). Turning it ON asks for explicit
 * confirmation first; turning it OFF is immediate. The server enforces the same rules.
 */
export default function SendingSwitch({ enabled, loaded = true, isAdmin, onChanged, showToast }: Props) {
	const [ask, setAsk] = useState(false);
	const [understood, setUnderstood] = useState(false);
	const [busy, setBusy] = useState(false);

	async function apply(next: boolean) {
		setBusy(true);
		try {
			await api.put('/target-collabs/switch', { enabled: next, confirm: next });
			onChanged(next);
			setAsk(false);
			setUnderstood(false);
			showToast(next ? 'Đã BẬT gửi lời mời thật. Hãy cẩn thận với từng lần gửi.' : 'Đã tắt gửi lời mời thật.');
		} catch (e) {
			showToast(apiMessage(e, 'Không đổi được công tắc.'), 'error');
		} finally {
			setBusy(false);
		}
	}

	const locked = !isAdmin || busy || !loaded;
	return (
		<>
			<label className={`ti-switch${locked ? ' disabled' : ''}`} title={isAdmin ? '' : 'Chỉ quản trị viên'}>
				<input type="checkbox" checked={enabled} disabled={locked} onChange={(e) => (e.target.checked ? setAsk(true) : apply(false))} />
				<span className="ti-track"><i /></span>
				<b>{enabled ? 'Bật' : 'Tắt'}</b>
			</label>

			{ask && (
				<Portal>
					<div className="cp-backdrop sw-front" onClick={() => !busy && setAsk(false)}>
						<div className="cp-modal" role="alertdialog" aria-modal="true" aria-labelledby="sw-on-title" onClick={(e) => e.stopPropagation()}>
							<h4 id="sw-on-title">Bật gửi lời mời thật?</h4>
							<p className="cp-muted cp-dialog-text">Khi bật, nút <b>Gửi lời mời</b> sẽ tạo lời mời thật trên TikTok Shop và nhà sáng tạo được chọn sẽ nhận được nó. Mỗi lần gửi vẫn cần kiểm tra và xác nhận riêng, và chỉ quản trị viên gửi được.</p>
							<label className="ti-confirm"><input type="checkbox" checked={understood} onChange={(e) => setUnderstood(e.target.checked)} /><span>Tôi hiểu lời mời được gửi tới nhà sáng tạo thật và ảnh hưởng tới shop.</span></label>
							<div className="cp-row end">
								<button type="button" className="cp-btn ghost" onClick={() => { setAsk(false); setUnderstood(false); }} disabled={busy}>Giữ tắt</button>
								<button type="button" className="cp-btn" disabled={!understood || busy} onClick={() => apply(true)}>{busy ? 'Đang bật…' : 'Bật gửi lời mời'}</button>
							</div>
						</div>
					</div>
				</Portal>
			)}
		</>
	);
}
