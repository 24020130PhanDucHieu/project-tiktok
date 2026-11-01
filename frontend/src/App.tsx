import { Suspense, lazy, useCallback, useEffect, useState } from 'react';
import AccountPage from './features/account/AccountPage';
import MessagingWorkspace from './features/inbox/MessagingWorkspace';
import AddKocModal from './features/kocs/AddKocModal';
import KocDirectoryPage from './features/kocs/KocDirectoryPage';
import SidebarLayout from './layouts/SidebarLayout';
import api from './lib/axios';
import { INITIAL_FOLDERS, initialCreators, previewConversations } from './lib/constants';
import { mapItemToCreator, stageToCollabStep } from './lib/formatters';
import ForgotPasswordPage from './pages/ForgotPasswordPage';
import LoginPage from './pages/LoginPage';
import { clearSession, getToken, getUser } from './stores/useAuthStore';
import type { ConversationItem, Creator, ToastNotice, UserProfile } from './types/koc';

const AuditPage = lazy(() => import('./features/admin/AuditPage'));
const SystemPage = lazy(() => import('./features/admin/SystemPage'));
const UsersPage = lazy(() => import('./features/admin/UsersPage'));
const DashboardPage = lazy(() => import('./features/dashboard/DashboardPage'));

function ViewLoading() {
	return <div className="view-loading"><span className="qr-spinner" /></div>;
}

