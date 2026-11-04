import { type Dispatch, type FormEvent, type KeyboardEvent, type ReactNode, type SetStateAction, useEffect, useMemo, useRef, useState } from 'react';
import api from '../../lib/axios';
import { MESSAGE_TEMPLATES } from '../../lib/constants';
import type { ChatChannel, ChatMessage, ConversationItem, Creator, GmailStatus, KocContact, SavedTemplate } from '../../types/koc';
import Chevron from '../../components/Chevron';

export function messageStatusLabel(status?: string): string {
	switch (status) {
		case 'queued':
			return 'Chờ gửi';
		case 'pending':
		case 'sending':
			return 'Đang gửi…';
		case 'failed':
			return 'Gửi lỗi';
		default:
			return 'Đã nhận';
	}
}

const EMOJIS = ['😀', '😊', '😍', '🥰', '👍', '🙏', '🎉', '💖', '🔥', '😅', '😉', '🤝'];
const fmtDate = (iso?: string) => (iso ? new Date(iso).toLocaleDateString('vi-VN') : '');
const maskPhone = (p: string) => (p.length > 6 ? `${p.slice(0, 4)}…${p.slice(-3)}` : p);

interface Props {
	conv: ConversationItem;
	koc: Creator | null;
	live: boolean;
	channel: ChatChannel;
	setChannel: (c: ChatChannel) => void;
	setMessages: Dispatch<SetStateAction<ChatMessage[]>>;
	setConversations: Dispatch<SetStateAction<ConversationItem[]>>;
	draft: string;
	setDraft: Dispatch<SetStateAction<string>>;
	zaloConnected: boolean;
	accountName: string;
	onReconnectZalo: () => void;
	gmail: GmailStatus;
	onConnectGmail: () => void;
	showToast: (msg: string, type?: 'success' | 'error') => void;
	extraTools?: ReactNode;
}

