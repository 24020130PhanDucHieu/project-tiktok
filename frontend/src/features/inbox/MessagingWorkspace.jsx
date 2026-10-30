import { useEffect, useMemo, useState } from 'react';
import api from '../../lib/axios';
import { MESSAGE_TEMPLATES, previewMessagesMap } from '../../lib/constants';
import { formatNumber, stageBadgeText } from '../../lib/formatters';
import CollabStepModal from './CollabStepModal';
import InspectorPanel from './InspectorPanel';

export default function MessagingWorkspace({
	creators,
	conversations,
	setConversations,
	apiReady,
	currentUser,
	onUpdateKoc,
	onSyncChannel,
	onTrackShipment,
	showToast,
}) {
	const [selectedConv, setSelectedConv] = useState(conversations[0] || null);
	const [messages, setMessages] = useState([]);
	const [draft, setDraft] = useState('');
	const [search, setSearch] = useState('');
	const [tabFilter, setTabFilter] = useState('ALL');
	const [rightTab, setRightTab] = useState('Hợp tác'); // 'Hợp tác' | 'Kênh'
	const [zaloStatus, setZaloStatus] = useState({ connected: false });
	const [showQrModal, setShowQrModal] = useState(false);
	const [syncingChannel, setSyncingChannel] = useState(false);
	const [chatbotEnabled, setChatbotEnabled] = useState(true);
	const [showClearChatModal, setShowClearChatModal] = useState(false);
	const [zaloBannerDismissed, setZaloBannerDismissed] = useState(false);

	// Trạng thái mở rộng inline cho bước 4 ('video') & bước 5 ('result') và liên kết TikTok Shop
	const [expandedStepKey, setExpandedStepKey] = useState('video');
	const [tiktokShopLinked, setTiktokShopLinked] = useState(false);
	const [shopName, setShopName] = useState('Cooltech Official Store');
	const [showLinkShopModal, setShowLinkShopModal] = useState(false);

	// Bộ lọc thật cho tab Kênh
	const [channelFilterCart, setChannelFilterCart] = useState('ALL');
	const [channelFilterType, setChannelFilterType] = useState('ALL');
	const [channelSortOrder, setChannelSortOrder] = useState('NEWEST');

	// Step action modals / inline forms inside 'Hợp tác'
	const [activeStepModal, setActiveStepModal] = useState(null);

	// Find matching KOC record for the currently selected conversation
	const activeKoc = useMemo(() => {
		if (!selectedConv) return creators[0] || null;
		const cleanHandle = (selectedConv.contactHandle || '').replace(/^@/, '').toLowerCase();
		return (
			creators.find(
				(c) =>
					c.tiktokId?.toLowerCase() === cleanHandle ||
					c.name?.toLowerCase() === selectedConv.contactName?.toLowerCase(),
			) ||
			creators[0] ||
			null
		);
	}, [selectedConv, creators]);

	useEffect(() => {
		api.get('/zalo/status')
			.then(({ data }) => setZaloStatus(data.data || { connected: false }))
			.catch(() => {});
	}, []);

	useEffect(() => {
		if (!showQrModal) return undefined;
		const timer = window.setInterval(() => {
			api.get('/zalo/status')
				.then(({ data }) => {
					setZaloStatus(data.data || { connected: false });
					if (data.data?.connected) {
						setShowQrModal(false);
						showToast('Đã kết nối tài khoản Zalo thành công.');
					}
				})
				.catch(() => {});
		}, 2200);
		return () => window.clearInterval(timer);
	}, [showQrModal]);

	useEffect(() => {
		if (!selectedConv) return;
		if (String(selectedConv._id).startsWith('preview-') || String(selectedConv._id).startsWith('conv-')) {
			setMessages(previewMessagesMap[selectedConv._id] || previewMessagesMap['preview-1'] || []);
			return;
		}
		api.get(`/inbox/conversations/${selectedConv._id}/messages`)
			.then(({ data }) => setMessages(data.data || []))
			.catch(() => setMessages([]));
	}, [selectedConv]);

	useEffect(() => {
		if (rightTab === 'Kênh' && activeKoc && !activeKoc.channelSyncedAt) {
			setSyncingChannel(true);
			Promise.resolve(onSyncChannel?.(activeKoc)).finally(() => {
				setTimeout(() => setSyncingChannel(false), 600);
			});
		}
	}, [rightTab, activeKoc?.id]);

	const filteredConversations = useMemo(() => {
		return conversations.filter((conv) => {
			if (tabFilter === 'UNREAD' && !conv.unreadCount) return false;
			if (search.trim()) {
				const q = search.toLowerCase();
				const text = `${conv.contactName} ${conv.contactHandle} ${conv.contactPhone || ''}`.toLowerCase();
				if (!text.includes(q)) return false;
			}
			return true;
		});
	}, [conversations, tabFilter, search]);

	function handleSendMessage(e) {
		e.preventDefault();
		const text = draft.trim();
		if (!text || !selectedConv) return;

		const newMsg = {
			_id: `msg-${Date.now()}`,
			direction: 'outbound',
			text,
			sentAt: new Date().toISOString(),
			status: 'Đã nhận',
		};

		if (!apiReady || String(selectedConv._id).startsWith('preview-') || String(selectedConv._id).startsWith('conv-')) {
			setMessages((prev) => [...prev, newMsg]);
			setConversations((prev) =>
				prev.map((c) =>
					c._id === selectedConv._id ? { ...c, lastMessage: text, lastMessageAt: newMsg.sentAt } : c,
				),
			);
			setDraft('');
			return;
		}

		api.post(`/inbox/conversations/${selectedConv._id}/messages`, { text })
			.then(({ data }) => {
				setMessages((prev) => [...prev, data.data]);
				setConversations((prev) =>
					prev.map((c) =>
						c._id === selectedConv._id ? { ...c, lastMessage: text, lastMessageAt: data.data.sentAt } : c,
					),
				);
				setDraft('');
			})
			.catch(() => {
				setMessages((prev) => [...prev, newMsg]);
				setDraft('');
			});
	}

	function handleStartZaloQr() {
		setShowQrModal(true);
		api.post('/zalo/qr/start')
			.then(({ data }) => setZaloStatus(data.data || { connected: false }))
			.catch(() => {});
	}

	function handleSelectTemplate(e) {
		const tplId = e.target.value;
		if (!tplId) return;
		const found = MESSAGE_TEMPLATES.find((t) => t.id === tplId);
		if (found) setDraft(found.build(activeKoc));
		e.target.value = '';
	}

	function handleConfirmClearChat() {
		if (!selectedConv) return;
		setShowClearChatModal(false);

		if (!apiReady || String(selectedConv._id).startsWith('preview-') || String(selectedConv._id).startsWith('conv-')) {
			setMessages([]);
			setConversations((prev) =>
				prev.map((c) =>
					c._id === selectedConv._id ? { ...c, lastMessage: 'Đã làm mới cuộc trò chuyện' } : c,
				),
			);
			showToast(`Đã xóa lịch sử đoạn chat với ${selectedConv.contactName}.`);
			return;
		}

		api.delete(`/inbox/conversations/${selectedConv._id}/messages`)
			.then(({ data }) => {
				setMessages([]);
				setConversations((prev) =>
					prev.map((c) =>
						c._id === selectedConv._id ? { ...c, lastMessage: 'Đã làm mới cuộc trò chuyện' } : c,
					),
				);
				showToast(data.message || `Đã xóa lịch sử đoạn chat với ${selectedConv.contactName}.`);
			})
			.catch(() => showToast('Không thể xóa đoạn chat.', 'error'));
	}

	function handleDeleteSingleMessage(msgId) {
		if (!selectedConv || !msgId) return;
		const nextList = messages.filter((m) => m._id !== msgId);
		setMessages(nextList);
		const lastMsgText = nextList.length ? nextList[nextList.length - 1].text : 'Chưa có tin nhắn';
		setConversations((prev) =>
			prev.map((c) => (c._id === selectedConv._id ? { ...c, lastMessage: lastMsgText } : c)),
		);

		if (apiReady && !String(selectedConv._id).startsWith('preview-') && !String(selectedConv._id).startsWith('conv-')) {
			api.delete(`/inbox/conversations/${selectedConv._id}/messages/${msgId}`).catch(() => {});
		}
	}

	function handleSimulateKocInbound(kocText) {
		if (!selectedConv || !kocText) return;
		const nowIso = new Date().toISOString();
		const inboundMsg = {
			_id: `in-${Date.now()}`,
			direction: 'inbound',
			text: kocText,
			sentAt: nowIso,
			status: 'delivered',
		};

		if (!apiReady || String(selectedConv._id).startsWith('preview-') || String(selectedConv._id).startsWith('conv-')) {
			setMessages((prev) => [...prev, inboundMsg]);

			api.post('/inbox/chatbot/parse', { text: kocText, koc: activeKoc || {} })
				.then(({ data }) => {
					const { parsed, botReply } = data.data || {};
					if (parsed && activeKoc) {
						if ((parsed.intent === 'SUBMITTED_VIDEO' || parsed.intent === 'VIDEO_POSTED') && parsed.videoUrl) {
							onUpdateKoc(
								activeKoc,
								{ action: 'post_video', videoUrl: parsed.videoUrl, stage: 'VIDEO_POSTED', collabStep: 4 },
								'Chatbot đã tự động nhận diện link video TikTok của KOC!',
							);
						} else if (parsed.intent === 'PROVIDED_ADDRESS' || parsed.shippingAddress) {
							onUpdateKoc(
								activeKoc,
								{
									action: 'agree',
									phone: parsed.phone || activeKoc.phone,
									shippingAddress: parsed.shippingAddress || kocText,
									recipient: parsed.recipient || activeKoc.name,
									stage: 'AGREED',
									collabStep: 2,
								},
								'Chatbot đã tự động trích xuất SĐT + Địa chỉ nhận mẫu và chuyển sang bước Duyệt & gửi mẫu!',
							);
						} else if (parsed.intent === 'NEGOTIATE_CAST' || parsed.intent === 'ASK_CAST') {
							onUpdateKoc(
								activeKoc,
								{
									action: 'set_stage',
									stage: 'AWAITING_APPROVAL',
									suggestedDeclineCode: parsed.suggestedDeclineCode || 'R02',
									collabStep: 0,
								},
								'KOC đòi giá cast ngoài bảng -> Chatbot đã chuyển sang chờ Chủ shop duyệt!',
							);
						} else if (parsed.intent === 'CONFIRMED_RECEIVED' || parsed.intent === 'RECEIVED_CONFIRMED') {
							onUpdateKoc(
								activeKoc,
								{ action: 'mark_delivered', stage: 'DELIVERED', collabStep: 3 },
								'KOC xác nhận đã nhận hàng tự vận chuyển -> Đã ghi nhận DELIVERED!',
							);
						} else if (parsed.intent === 'NOT_FIT_RETURN' || parsed.intent === 'NOT_FIT') {
							onUpdateKoc(
								activeKoc,
								{
									action: 'video_reply',
									kocVideoReply: 'NOT_FIT',
									postReceiveCode: parsed.postReceiveCode || 'P01',
									stage: 'NOT_FIT',
									collabStep: 3,
								},
								'KOC báo không hợp sản phẩm -> Chatbot đã hướng dẫn gửi hoàn mẫu (NOT_FIT)!',
							);
						} else if (parsed.intent === 'AGREE_NO_ADDRESS' || parsed.intent === 'AGREED') {
							onUpdateKoc(
								activeKoc,
								{ action: 'mark_replied', stage: 'REPLIED', collabStep: 1 },
								'Chatbot đã ghi nhận KOC đồng ý hợp tác và tự động xin địa chỉ!',
							);
						}
					}

					if (chatbotEnabled && botReply) {
						const botMsg = {
							_id: `bot-${Date.now()}`,
							direction: 'outbound',
							text: botReply,
							sentAt: new Date().toISOString(),
							status: 'Chatbot đã trả lời',
						};
						setMessages((prev) => [...prev, botMsg]);
						setConversations((prev) =>
							prev.map((c) =>
								c._id === selectedConv._id ? { ...c, lastMessage: botReply, lastMessageAt: botMsg.sentAt } : c,
							),
						);
					}
				})
				.catch(() => {});
			return;
		}

		api.post(`/inbox/conversations/${selectedConv._id}/inbound`, {
			text: kocText,
			autoReply: chatbotEnabled,
		})
			.then(({ data }) => {
				const res = data.data;
				const added = [res.inboundMessage];
				if (res.botMessage) added.push(res.botMessage);
				setMessages((prev) => [...prev, ...added]);
				if (res.koc && activeKoc) {
					onUpdateKoc(
						activeKoc,
						{
							...res.koc,
							action: 'set_stage',
							stage: res.koc.stage,
							collabStep: res.koc.collabStep,
						},
						`Chatbot đã phân loại ý định (${res.parsed?.intent}) và tự động cập nhật hồ sơ KOC!`,
					);
				}
			})
			.catch(() => {});
	}

	const currentStepIndex = activeKoc?.collabStep ?? 0;

	return (
		<div className="messaging-3col">
			{/* CỘT 1: DANH SÁCH HỘI THOẠI */}
			<aside className="msg-col-list">
				<div
					className="zalo-account-switcher clickable"
					onClick={handleStartZaloQr}
					title="Bấm để kết nối lại tài khoản Zalo bằng mã QR"
				>
					<div className="zalo-badge-icon">Zalo</div>
					<div className="zalo-acc-meta">
						<strong>{currentUser?.name || 'Bùi Văn Việt'}</strong>
						<small>{zaloStatus?.connected ? 'Zalo · đã kết nối' : 'Zalo · mất kết nối (Bấm kết nối)'}</small>
					</div>
					<span className="caret">⌄</span>
				</div>

				<div className="msg-search-box">
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
						<circle cx="11" cy="11" r="7" />
						<path d="m20 20-3.5-3.5" strokeLinecap="round" />
					</svg>
					<input
						value={search}
						onChange={(e) => setSearch(e.target.value)}
						placeholder="Tìm KOC..."
					/>
				</div>

				<div className="msg-filter-tabs">
					<button
						className={tabFilter === 'ALL' ? 'active' : ''}
						onClick={() => setTabFilter('ALL')}
					>
						Tất cả
					</button>
					<button
						className={tabFilter === 'UNREAD' ? 'active' : ''}
						onClick={() => setTabFilter('UNREAD')}
					>
						Chưa đọc
					</button>
					<span className="msg-sort-label">Phân loại ⌄</span>
				</div>

				<div className="msg-conv-scroll">
					{filteredConversations.map((conv) => {
						const matchedKoc = creators.find(
							(c) => `@${c.tiktokId}` === conv.contactHandle || c.name === conv.contactName,
						);
						const badgeLabel = stageBadgeText(matchedKoc?.stage);
						const dateStr = new Date(conv.lastMessageAt).toLocaleDateString('vi-VN', {
							day: '2-digit',
							month: '2-digit',
						});
						return (
							<button
								key={conv._id}
								className={selectedConv?._id === conv._id ? 'msg-conv-item active' : 'msg-conv-item'}
								onClick={() => setSelectedConv(conv)}
							>
								<div className="avatar-circle">{conv.contactName.slice(0, 2).toUpperCase()}</div>
								<div className="msg-conv-body">
									<div className="msg-conv-top">
										<strong>{conv.contactName}</strong>
										<time>{dateStr}</time>
									</div>
									<div className="msg-conv-bottom">
										<p>{conv.lastMessage}</p>
										<span className="conv-task-chip">{badgeLabel}</span>
									</div>
								</div>
							</button>
						);
					})}
				</div>
			</aside>

			{/* CỘT 2: KHUNG CHAT GIỮA */}
			<section className="msg-col-chat">
				{selectedConv ? (
					<>
						<header className="msg-chat-header">
							<div className="avatar-circle">{selectedConv.contactName.slice(0, 2).toUpperCase()}</div>
							<div>
								<strong>{selectedConv.contactName}</strong>
								<small>
									{selectedConv.contactHandle} · {formatNumber(activeKoc?.followers || 4000)} follower
								</small>
							</div>
							<button
								type="button"
								className={chatbotEnabled ? 'chatbot-toggle-pill on' : 'chatbot-toggle-pill'}
								onClick={() => setChatbotEnabled((v) => !v)}
								title="Khi bật: Chatbot tự động phân loại ý định tin nhắn KOC, tự bóc tách SĐT + Địa chỉ nhận mẫu và tự trả lời"
							>
								<span className="bot-dot" />
								{chatbotEnabled ? 'Chatbot tự động: BẬT' : 'Chatbot tự động: TẮT'}
							</button>
							<button
								type="button"
								className="btn-clear-chat"
								onClick={() => setShowClearChatModal(true)}
								disabled={messages.length === 0}
								title="Xóa toàn bộ lịch sử tin nhắn trong cuộc trò chuyện này"
							>
								Xóa đoạn chat
							</button>
						</header>

						{!zaloStatus?.connected && !zaloBannerDismissed && (
							<div className="zalo-disconnect-banner top">
								<div className="zalo-badge-icon">Zalo</div>
								<div className="zalo-banner-text">
									<strong>Tài khoản Zalo {currentUser?.name || 'Bùi Văn Việt'} đang mất kết nối</strong>
									<small>Bấm Kết nối lại để quét QR Zalo gửi tin thật ra ngoài.</small>
								</div>
								<button type="button" className="btn-reconnect-zalo" onClick={handleStartZaloQr}>
									Kết nối lại
								</button>
								<button
									type="button"
									className="btn-dismiss-banner"
									onClick={() => setZaloBannerDismissed(true)}
									title="Ẩn thông báo này"
								>
									Ẩn
								</button>
							</div>
						)}

						<div className="msg-chat-stream">
							{messages.length > 0 ? (
								<>
									<div className="chat-date-divider">
										<span>
											{new Date(selectedConv.lastMessageAt).toLocaleDateString('vi-VN', {
												day: '2-digit',
												month: '2-digit',
											})}
										</span>
									</div>

									{messages.map((msg) => (
										<div
											key={msg._id}
											className={`chat-bubble-wrap ${msg.direction === 'inbound' ? 'inbound' : 'outbound'}`}
										>
											<div className="chat-bubble-row">
												<div className="chat-bubble">
													<p>{msg.text}</p>
													<time>
														{new Date(msg.sentAt).toLocaleTimeString('vi-VN', {
															hour: '2-digit',
															minute: '2-digit',
														})}
													</time>
												</div>
												<button
													type="button"
													className="btn-del-msg"
													onClick={() => handleDeleteSingleMessage(msg._id)}
													title="Thu hồi / Xóa tin nhắn này"
												>
													Xóa
												</button>
											</div>
											{msg.direction === 'outbound' && (
												<span className="delivery-receipt">✓ {msg.status || 'Đã nhận'}</span>
											)}
										</div>
									))}
								</>
							) : (
								<div className="chat-empty-stream">
									Chưa có tin nhắn nào trong cuộc trò chuyện này. Bạn có thể chọn mẫu lời mời bên dưới hoặc test KOC nhắn đến.
								</div>
							)}
						</div>

						<footer className="msg-chat-footer">
							<div className="composer-meta-bar">
								<span>
									Từ <b>{currentUser?.name || 'Bùi Văn Việt'}</b> →{' '}
									<b>{zaloStatus?.connected ? 'Zalo · đã kết nối' : 'Zalo · mất kết nối'}</b>
								</span>
								<div className="composer-tools">
									<select
										defaultValue=""
										onChange={(e) => {
											if (!e.target.value) return;
											handleSimulateKocInbound(e.target.value);
											e.target.value = '';
										}}
									>
										<option value="" disabled>
											Test KOC nhắn đến (Chatbot)...
										</option>
										<option value="Ok shop, mình đồng ý nhận mẫu hợp tác nhé!">
											1. KOC nhắn: "Ok shop, mình đồng ý nhận mẫu nhé!" (#7)
										</option>
										<option value="Địa chỉ nhận mẫu: Ký túc xá Ngoại ngữ VNU, Cầu Giấy, Hà Nội - SĐT 0988123456">
											2. KOC nhắn Địa chỉ + SĐT (Chatbot tự thu địa chỉ #7)
										</option>
										<option value="Giá cast của mình là 1 triệu và tăng hoa hồng lên 20% mới nhận nhé">
											3. KOC đòi giá cast ngoài bảng (Chuyển chủ shop duyệt)
										</option>
										<option value="Mình nhận được hàng rồi shop ơi">
											4. KOC xác nhận "rồi" sau 3 ngày tự vận chuyển (#9)
										</option>
										<option value="Mình gửi shop link video mới đăng nhé: https://www.tiktok.com/@trlyng.review/video/7619258382867811601">
											5. KOC gửi link Video TikTok (Đổi VIDEO_POSTED #13)
										</option>
										<option value="Sản phẩm này không hợp sản phẩm với da mình, cho mình xin gửi trả lại mẫu nhé">
											6. KOC báo "không hợp" (Chatbot hướng dẫn gửi lại #15)
										</option>
									</select>

									<select defaultValue="" onChange={handleSelectTemplate}>
										<option value="" disabled>
											Mẫu: chọn mẫu tin nhắn
										</option>
										{MESSAGE_TEMPLATES.map((t) => (
											<option key={t.id} value={t.id}>
												{t.label}
											</option>
										))}
									</select>
								</div>
							</div>

							<form className="composer-input-row" onSubmit={handleSendMessage}>
								<input
									value={draft}
									onChange={(e) => setDraft(e.target.value)}
									placeholder="Nhập tin nhắn hoặc chọn mẫu lời mời hợp tác..."
								/>
								<button type="submit" disabled={!draft.trim()}>
									Gửi
								</button>
							</form>
						</footer>
					</>
				) : (
					<div className="empty-state">Chọn một KOC để bắt đầu trò chuyện.</div>
				)}
			</section>

			{/* CỘT 3: PANEL PHẢI — 2 TAB: HỢP TÁC | KÊNH */}
			<InspectorPanel
				rightTab={rightTab}
				setRightTab={setRightTab}
				activeKoc={activeKoc}
				currentStepIndex={currentStepIndex}
				expandedStepKey={expandedStepKey}
				setExpandedStepKey={setExpandedStepKey}
				setActiveStepModal={setActiveStepModal}
				tiktokShopLinked={tiktokShopLinked}
				shopName={shopName}
				setShowLinkShopModal={setShowLinkShopModal}
				syncingChannel={syncingChannel}
				setSyncingChannel={setSyncingChannel}
				onSyncChannel={onSyncChannel}
				channelFilterCart={channelFilterCart}
				setChannelFilterCart={setChannelFilterCart}
				channelFilterType={channelFilterType}
				setChannelFilterType={setChannelFilterType}
				channelSortOrder={channelSortOrder}
				setChannelSortOrder={setChannelSortOrder}
			/>

			{/* MODAL CẬP NHẬT TỪNG BƯỚC TRONG TAB HỢP TÁC */}
			{activeStepModal && activeKoc && (
				<CollabStepModal
					stepKey={activeStepModal}
					koc={activeKoc}
					onClose={() => setActiveStepModal(null)}
					onSyncChannel={onSyncChannel}
					onTrackShipment={onTrackShipment}
					onSave={(payload, notice) => {
						onUpdateKoc(activeKoc, payload, notice);
						setActiveStepModal(null);
					}}
				/>
			)}

			{/* MODAL LIÊN KẾT CỬA HÀNG TIKTOK SHOP */}
			{showLinkShopModal && (
				<div
					className="modal-backdrop"
					onMouseDown={(e) => e.target === e.currentTarget && setShowLinkShopModal(false)}
				>
					<form
						className="tiktoday-modal"
						onSubmit={(e) => {
							e.preventDefault();
							setTiktokShopLinked(true);
							setShowLinkShopModal(false);
							showToast(`Đã liên kết cửa hàng TikTok Shop "${shopName}" thành công!`);
						}}
					>
						<div className="tm-header">
							<div>
								<h2>Liên kết cửa hàng TikTok Shop</h2>
								<p>Tự động đồng bộ bài đăng gắn giỏ hàng, doanh thu (GMV), hoa hồng và lãi/lỗ thật</p>
							</div>
							<button type="button" className="tm-close" onClick={() => setShowLinkShopModal(false)}>
								×
							</button>
						</div>
						<div className="tm-body">
							<label className="tm-section-label">Tên gian hàng TikTok Shop của bạn</label>
							<input
								value={shopName}
								onChange={(e) => setShopName(e.target.value)}
								placeholder="VD: Cooltech Official Store"
								required
							/>
							<label className="tm-section-label">Mã cửa hàng (Shop Code / Seller ID)</label>
							<input defaultValue="VNLC88294102" placeholder="Nhập mã cửa hàng TikTok Shop" />
						</div>
						<div className="tm-footer">
							<button type="button" className="tm-btn-cancel" onClick={() => setShowLinkShopModal(false)}>
								Huỷ
							</button>
							<button type="submit" className="tm-btn-submit">
								Xác nhận liên kết
							</button>
						</div>
					</form>
				</div>
			)}

			{/* MODAL QUÉT QR ZALO */}
			{showQrModal && (
				<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && setShowQrModal(false)}>
					<div className="tiktoday-modal qr-modal-box">
						<div className="tm-header">
							<div>
								<h2>Kết nối tài khoản Zalo bằng mã QR</h2>
								<p>Mở ứng dụng Zalo trên điện thoại và quét mã QR bên dưới</p>
							</div>
							<button type="button" className="tm-close" onClick={() => setShowQrModal(false)}>
								×
							</button>
						</div>
						<div className="tm-body qr-center">
							{zaloStatus?.qrImage ? (
								<img src={zaloStatus.qrImage} alt="Mã QR Zalo" className="qr-image" />
							) : (
								<div className="qr-loading">Đang khởi tạo mã QR từ máy chủ Zalo...</div>
							)}
						</div>
					</div>
				</div>
			)}

			{/* MODAL XÁC NHẬN XÓA ĐOẠN CHAT TRONG WEB */}
			{showClearChatModal && selectedConv && (
				<div
					className="modal-backdrop"
					onMouseDown={(e) => e.target === e.currentTarget && setShowClearChatModal(false)}
				>
					<div className="tiktoday-modal confirm-web-modal">
						<div className="confirm-web-body">
							<div className="confirm-web-icon">
								<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1">
									<path d="M3 6h18" strokeLinecap="round" />
									<path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" strokeLinecap="round" />
									<path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" strokeLinecap="round" />
								</svg>
							</div>
							<div className="confirm-web-text">
								<h3>Xóa lịch sử đoạn chat?</h3>
								<p>
									Bạn có chắc muốn xóa toàn bộ tin nhắn trong cuộc trò chuyện với <b>{selectedConv.contactName}</b> để làm mới phiên trao đổi không?
								</p>
							</div>
						</div>
						<div className="tm-footer">
							<button
								type="button"
								className="tm-btn-cancel"
								onClick={() => setShowClearChatModal(false)}
							>
								Huỷ bỏ
							</button>
							<button
								type="button"
								className="tm-btn-danger"
								onClick={handleConfirmClearChat}
							>
								Xác nhận xóa
							</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
}
