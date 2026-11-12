import { useEffect, useRef, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { ChatMessage } from '../../types/koc';

/** The reactions the chat offers. The key is what the server knows; it turns it into the code Zalo uses. */
export const REACTIONS: { key: string; emoji: string; label: string }[] = [
	{ key: 'heart', emoji: '❤️', label: 'Thả tim' },
	{ key: 'like', emoji: '👍', label: 'Thích' },
	{ key: 'haha', emoji: '😆', label: 'Haha' },
	{ key: 'wow', emoji: '😮', label: 'Ồ' },
	{ key: 'cry', emoji: '😢', label: 'Buồn' },
	{ key: 'angry', emoji: '😡', label: 'Giận' },
	{ key: 'pray', emoji: '🙏', label: 'Cảm ơn' },
	{ key: 'ok', emoji: '👌', label: 'OK' },
];

export const reactionEmoji = (key?: string) => REACTIONS.find((r) => r.key === key)?.emoji || '';

interface Props {
	conversationId: string;
	msg: ChatMessage;
	onChanged: (updated: ChatMessage) => void;
	showToast: (msg: string, type?: 'success' | 'error') => void;
}

/** "Thả cảm xúc" on a message the KOC sent: it appears on their Zalo too. Choosing the same one again takes it back. */
export default function ReactionButton({ conversationId, msg, onChanged, showToast }: Props) {
	const [open, setOpen] = useState(false);
	const [busy, setBusy] = useState(false);
	const root = useRef<HTMLSpanElement>(null);

	useEffect(() => {
		if (!open) return undefined;
		const close = (e: MouseEvent) => {
			if (!root.current?.contains(e.target as Node)) setOpen(false);
		};
		document.addEventListener('mousedown', close);
		return () => document.removeEventListener('mousedown', close);
	}, [open]);

	async function choose(key: string) {
		if (busy) return;
		const next = msg.reaction === key ? '' : key;
		setBusy(true);
		try {
			const { data } = await api.put(`/inbox/conversations/${conversationId}/messages/${msg._id}/reaction`, { reaction: next });
			onChanged({ ...msg, ...data.data });
			setOpen(false);
		} catch (e) {
			showToast(apiMessage(e, 'Không thả được cảm xúc.'), 'error');
		} finally {
			setBusy(false);
		}
	}

	return (
		<span className="react-wrap" ref={root}>
			<button type="button" className={`btn-react${msg.reaction ? ' on' : ''}`} onClick={() => setOpen((v) => !v)} title="Thả cảm xúc" aria-label="Thả cảm xúc" aria-expanded={open}>
				☺
			</button>
			{open && (
				<div className="react-pop" role="menu">
					{REACTIONS.map((r) => (
						<button key={r.key} type="button" role="menuitem" className={msg.reaction === r.key ? 'on' : ''} title={r.label} disabled={busy} onClick={() => choose(r.key)}>
							{r.emoji}
						</button>
					))}
				</div>
			)}
		</span>
	);
}