export default function Composer({ conv, koc, live, channel, setChannel, setMessages, setConversations, draft, setDraft, zaloConnected, accountName, onReconnectZalo, gmail, onConnectGmail, showToast, extraTools }: Props) {
	const [contacts, setContacts] = useState<KocContact[]>([]);
	const [pick, setPick] = useState<Record<string, string>>({});
	const [templates, setTemplates] = useState<SavedTemplate[]>([]);
	const [menu, setMenu] = useState<'contact' | 'template' | 'emoji' | null>(null);
	const [addOpen, setAddOpen] = useState<null | 'phone' | 'email'>(null);
	const [addValue, setAddValue] = useState('');
	const [tplForm, setTplForm] = useState<null | { name: string; body: string }>(null);
	const rootRef = useRef<HTMLDivElement>(null);
	const queue = useRef<Promise<void>>(Promise.resolve());
	const kocId = koc && live ? String(koc.id) : '';

	const loadContacts = () => {
		if (!kocId) return setContacts([]);
		api.get(`/kocs/${kocId}/contacts`)
			.then(({ data }) => setContacts(data.data || []))
			.catch(() => setContacts([]));
	};
	useEffect(loadContacts, [kocId]);
	useEffect(() => {
		if (!live) return;
		api.get('/inbox/templates')
			.then(({ data }) => setTemplates(data.data || []))
			.catch(() => {});
	}, [live]);
	useEffect(() => {
		if (menu === null) return undefined;
		const onDown = (e: MouseEvent) => {
			if (!rootRef.current?.contains(e.target as Node)) setMenu(null);
		};
		document.addEventListener('mousedown', onDown);
		return () => document.removeEventListener('mousedown', onDown);
	}, [menu]);

	const phones = contacts.filter((c) => c.type === 'phone');
	const emails = contacts.filter((c) => c.type === 'email');
	const list = channel === 'email' ? emails : phones;
	const current = useMemo(() => {
		const chosen = list.find((c) => c.id === pick[channel] && !c.unreachable);
		if (chosen) return chosen;
		if (channel === 'zalo') {
			const prefer = list.find((c) => !c.unreachable && c.value === conv.contactPhone);
			if (prefer) return prefer;
		}
		return list.find((c) => !c.unreachable) || null;
	}, [list, pick, channel, conv.contactPhone]);

	const fallbackPhone = !live ? conv.contactPhone || '' : '';
	const target = current?.value || fallbackPhone;
	const gmailBlock = channel === 'email' && (!gmail.connected || gmail.broken);
	const noTarget = live && !target;
	const blocked = gmailBlock || noTarget;

	function choose(c: KocContact) {
		if (c.unreachable) return;
		setChannel(c.type === 'email' ? 'email' : 'zalo');
		setPick((p) => ({ ...p, [c.type === 'email' ? 'email' : 'zalo']: c.id }));
		setMenu(null);
	}

	async function toggleUnreachable(c: KocContact) {
		try {
			await api.patch(`/kocs/${kocId}/contacts/${c.id}`, { unreachable: !c.unreachable });
			loadContacts();
		} catch {
			showToast('Không cập nhật được liên hệ.', 'error');
		}
	}

	async function saveContact() {
		if (!addOpen || !addValue.trim()) return;
		try {
			await api.post(`/kocs/${kocId}/contacts`, { type: addOpen, value: addValue.trim() });
			setAddOpen(null);
			setAddValue('');
			loadContacts();
		} catch (e: any) {
			showToast(e?.response?.data?.message || 'Không lưu được liên hệ.', 'error');
		}
	}

	function applyTemplate(body: string) {
		const first = (koc?.name || conv.contactName || '').trim().split(/\s+/).slice(-1)[0] || '';
		setDraft(body.replaceAll('{ten}', first));
		setMenu(null);
	}

	async function saveTemplate() {
		if (!tplForm?.name.trim() || !tplForm.body.trim()) return;
		try {
			const { data } = await api.post('/inbox/templates', { name: tplForm.name.trim(), body: tplForm.body });
			setTemplates((t) => [...t, data.data]);
			setTplForm(null);
		} catch (e: any) {
			showToast(e?.response?.data?.message || 'Không lưu được mẫu.', 'error');
		}
	}

	async function removeTemplate(id: string) {
		try {
			await api.delete(`/inbox/templates/${id}`);
			setTemplates((t) => t.filter((x) => x._id !== id));
		} catch {
			showToast('Không xoá được mẫu.', 'error');
		}
	}

	function patchMsg(id: string, next: Partial<ChatMessage> | ChatMessage, replace = false) {
		setMessages((prev) => prev.map((m) => (m._id === id ? (replace ? (next as ChatMessage) : { ...m, ...next }) : m)));
	}

	function send(text: string) {
		const tmp: ChatMessage = { _id: `tmp-${Date.now()}`, direction: 'outbound', text, sentAt: new Date().toISOString(), status: live ? 'queued' : 'sent', channel, to: target };
		setMessages((prev) => [...prev, tmp]);
		setConversations((prev) => prev.map((c) => (c._id === conv._id ? { ...c, lastMessage: text, lastMessageAt: tmp.sentAt } : c)));
		if (!live) return;
		queue.current = queue.current.then(async () => {
			patchMsg(tmp._id, { status: 'sending' });
			try {
				const { data } = await api.post(`/inbox/conversations/${conv._id}/messages`, { text, channel, to: target });
				patchMsg(tmp._id, data.data, true);
			} catch (e: any) {
				const saved = e?.response?.data?.data as ChatMessage | undefined;
				if (saved) patchMsg(tmp._id, saved, true);
				else patchMsg(tmp._id, { status: 'failed' });
				showToast(e?.response?.data?.message || 'Không gửi được tin nhắn.', 'error');
			}
		});
	}

	function submit(e?: FormEvent) {
		e?.preventDefault();
		const text = draft.trim();
		if (!text || blocked) return;
		send(text);
		setDraft('');
	}

	function onKey(e: KeyboardEvent<HTMLTextAreaElement>) {
		if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
			e.preventDefault();
			submit();
		}
	}

	const senderLabel = channel === 'email' ? (gmail.connected ? gmail.email : 'chưa kết nối') : zaloConnected ? 'đã kết nối' : 'mất kết nối';
	const channelTitle = channel === 'email' ? 'Email' : 'Zalo';
	const koName = koc?.name || conv.contactName;

	return (
		<footer className="msg-chat-footer" ref={rootRef}>
			{live && channel === 'email' && !gmail.connected && !gmail.broken && (
				<div className="cmp-banner">
					<span>{gmail.configured ? 'Chưa kết nối tài khoản Gmail' : 'Máy chủ chưa cấu hình Google OAuth (GOOGLE_CLIENT_ID)'}</span>
					{gmail.configured && <button type="button" onClick={onConnectGmail}>Kết nối Gmail</button>}
				</div>
			)}
			{live && channel === 'email' && gmail.broken && (
				<div className="cmp-banner">
					<span>Tài khoản Gmail {gmail.email} đã mất kết nối</span>
					<button type="button" onClick={onConnectGmail}>Kết nối lại</button>
				</div>
			)}
			{live && !gmailBlock && noTarget && (
				<div className="cmp-banner">
					<span>{channel === 'email' ? 'KOC chưa có email' : 'KOC chưa có số điện thoại Zalo'}</span>
					<button type="button" onClick={() => { setAddOpen(channel === 'email' ? 'email' : 'phone'); setAddValue(''); }}>
						{channel === 'email' ? 'Thêm email' : 'Thêm zalo'}
					</button>
				</div>
			)}

			<div className="composer-meta-bar">
				<span className="cmp-sender">
					Từ <b>{channel === 'email' ? gmail.email || 'Gmail' : accountName}</b>
					<span className="cmp-arrow">→</span>
					<button type="button" className="cmp-contact-btn" onClick={() => setMenu(menu === 'contact' ? null : 'contact')}>
						<b>{channelTitle}</b> · {target ? (channel === 'email' ? target : maskPhone(target)) : senderLabel} <Chevron />
					</button>
				</span>
				<div className="composer-tools">
					{extraTools}
					<button type="button" className="cmp-tpl-btn" onClick={() => setMenu(menu === 'template' ? null : 'template')}>
						Mẫu: {templates.length ? `${templates.length} mẫu` : 'chưa có mẫu nào'} <Chevron />
					</button>
				</div>
			</div>

			{menu === 'contact' && (
				<div className="cmp-menu cmp-contact-menu">
					<div className="msg-menu-title">SĐT · Zalo</div>
					{phones.length === 0 && <div className="msg-menu-muted">Chưa có số điện thoại.</div>}
					{phones.map((c) => (
						<div key={c.id} className={`cmp-item ${current?.id === c.id && channel === 'zalo' ? 'on' : ''} ${c.unreachable ? 'bad' : ''}`}>
							<button type="button" className="cmp-item-main" onClick={() => choose(c)}>
								<span className="cmp-radio" />
								<span>
									<b>{c.value}</b>
									<small>{c.unreachable ? 'không tới được' : `${c.source} · ${fmtDate(c.updatedAt)}`}</small>
								</span>
							</button>
							<button type="button" className="cmp-ban" title={c.unreachable ? 'Bỏ đánh dấu' : 'Đánh dấu không tới được'} onClick={() => toggleUnreachable(c)}>{c.unreachable ? '↺' : '⊘'}</button>
						</div>
					))}
					<div className="msg-menu-sep" />
					<div className="msg-menu-title">Email</div>
					{emails.length === 0 && <div className="msg-menu-muted">Chưa có email.</div>}
					{emails.map((c) => (
						<div key={c.id} className={`cmp-item ${current?.id === c.id && channel === 'email' ? 'on' : ''} ${c.unreachable ? 'bad' : ''}`}>
							<button type="button" className="cmp-item-main" onClick={() => choose(c)}>
								<span className="cmp-radio" />
								<span>
									<b>{c.value}</b>
									<small>{c.unreachable ? 'không tới được' : `${c.source} · ${fmtDate(c.updatedAt)}`}</small>
								</span>
							</button>
							<button type="button" className="cmp-ban" title={c.unreachable ? 'Bỏ đánh dấu' : 'Đánh dấu không tới được'} onClick={() => toggleUnreachable(c)}>{c.unreachable ? '↺' : '⊘'}</button>
						</div>
					))}
					<div className="msg-menu-sep" />
					<div className="msg-menu-title">TikTok</div>
					<div className="msg-menu-muted">Kênh TikTok chưa mở.</div>
					<div className="msg-menu-sep" />
					<button type="button" className="msg-menu-link" disabled={!live} onClick={() => { setMenu(null); setAddOpen(channel === 'email' ? 'email' : 'phone'); setAddValue(''); }}>+ Thêm liên hệ</button>
				</div>
			)}

			{menu === 'template' && (
				<div className="cmp-menu cmp-tpl-menu">
					{tplForm ? (
						<div className="cmp-tpl-form">
							<input autoFocus placeholder="Tên mẫu" maxLength={60} value={tplForm.name} onChange={(e) => setTplForm({ ...tplForm, name: e.target.value })} />
							<textarea placeholder="Nội dung (dùng {ten} để chèn tên KOC)" rows={4} value={tplForm.body} onChange={(e) => setTplForm({ ...tplForm, body: e.target.value })} />
							<div className="cp-row end">
								<button type="button" className="cp-btn ghost" onClick={() => setTplForm(null)}>Huỷ</button>
								<button type="button" className="cp-btn" disabled={!tplForm.name.trim() || !tplForm.body.trim()} onClick={saveTemplate}>Lưu</button>
							</div>
						</div>
					) : (
						<>
							{templates.length > 0 && <div className="msg-menu-title">Mẫu của bạn</div>}
							{templates.map((t) => (
								<div key={t._id} className="cmp-item">
									<button type="button" className="cmp-item-main" onClick={() => applyTemplate(t.body)}><span><b>{t.name}</b><small>{t.body.slice(0, 60)}</small></span></button>
									<button type="button" className="cmp-ban" title="Xoá mẫu" onClick={() => removeTemplate(t._id)}>🗑</button>
								</div>
							))}
							<div className="msg-menu-title">Mẫu có sẵn</div>
							{MESSAGE_TEMPLATES.map((t) => (
								<div key={t.id} className="cmp-item">
									<button type="button" className="cmp-item-main" onClick={() => applyTemplate(t.build(koc))}><span><b>{t.label}</b></span></button>
								</div>
							))}
							<div className="msg-menu-sep" />
							<button type="button" className="msg-menu-link" disabled={!live} onClick={() => setTplForm({ name: '', body: '' })}>+ Thêm mẫu</button>
						</>
					)}
				</div>
			)}

			{menu === 'emoji' && (
				<div className="cmp-menu cmp-emoji">
					{EMOJIS.map((e) => (
						<button key={e} type="button" onClick={() => { setDraft((d) => d + e); setMenu(null); }}>{e}</button>
					))}
				</div>
			)}

			<form className="composer-input-row" onSubmit={submit}>
				<textarea
					rows={1}
					value={draft}
					disabled={blocked}
					onChange={(e) => setDraft(e.target.value)}
					onKeyDown={onKey}
					placeholder={blocked ? 'Hoàn tất kết nối ở thanh phía trên để nhắn tin…' : `Nhắn ${koName} qua ${channelTitle}… (Enter để gửi, Shift+Enter xuống dòng)`}
				/>
				<button type="button" className="cmp-emoji-btn" onClick={() => setMenu(menu === 'emoji' ? null : 'emoji')} title="Biểu tượng cảm xúc">☺</button>
				<button type="submit" className="cmp-send" disabled={!draft.trim() || blocked} title="Gửi (Enter)" aria-label="Gửi"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M22 2 11 13" /><path d="M22 2 15 22l-4-9-9-4 20-7Z" /></svg></button>
			</form>

			{addOpen && (
				<div className="cp-backdrop" onClick={() => setAddOpen(null)}>
					<div className="cp-modal" onClick={(e) => e.stopPropagation()}>
						<h4>Thêm liên hệ</h4>
						<div className="cmp-tabs">
							<button type="button" className={addOpen === 'phone' ? 'on' : ''} onClick={() => { setAddOpen('phone'); setAddValue(''); }}>Số điện thoại</button>
							<button type="button" className={addOpen === 'email' ? 'on' : ''} onClick={() => { setAddOpen('email'); setAddValue(''); }}>Email</button>
						</div>
						<input autoFocus value={addValue} inputMode={addOpen === 'phone' ? 'tel' : 'email'} placeholder={addOpen === 'phone' ? '0912 345 678' : 'ten@email.com'} onChange={(e) => setAddValue(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && saveContact()} />
						<small className="cp-muted">Liên hệ bạn thêm chỉ hiển thị với bạn.</small>
						<div className="cp-row end">
							<button type="button" className="cp-btn ghost" onClick={() => setAddOpen(null)}>Huỷ</button>
							<button type="button" className="cp-btn" disabled={!addValue.trim()} onClick={saveContact}>Lưu</button>
						</div>
					</div>
				</div>
			)}
		</footer>
	);
}
