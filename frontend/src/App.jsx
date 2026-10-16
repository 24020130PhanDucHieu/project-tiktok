import { useEffect, useMemo, useState } from 'react';
import api from './lib/axios';
import LoginPage from './pages/LoginPage';
import { clearSession, getToken, getUser } from './stores/useAuthStore';

const initialCreators = [
	{ id: 1, name: 'Minh Ái 🎀', handle: '@minhaireview', contact: '0327 637 213', followers: '12,7k', gmv: '1Mđ+', category: 'Làm đẹp', avatar: 'MA', tone: 'pink' },
	{ id: 2, name: 'Gia đình trái cây 🍋🍊', handle: '@giadinhtraicay1202', contact: '0866 073 103', followers: '3,1k', gmv: '—', category: 'Đời sống', avatar: 'GT', tone: 'red' },
	{ id: 3, name: 'Cô bé lười đi tạp 🧚', handle: '@changditap', contact: '—', followers: '126', gmv: '—', category: 'Chưa phân loại', avatar: 'C', tone: 'green' },
];

const previewConversations = [{ _id: 'preview-1', contactName: 'Gia đình trái cây 🍋🍊', contactHandle: '@giadinhtraicay1202', lastMessage: 'Mình đã nhận được thông tin rồi ạ', lastMessageAt: new Date().toISOString(), unreadCount: 2 }];

const navItems = [
	['▱', 'Hộp thư'],
	['▣', 'Danh bạ KOC'],
];