export default function App() {
	const [authenticated, setAuthenticated] = useState<boolean>(Boolean(getToken()));
	const [currentUser, setCurrentUser] = useState<UserProfile | null>(getUser());
	const [creators, setCreators] = useState<Creator[]>(() => (import.meta.env.DEV ? initialCreators.map(mapItemToCreator) : []));
	const [conversations, setConversations] = useState<ConversationItem[]>(import.meta.env.DEV ? previewConversations : []);
	const [apiReady, setApiReady] = useState<boolean>(false);

	// Điều hướng giữa 2 phân hệ chính: Quản lý KOC & Hộp thư
	const [selectedNav, setSelectedNav] = useState<string>('Tổng quan');
	const [authView, setAuthView] = useState<'login' | 'forgot'>('login');
	const [folders, setFolders] = useState<string[]>(INITIAL_FOLDERS);
	const [isAddModalOpen, setAddModalOpen] = useState<boolean>(false);
	const [pendingDeleteKoc, setPendingDeleteKoc] = useState<Creator | null>(null);
	const [toast, setToast] = useState<ToastNotice | null>(null);

	const showToast = useCallback((msg: string, type: 'success' | 'error' = 'success') => {
		if (!msg) {
			setToast(null);
			return;
		}
		const isError =
			type === 'error' || /^(không thể|vui lòng|lỗi|chặn)/i.test(String(msg).trim());
		setToast({
			title: isError ? 'Cần lưu ý' : 'Cập nhật thành công',
			message: String(msg),
			type: isError ? 'error' : 'success',
		});
		window.clearTimeout(window.__toastTimer);
		window.__toastTimer = window.setTimeout(() => setToast(null), 3600);
	}, []);

	function refreshAll() {
		if (!authenticated) return;
		Promise.all([api.get('/kocs', { params: { limit: 100 } }), api.get('/inbox/conversations')])
			.then(([kocRes, inboxRes]) => {
				const mapped = (kocRes.data.data.items || []).map(mapItemToCreator);
				setCreators(mapped);
				if (inboxRes.data.data?.length) {
					setConversations(inboxRes.data.data);
				}
				setApiReady(true);
			})
			.catch(() => setApiReady(false));
	}

	useEffect(() => {
		refreshAll();
	}, [authenticated]);

	if (!authenticated) {
		if (authView === 'forgot') return <ForgotPasswordPage onBack={() => setAuthView('login')} />;
		return (
			<LoginPage
				onForgot={() => setAuthView('forgot')}
				onLogin={(user) => {
					setCurrentUser(user);
					setAuthenticated(true);
					setSelectedNav('Tổng quan');
				}}
			/>
		);
	}

	function logout() {
		clearSession();
		setAuthenticated(false);
		setCurrentUser(null);
		setSelectedNav('Tổng quan');
	}

	const isAdmin = currentUser?.role === 'admin';

	function handleAddKoc(payload: Record<string, any>) {
		if (!apiReady) {
			const created = mapItemToCreator({
				...payload,
				id: `local-${Date.now()}`,
				stage: payload.castFee > 0 ? 'AWAITING_APPROVAL' : 'QUALIFIED',
			});
			setCreators((prev) => [created, ...prev]);
			setConversations((prev) => [
				{
					_id: `conv-${Date.now()}`,
					contactName: created.name,
					contactHandle: created.handle,
					contactPhone: created.phone,
					lastMessage: 'Đã thêm KOC vào danh bạ',
					lastMessageAt: new Date().toISOString(),
					unreadCount: 0,
				},
				...prev,
			]);
			setAddModalOpen(false);
			showToast(`Đã thêm ${created.name} vào danh bạ.`);
			return;
		}

		api.post('/kocs', payload)
			.then(({ data }) => {
				const created = mapItemToCreator(data.data);
				setCreators((prev) => [created, ...prev]);
				setAddModalOpen(false);
				showToast(`Đã thêm ${created.name} vào danh bạ.`);
				refreshAll();
			})
			.catch((err) => {
				showToast(err?.response?.data?.message || 'Không thể thêm KOC (Kiểm tra trùng ID hoặc SĐT).');
			});
	}

	function handleUpdateKocStage(creator: Creator, payload: Record<string, any>, noticeText: string) {
		if (!apiReady || String(creator.id).startsWith('seed-') || String(creator.id).startsWith('local-')) {
			setCreators((prev) =>
				prev.map((item) => {
					if (item.id !== creator.id) return item;
					const nextFee = payload.castFee !== undefined ? Number(payload.castFee) : item.castFee;
					const nextStage = payload.stage || item.stage;
					const nextStep =
						payload.collabStep !== undefined ? payload.collabStep : stageToCollabStep(nextStage);
					return {
						...item,
						...payload,
						castFee: nextFee,
						stage: nextStage,
						collabStep: nextStep,
					};
				}),
			);
			showToast(noticeText);
			return;
		}

		api.patch(`/kocs/${creator.id}/stage`, payload)
			.then(({ data }) => {
				const updated = mapItemToCreator(data.data);
				if (payload.collabStep !== undefined) updated.collabStep = payload.collabStep;
				setCreators((prev) => prev.map((item) => (item.id === creator.id ? updated : item)));
				showToast(noticeText);
			})
			.catch((err) => showToast(err?.response?.data?.message || 'Không thể cập nhật tiến trình hợp tác.'));
	}

	function handleSyncChannel(
		creator: Creator | null,
		{ autoDetectCollabVideo = false }: { autoDetectCollabVideo?: boolean } = {},
	) {
		if (!creator) return;
		if (!apiReady || String(creator.id).startsWith('seed-') || String(creator.id).startsWith('local-')) {
			api.post('/kocs/lookup', { tiktokId: creator.tiktokId })
				.then(({ data }) => {
					const res = data.data;
					if (res?.profile) {
						setCreators((prev) =>
							prev.map((c) =>
								c.id === creator.id
									? {
											...c,
											followers: res.profile.followers || c.followers,
											avatarUrl: res.profile.avatarUrl || c.avatarUrl,
											videos: res.videos?.length ? res.videos : c.videos,
											channelSyncedAt: new Date().toISOString(),
										}
									: c,
							),
						);
						showToast(`Đã đồng bộ kênh thật @${creator.tiktokId} từ TikTok.`);
					}
				})
				.catch(() => {});
			return;
		}
		api.post(`/kocs/${creator.id}/sync-channel`, { autoDetectCollabVideo })
			.then(({ data }) => {
				const updated = mapItemToCreator(data.data);
				setCreators((prev) => prev.map((item) => (item.id === creator.id ? updated : item)));
				showToast(data.message || `Đã đồng bộ kênh @${creator.tiktokId}`);
			})
			.catch((err) => showToast(err?.response?.data?.message || 'Không thể đồng bộ kênh TikTok.'));
	}

	function handleTrackShipment(creator: Creator | null, customTrackingCode = '') {
		if (!creator) return;
		const code = customTrackingCode || creator.trackingCode;
		if (!code) {
			showToast('Vui lòng nhập mã vận đơn J&T Express trước khi tra cứu.');
			return;
		}
		if (!apiReady || String(creator.id).startsWith('seed-') || String(creator.id).startsWith('local-')) {
			api.post('/kocs/track-shipment/lookup', { trackingCode: code })
				.then(({ data }) => {
					const tracking = data.data;
					setCreators((prev) =>
						prev.map((c) =>
							c.id === creator.id
								? {
										...c,
										trackingCode: code,
										stage: tracking?.delivered ? 'DELIVERED' : c.stage,
										trackingStatusText:
											tracking?.statusText || `J&T Express #${code}: Đã ghi nhận trên cổng J&T`,
										lastTrackedAt: new Date().toISOString(),
									}
								: c,
						),
					);
					showToast(data.message || `Đã tra cứu vận đơn J&T Express #${code}.`);
				})
				.catch((err) => showToast(err?.response?.data?.message || 'Không thể kết nối cổng J&T Express.'));
			return;
		}
		api.post(`/kocs/${creator.id}/track-shipment`, { trackingCode: code })
			.then(({ data }) => {
				const updated = mapItemToCreator(data.data);
				setCreators((prev) => prev.map((item) => (item.id === creator.id ? updated : item)));
				showToast(data.message || `Đã cập nhật vận đơn #${code}`);
			})
			.catch((err) => showToast(err?.response?.data?.message || 'Không thể tra cứu vận đơn J&T.'));
	}

	function handleDeleteKoc(creator: Creator) {
		if (!creator) return;
		setPendingDeleteKoc(creator);
	}

	function confirmDeleteKoc() {
		const creator = pendingDeleteKoc;
		if (!creator) return;
		setPendingDeleteKoc(null);
		if (!apiReady || String(creator.id).startsWith('seed-') || String(creator.id).startsWith('local-')) {
			setCreators((prev) => prev.filter((c) => c.id !== creator.id));
			showToast(`Đã xóa ${creator.name} khỏi danh bạ.`);
			return;
		}
		api.delete(`/kocs/${creator.id}`)
			.then(() => {
				setCreators((prev) => prev.filter((c) => c.id !== creator.id));
				showToast(`Đã xóa ${creator.name} khỏi danh bạ.`);
			})
			.catch(() => showToast('Không thể xóa KOC.', 'error'));
	}

	return (
		<>
			<SidebarLayout
				selectedNav={selectedNav}
				onSelectNav={setSelectedNav}
				onOpenAddKoc={() => {
					setSelectedNav('Quản lý KOC');
					setAddModalOpen(true);
				}}
				currentUser={currentUser}
				onOpenAccount={() => setSelectedNav('Tài khoản')}
				toast={toast}
				onDismissToast={() => setToast(null)}
			>
				<div className="view-enter" key={selectedNav}>
				<Suspense fallback={<ViewLoading />}>
				{selectedNav === 'Tổng quan' ? (
					<DashboardPage onOpenKoc={() => setSelectedNav('Quản lý KOC')} showToast={showToast} />
				) : selectedNav === 'Người dùng' && isAdmin ? (
					<UsersPage currentUser={currentUser} showToast={showToast} />
				) : selectedNav === 'Nhật ký' && isAdmin ? (
					<AuditPage showToast={showToast} />
				) : selectedNav === 'Hệ thống' && isAdmin ? (
					<SystemPage showToast={showToast} />
				) : selectedNav === 'Tài khoản' && currentUser ? (
					<AccountPage user={currentUser} onUserChange={setCurrentUser} onLogout={logout} showToast={showToast} />
				) : selectedNav === 'Quản lý KOC' ? (
					<KocDirectoryPage
						creators={creators}
						folders={folders}
						onCreateFolder={(name) => {
							if (!folders.includes(name)) setFolders((prev) => [...prev, name]);
						}}
						onOpenAddKoc={() => setAddModalOpen(true)}
						onSelectChat={() => setSelectedNav('Hộp thư')}
						onDeleteKoc={handleDeleteKoc}
					/>
				) : (
					<MessagingWorkspace
						creators={creators}
						conversations={conversations}
						setConversations={setConversations}
						apiReady={apiReady}
						currentUser={currentUser}
						onUpdateKoc={handleUpdateKocStage}
						onSyncChannel={handleSyncChannel}
						onTrackShipment={handleTrackShipment}
						showToast={showToast}
					/>
				)}
				</Suspense>
				</div>
			</SidebarLayout>

			{isAddModalOpen && (
				<AddKocModal
					folders={folders}
					onClose={() => setAddModalOpen(false)}
					onAdd={handleAddKoc}
				/>
			)}

			{pendingDeleteKoc && (
				<div
					className="modal-backdrop"
					onMouseDown={(e) => e.target === e.currentTarget && setPendingDeleteKoc(null)}
				>
					<div className="tiktoday-modal confirm-web-modal">
						<div className="confirm-web-body">
							<div className="confirm-web-icon">
								<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1">
									<path d="M3 6h18" strokeLinecap="round" />
									<path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" strokeLinecap="round" />
									<path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" strokeLinecap="round" />
									<line x1="10" y1="11" x2="10" y2="17" strokeLinecap="round" />
									<line x1="14" y1="11" x2="14" y2="17" strokeLinecap="round" />
								</svg>
							</div>
							<div className="confirm-web-text">
								<h3>Xóa nhà sáng tạo khỏi danh bạ?</h3>
								<p>
									Bạn đang chuẩn bị xóa <b>{pendingDeleteKoc.name}</b> ({pendingDeleteKoc.handle}) khỏi danh bạ KOC. Thao tác này sẽ ẩn KOC khỏi danh sách quản lý.
								</p>
							</div>
						</div>
						<div className="tm-footer">
							<button
								type="button"
								className="tm-btn-cancel"
								onClick={() => setPendingDeleteKoc(null)}
							>
								Huỷ bỏ
							</button>
							<button
								type="button"
								className="tm-btn-danger"
								onClick={confirmDeleteKoc}
							>
								Xác nhận xóa
							</button>
						</div>
					</div>
				</div>
			)}
		</>
	);
}

