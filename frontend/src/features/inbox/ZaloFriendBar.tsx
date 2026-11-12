import { useEffect, useState } from 'react';
import Portal from '../../components/Portal';
import api, { apiMessage } from '../../lib/axios';
import type { ConversationItem, Creator } from '../../types/koc';

interface FriendState {
	available: boolean;
	reason?: string;
	isFriend: boolean;
	requestSent: boolean;
	requestReceived: boolean;
	noteMax: number;
}

interface Props {
	conv: ConversationItem;
	koc: Creator | null;
	live: boolean;
	/** the personal Zalo line is logged in */
	zaloConnected: boolean;
	accountName: string;
	showToast: (msg: string, type?: 'success' | 'error') => void;
}

const NOTE_MAX = 150;
const chars = (s: string) => [...s].length; // Zalo counts characters, not bytes: "ạ" is one

const fit = (text: string) => [...text].slice(0, NOTE_MAX).join('');

/**
 * Notes worth sending, written the way a person would: who you are, why you write, no tags or quotation marks. They are
 * built from what is known (the KOC's name, who is sending, the sample on offer) and each fits the 150 characters.
 */
export function noteSuggestions(kocName: string, me: string, product?: string): string[] {
	const first = kocName.trim().split(/\s+/).slice(-1)[0];
	const hello = first ? `Chào ${first}` : 'Xin chào';
	const item = product?.trim() ? `mẫu ${product.trim()}` : 'sản phẩm bên mình';
	return [
		`${hello}, mình là ${me} bên nhãn hàng. Mình muốn gửi bạn sản phẩm để trải nghiệm, kết bạn với mình nhé!`,
		`${hello}, mình là ${me}. Mình xem kênh của bạn thấy rất hợp với ${item}, kết bạn để mình trao đổi thêm nhé.`,
		`${hello}, mình là ${me} bên nhãn hàng, bên mình đang tìm bạn đồng hành lên video. Kết bạn với mình để nói chuyện nhé!`,
		`Xin chào, mình là ${me}. Mình đã nhắn bạn về ${item} mà chưa thấy phản hồi, kết bạn để tiện trao đổi nhé.`,
		`${hello}, mình là ${me}. Mình muốn gửi tặng bạn ${item} để dùng thử, bạn kết bạn để mình xin địa chỉ nhận hàng nhé!`,
	]
		.map(fit)
		.filter((t, i, all) => all.indexOf(t) === i);
}

/**
 * Shown above the chat when the KOC is on Zalo but not yet a friend of the connected account: the same "Gửi yêu cầu
 * kết bạn" Zalo shows, with the note it lets you attach (150 characters). Only a click sends it; nothing here is automatic.
 */
export default function ZaloFriendBar({ conv, koc, live, zaloConnected, accountName, showToast }: Props) {
	const [state, setState] = useState<FriendState | null>(null);
	const [open, setOpen] = useState(false);
	const [note, setNote] = useState('');
	const [busy, setBusy] = useState(false);
	const [ideas, setIdeas] = useState(false);
	const askable = live && zaloConnected && !!(conv.externalContactId || conv.contactPhone);

	useEffect(() => {
		setState(null);
		if (!askable) return undefined;
		let current = true;
		api.get(`/inbox/conversations/${conv._id}/zalo/friend`)
			.then(({ data }) => current && setState(data.data))
			.catch(() => current && setState(null));
		return () => {
			current = false;
		};
	}, [conv._id, askable]);

	if (!askable || !state?.available || state.isFriend) return null;

	const limit = state.noteMax || NOTE_MAX;
	const used = chars(note);

	async function send() {
		if (!note.trim() || used > limit || busy) return;
		setBusy(true);
		try {
			await api.post(`/inbox/conversations/${conv._id}/zalo/friend-request`, { message: note.trim() });
			setState((s) => (s ? { ...s, requestSent: true } : s));
			setOpen(false);
			showToast('Đã gửi lời mời kết bạn Zalo. Khi KOC chấp nhận, hai bên là bạn bè.');
		} catch (e) {
			showToast(apiMessage(e, 'Không gửi được lời mời kết bạn.'), 'error');
		} finally {
			setBusy(false);
		}
	}

	return (
		<>
			<div className="zfb">
				<span className="zfb-text">
					{state.requestSent
						? 'Đã gửi lời mời kết bạn Zalo, đang chờ KOC chấp nhận.'
						: state.requestReceived
							? 'KOC đã gửi lời mời kết bạn cho bạn. Hãy chấp nhận trong ứng dụng Zalo.'
							: 'Gửi yêu cầu kết bạn tới người này'}
				</span>
				{!state.requestSent && !state.requestReceived && (
					<button
						type="button"
						className="zfb-btn"
						onClick={() => {
							setNote(noteSuggestions(koc?.name || conv.contactName || '', accountName, koc?.productName)[0]);
							setIdeas(false);
							setOpen(true);
						}}
					>
						Gửi kết bạn
					</button>
				)}
			</div>

			{open && (
				<Portal>
					<div className="cp-backdrop sw-front" onClick={() => !busy && setOpen(false)}>
						<div className="cp-modal zfb-modal" role="dialog" aria-modal="true" aria-labelledby="zfb-title" onClick={(e) => e.stopPropagation()}>
							<h4 id="zfb-title">Kết bạn Zalo với {koc?.name || conv.contactName}</h4>
							<p className="cp-muted cp-dialog-text">Lời nhắn này hiện cùng lời mời để KOC biết bạn là ai.</p>
							<div className="zfb-note">
								<textarea autoFocus rows={4} value={note} onChange={(e) => setNote(e.target.value)} aria-label="Lời nhắn kết bạn" />
								<small className={used > limit ? 'over' : ''}>
									{used}/{limit} ký tự
								</small>
							</div>
							<button type="button" className="zfb-ideas-btn" aria-expanded={ideas} onClick={() => setIdeas((v) => !v)}>
								{ideas ? 'Ẩn gợi ý' : 'Gợi ý lời nhắn'}
							</button>
							{ideas && (
								<ul className="zfb-ideas">
									{noteSuggestions(koc?.name || conv.contactName || '', accountName, koc?.productName).map((t) => (
										<li key={t}>
											<button type="button" className={t === note ? 'on' : ''} onClick={() => setNote(t)}>
												{t}
											</button>
										</li>
									))}
								</ul>
							)}
							<div className="cp-row end">
								<button type="button" className="cp-btn ghost" onClick={() => setOpen(false)} disabled={busy}>
									Huỷ
								</button>
								<button type="button" className="cp-btn" disabled={busy || !note.trim() || used > limit} onClick={send}>
									{busy ? 'Đang gửi…' : 'Kết bạn'}
								</button>
							</div>
						</div>
					</div>
				</Portal>
			)}
		</>
	);
}
