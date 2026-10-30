import { useEffect, useState } from 'react';
import MessagingWorkspace from './features/inbox/MessagingWorkspace';
import AddKocModal from './features/kocs/AddKocModal';
import KocDirectoryPage from './features/kocs/KocDirectoryPage';
import SidebarLayout from './layouts/SidebarLayout';
import api from './lib/axios';
import { INITIAL_FOLDERS, initialCreators, previewConversations } from './lib/constants';
import { mapItemToCreator, stageToCollabStep } from './lib/formatters';
import LoginPage from './pages/LoginPage';
import { clearSession, getToken, getUser } from './stores/useAuthStore';

export default function App() {
	const [authenticated, setAuthenticated] = useState(Boolean(getToken()));
	const [currentUser, setCurrentUser] = useState(getUser());
	const [creators, setCreators] = useState(() => initialCreators.map(mapItemToCreator));
	const [conversations, setConversations] = useState(previewConversations);
	const [apiReady, setApiReady] = useState(false);

	// Điều hướng giữa 2 phân hệ chính: Quản lý KOC & Hộp thư
	const [selectedNav, setSelectedNav] = useState('Quản lý KOC');
	const [folders, setFolders] = useState(INITIAL_FOLDERS);
	const [isAddModalOpen, setAddModalOpen] = useState(false);
	const [toast, setToast] = useState('');

	function showToast(msg) {
		setToast(msg);
		window.clearTimeout(window.__toastTimer);
		window.__toastTimer = window.setTimeout(() => setToast(''), 3200);
	}

	function refreshAll() {
		if (!authenticated) return;
		Promise.all([api.get('/kocs'), api.get('/inbox/conversations')])
			.then(([kocRes, inboxRes]) => {
				const mapped = kocRes.data.data.items.map(mapItemToCreator);
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
		return (
			<LoginPage
				onLogin={(user) => {
					setCurrentUser(user);
					setAuthenticated(true);
				}}
			/>
		);
	}

	function handleAddKoc(payload) {
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

	function handleUpdateKocStage(creator, payload, noticeText) {
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

	function handleSyncChannel(creator, { autoDetectCollabVideo = false } = {}) {
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

	function handleTrackShipment(creator, customTrackingCode = '') {
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

	function handleDeleteKoc(creator) {
		if (!window.confirm(`Xóa ${creator.name} khỏi danh bạ?`)) return;
		if (!apiReady || String(creator.id).startsWith('seed-') || String(creator.id).startsWith('local-')) {
			setCreators((prev) => prev.filter((c) => c.id !== creator.id));
			showToast(`Đã xóa ${creator.name}.`);
			return;
		}
		api.delete(`/kocs/${creator.id}`)
			.then(() => {
				setCreators((prev) => prev.filter((c) => c.id !== creator.id));
				showToast(`Đã xóa ${creator.name}.`);
			})
			.catch(() => showToast('Không thể xóa KOC.'));
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
				onLogout={() => {
					clearSession();
					setAuthenticated(false);
					setCurrentUser(null);
				}}
				toast={toast}
			>
				{selectedNav === 'Quản lý KOC' ? (
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
			</SidebarLayout>

			{isAddModalOpen && (
				<AddKocModal
					folders={folders}
					onClose={() => setAddModalOpen(false)}
					onAdd={handleAddKoc}
				/>
			)}
		</>
	);
}
