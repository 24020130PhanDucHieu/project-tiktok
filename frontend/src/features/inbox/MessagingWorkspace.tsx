import { type Dispatch, type SetStateAction, useEffect, useMemo, useState } from 'react';
import api from '../../lib/axios';
import { previewMessagesMap } from '../../lib/constants';
import { formatNumber, stageBadgeText } from '../../lib/formatters';
import type { ChatMessage, CollabStepKey, ConversationItem, Creator, InboxLabel, UserProfile, ZaloStatus, ChatChannel, GmailStatus } from '../../types/koc';
import CollabStepModal from './CollabStepModal';
import CollabPanel from './CollabPanel';
import Composer, { messageStatusLabel } from './Composer';
import ConversationFilterMenu, { TASK_OPTIONS } from './ConversationFilterMenu';
import InspectorPanel from './InspectorPanel';
import Select from '../../components/Select';

interface MessagingWorkspaceProps {
	creators: Creator[];
	conversations: ConversationItem[];
	setConversations: Dispatch<SetStateAction<ConversationItem[]>>;
	apiReady: boolean;
	currentUser: UserProfile | null;
	onUpdateKoc: (creator: Creator, payload: Record<string, any>, noticeText: string) => void;
	onSyncChannel: (creator: Creator | null, opts?: { autoDetectCollabVideo?: boolean }) => void;
	onTrackShipment: (creator: Creator | null, customTrackingCode?: string, customPhoneLast4?: string) => void;
	showToast: (msg: string, type?: 'success' | 'error') => void;
}

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
}: MessagingWorkspaceProps) {
	const [selectedConv, setSelectedConv] = useState<ConversationItem | null>(null);
	const [labels, setLabels] = useState<InboxLabel[]>([]);
	const [taskFilter, setTaskFilter] = useState<string[]>([]);
	const [labelFilter, setLabelFilter] = useState<string[]>([]);
	const [showHidden, setShowHidden] = useState(false);
	const [hiddenConvs, setHiddenConvs] = useState<ConversationItem[]>([]);
	const [rowMenuId, setRowMenuId] = useState<string | null>(null);
	const [channel, setChannel] = useState<ChatChannel>('zalo');
	const [gmail, setGmail] = useState<GmailStatus>({ configured: false, connected: false, broken: false, email: '' });
	const [messages, setMessages] = useState<ChatMessage[]>([]);
	const [draft, setDraft] = useState('');
	const [search, setSearch] = useState('');
	const [tabFilter, setTabFilter] = useState('ALL');
	const [rightTab, setRightTab] = useState('Hợp tác'); // 'Hợp tác' | 'Kênh'
	const [zaloStatus, setZaloStatus] = useState<ZaloStatus>({ connected: false });
	const [qrRequestError, setQrRequestError] = useState<string>('');
	const [showQrModal, setShowQrModal] = useState(false);
	const [syncingChannel, setSyncingChannel] = useState(false);
	const [chatbotEnabled, setChatbotEnabled] = useState(true);
	const [showClearChatModal, setShowClearChatModal] = useState(false);
	const [zaloBannerDismissed, setZaloBannerDismissed] = useState(false);

	// Trạng thái mở rộng inline cho bước 4 ('video') & bước 5 ('result') và liên kết TikTok Shop
	const [expandedStepKey, setExpandedStepKey] = useState<string | null>('video');
	const [tiktokShopLinked, setTiktokShopLinked] = useState(false);
	const [shopName, setShopName] = useState('Cooltech Official Store');
	const [showLinkShopModal, setShowLinkShopModal] = useState(false);

	// Step action modals / inline forms inside 'Hợp tác'
	const [activeStepModal, setActiveStepModal] = useState<CollabStepKey | null>(null);

	// Find matching KOC record for the currently selected conversation
	const activeKoc = useMemo<Creator | null>(() => {
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
		api.get(`/inbox/conversations/${selectedConv._id}/messages`, { params: { channel } })
			.then(({ data }) => setMessages(data.data || []))
			.catch(() => setMessages([]));
	}, [selectedConv?._id, channel]);

	useEffect(() => {
		setChannel('zalo');
	}, [selectedConv?._id]);

	useEffect(() => {
		if (!apiReady) return;
		api.get('/gmail/status')
			.then(({ data }) => setGmail(data.data))
			.catch(() => {});
		const flag = new URLSearchParams(window.location.search).get('gmail');
		if (flag) {
			showToast(flag === 'connected' ? 'Đã kết nối Gmail.' : 'Kết nối Gmail không thành công.', flag === 'connected' ? 'success' : 'error');
			window.history.replaceState({}, '', window.location.pathname);
		}
	}, [apiReady]);

	// Pull email replies in while an email thread is open.
	useEffect(() => {
		if (channel !== 'email' || !selectedConv || !apiReady) return undefined;
		const timer = window.setInterval(() => {
			api.get(`/inbox/conversations/${selectedConv._id}/messages`, { params: { channel } })
				.then(({ data }) => setMessages((prev) => (prev.some((m) => m._id.startsWith('tmp-')) ? prev : data.data || prev)))
				.catch(() => {});
		}, 30000);
		return () => window.clearInterval(timer);
	}, [channel, selectedConv?._id, apiReady]);

	function handleConnectGmail() {
		api.get('/gmail/connect')
			.then(({ data }) => {
				window.location.href = data.data.url;
			})
			.catch((e) => showToast(e?.response?.data?.message || 'Không bắt đầu được kết nối Gmail.', 'error'));
	}

	function handleRetry(msg: ChatMessage) {
		if (!selectedConv) return;
		setMessages((prev) => prev.map((m) => (m._id === msg._id ? { ...m, status: 'sending' } : m)));
		api.post(`/inbox/conversations/${selectedConv._id}/messages/${msg._id}/retry`)
			.then(({ data }) => setMessages((prev) => prev.map((m) => (m._id === msg._id ? data.data : m))))
			.catch((e) => {
				setMessages((prev) => prev.map((m) => (m._id === msg._id ? e?.response?.data?.data || { ...m, status: 'failed' } : m)));
				showToast(e?.response?.data?.message || 'Gửi lại không thành công.', 'error');
			});
	}

	useEffect(() => {
		if (rightTab === 'Kênh' && activeKoc && !activeKoc.channelSyncedAt) {
			setSyncingChannel(true);
			Promise.resolve(onSyncChannel?.(activeKoc)).finally(() => {
				setTimeout(() => setSyncingChannel(false), 600);
			});
		}
	}, [rightTab, activeKoc?.id]);

	useEffect(() => {
		if (!apiReady) return;
		api.get('/inbox/labels')
			.then(({ data }) => setLabels(data.data || []))
			.catch(() => {});
	}, [apiReady]);

	useEffect(() => {
		if (!apiReady || !showHidden) return;
		api.get('/inbox/conversations', { params: { hidden: 1 } })
			.then(({ data }) => setHiddenConvs(data.data || []))
			.catch(() => setHiddenConvs([]));
	}, [apiReady, showHidden]);

	const filteredConversations = useMemo(() => {
		const source = showHidden ? hiddenConvs : conversations;
		return source.filter((conv) => {
			if (tabFilter === 'UNREAD' && !conv.unreadCount) return false;
			if (taskFilter.length && !taskFilter.includes(conv.nextTask || '')) return false;
			if (labelFilter.length && !(conv.labels || []).some((id) => labelFilter.includes(id))) return false;
			if (search.trim()) {
				const q = search.toLowerCase();
				const text = `${conv.contactName} ${conv.contactHandle} ${conv.contactPhone || ''}`.toLowerCase();
				if (!text.includes(q)) return false;
			}
			return true;
		});
	}, [conversations, hiddenConvs, showHidden, tabFilter, taskFilter, labelFilter, search]);

	const labelById = useMemo(() => new Map(labels.map((l) => [l._id, l])), [labels]);
	const recentConvs = useMemo(() => conversations.slice(0, 8), [conversations]);

	function reloadAfterCollabChange() {
		if (!apiReady || !selectedConv || String(selectedConv._id).startsWith('preview-') || String(selectedConv._id).startsWith('conv-')) return;
		api.get(`/inbox/conversations/${selectedConv._id}/messages`)
			.then(({ data }) => setMessages(data.data || []))
			.catch(() => {});
		api.get('/inbox/conversations')
			.then(({ data }) => setConversations(data.data || []))
			.catch(() => {});
	}

	function patchConv(id: string, patch: Partial<ConversationItem>) {
		setConversations((prev) => prev.map((c) => (c._id === id ? { ...c, ...patch } : c)));
		setSelectedConv((prev) => (prev && prev._id === id ? { ...prev, ...patch } : prev));
	}

	function openConversation(conv: ConversationItem) {
		setSelectedConv(conv);
		setRowMenuId(null);
		if (!conv.unreadCount) return;
		patchConv(conv._id, { unreadCount: 0 });
		if (apiReady && !String(conv._id).startsWith('preview-') && !String(conv._id).startsWith('conv-')) {
			api.post(`/inbox/conversations/${conv._id}/read`).catch(() => {});
		}
	}

	function handleToggleConvLabel(conv: ConversationItem, labelId: string) {
		const current = conv.labels || [];
		const next = current.includes(labelId) ? current.filter((x) => x !== labelId) : [...current, labelId];
		patchConv(conv._id, { labels: next });
		if (apiReady) {
			api.put(`/inbox/conversations/${conv._id}/labels`, { labels: next }).catch(() => {
				patchConv(conv._id, { labels: current });
				showToast('Không gắn được nhãn.', 'error');
			});
		}
	}

	function handleHideToggle(conv: ConversationItem) {
		setRowMenuId(null);
		const hide = !showHidden;
		if (hide) {
			setConversations((prev) => prev.filter((c) => c._id !== conv._id));
			if (selectedConv?._id === conv._id) setSelectedConv(null);
		} else {
			setHiddenConvs((prev) => prev.filter((c) => c._id !== conv._id));
			setConversations((prev) => [{ ...conv, hidden: false }, ...prev]);
		}
		if (apiReady) {
			api.post(`/inbox/conversations/${conv._id}/${hide ? 'hide' : 'unhide'}`).catch(() => showToast('Không cập nhật được trạng thái ẩn.', 'error'));
		}
		showToast(hide ? 'Đã ẩn hội thoại. KOC nhắn lại sẽ hiện lại.' : 'Đã bỏ ẩn hội thoại.');
	}

	async function handleCreateLabel(name: string, color: string) {
		if (!apiReady) {
			setLabels((prev) => [...prev, { _id: `local-${Date.now()}`, name, color }]);
			return;
		}
		try {
			const { data } = await api.post('/inbox/labels', { name, color });
			setLabels((prev) => [...prev, data.data]);
		} catch {
			showToast('Không tạo được nhãn.', 'error');
		}
	}

	async function handleDeleteLabel(id: string) {
		try {
			if (apiReady) await api.delete(`/inbox/labels/${id}`);
			setLabels((prev) => prev.filter((l) => l._id !== id));
			setLabelFilter((prev) => prev.filter((x) => x !== id));
			setConversations((prev) => prev.map((c) => ((c.labels || []).includes(id) ? { ...c, labels: (c.labels || []).filter((x) => x !== id) } : c)));
		} catch {
			showToast('Không xoá được nhãn.', 'error');
		}
	}

	function handleStartZaloQr() {
		setShowQrModal(true);
		setQrRequestError('');
		setZaloStatus((prev) => ({ ...prev, state: 'waiting_qr', qrImage: '', error: '' }));
		api.post('/zalo/qr/start')
			.then(({ data }) => setZaloStatus(data.data || { connected: false }))
			.catch((e) => {
				setQrRequestError(e?.response?.data?.message || 'Không gọi được máy chủ API. Kiểm tra backend đang chạy.');
			});
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

	function handleDeleteSingleMessage(msgId: string) {
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

	function handleSimulateKocInbound(kocText: string) {
		if (!selectedConv || !kocText) return;
		const nowIso = new Date().toISOString();
		const inboundMsg: ChatMessage = {
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
						const botMsg: ChatMessage = {
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
				const added: ChatMessage[] = [res.inboundMessage];
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
					className={`zalo-account-switcher clickable${zaloStatus?.connected ? '' : ' off'}`}
					onClick={handleStartZaloQr}
					title="Bấm để kết nối lại tài khoản Zalo bằng mã QR"
				>
					<div className="zalo-badge-icon">Zalo</div>
					<div className="zalo-acc-meta">
						<strong>{currentUser?.name || 'Bùi Văn Việt'}</strong>
						<small>{zaloStatus?.connected ? 'Zalo · đã kết nối' : 'Zalo · mất kết nối'}</small>
					</div>
					{zaloStatus?.connected ? <span className="caret">⌄</span> : <button type="button" className="zalo-connect-btn" onClick={(e) => { e.stopPropagation(); handleStartZaloQr(); }}><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M21 12a9 9 0 1 1-2.64-6.36" /><path d="M21 3v6h-6" /></svg> Kết nối</button>}
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
					<ConversationFilterMenu
						labels={labels}
						tasks={taskFilter}
						labelIds={labelFilter}
						showHidden={showHidden}
						onTasks={setTaskFilter}
						onLabelIds={setLabelFilter}
						onShowHidden={setShowHidden}
						onCreateLabel={handleCreateLabel}
						onDeleteLabel={handleDeleteLabel}
					/>
				</div>

				<div className="msg-conv-scroll">
					{filteredConversations.length === 0 && (
						<div className="msg-list-empty">
							<div className="msg-list-empty-icon">✓</div>
							<strong>
								{showHidden
									? 'Không có hội thoại bị ẩn'
									: tabFilter === 'UNREAD'
										? 'Không có tin chưa đọc'
										: 'Không có hội thoại phù hợp'}
							</strong>
							<small>
								{tabFilter === 'UNREAD' && !showHidden
									? 'Cuộc trò chuyện có tin mới từ KOC sẽ hiện ở đây.'
									: 'Thử đổi từ khoá hoặc bỏ bớt bộ lọc.'}
							</small>
						</div>
					)}
					{filteredConversations.map((conv) => {
						const matchedKoc = creators.find(
							(c) => `@${c.tiktokId}` === conv.contactHandle || c.name === conv.contactName,
						);
						const task = TASK_OPTIONS.find((t) => t.key === conv.nextTask);
						const badgeLabel = task ? task.label : apiReady ? 'Chưa có việc' : stageBadgeText(matchedKoc?.stage);
						const dateStr = new Date(conv.lastMessageAt).toLocaleDateString('vi-VN', {
							day: '2-digit',
							month: '2-digit',
						});
						const convLabels = (conv.labels || []).map((id) => labelById.get(id)).filter(Boolean) as InboxLabel[];
						return (
							<div
								key={conv._id}
								className={selectedConv?._id === conv._id ? 'msg-conv-item active' : 'msg-conv-item'}
								role="button"
								tabIndex={0}
								onClick={() => openConversation(conv)}
								onKeyDown={(e) => e.key === 'Enter' && openConversation(conv)}
							>
								<div className="avatar-circle">{conv.contactName.slice(0, 2).toUpperCase()}</div>
								<div className="msg-conv-body">
									<div className="msg-conv-top">
										<strong>
											{conv.unreadCount > 0 && <span className="msg-unread-dot" />}
											{conv.contactName}
											{convLabels.map((l) => (
												<i key={l._id} className="msg-label-dot" style={{ background: l.color }} title={l.name} />
											))}
										</strong>
										<time>{dateStr}</time>
									</div>
									<div className="msg-conv-bottom">
										<p className={conv.unreadCount > 0 ? 'unread' : ''}>{conv.lastMessage}</p>
										<span
											className={task ? 'conv-task-chip has-task' : 'conv-task-chip'}
											style={task ? { color: task.color, background: `${task.color}1a` } : undefined}
										>
											{badgeLabel}
										</span>
									</div>
								</div>
								<button
									type="button"
									className="msg-row-more"
									title="Tuỳ chọn"
									onClick={(e) => {
										e.stopPropagation();
										setRowMenuId((id) => (id === conv._id ? null : conv._id));
									}}
								>
									⋯
								</button>
								{rowMenuId === conv._id && (
									<div className="msg-row-menu" onClick={(e) => e.stopPropagation()}>
										<div className="msg-menu-title">Gắn nhãn</div>
										{labels.length === 0 && <div className="msg-menu-muted">Chưa có nhãn. Tạo ở menu Phân loại.</div>}
										{labels.map((l) => (
											<label key={l._id} className="msg-menu-check">
												<input
													type="checkbox"
													checked={(conv.labels || []).includes(l._id)}
													onChange={() => handleToggleConvLabel(conv, l._id)}
												/>
												<i style={{ background: l.color }} />
												{l.name}
											</label>
										))}
										<div className="msg-menu-sep" />
										<button type="button" className="msg-menu-link" onClick={() => handleHideToggle(conv)}>
											{showHidden ? 'Bỏ ẩn hội thoại' : 'Ẩn hội thoại'}
										</button>
									</div>
								)}
							</div>
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

									{messages.map((msg) => msg.kind === 'system' ? (
										<div key={msg._id} className="msg-system">
											<span>{msg.text}</span>
										</div>
									) : (
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
												<span className={msg.status === 'failed' ? 'delivery-receipt failed' : 'delivery-receipt'}>
												{messageStatusLabel(msg.status)}
												{msg.status === 'failed' && !msg._id.startsWith('tmp-') && (
													<button type="button" className="cmp-retry" onClick={() => handleRetry(msg)}>
														Thử lại
													</button>
												)}
											</span>
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

						<Composer
							conv={selectedConv}
							koc={activeKoc}
							live={apiReady && !String(selectedConv._id).startsWith('preview-') && !String(selectedConv._id).startsWith('conv-')}
							channel={channel}
							setChannel={setChannel}
							setMessages={setMessages}
							setConversations={setConversations}
							draft={draft}
							setDraft={setDraft}
							zaloConnected={!!zaloStatus?.connected}
							accountName={currentUser?.name || 'Bùi Văn Việt'}
							onReconnectZalo={handleStartZaloQr}
							gmail={gmail}
							onConnectGmail={handleConnectGmail}
							showToast={showToast}
							extraTools={
								<Select
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
									</Select>
							}
						/>
					</>
				) : (
					<div className="msg-quick-open">
						<h3>Mở lại nhanh</h3>
						<p>Chọn một KOC bên trái hoặc mở lại người bạn vừa làm việc.</p>
						<div className="msg-quick-grid">
							{recentConvs.map((conv) => (
								<button key={conv._id} type="button" className="msg-quick-card" onClick={() => openConversation(conv)}>
									<div className="avatar-circle">{conv.contactName.slice(0, 2).toUpperCase()}</div>
									<strong>{conv.contactName}</strong>
									<small>{conv.lastMessage}</small>
								</button>
							))}
						</div>
					</div>
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
				onTrackShipment={onTrackShipment}
				collabPanel={
					<CollabPanel
						koc={activeKoc}
						apiReady={apiReady}
						onOpenShipping={() => setActiveStepModal('sample')}
						onTrackShipment={onTrackShipment}
						onInsertDraft={(text) => setDraft((prev) => (prev ? `${prev}\n${text}` : text))}
						onChanged={reloadAfterCollabChange}
						showToast={showToast}
					/>
				}
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
							{(() => {
								const failed = qrRequestError || zaloStatus?.state === 'error' || zaloStatus?.bridgeOnline === false;
								if (failed) {
									return (
										<div className="qr-state qr-error">
											<strong>Không tạo được mã QR</strong>
											<p>{qrRequestError || zaloStatus?.error || 'Dịch vụ Zalo chưa sẵn sàng.'}</p>
											<button type="button" className="tm-btn-submit" onClick={handleStartZaloQr}>Thử lại</button>
										</div>
									);
								}
								if (zaloStatus?.state === 'expired') {
									return (
										<div className="qr-state">
											<strong>Mã QR đã hết hạn</strong>
											<p>Mã chỉ có hiệu lực khoảng 1 phút. Tạo mã mới để quét lại.</p>
											<button type="button" className="tm-btn-submit" onClick={handleStartZaloQr}>Tạo mã mới</button>
										</div>
									);
								}
								if (zaloStatus?.state === 'scanned') {
									return (
										<div className="qr-state">
											<span className="qr-spinner" />
											<strong>Đã quét mã</strong>
											<p>Xác nhận đăng nhập trên ứng dụng Zalo của điện thoại.</p>
										</div>
									);
								}
								if (zaloStatus?.qrImage) {
									return (
										<>
											<img src={zaloStatus.qrImage} alt="Mã QR Zalo" className="qr-image" />
											<button type="button" className="link-btn" onClick={handleStartZaloQr}>Tạo mã khác</button>
										</>
									);
								}
								return (
									<div className="qr-state">
										<span className="qr-spinner" />
										<p>Đang lấy mã QR từ máy chủ Zalo...</p>
									</div>
								);
							})()}
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