function App() {
	const [authenticated, setAuthenticated] = useState(Boolean(getToken()));
	const [currentUser, setCurrentUser] = useState(getUser());
	const [creators, setCreators] = useState(initialCreators);
	const [conversations, setConversations] = useState(previewConversations);
	const [apiReady, setApiReady] = useState(false);
	const [isModalOpen, setModalOpen] = useState(false);
	const [selectedNav, setSelectedNav] = useState('Danh bạ KOC');
	const [query, setQuery] = useState('');
	const [category, setCategory] = useState('Tất cả');
	const [actionError, setActionError] = useState('');

	useEffect(() => {
		if (!authenticated) return;
		Promise.all([api.get('/kocs'), api.get('/inbox/conversations')]).then(([kocResponse, inboxResponse]) => {
			setCreators(kocResponse.data.data.items.map((item) => ({ ...item, id: item._id, name: item.displayName, handle: `@${item.tiktokId}`, contact: item.phone || '—', followers: formatNumber(item.followers), gmv: item.gmv ? `${formatNumber(item.gmv)}đ+` : '—', avatar: item.displayName.slice(0, 2).toUpperCase(), tone: 'green' })));
			setConversations(inboxResponse.data.data);
			setApiReady(true);
		}).catch(() => setApiReady(false));
	}, [authenticated]);

	const filteredCreators = useMemo(() => creators.filter((creator) => {
		const matchesQuery = `${creator.name} ${creator.handle} ${creator.contact}`.toLowerCase().includes(query.toLowerCase());
		const matchesCategory = category === 'Tất cả' || creator.category === category;
		return matchesQuery && matchesCategory;
	}), [category, creators, query]);
	if (!authenticated) return <LoginPage onLogin={(user) => { setCurrentUser(user); setAuthenticated(true); }} />;

	function addCreator(creator) {
		const payload = { tiktokId: creator.handle, displayName: creator.name, followers: 0, category: 'Chưa phân loại', email: creator.email, phone: creator.contact === '—' ? '' : creator.contact, recipient: creator.recipient, shippingAddress: creator.shippingAddress, castFee: creator.castFee };
		api.post('/kocs', payload).then(({ data }) => {
			const item = data.data;
			setCreators((current) => [...current, { ...creator, ...item, id: item._id, name: item.displayName, handle: `@${item.tiktokId}`, contact: item.phone || '—', avatar: item.displayName.slice(0, 2).toUpperCase() }]);
			setModalOpen(false);
		}).catch((error) => setActionError(error.response?.data?.message || 'Không thể thêm KOC. Vui lòng thử lại.'));
	}

	function deleteCreator(creator) {
		if (!window.confirm(`Xóa ${creator.name} khỏi danh bạ?`)) return;
		api.delete(`/kocs/${creator.id}`).then(() => setCreators((current) => current.filter((item) => item.id !== creator.id))).catch((error) => setActionError(error.response?.data?.message || 'Không thể xóa KOC.'));
	}

	function emailCreator(creator) {
		if (!creator.email) { setActionError('KOC này chưa có email.'); return; }
		api.post(`/kocs/${creator.id}/email`, {}).then(() => setActionError(`Đã gửi email tới ${creator.email}.`)).catch((error) => setActionError(error.response?.data?.message || 'Không thể gửi email.'));
	}

	return (
		<div className="app-shell">
			<aside className="sidebar">
				<div className="brand"><span className="brand-mark">N</span><span>Tiktoday</span><small>BOOKING</small></div>
				<button className="search-button" onClick={() => setModalOpen(true)}><span>⌕</span> Tìm kiếm KOC <b>↗</b></button>
				<nav>
					{navItems.map(([icon, label]) => (
						<button className={selectedNav === label ? 'nav-item active' : 'nav-item'} key={label} onClick={() => setSelectedNav(label)}>
							<span>{icon}</span>{label}
						</button>
					))}
				</nav>
				<button className="account" onClick={() => { clearSession(); setAuthenticated(false); setCurrentUser(null); }}><div className="account-avatar">{currentUser?.name?.slice(0, 1) || 'U'}</div><div><strong>{currentUser?.name || 'Workspace user'}</strong><small>{currentUser?.email || ''}</small></div><span>↪</span></button>
			</aside>

			<main className="main-content">
				<header className="topbar"><div className="crumb">{selectedNav} <span>/</span> <b>{selectedNav === 'Hộp thư' ? 'Tất cả hội thoại' : 'Tất cả KOC'}</b></div><div className="top-actions"><button className="ghost-button">⌕</button><button className="ghost-button">?</button><div className="notification">2</div><div className="mini-avatar">Đ</div></div></header>
				<section className="content">
					{selectedNav === 'Hộp thư' ? <InboxView conversations={conversations} setConversations={setConversations} apiReady={apiReady} /> : <><div className="page-heading"><div><p className="eyebrow">DANH BẠ NHÀ SÁNG TẠO</p><h1>Tất cả KOC <span>{creators.length} KOC</span></h1></div><button className="primary-button" onClick={() => { setActionError(''); setModalOpen(true); }}><span>♙</span> Thêm KOC <b>⌄</b></button></div>
					{actionError && <div className="status error-status">{actionError}</div>}
					<div className="stats-row"><div><strong>{creators.length}</strong><span>Tổng KOC</span></div><div><strong>3</strong><span>Đã phân loại</span></div><div><strong>0</strong><span>Đang hợp tác</span></div><div className="stats-note">Cập nhật lần cuối <b>vừa xong</b></div></div>
					<div className="toolbar"><select value={category} onChange={(event) => setCategory(event.target.value)}><option value="Tất cả">Tất cả thư mục</option><option value="Làm đẹp">Làm đẹp</option><option value="Đời sống">Đời sống</option><option value="Chưa phân loại">Chưa phân loại</option></select><select><option>Follower</option></select><select><option>GMV</option></select><div className="table-search"><span>⌕</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Tìm tên, kênh, email, SĐT..." /></div></div>
					<div className="table-wrap"><table><thead><tr><th><input type="checkbox" /></th><th>#</th><th>NHÀ SÁNG TẠO</th><th>LIÊN HỆ</th><th>THƯ MỤC</th><th>FOLLOWER</th><th>GMV</th><th>PHÍ CAST</th><th></th></tr></thead><tbody>{filteredCreators.map((creator, index) => <tr key={creator.id}><td><input type="checkbox" /></td><td className="muted">{index + 1}</td><td><div className="creator"><div className={`creator-avatar ${creator.tone}`}>{creator.avatar}</div><div><strong>{creator.name}</strong><small>{creator.handle} <i>♪</i><i>▣</i></small></div></div></td><td>{creator.contact}</td><td><button className="add-category">＋ <span>{creator.category === 'Chưa phân loại' ? 'Thêm' : creator.category}</span></button></td><td>{creator.followers}</td><td>{creator.gmv}</td><td>{creator.castFee ? `${formatNumber(creator.castFee)}đ` : '—'}</td><td className="row-actions"><button className="icon-button" title="Gửi email" onClick={() => emailCreator(creator)}>✉</button><button className="icon-button danger" title="Xóa KOC" onClick={() => deleteCreator(creator)}>×</button></td></tr>)}</tbody></table>{filteredCreators.length === 0 && <div className="empty-state">Không tìm thấy KOC phù hợp.</div>}<footer className="table-footer">Hiển thị <b>{filteredCreators.length ? 1 : 0}-{filteredCreators.length}</b> / {filteredCreators.length} <select><option>20 / trang</option></select><div className="pagination"><button>‹</button><button className="current">1</button><button>›</button></div></footer></div></>}
				</section>
			</main>
			{isModalOpen && <AddCreatorModal onClose={() => setModalOpen(false)} onAdd={addCreator} />}
		</div>
	);
}

function AddCreatorModal({ onClose, onAdd }) {
	const [tiktokId, setTiktokId] = useState('');
	const [creator, setCreator] = useState(null);
	const [form, setForm] = useState({ email: '', phone: '', recipient: '', address: '', fee: '' });
	const [status, setStatus] = useState('');

	function findTikTok() {
		const normalized = tiktokId.trim().replace(/^@/, '').toLowerCase();
		const found = initialCreators.find((item) => item.handle.slice(1).toLowerCase() === normalized);
		if (found) { setCreator(found); setStatus(''); } else if (normalized) { setCreator({ name: `KOC ${tiktokId.trim()}`, handle: `@${normalized}`, followers: 'Đang cập nhật', gmv: '—', category: 'Chưa phân loại', avatar: normalized.slice(0, 1).toUpperCase(), tone: 'green' }); setStatus('Đã tìm thấy kênh TikTok. Bạn có thể bổ sung thông tin liên hệ.'); } else setStatus('Vui lòng nhập TikTok ID.');
	}

	function updateField(event) { setForm({ ...form, [event.target.name]: event.target.value }); }
	function submit(event) { event.preventDefault(); if (!creator) { setStatus('Hãy tìm TikTok ID trước khi thêm KOC.'); return; } onAdd({ ...creator, email: form.email, contact: form.phone || '—', recipient: form.recipient, shippingAddress: form.address, castFee: Number(form.fee) || 0, category: 'Chưa phân loại' }); }

	return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><form className="modal" onSubmit={submit}><div className="modal-header"><div><h2>Thêm KOC vào danh bạ</h2><p>Tìm theo TikTok ID rồi chọn kênh để lấy follower & GMV</p></div><button type="button" className="close-button" onClick={onClose}>×</button></div><div className="modal-body"><label>Nhà sáng tạo</label><div className="lookup"><input value={tiktokId} onChange={(event) => setTiktokId(event.target.value)} placeholder="TikTok ID *" /><button type="button" onClick={findTikTok}>⌕ &nbsp;Tìm kênh</button></div><small className="hint">Nhập ID TikTok, ví dụ: @giadinhtraicay1202</small>{status && <div className="status">{status}</div>}{creator && <div className="found-creator"><div className={`creator-avatar ${creator.tone}`}>{creator.avatar}</div><div><strong>{creator.name}</strong><small>{creator.handle} · {creator.followers} follower</small></div><span>✓</span></div>}<label>Thông tin liên hệ</label>{[['email', 'Email'], ['phone', 'Số điện thoại'], ['recipient', 'Người nhận mẫu']].map(([name, placeholder]) => <input key={name} name={name} value={form[name]} onChange={updateField} placeholder={placeholder} />)}<input name="address" value={form.address} onChange={updateField} placeholder="Địa chỉ nhận mẫu" /><div className="fee-input"><input name="fee" value={form.fee} onChange={updateField} placeholder="Phí cast" /><span>đ</span></div><div className="fee-chips"><button type="button">100.000đ</button><button type="button">200.000đ</button><button type="button">500.000đ</button><button type="button">1.000.000đ</button></div><label>Thư mục</label><select><option>Chưa phân loại</option><option>Làm đẹp</option><option>Đời sống</option></select></div><div className="modal-footer"><button type="button" className="secondary-button" onClick={onClose}>Hủy</button><button className="primary-button" type="submit" disabled={!creator}>♙ &nbsp;Thêm KOC</button></div></form></div>;
}

function formatNumber(value) { return new Intl.NumberFormat('vi-VN', { notation: 'compact', maximumFractionDigits: 1 }).format(value || 0); }

function InboxView({ conversations, setConversations, apiReady }) {
	const [selectedConversation, setSelectedConversation] = useState(conversations[0]);
	const [messages, setMessages] = useState([]);
	const [draft, setDraft] = useState('');
	const [loading, setLoading] = useState(false);
	const [sending, setSending] = useState(false);
	const [error, setError] = useState('');
	const [search, setSearch] = useState('');
	const visibleConversations = conversations.filter((conversation) => `${conversation.contactName} ${conversation.contactHandle} ${conversation.contactPhone || ''}`.toLowerCase().includes(search.toLowerCase()));

	useEffect(() => {
		if (!selectedConversation && visibleConversations[0]) setSelectedConversation(visibleConversations[0]);
		if (selectedConversation && !conversations.some((item) => item._id === selectedConversation._id)) setSelectedConversation(visibleConversations[0]);
	}, [conversations, selectedConversation]);

	useEffect(() => {
		if (!selectedConversation || selectedConversation._id.startsWith('preview-')) return;
		setLoading(true);
		setError('');
		api.get(`/inbox/conversations/${selectedConversation._id}/messages`).then(({ data }) => setMessages(data.data)).catch(() => setError('Không thể tải lịch sử tin nhắn.')).finally(() => setLoading(false));
	}, [selectedConversation]);

	function sendMessage(event) {
		event.preventDefault();
		const text = draft.trim();
		if (!text || !selectedConversation || selectedConversation._id.startsWith('preview-')) return;
		setSending(true);
		api.post(`/inbox/conversations/${selectedConversation._id}/messages`, { text }).then(({ data }) => {
			setMessages((current) => [...current, data.data]);
			setConversations((current) => current.map((item) => item._id === selectedConversation._id ? { ...item, lastMessage: text, lastMessageAt: data.data.sentAt } : item));
			setSelectedConversation((current) => ({ ...current, lastMessage: text, lastMessageAt: data.data.sentAt }));
			setDraft('');
		}).catch(() => setError('Không thể gửi tin nhắn.')).finally(() => setSending(false));
	}

	return <div className="inbox-page"><div className="page-heading"><div><p className="eyebrow">TIN NHẮN ĐỐI TÁC</p><h1>Hộp thư <span>{conversations.length} hội thoại</span></h1></div><div className="api-badge">{apiReady ? '● Đã kết nối dữ liệu' : '○ Đang dùng bản xem trước'}</div></div><div className="inbox-layout"><div className="conversation-list"><div className="inbox-search"><span>⌕</span><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Tìm tên, TikTok ID, số điện thoại..." /></div><div className="inbox-tabs"><b>Tất cả</b><span>Chưa đọc</span></div>{visibleConversations.map((conversation) => <button className={selectedConversation?._id === conversation._id ? 'conversation active' : 'conversation'} key={conversation._id} onClick={() => setSelectedConversation(conversation)}><div className="creator-avatar red">{conversation.contactName.slice(0, 2)}</div><div><strong>{conversation.contactName}</strong><small>{conversation.contactHandle} · {conversation.lastMessage}</small></div><time>{new Date(conversation.lastMessageAt).toLocaleTimeString('vi-VN', { hour: '2-digit', minute: '2-digit' })}</time></button>)}{!visibleConversations.length && <div className="empty-state">Không tìm thấy hội thoại.</div>}</div><div className="chat-panel">{selectedConversation ? <><div className="chat-header"><div className="creator-avatar red">{selectedConversation.contactName.slice(0, 2)}</div><div><strong>{selectedConversation.contactName}</strong><small>{selectedConversation.contactHandle}{selectedConversation.contactPhone ? ` · ${selectedConversation.contactPhone}` : ''}</small></div><span className="channel-tag">{selectedConversation.channel === 'zalo' ? 'Zalo · kết nối sau' : 'Nội bộ'}</span></div><div className="chat-messages">{loading && <div className="empty-state">Đang tải tin nhắn...</div>}{error && <div className="status error-status">{error}</div>}{!loading && !messages.length && <div className="empty-state">Chưa có tin nhắn.</div>}{messages.map((message) => <div className={`message ${message.direction === 'inbound' ? 'incoming' : 'outgoing'}`} key={message._id}><span>{message.text}</span><small>{new Date(message.sentAt).toLocaleTimeString('vi-VN', { hour: '2-digit', minute: '2-digit' })} · {message.status}</small></div>)}</div><form className="chat-composer" onSubmit={sendMessage}><input value={draft} onChange={(event) => setDraft(event.target.value)} placeholder={selectedConversation._id.startsWith('preview-') ? 'Dữ liệu xem trước chưa thể nhắn' : 'Nhắn tin cho KOC...'} disabled={sending || selectedConversation._id.startsWith('preview-')} /><button className="primary-button" disabled={sending || !draft.trim() || selectedConversation._id.startsWith('preview-')}>{sending ? 'Đang gửi...' : 'Gửi'}</button></form></> : <div className="empty-state">Chọn một hội thoại để xem tin nhắn.</div>}</div></div></div>;
}

export default App;
