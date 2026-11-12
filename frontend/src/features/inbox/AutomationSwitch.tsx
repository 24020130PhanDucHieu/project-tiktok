import { useState } from 'react';
import Portal from '../../components/Portal';
import api, { apiMessage } from '../../lib/axios';

interface Props {
	enabled: boolean;
	/** false until the switch has been read: it stays locked */
	loaded: boolean;
	isAdmin: boolean;
	onChanged: (enabled: boolean) => void;
	showToast: (m: string, t?: 'success' | 'error') => void;
}

/**
 * The master switch for everything the system sends to KOCs on its own: the chatbot's answers and the reminders on a
 * schedule. It is OFF until an admin turns it on. Turning it ON asks for a confirmation first; turning it OFF is
 * immediate. What a booker writes and sends in the chat is never affected. The server enforces the same rules.
 */
export default function AutomationSwitch({ enabled, loaded, isAdmin, onChanged, showToast }: Props) {
	const [ask, setAsk] = useState(false);
	const [understood, setUnderstood] = useState(false);
	const [busy, setBusy] = useState(false);

	async function apply(next: boolean) {
		setBusy(true);
		try {
			await api.put('/automation', { enabled: next, confirm: next });
			onChanged(next);
			setAsk(false);
			setUnderstood(false);
			showToast(next ? 'Đã BẬT gửi tin tự động. Chatbot và lịch nhắc sẽ nhắn cho KOC thật.' : 'Đã tắt gửi tin tự động.');
		} catch (e) {
			showToast(apiMessage(e, 'Không đổi được công tắc.'), 'error');
		} finally {
			setBusy(false);
		}
	}

	const locked = !isAdmin || busy || !loaded;
	return (
		<>
			<label
				className={`ti-switch auto-switch${locked ? ' disabled' : ''}${enabled ? ' on' : ''}`}
				title={isAdmin ? 'Bật hoặc tắt việc hệ thống tự nhắn cho KOC' : 'Chỉ quản trị viên đổi được'}
			>
				<input type="checkbox" checked={enabled} disabled={locked} onChange={(e) => (e.target.checked ? setAsk(true) : apply(false))} />
				<span className="ti-track"><i /></span>
				<b>Tin tự động: {enabled ? 'Bật' : 'Tắt'}</b>
			</label>

			{ask && (
				<Portal>
					<div className="cp-backdrop sw-front" onClick={() => !busy && setAsk(false)}>
						<div className="cp-modal" role="alertdialog" aria-modal="true" aria-labelledby="auto-on-title" onClick={(e) => e.stopPropagation()}>
							<h4 id="auto-on-title">Bật gửi tin tự động?</h4>
							<p className="cp-muted cp-dialog-text">
								Khi bật, hệ thống sẽ tự nhắn cho KOC thật mà không cần ai bấm gửi: chatbot trả lời tin KOC nhắn đến, và lịch nền nhắc KOC chưa phản hồi, nhắc đăng video, hỏi đã nhận mẫu chưa, nhắc gửi hoàn mẫu.
							</p>
							<p className="cp-muted cp-dialog-text">Tin bạn tự viết và gửi trong khung chat không bị ảnh hưởng. Tắt công tắc là dừng ngay.</p>
							<label className="ti-confirm">
								<input type="checkbox" checked={understood} onChange={(e) => setUnderstood(e.target.checked)} />
								<span>Tôi hiểu tin sẽ được gửi tới KOC thật.</span>
							</label>
							<div className="cp-row end">
								<button type="button" className="cp-btn ghost" onClick={() => { setAsk(false); setUnderstood(false); }} disabled={busy}>Giữ tắt</button>
								<button type="button" className="cp-btn" disabled={!understood || busy} onClick={() => apply(true)}>{busy ? 'Đang bật…' : 'Bật gửi tin tự động'}</button>
							</div>
						</div>
					</div>
				</Portal>
			)}
		</>
	);
}
