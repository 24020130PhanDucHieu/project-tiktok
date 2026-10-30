import { useEffect, useMemo, useState } from 'react';
import api from './lib/axios';
import LoginPage from './pages/LoginPage';
import { clearSession, getToken, getUser } from './stores/useAuthStore';

const INITIAL_FOLDERS = ['Làm đẹp', 'Đời sống', 'Thời trang'];

const COLLAB_STEPS = [
	{ key: 'cast', title: 'Chốt phí cast', buttonLabel: 'Chốt phí cast' },
	{ key: 'mode', title: 'Chọn hình thức hợp tác', buttonLabel: 'Chọn hình thức hợp tác' },
	{ key: 'sample', title: 'Duyệt & gửi mẫu', buttonLabel: 'Duyệt & gửi mẫu' },
	{ key: 'video', title: 'KOC đăng bài', buttonLabel: 'Cập nhật video' },
	{ key: 'result', title: 'Hiệu quả', buttonLabel: 'Cập nhật hiệu quả' },
];

const DECLINE_REASONS = [
	{ code: 'R01', label: 'R01 — Không nhận Freecast (Yêu cầu có phí)' },
	{ code: 'R02', label: 'R02 — Giá cast chưa phù hợp ngân sách' },
	{ code: 'R03', label: 'R03 — Hoa hồng Affiliate thấp' },
	{ code: 'R04', label: 'R04 — KOC đang kín lịch đăng bài' },
	{ code: 'R05', label: 'R05 — Sản phẩm không hợp định hướng kênh' },
	{ code: 'R09', label: 'R09 — Lý do khác' },
];

const POST_RECEIVE_REASONS = [
	{ code: 'P01', label: 'P01 — Không hợp sản phẩm (Kích ứng / Không đúng tông / Không vừa size)' },
	{ code: 'P02', label: 'P02 — Sản phẩm lỗi ngoại quan / Hư hỏng khi vận chuyển' },
	{ code: 'P03', label: 'P03 — KOC đổi định hướng nội dung, xin hoàn trả mẫu' },
];

const MESSAGE_TEMPLATES = [
	{
		id: 'invite-1',
		label: 'Mẫu 1: Lời mời hợp tác lần đầu (Gửi mẫu trải nghiệm)',
		build: (koc) =>
			`Chúng mình có theo dõi kênh ${koc?.handle || '@koc'} và nhận thấy định hướng nội dung, hình ảnh của bạn khá phù hợp với thương hiệu bên mình.\nBên mình mong muốn gửi sản phẩm ${koc?.productName || 'mẫu'} để bạn trải nghiệm (nếu không phù hợp bạn có thể gửi hoàn lại). Sau khi nhận mẫu, bạn có thể lên video giới thiệu hoặc dựng bối cảnh phù hợp với phong cách kênh của bạn. Mức hoa hồng đề xuất là ${koc?.commissionRate || 10}% cho đơn tự nhiên.`,
	},
	{
		id: 'remind-2d',
		label: 'Mẫu 2: Nhắc lần 2 sau 48h (+2 ngày)',
		build: (koc) =>
			`Chào ${koc?.name || 'bạn'}, bên mình có gửi lời mời hợp tác trải nghiệm sản phẩm thuộc chiến dịch ${koc?.campaign || 'mới'} cách đây 2 ngày. Bạn xem qua thông tin rồi phản hồi giúp mình nhé!`,
	},
	{
		id: 'remind-4d-exit',
		label: 'Mẫu 3: Nhắc lần 3 sau +4 ngày (Có lối thoát lịch sự)',
		build: (koc) =>
			`Chào ${koc?.name || 'bạn'}, bên mình nhắn ngắn hỏi thăm về lời mời trải nghiệm sản phẩm hôm trước ạ. Nếu đợt này lịch kênh chưa phù hợp hoặc sản phẩm không phù hợp cũng báo giúp bên mình một tiếng nhé!`,
	},
	{
		id: 'ask-address',
		label: 'Mẫu 4: Xin địa chỉ gửi sản phẩm mẫu',
		build: (koc) =>
			`Cảm ơn ${koc?.name || 'bạn'} đã đồng ý hợp tác. Bạn cho mình xin Tên người nhận, Số điện thoại và Địa chỉ chi tiết để bên mình lên đơn gửi sản phẩm mẫu qua bạn trải nghiệm nhé!`,
	},
];

const initialCreators = [
	{
		id: 'seed-1',
		name: 'Tủ đồ da ngăm',
		handle: '@trlyng.review',
		tiktokId: 'trlyng.review',
		contact: '0327637213',
		phone: '0327637213',
		email: 'trlyng.review@gmail.com',
		followers: 4000,
		gmv: 125500000,
		category: 'Thời trang',
		recipient: 'Trần Lý',
		shippingAddress: 'Quận Cầu Giấy, Hà Nội',
		productName: 'Khăn lạnh thể thao Cooltech',
		campaign: 'Đợt T11 - Cooltech',
		commissionRate: 10,
		castFee: 0,
		stage: 'CONTACTED',
		collabStep: 0,
		avatar: 'TĐ',
		videos: [
			{
				id: 'v1',
				date: '26/10',
				title: 'Một bước nhỏ thêm vào chu trình skincare nhưng có thể...',
				views: '125',
				likes: '2',
				comments: '0',
				shares: '0',
				product: '[HÀNG TẶNG KHÔNG BÁN] Kem dưỡng...',
			},
			{
				id: 'v2',
				date: '23/10',
				title: 'Đồ chạy bộ nhà Coolmate #dochaybo #chaybo...',
				views: '4k',
				likes: '12',
				comments: '0',
				shares: '0',
				product: 'Coolmate Core Tee - Áo Thể Thao...',
			},
			{
				id: 'v3',
				date: '20/10',
				title: 'Một brand đồ thể thao của Việt Nam mà có thể bạn chưa biết...',
				views: '284',
				likes: '10',
				comments: '1',
				shares: '2',
				product: 'Quần tập bike dệt liền Core...',
			},
		],
	},
	{
		id: 'seed-2',
		name: 'Huế Babi',
		handle: '@huebabi.official',
		tiktokId: 'huebabi.official',
		contact: '0866073103',
		phone: '0866073103',
		email: 'huebabi@gmail.com',
		followers: 12700,
		gmv: 45000000,
		category: 'Làm đẹp',
		recipient: 'Huế Babi',
		shippingAddress: 'Quận 7, TP.HCM',
		productName: 'Serum phục hồi B5',
		campaign: 'Đợt T11 - Skincare',
		commissionRate: 15,
		castFee: 500000,
		stage: 'AWAITING_APPROVAL',
		collabStep: 0,
		avatar: 'HB',
		videos: [],
	},
	{
		id: 'seed-3',
		name: 'Gia đình trái cây',
		handle: '@giadinhtraicay1202',
		tiktokId: 'giadinhtraicay1202',
		contact: '0987654321',
		phone: '0987654321',
		email: 'giadinhtraicay@gmail.com',
		followers: 3100,
		gmv: 18500000,
		category: 'Đời sống',
		recipient: 'Gia đình trái cây',
		shippingAddress: 'Quận Thanh Xuân, Hà Nội',
		productName: 'Máy ép chậm mini',
		campaign: 'Đợt T11 - Gia dụng',
		commissionRate: 10,
		castFee: 0,
		stage: 'AGREED',
		collabStep: 2,
		avatar: 'GT',
		videos: [],
	},
];

const previewConversations = [
	{
		_id: 'preview-1',
		contactName: 'Tủ đồ da ngăm',
		contactHandle: '@trlyng.review',
		contactPhone: '0327637213',
		lastMessage: 'Chúng mình có theo dõi kênh @trlyng.review và nhận thấy định hướng...',
		lastMessageAt: new Date().toISOString(),
		unreadCount: 0,
	},
	{
		_id: 'preview-2',
		contactName: 'Huế Babi',
		contactHandle: '@huebabi.official',
		contactPhone: '0866073103',
		lastMessage: 'Chúng mình có theo dõi kênh và muốn gửi lời mời hợp tác...',
		lastMessageAt: new Date(Date.now() - 86400000).toISOString(),
		unreadCount: 1,
	},
	{
		_id: 'preview-3',
		contactName: 'Gia đình trái cây',
		contactHandle: '@giadinhtraicay1202',
		contactPhone: '0987654321',
		lastMessage: 'Okie cậu nha, mình đã nhận được thông tin rồi ạ',
		lastMessageAt: new Date(Date.now() - 172800000).toISOString(),
		unreadCount: 0,
	},
];

const previewMessagesMap = {
	'preview-1': [
		{
			_id: 'm1',
			direction: 'outbound',
			text: 'Chào bạn',
			sentAt: new Date(Date.now() - 3600000 * 5).toISOString(),
			status: 'Đã nhận',
		},
		{
			_id: 'm2',
			direction: 'outbound',
			text: 'Chúng mình xin giới thiệu một chút. Chúng mình là thương hiệu đồ thể thao Cooltech. Bên mình là một thương hiệu mới với ưu thế nổi bật ở phần chất liệu: Bên mình tự nghiên cứu và phát triển (R&D) công thức vải, sau đó đặt sản xuất độc quyền, nên kiểm soát được chất lượng từ chất liệu đến thành phẩm.',
			sentAt: new Date(Date.now() - 3600000 * 4).toISOString(),
			status: 'Đã nhận',
		},
		{
			_id: 'm3',
			direction: 'outbound',
			text: 'Chúng mình có theo dõi kênh @trlyng.review và nhận thấy định hướng nội dung, hình ảnh của bạn khá phù hợp với hướng phát triển mà Cooltech đang xây dựng. Vì vậy, chúng mình mong muốn đề xuất hợp tác cùng bạn với sản phẩm khăn lạnh thể thao Cooltech của bên mình.\nSau khi nhận sản phẩm, bạn có thể lên video review/giới thiệu sản phẩm, hoặc đưa vào tuyến nội dung sử dụng sản phẩm thật theo phong cách của bạn thường làm. Mức hoa hồng chúng mình đề xuất là 10% cho đơn tự nhiên và 3% cho đơn từ quảng cáo.',
			sentAt: new Date(Date.now() - 3600000 * 3).toISOString(),
			status: 'Đã nhận',
		},
	],
};

function stageToCollabStep(stage) {
	switch (stage) {
		case 'SOURCED':
		case 'AWAITING_APPROVAL':
		case 'QUALIFIED':
		case 'CONTACTED':
		case 'NO_REPLY':
			return 0;
		case 'REPLIED':
			return 1;
		case 'AGREED':
			return 2;
		case 'SHIPPING':
		case 'DELIVERED':
		case 'NOT_FIT':
		case 'RETURNING':
		case 'RETURNED':
		case 'STALLED':
			return 3;
		case 'VIDEO_POSTED':
			return 4;
		case 'COMPLETED':
			return 5;
		default:
			return 0;
	}
}

function stageBadgeText(stage) {
	switch (stage) {
		case 'AGREED':
			return 'Chờ gửi mẫu';
		case 'SHIPPING':
			return 'Đang giao J&T';
		case 'DELIVERED':
			return 'Chờ đăng bài';
		case 'VIDEO_POSTED':
			return 'Đã lên bài';
		case 'COMPLETED':
			return 'Hoàn thành';
		case 'NOT_FIT':
		case 'RETURNING':
			return 'Đang hoàn hàng';
		case 'RETURNED':
			return 'Đã nhận hoàn';
		case 'STALLED':
			return 'Cờ đỏ · Bùng';
		case 'REPLIED':
			return 'Đang trao đổi';
		case 'DECLINED':
			return 'Đã từ chối';
		case 'DORMANT':
			return 'Ngủ đông 30N';
		default:
			return 'Chưa có việc';
	}
}

function mapItemToCreator(item) {
	const rawName = (item.displayName || item.name || 'KOC').replace(/[^\p{L}\p{N}\s._-]/gu, '').trim() || item.displayName || 'KOC';
	const stage = item.stage || 'SOURCED';
	const computedStep = Math.max(Number(item.collabStep) || 0, stageToCollabStep(stage));
	return {
		...item,
		id: item._id || item.id,
		name: rawName,
		handle: `@${(item.tiktokId || '').replace(/^@/, '')}`,
		contact: item.phone || '—',
		followers: Number(item.followers) || 0,
		gmv: Number(item.gmv) || 0,
		category: item.category || 'Chưa phân loại',
		castFee: Number(item.castFee) || 0,
		stage,
		collabStep: computedStep,
		avatar: rawName.slice(0, 2).toUpperCase(),
		videos: item.videos?.length ? item.videos : initialCreators[0].videos,
	};
}

function formatNumber(value) {
	const num = Number(value) || 0;
	if (num >= 1000000) {
		return `${(num / 1000000).toFixed(1).replace('.0', '').replace('.', ',')}tr`;
	}
	if (num >= 1000) {
		return `${(num / 1000).toFixed(1).replace('.0', '').replace('.', ',')}k`;
	}
	return String(num);
}

function formatCurrency(value) {
	const num = Number(value) || 0;
	if (!num) return '—';
	return `${new Intl.NumberFormat('vi-VN').format(num)}đ`;
}

function App() {
	const [authenticated, setAuthenticated] = useState(Boolean(getToken()));
	const [currentUser, setCurrentUser] = useState(getUser());
	const [creators, setCreators] = useState(() => initialCreators.map(mapItemToCreator));
	const [conversations, setConversations] = useState(previewConversations);
	const [apiReady, setApiReady] = useState(false);

	// Navigation (Chỉ còn 2 tab chính: Quản lý KOC và Hộp thư)
	const [selectedNav, setSelectedNav] = useState('Quản lý KOC');

	// Folder & Filter states in Quản lý KOC
	const [folders, setFolders] = useState(INITIAL_FOLDERS);
	const [selectedFolder, setSelectedFolder] = useState('Tất cả KOC');
	const [followerFilter, setFollowerFilter] = useState('ALL');
	const [gmvFilter, setGmvFilter] = useState('ALL');
	const [contactFilter, setContactFilter] = useState('ALL');
	const [query, setQuery] = useState('');

	// Modals
	const [isAddModalOpen, setAddModalOpen] = useState(false);
	const [isCreateFolderOpen, setCreateFolderOpen] = useState(false);
	const [newFolderName, setNewFolderName] = useState('');
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

	const filteredCreators = useMemo(() => {
		return creators.filter((creator) => {
			if (selectedFolder === 'Chưa phân loại' && creator.category !== 'Chưa phân loại') return false;
			if (
				selectedFolder !== 'Tất cả KOC' &&
				selectedFolder !== 'Chưa phân loại' &&
				creator.category !== selectedFolder
			) {
				return false;
			}

			if (followerFilter === 'UNDER_10K' && creator.followers >= 10000) return false;
			if (followerFilter === '10K_50K' && (creator.followers < 10000 || creator.followers > 50000)) return false;
			if (followerFilter === 'OVER_50K' && creator.followers <= 50000) return false;

			if (gmvFilter === 'HAS_GMV' && creator.gmv <= 0) return false;
			if (gmvFilter === 'OVER_10M' && creator.gmv < 10000000) return false;

			if (contactFilter === 'HAS_PHONE' && (!creator.phone || creator.phone === '—')) return false;
			if (contactFilter === 'HAS_EMAIL' && !creator.email) return false;

			if (query.trim()) {
				const q = query.toLowerCase();
				const haystack = `${creator.name} ${creator.handle} ${creator.phone || ''} ${creator.email || ''}`.toLowerCase();
				if (!haystack.includes(q)) return false;
			}
			return true;
		});
	}, [creators, selectedFolder, followerFilter, gmvFilter, contactFilter, query]);

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

	function handleCreateFolder(e) {
		e.preventDefault();
		const name = newFolderName.trim();
		if (!name) return;
		if (!folders.includes(name)) setFolders((prev) => [...prev, name]);
		setSelectedFolder(name);
		setNewFolderName('');
		setCreateFolderOpen(false);
	}

	const uncategorizedCount = creators.filter((c) => c.category === 'Chưa phân loại').length;

	return (
		<div className="tiktoday-layout">
			{/* SIDEBAR TRÁI: Thu gọn thành thanh Icon, di chuột vào tự động trượt mượt ra */}
			<aside className="hover-sidebar">
				<div className="sb-brand">
					<div className="sb-logo-box">
						<svg width="20" height="20" viewBox="0 0 24 24" fill="none">
							<path
								d="M6.5 5v14M6.5 12l7.5-7M10.5 9.5L17.5 19"
								stroke="currentColor"
								strokeWidth="2.5"
								strokeLinecap="round"
								strokeLinejoin="round"
							/>
							<circle cx="17.5" cy="5.5" r="2.2" fill="#38bdf8" />
						</svg>
					</div>
					<div className="sb-brand-text">
						<span className="sb-title">KocFlow</span>
						<span className="sb-tag">STUDIO</span>
					</div>
				</div>

				<button
					className="sb-search-btn"
					onClick={() => {
						setSelectedNav('Quản lý KOC');
						setAddModalOpen(true);
					}}
				>
					<span className="sb-icon">
						<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2">
							<circle cx="11" cy="11" r="7" />
							<path d="m20 20-3.5-3.5" strokeLinecap="round" />
						</svg>
					</span>
					<span className="sb-label">Tìm kiếm KOC</span>
					<span className="sb-arrow">↗</span>
				</button>

				<nav className="sb-nav">
					<button
						className={selectedNav === 'Hộp thư' ? 'sb-nav-item active' : 'sb-nav-item'}
						onClick={() => setSelectedNav('Hộp thư')}
					>
						<span className="sb-icon">
							<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9">
								<path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" />
								<polyline points="22,6 12,13 2,6" />
							</svg>
						</span>
						<span className="sb-label">Hộp thư</span>
					</button>

					<button
						className={selectedNav === 'Quản lý KOC' ? 'sb-nav-item active' : 'sb-nav-item'}
						onClick={() => setSelectedNav('Quản lý KOC')}
					>
						<span className="sb-icon">
							<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9">
								<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
								<circle cx="9" cy="7" r="4" />
								<path d="M23 21v-2a4 4 0 0 0-3-3.87" />
								<path d="M16 3.13a4 4 0 0 1 0 7.75" />
							</svg>
						</span>
						<span className="sb-label">Quản lý KOC</span>
					</button>
				</nav>

				<div className="sb-footer">
					<button
						className="sb-account-btn"
						onClick={() => {
							clearSession();
							setAuthenticated(false);
							setCurrentUser(null);
						}}
						title="Bấm để đăng xuất"
					>
						<div className="sb-user-avatar">{currentUser?.name?.slice(0, 1)?.toUpperCase() || 'Đ'}</div>
						<div className="sb-user-info">
							<strong>{currentUser?.name || 'Đức Việt'}</strong>
							<small>{currentUser?.email || 'ducviet@kocflow.vn'}</small>
						</div>
					</button>
				</div>
			</aside>

			{/* KHU VỰC NỘI DUNG CHÍNH */}
			<div className="tiktoday-main">
				{toast && <div className="floating-toast">{toast}</div>}

				{selectedNav === 'Quản lý KOC' ? (
					<div className="koc-directory-layout">
						{/* Cột Danh bạ & Thư mục bên trái */}
						<aside className="koc-folder-panel">
							<div className="folder-section">
								<div className="folder-heading">DANH BẠ</div>
								<button
									className={selectedFolder === 'Tất cả KOC' ? 'folder-item active' : 'folder-item'}
									onClick={() => setSelectedFolder('Tất cả KOC')}
								>
									<span>Tất cả KOC</span>
									<small>{creators.length}</small>
								</button>
								<button
									className={selectedFolder === 'Chưa phân loại' ? 'folder-item active' : 'folder-item'}
									onClick={() => setSelectedFolder('Chưa phân loại')}
								>
									<span>Chưa phân loại</span>
									<small>{uncategorizedCount}</small>
								</button>
							</div>

							<div className="folder-section">
								<div className="folder-heading">THƯ MỤC</div>
								{folders.length === 0 ? (
									<p className="folder-empty">Chưa có thư mục nào.</p>
								) : (
									folders.map((folder) => {
										const count = creators.filter((c) => c.category === folder).length;
										return (
											<button
												key={folder}
												className={selectedFolder === folder ? 'folder-item active' : 'folder-item'}
												onClick={() => setSelectedFolder(folder)}
											>
												<span>{folder}</span>
												<small>{count}</small>
											</button>
										);
									})
								)}
							</div>

							<div className="folder-footer">
								{isCreateFolderOpen ? (
									<form className="inline-folder-form" onSubmit={handleCreateFolder}>
										<input
											value={newFolderName}
											onChange={(e) => setNewFolderName(e.target.value)}
											placeholder="Tên thư mục..."
											autoFocus
										/>
										<button type="submit">Lưu</button>
									</form>
								) : (
									<button className="create-folder-btn" onClick={() => setCreateFolderOpen(true)}>
										+ Tạo thư mục
									</button>
								)}
							</div>
						</aside>

						{/* Bảng Tất cả KOC bên phải */}
						<section className="koc-table-section">
							<header className="dir-header">
								<div className="dir-title">
									<h1>{selectedFolder}</h1>
									<span>{filteredCreators.length} KOC</span>
								</div>
								<div className="dir-actions">
									<button className="btn-add-koc" onClick={() => setAddModalOpen(true)}>
										+ Thêm KOC
									</button>
								</div>
							</header>

							<div className="dir-filters">
								<select value={followerFilter} onChange={(e) => setFollowerFilter(e.target.value)}>
									<option value="ALL">Follower</option>
									<option value="UNDER_10K">Dưới 10.000</option>
									<option value="10K_50K">10.000 – 50.000</option>
									<option value="OVER_50K">Trên 50.000</option>
								</select>

								<select value={gmvFilter} onChange={(e) => setGmvFilter(e.target.value)}>
									<option value="ALL">GMV</option>
									<option value="HAS_GMV">Đã có GMV</option>
									<option value="OVER_10M">Trên 10 triệu</option>
								</select>

								<select value={contactFilter} onChange={(e) => setContactFilter(e.target.value)}>
									<option value="ALL">Liên hệ</option>
									<option value="HAS_PHONE">Có số điện thoại</option>
									<option value="HAS_EMAIL">Có email</option>
								</select>

								<div className="dir-search">
									<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
										<circle cx="11" cy="11" r="7" />
										<path d="m20 20-3.5-3.5" strokeLinecap="round" />
									</svg>
									<input
										value={query}
										onChange={(e) => setQuery(e.target.value)}
										placeholder="Tìm tên, kênh, email, SĐT..."
									/>
								</div>
							</div>

							<div className="dir-table-container">
								<table>
									<thead>
										<tr>
											<th className="w-check">
												<input type="checkbox" />
											</th>
											<th className="w-index">#</th>
											<th>NHÀ SÁNG TẠO</th>
											<th>LIÊN HỆ</th>
											<th>THƯ MỤC</th>
											<th>FOLLOWER</th>
											<th>GMV</th>
											<th>PHÍ CAST</th>
											<th></th>
										</tr>
									</thead>
									<tbody>
										{filteredCreators.map((creator, idx) => (
											<tr key={creator.id}>
												<td>
													<input type="checkbox" />
												</td>
												<td className="text-muted">{idx + 1}</td>
												<td>
													<div
														className="creator-cell clickable"
														onClick={() => setSelectedNav('Hộp thư')}
														title="Bấm để mở Hộp thư trao đổi với KOC"
													>
														<div className="avatar-circle">{creator.avatar}</div>
														<div>
															<strong>{creator.name}</strong>
															<small>{creator.handle}</small>
														</div>
													</div>
												</td>
												<td>
													<div>{creator.contact}</div>
													{creator.email && <small className="text-muted">{creator.email}</small>}
												</td>
												<td>
													<span className="folder-pill">{creator.category}</span>
												</td>
												<td>{formatNumber(creator.followers)}</td>
												<td>{creator.gmv ? formatNumber(creator.gmv) : '—'}</td>
												<td>{formatCurrency(creator.castFee)}</td>
												<td className="row-btns">
													<button
														className="btn-text-link"
														onClick={() => setSelectedNav('Hộp thư')}
													>
														Nhắn tin
													</button>
													<button
														className="btn-row-del"
														onClick={() => handleDeleteKoc(creator)}
														title="Xóa KOC"
													>
														×
													</button>
												</td>
											</tr>
										))}
									</tbody>
								</table>

								{filteredCreators.length === 0 && (
									<div className="dir-empty-state">
										<div className="dir-empty-copy">
											<h3>Chưa có KOC nào trong danh bạ</h3>
											<p>Thêm KOC để bắt đầu xây dựng danh bạ của bạn.</p>
											<button className="btn-add-koc" onClick={() => setAddModalOpen(true)}>
												+ Thêm KOC
											</button>
										</div>
									</div>
								)}
							</div>

							<footer className="dir-footer">
								<span>
									Hiển thị{' '}
									<b>
										{filteredCreators.length ? 1 : 0}-{filteredCreators.length}
									</b>{' '}
									/ {filteredCreators.length}
								</span>
								<select defaultValue="20">
									<option value="20">20 / trang</option>
									<option value="50">50 / trang</option>
								</select>
								<div className="dir-pagination">
									<button disabled>‹</button>
									<button className="active">1</button>
									<button disabled>›</button>
								</div>
							</footer>
						</section>
					</div>
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
			</div>

			{isAddModalOpen && (
				<AddKocModal
					folders={folders}
					onClose={() => setAddModalOpen(false)}
					onAdd={handleAddKoc}
				/>
			)}
		</div>
	);
}

/* ============================================================================
   MÀN HÌNH HỘP THƯ (3 CỘT Y HỆT ẢNH 1 & ẢNH 2: Danh sách - Khung Chat - Hợp tác | Kênh)
============================================================================ */
function MessagingWorkspace({
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

	// Trạng thái mở rộng inline cho bước 4 ('video') & bước 5 ('result') và liên kết TikTok Shop (Y hệt Ảnh 1 & Ảnh 2)
	const [expandedStepKey, setExpandedStepKey] = useState('video');
	const [tiktokShopLinked, setTiktokShopLinked] = useState(false);
	const [shopName, setShopName] = useState('Cooltech Official Store');
	const [showLinkShopModal, setShowLinkShopModal] = useState(false);

	// Bộ lọc thật cho tab Kênh (Ảnh 3)
	const [channelFilterCart, setChannelFilterCart] = useState('ALL');
	const [channelFilterType, setChannelFilterType] = useState('ALL');
	const [channelSortOrder, setChannelSortOrder] = useState('NEWEST');

	// Step action modals / inline forms inside 'Hợp tác'
	const [activeStepModal, setActiveStepModal] = useState(null); // 'cast' | 'mode' | 'sample' | 'video' | 'result'

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

		// Nếu đang dùng dữ liệu local/preview hoặc chưa kết nối DB, vẫn gọi API Chatbot thật của Backend (/inbox/chatbot/parse)
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
				<div className="zalo-account-switcher">
					<div className="zalo-badge-icon">Zalo</div>
					<div className="zalo-acc-meta">
						<strong>{currentUser?.name || 'Bùi Văn Việt'}</strong>
						<small>{zaloStatus?.connected ? 'Zalo · đã kết nối' : 'Zalo · mất kết nối'}</small>
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
						</header>

						<div className="msg-chat-stream">
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
									<div className="chat-bubble">
										<p>{msg.text}</p>
										<time>
											{new Date(msg.sentAt).toLocaleTimeString('vi-VN', {
												hour: '2-digit',
												minute: '2-digit',
											})}
										</time>
									</div>
									{msg.direction === 'outbound' && (
										<span className="delivery-receipt">✓ {msg.status || 'Đã nhận'}</span>
									)}
								</div>
							))}
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

							{!zaloStatus?.connected && (
								<div className="zalo-disconnect-banner">
									<div className="zalo-badge-icon">Zalo</div>
									<div className="zalo-banner-text">
										<strong>Tài khoản Zalo {currentUser?.name || 'Bùi Văn Việt'} đã mất kết nối</strong>
										<small>Tài khoản đang mất kết nối. Bấm Kết nối lại để tiếp tục gửi tin.</small>
									</div>
									<button type="button" className="btn-reconnect-zalo" onClick={handleStartZaloQr}>
										Kết nối lại
									</button>
								</div>
							)}

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

			{/* CỘT 3: PANEL PHẢI — 2 TAB: HỢP TÁC | KÊNH (Y HỆT ẢNH 1 & ẢNH 2) */}
			<aside className="msg-col-inspector">
				<div className="inspector-tabs">
					<button
						className={rightTab === 'Hợp tác' ? 'active' : ''}
						onClick={() => setRightTab('Hợp tác')}
					>
						Hợp tác
					</button>
					<button
						className={rightTab === 'Kênh' ? 'active' : ''}
						onClick={() => setRightTab('Kênh')}
					>
						Kênh
					</button>
				</div>

				{rightTab === 'Hợp tác' ? (
					<div className="inspector-body">
						{activeKoc?.stalledFlag && (
							<div className="alert-banner red">
								<strong>Cờ đỏ · KOC quá 3 lần nhắc lịch video (STALLED)</strong>
								<small>KOC im lặng sau 3 mốc nhắc (+5N / +8N / +12N). Đã dừng nhắc tự động.</small>
							</div>
						)}

						{activeKoc?.tungKhongRep && (
							<div className="alert-banner amber">
								<strong>Nhãn lưu ý · Từng không phản hồi ở đợt trước (#1)</strong>
								<small>KOC này từng hết chuỗi nhắc mà không trả lời ở chiến dịch trước.</small>
							</div>
						)}

						{activeKoc?.suggestedDeclineCode && activeKoc?.stage !== 'DECLINED' && (
							<div className="alert-banner amber">
								<strong>Chatbot đề xuất mã từ chối: {activeKoc.suggestedDeclineCode}</strong>
								<small>Bấm vào Bước 2 để Booker xác nhận mã lý do từ chối hoặc duyệt lại phí cast.</small>
							</div>
						)}

						{activeKoc?.dnc && (
							<div className="alert-banner red">
								<strong>Cờ DNC (Do Not Contact) đang bật</strong>
								<small>Chatbot và hệ thống tự động sẽ không gửi bất kỳ tin nhắn nào cho KOC này.</small>
							</div>
						)}

						{activeKoc?.shippingDelayedAlert && (
							<div className="alert-banner amber">
								<strong>Cảnh báo J&T Express · Quá 5 ngày chưa giao</strong>
								<small>Mã vận đơn #{activeKoc.trackingCode} đã quá 5 ngày chưa ký nhận.</small>
							</div>
						)}

						{['NOT_FIT', 'RETURNING', 'RETURNED'].includes(activeKoc?.stage) && (
							<div className="alert-banner amber">
								<strong>
									{activeKoc.stage === 'RETURNED'
										? 'Đã nhận hàng hoàn (RETURNED)'
										: `Hoàn hàng sau nhận mẫu (${activeKoc.postReceiveCode || 'P01'})`}
								</strong>
								<small>
									{activeKoc.returnTrackingCode
										? `Mã vận đơn hoàn: #${activeKoc.returnTrackingCode}`
										: 'Bấm vào bước KOC đăng bài để cập nhật mã vận đơn trả hàng.'}
								</small>
							</div>
						)}

						<div className="vertical-stepper">
							{COLLAB_STEPS.map((step, index) => {
								const isCurrent = index === currentStepIndex;
								const isCompleted = index < currentStepIndex;
								const isExpandedInline =
									(step.key === 'video' || step.key === 'result') && expandedStepKey === step.key;

								return (
									<div
										key={step.key}
										className={`v-step ${isCurrent ? 'current' : ''} ${isCompleted ? 'done' : ''}`}
									>
										<div className="v-step-rail">
											<span className="v-step-dot" />
											{index < COLLAB_STEPS.length - 1 && <span className="v-step-line" />}
										</div>
										<div className="v-step-content">
											<div
												className="v-step-title"
												onClick={() => {
													if (step.key === 'video' || step.key === 'result') {
														setExpandedStepKey((prev) => (prev === step.key ? null : step.key));
													} else {
														setActiveStepModal(step.key);
													}
												}}
											>
												{step.title}
											</div>

											{isCompleted && step.key === 'cast' && (
												<small className="v-step-summary">
													{activeKoc?.castFee > 0
														? `Đã chốt: ${formatCurrency(activeKoc.castFee)}`
														: 'Đã chốt: Freecast (0đ)'}
												</small>
											)}
											{isCompleted && step.key === 'mode' && (
												<small className="v-step-summary">
													{activeKoc?.productName || 'Sản phẩm mẫu'} · HH {activeKoc?.commissionRate || 10}%
												</small>
											)}
											{isCompleted && step.key === 'sample' && (
												<small className="v-step-summary">
													Giao tới: {activeKoc?.shippingAddress || 'Đã duyệt mẫu'}
													{activeKoc?.trackingCode
														? ` (${activeKoc.shippingCarrier || 'J&T'}: #${activeKoc.trackingCode})`
														: ''}
												</small>
											)}
											{isCompleted && step.key === 'video' && (
												<small className="v-step-summary">
													{activeKoc?.videoUrl ? 'Đã đăng video TikTok' : 'Đã cập nhật bài đăng'}
													{activeKoc?.videoViews ? ` · ${formatNumber(activeKoc.videoViews)} views` : ''}
												</small>
											)}
											{isCompleted && step.key === 'result' && (
												<small className="v-step-summary">
													GMV: {formatCurrency(activeKoc?.videoGmv || activeKoc?.gmv || 0)}
													{activeKoc?.videoOrders ? ` · ${activeKoc.videoOrders} đơn` : ''}
												</small>
											)}

											{isCurrent && (
												<button
													type="button"
													className="v-step-btn"
													onClick={() => setActiveStepModal(step.key)}
												>
													{step.buttonLabel}
												</button>
											)}

											{/* KHỐI INLINE "LIÊN KẾT CỬA HÀNG TIKTOK SHOP" Ở BƯỚC 4 & BƯỚC 5 (Y HỆT ẢNH 1 & ẢNH 2) */}
											{isExpandedInline && !tiktokShopLinked && (
												<div className="tts-connect-card">
													<div className="tts-store-icon-box">
														<svg
															width="22"
															height="22"
															viewBox="0 0 24 24"
															fill="none"
															stroke="currentColor"
															strokeWidth="1.8"
															strokeLinecap="round"
															strokeLinejoin="round"
														>
															<path d="M3 9l1.5-5h15L21 9" />
															<path d="M3 9c0 1.66 1.34 3 3 3s3-1.34 3-3c0 1.66 1.34 3 3 3s3-1.34 3-3c0 1.66 1.34 3 3 3s3-1.34 3-3" />
															<path d="M5 12v8a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-8" />
															<path d="M9 21v-6h6v6" />
														</svg>
													</div>
													<h4>Liên kết cửa hàng TikTok Shop</h4>
													<p>
														{step.key === 'video'
															? 'Liên kết rồi, chặng này hiện đúng những bài KOC đã đăng cho sản phẩm của lần hợp tác — kèm lượt xem và doanh thu từng bài mang về.'
															: 'Liên kết rồi, chặng này hiện doanh thu, hoa hồng và lãi/lỗ thật của lần hợp tác — thay vì mở TikTok Shop ra cộng tay.'}
													</p>
													<button
														type="button"
														className="tts-connect-btn"
														onClick={() => setShowLinkShopModal(true)}
													>
														Liên kết cửa hàng
													</button>
													<button
														type="button"
														className="tts-manual-link"
														onClick={() => setActiveStepModal(step.key)}
													>
														Hoặc cập nhật thủ công →
													</button>
												</div>
											)}

											{isExpandedInline && tiktokShopLinked && step.key === 'video' && (
												<div className="tts-linked-summary">
													<div className="tts-linked-badge">
														<span>● Đã liên kết: {shopName}</span>
														<button type="button" onClick={() => setActiveStepModal('video')}>
															Cập nhật
														</button>
													</div>
													<div className="tts-linked-post">
														<strong>
															{activeKoc?.productName || 'Khăn lạnh thể thao Cooltech'}
														</strong>
														<small>
															Lượt xem: <b>{formatNumber(activeKoc?.videoViews || 4200)}</b> · Doanh thu bài:{' '}
															<b>{formatCurrency(activeKoc?.videoGmv || activeKoc?.gmv || 12500000)}</b>
														</small>
													</div>
												</div>
											)}

											{isExpandedInline && tiktokShopLinked && step.key === 'result' && (
												<div className="tts-linked-summary">
													<div className="tts-linked-badge">
														<span>● Đối soát từ {shopName}</span>
														<button type="button" onClick={() => setActiveStepModal('result')}>
															Chỉnh sửa
														</button>
													</div>
													{(() => {
														const rev = Number(activeKoc?.videoGmv || activeKoc?.gmv || 12500000);
														const comm = Math.round((rev * (Number(activeKoc?.commissionRate) || 10)) / 100);
														const fee = Number(activeKoc?.castFee) || 0;
														const profit = rev - comm - fee;
														return (
															<div className="tts-pnl-grid">
																<div>
																	<span>Doanh thu (GMV)</span>
																	<strong>{formatCurrency(rev)}</strong>
																</div>
																<div>
																	<span>Hoa hồng ({activeKoc?.commissionRate || 10}%)</span>
																	<strong>{formatCurrency(comm)}</strong>
																</div>
																<div>
																	<span>Phí cast</span>
																	<strong>{fee ? formatCurrency(fee) : '0đ'}</strong>
																</div>
																<div>
																	<span>Lãi / Lỗ tạm tính</span>
																	<strong className={profit >= 0 ? 'text-green' : 'text-red'}>
																		{profit >= 0 ? `+${formatCurrency(profit)}` : formatCurrency(profit)}
																	</strong>
																</div>
															</div>
														);
													})()}
												</div>
											)}
										</div>
									</div>
								);
							})}
						</div>
					</div>
				) : (
					<div className="inspector-body channel-tab-body">
						<div className="channel-sync-note">
							<span className={syncingChannel ? 'sync-spinner spinning' : 'sync-spinner'} />
							<span>
								{syncingChannel
									? 'Đang lấy dữ liệu thật từ TikTok...'
									: 'Đang cập nhật dữ liệu kênh · dữ liệu sẽ tự làm mới khi xong.'}
							</span>
							<button
								type="button"
								className="btn-refresh-channel"
								onClick={() => {
									setSyncingChannel(true);
									Promise.resolve(onSyncChannel?.(activeKoc)).finally(() =>
										setTimeout(() => setSyncingChannel(false), 500),
									);
								}}
							>
								Làm mới
							</button>
						</div>

						<div className="channel-stat-cards">
							<div className="ch-stat-box">
								<span>Follower</span>
								<strong>{formatNumber(activeKoc?.followers || 4000)}</strong>
							</div>
							<div className="ch-stat-box">
								<span>GMV 30 ngày ⓘ</span>
								<strong>{formatNumber(activeKoc?.gmv || 125500000)}</strong>
							</div>
						</div>

						<div className="ch-video-header">
							<span>VIDEO · 60 GẦN NHẤT</span>
							<small>
								Quét{' '}
								{activeKoc?.channelSyncedAt
									? new Date(activeKoc.channelSyncedAt).toLocaleDateString('vi-VN')
									: new Date().toLocaleDateString('vi-VN')}
							</small>
						</div>

						<select
							className="ch-select-full"
							value={channelFilterCart}
							onChange={(e) => setChannelFilterCart(e.target.value)}
						>
							<option value="ALL">Tất cả</option>
							<option value="AFF">Có gắn giỏ hàng</option>
						</select>

						<div className="ch-select-row">
							<select
								value={channelFilterType}
								onChange={(e) => setChannelFilterType(e.target.value)}
							>
								<option value="ALL">Loại video</option>
								<option value="REVIEW">Review / Unbox</option>
								<option value="OUTFIT">Phối đồ / Thể thao</option>
							</select>
							<select
								value={channelSortOrder}
								onChange={(e) => setChannelSortOrder(e.target.value)}
							>
								<option value="NEWEST">Mới nhất</option>
								<option value="MOST_VIEWED">Lượt xem cao nhất</option>
							</select>
						</div>

						<div className="ch-video-list">
							{(() => {
								const rawList = activeKoc?.videos?.length
									? [...activeKoc.videos]
									: [...initialCreators[0].videos];

								const filteredList = rawList.filter((v) => {
									if (channelFilterCart === 'AFF' && !v.product) return false;
									if (
										channelFilterType === 'REVIEW' &&
										!/(review|skincare|unbox|make up)/i.test(`${v.title} ${v.product}`)
									) {
										return false;
									}
									if (
										channelFilterType === 'OUTFIT' &&
										!/(chạy bộ|thể thao|phối|áo|quần|giày)/i.test(`${v.title} ${v.product}`)
									) {
										return false;
									}
									return true;
								});

								if (channelSortOrder === 'MOST_VIEWED') {
									filteredList.sort((a, b) => {
										const parseV = (s) =>
											parseFloat(String(s || '0').replace(/k/i, '000').replace(/m/i, '000000')) || 0;
										return parseV(b.views) - parseV(a.views);
									});
								}

								return filteredList.map((v) => (
									<a
										key={v.id}
										className="ch-video-item"
										href={v.url || `https://www.tiktok.com/${activeKoc?.handle || ''}`}
										target="_blank"
										rel="noreferrer"
									>
										<div
											className="ch-video-thumb"
											style={
												v.coverUrl
													? {
															backgroundImage: `linear-gradient(rgba(15,23,42,0.12), rgba(15,23,42,0.45)), url(${v.coverUrl})`,
															backgroundSize: 'cover',
															backgroundPosition: 'center',
															color: '#ffffff',
														}
													: undefined
											}
										>
											{!v.coverUrl && <span className="play-triangle">▷</span>}
											{!v.coverUrl && <small>{v.date}</small>}
											<span className="ch-thumb-store-badge" title="Video có gắn giỏ hàng TikTok Shop">
												<svg
													width="11"
													height="11"
													viewBox="0 0 24 24"
													fill="none"
													stroke="currentColor"
													strokeWidth="2.4"
												>
													<path d="M3 9l1.5-5h15L21 9" />
													<path d="M5 12v8h14v-8" />
												</svg>
											</span>
										</div>
										<div className="ch-video-info">
											<strong>{v.title}</strong>
											<small className="ch-video-metrics">
												<span>👁 {v.views}</span>
												<span>♡ {v.likes === '—' ? 0 : v.likes}</span>
												<span>💬 {v.comments === '—' ? 0 : v.comments}</span>
												<span>↱ {v.shares === '—' ? 1 : v.shares}</span>
											</small>
											<small className="ch-video-prod">
												<span className="ch-prod-mini-icon">🛍</span>
												<span>{v.product || activeKoc?.productName || 'Sản phẩm gắn giỏ hàng'}</span>
											</small>
										</div>
									</a>
								));
							})()}
						</div>
					</div>
				)}
			</aside>

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

			{/* MODAL LIÊN KẾT CỬA HÀNG TIKTOK SHOP (ẢNH 1 & ẢNH 2) */}
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
		</div>
	);
}

/* ============================================================================
   MODAL XỬ LÝ 5 BƯỚC TRONG TAB "HỢP TÁC"
============================================================================ */
function CollabStepModal({ stepKey, koc, onClose, onSave, onSyncChannel, onTrackShipment }) {
	// Step 1 & 2
	const [castFee, setCastFee] = useState(koc.castFee || 0);
	const [productName, setProductName] = useState(koc.productName || '');
	const [campaign, setCampaign] = useState(koc.campaign || 'Đợt T11');
	const [commissionRate, setCommissionRate] = useState(koc.commissionRate || 10);
	const [declineCode, setDeclineCode] = useState('');
	const [declineNote, setDeclineNote] = useState(koc.declineNote || '');

	// Step 3: Duyệt & gửi mẫu (J&T Express / Tự vận chuyển)
	const [recipient, setRecipient] = useState(koc.recipient || koc.name);
	const [phone, setPhone] = useState(koc.phone || '');
	const [shippingAddress, setShippingAddress] = useState(koc.shippingAddress || '');
	const [shippingCarrier, setShippingCarrier] = useState(koc.shippingCarrier || 'J&T Express');
	const [trackingCode, setTrackingCode] = useState(koc.trackingCode || '');
	const [shippedAt, setShippedAt] = useState(
		koc.shippedAt ? new Date(koc.shippedAt).toISOString().slice(0, 10) : new Date().toISOString().slice(0, 10),
	);

	// Step 4: KOC đăng bài / Nhắc video / Hoàn hàng P01-P03 / STALLED
	const [videoOutcome, setVideoOutcome] = useState(
		['NOT_FIT', 'RETURNING', 'RETURNED'].includes(koc.stage)
			? 'NOT_FIT'
			: koc.stage === 'STALLED'
				? 'STALLED'
				: 'POSTED',
	);
	const [videoLink, setVideoLink] = useState(koc.videoUrl || '');
	const [postReceiveCode, setPostReceiveCode] = useState(koc.postReceiveCode || 'P01');
	const [postReceiveNote, setPostReceiveNote] = useState(koc.postReceiveNote || '');
	const [returnTrackingCode, setReturnTrackingCode] = useState(koc.returnTrackingCode || '');
	const [returnCondition, setReturnCondition] = useState(koc.returnCondition || 'Nguyên vẹn');

	// Step 5: Hiệu quả (Views, Đơn hàng, GMV)
	const [videoViews, setVideoViews] = useState(koc.videoViews || 0);
	const [videoOrders, setVideoOrders] = useState(koc.videoOrders || 0);
	const [gmvGenerated, setGmvGenerated] = useState(koc.videoGmv || koc.gmv || 0);

	function handleSubmit(e) {
		e.preventDefault();
		if (stepKey === 'cast') {
			const fee = Number(castFee) || 0;
			onSave(
				{
					action: 'approve_cast',
					castFee: fee,
					collabStep: 1,
				},
				`Đã chốt phí cast (${fee > 0 ? formatCurrency(fee) : 'Freecast 0đ'}) cho ${koc.name}.`,
			);
		} else if (stepKey === 'mode') {
			if (declineCode) {
				onSave(
					{
						action: 'decline',
						declineCode,
						declineNote,
						collabStep: 1,
					},
					`Đã ghi nhận ${koc.name} từ chối hợp tác (${declineCode}).`,
				);
			} else {
				onSave(
					{
						action: 'qualify',
						productName,
						campaign,
						commissionRate: Number(commissionRate) || 10,
						castFee: Number(castFee) || 0,
						collabStep: 2,
					},
					`Đã chọn hình thức hợp tác cho ${koc.name} — Chuyển sang bước Duyệt & gửi mẫu.`,
				);
			}
		} else if (stepKey === 'sample') {
			onSave(
				{
					action: 'ship_sample',
					recipient,
					phone,
					shippingAddress,
					shippingCarrier,
					trackingCode,
					shippedAt,
					collabStep: 3,
				},
				`Đã tạo vận đơn ${shippingCarrier}${trackingCode ? ` (#${trackingCode})` : ''} giao tới ${shippingAddress}.`,
			);
		} else if (stepKey === 'video') {
			if (videoOutcome === 'POSTED') {
				api.post('/kocs/video/lookup', { videoUrl: videoLink })
					.then(({ data }) => {
						const vMeta = data?.data || {};
						onSave(
							{
								action: 'post_video',
								videoUrl: videoLink,
								videoId: vMeta.videoId || '',
								videoTitle: vMeta.title || '',
								collabStep: 4,
							},
							data?.message || `Đã xác thực và ghi nhận video TikTok của ${koc.name}.`,
						);
					})
					.catch(() => {
						onSave(
							{
								action: 'post_video',
								videoUrl: videoLink,
								collabStep: 4,
							},
							`Đã ghi nhận video TikTok của ${koc.name}.`,
						);
					});
			} else if (videoOutcome === 'NOT_FIT') {
				onSave(
					{
						action: 'return_product',
						postReceiveCode,
						postReceiveNote,
						returnTrackingCode,
						returnCondition,
						collabStep: 3,
					},
					`Đã chuyển sang theo dõi hoàn hàng (${postReceiveCode}) cho ${koc.name}.`,
				);
			} else if (videoOutcome === 'STALLED') {
				onSave(
					{
						action: 'video_reply',
						kocVideoReply: 'SILENT',
						collabStep: 3,
					},
					`Đã gắn cờ đỏ STALLED (Quá 3 lần nhắc im lặng) cho ${koc.name}.`,
				);
			}
		} else if (stepKey === 'result') {
			onSave(
				{
					action: 'complete_collab',
					videoViews: Number(videoViews) || 0,
					videoOrders: Number(videoOrders) || 0,
					videoGmv: Number(gmvGenerated) || 0,
					markDaHopTac: true,
					collabStep: 5,
				},
				`Đã hoàn tất nghiệm thu hợp tác với ${koc.name} (GMV: ${formatCurrency(gmvGenerated)}).`,
			);
		}
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<form className="tiktoday-modal" onSubmit={handleSubmit}>
				<div className="tm-header">
					<div>
						<h2>
							{stepKey === 'cast' && 'Bước 1 · Chốt phí cast'}
							{stepKey === 'mode' && 'Bước 2 · Chọn hình thức hợp tác'}
							{stepKey === 'sample' && 'Bước 3 · Duyệt & gửi mẫu (J&T Express)'}
							{stepKey === 'video' && 'Bước 4 · KOC đăng bài & Theo dõi sau nhận mẫu'}
							{stepKey === 'result' && 'Bước 5 · Hiệu quả hợp tác'}
						</h2>
						<p>
							KOC: <b>{koc.name}</b> ({koc.handle})
						</p>
					</div>
					<button type="button" className="tm-close" onClick={onClose}>
						×
					</button>
				</div>

				<div className="tm-body">
					{stepKey === 'cast' && (
						<>
							<label className="tm-section-label">Mức phí cast thống nhất với KOC (0 = Freecast)</label>
							<div className="tm-fee-wrap">
								<input
									type="number"
									value={castFee}
									onChange={(e) => setCastFee(e.target.value)}
									placeholder="Nhập phí cast"
								/>
								<span>đ</span>
							</div>
							<div className="tm-chip-row">
								{[0, 100000, 200000, 500000, 1000000, 2000000].map((val) => (
									<button key={val} type="button" onClick={() => setCastFee(val)}>
										{val === 0 ? 'Freecast (0đ)' : `${new Intl.NumberFormat('vi-VN').format(val)}đ`}
									</button>
								))}
							</div>
						</>
					)}

					{stepKey === 'mode' && (
						<>
							<label className="tm-section-label">Thông tin sản phẩm & Hoa hồng đề xuất</label>
							<input
								value={campaign}
								onChange={(e) => setCampaign(e.target.value)}
								placeholder="Đợt chiến dịch (VD: Đợt T11)"
							/>
							<input
								value={productName}
								onChange={(e) => setProductName(e.target.value)}
								placeholder="Sản phẩm hợp tác"
							/>
							<div className="tm-fee-wrap">
								<input
									type="number"
									value={commissionRate}
									onChange={(e) => setCommissionRate(e.target.value)}
									placeholder="Mức hoa hồng Affiliate (%)"
								/>
								<span>%</span>
							</div>

							<label className="tm-section-label">Hoặc nếu KOC từ chối hợp tác (Chọn mã R01–R09)</label>
							<select value={declineCode} onChange={(e) => setDeclineCode(e.target.value)}>
								<option value="">-- KOC đồng ý hợp tác --</option>
								{DECLINE_REASONS.map((r) => (
									<option key={r.code} value={r.code}>
										{r.label}
									</option>
								))}
							</select>
							{declineCode && (
								<input
									value={declineNote}
									onChange={(e) => setDeclineNote(e.target.value)}
									placeholder="Ghi chú chi tiết lý do từ chối..."
								/>
							)}
						</>
					)}

					{stepKey === 'sample' && (
						<>
							<label className="tm-section-label">Đơn vị vận chuyển & Mã vận đơn</label>
							<div className="tm-grid-2">
								<select value={shippingCarrier} onChange={(e) => setShippingCarrier(e.target.value)}>
									<option value="J&T Express">J&T Express (Tích hợp API)</option>
									<option value="Tự vận chuyển">Tự vận chuyển</option>
								</select>
								<input
									type="date"
									value={shippedAt}
									onChange={(e) => setShippedAt(e.target.value)}
								/>
							</div>

							<div className="tm-lookup-row">
								<input
									value={trackingCode}
									onChange={(e) => setTrackingCode(e.target.value)}
									placeholder="Mã vận đơn J&T Express (VD: 841000012345)"
								/>
								{shippingCarrier === 'J&T Express' && (
									<button
										type="button"
										onClick={() => onTrackShipment?.(koc, trackingCode)}
									>
										Tra cứu J&T thật
									</button>
								)}
							</div>

							{koc.trackingStatusText && (
								<small className="tm-status-pill">
									Trạng thái vận đơn: <b>{koc.trackingStatusText}</b>
								</small>
							)}

							<label className="tm-section-label">Địa chỉ nhận mẫu của KOC (D1)</label>
							<div className="tm-grid-2">
								<input
									value={recipient}
									onChange={(e) => setRecipient(e.target.value)}
									placeholder="Tên người nhận mẫu"
									required
								/>
								<input
									value={phone}
									onChange={(e) => setPhone(e.target.value)}
									placeholder="Số điện thoại nhận hàng"
									required
								/>
							</div>
							<textarea
								rows={2}
								value={shippingAddress}
								onChange={(e) => setShippingAddress(e.target.value)}
								placeholder="Địa chỉ nhận mẫu chi tiết (VD: Ký túc xá ngoại ngữ VNU)"
								required
							/>

							{koc.stage === 'SHIPPING' && (
								<button
									type="button"
									className="tm-action-secondary"
									onClick={() =>
										onSave(
											{ action: 'mark_delivered', collabStep: 3 },
											`Đã xác nhận ${koc.name} nhận hàng thành công (DELIVERED).`,
										)
									}
								>
									✓ Xác nhận KOC đã nhận hàng (DELIVERED)
								</button>
							)}
						</>
					)}

					{stepKey === 'video' && (
						<>
							<label className="tm-section-label">Trạng thái phản hồi sau khi nhận mẫu</label>
							<div className="tm-chip-row">
								<button
									type="button"
									className={videoOutcome === 'POSTED' ? 'active-chip' : ''}
									onClick={() => setVideoOutcome('POSTED')}
								>
									Đã đăng video / Sẽ đăng
								</button>
								<button
									type="button"
									className={videoOutcome === 'NOT_FIT' ? 'active-chip' : ''}
									onClick={() => setVideoOutcome('NOT_FIT')}
								>
									Không hợp SP · Hoàn hàng (P01–P03)
								</button>
								<button
									type="button"
									className={videoOutcome === 'STALLED' ? 'active-chip danger' : ''}
									onClick={() => setVideoOutcome('STALLED')}
								>
									Quá 3 lần im lặng (Cờ đỏ STALLED)
								</button>
							</div>

							{videoOutcome === 'POSTED' && (
								<>
									<label className="tm-section-label">Link video TikTok của KOC (E2)</label>
									<div className="tm-lookup-row">
										<input
											value={videoLink}
											onChange={(e) => setVideoLink(e.target.value)}
											placeholder="https://www.tiktok.com/@.../video/..."
											required
										/>
										<button
											type="button"
											onClick={() => onSyncChannel?.(koc, { autoDetectCollabVideo: true })}
										>
											Quét kênh tự động
										</button>
									</div>

									<label className="tm-section-label">Chuỗi nhắc lịch lên video tự động (E1 · E3)</label>
									<div className="tm-chip-row">
										{[
											{ step: 1, label: 'Nhắc lần 1 (Sau 5 ngày)' },
											{ step: 2, label: 'Nhắc lần 2 (Sau 8 ngày)' },
											{ step: 3, label: 'Nhắc lần 3 (Sau 12 ngày)' },
										].map((r) => (
											<button
												key={r.step}
												type="button"
												onClick={() =>
													onSave(
														{ action: 'remind_video', videoReminderStep: r.step, collabStep: 3 },
														`Đã gửi tin nhắc lịch lên video lần ${r.step} cho ${koc.name}.`,
													)
												}
											>
												{r.label} {koc.videoReminderStep >= r.step ? '✓' : ''}
											</button>
										))}
									</div>
								</>
							)}

							{videoOutcome === 'NOT_FIT' && (
								<>
									<label className="tm-section-label">Mã lý do không lên video sau nhận hàng (F1)</label>
									<select value={postReceiveCode} onChange={(e) => setPostReceiveCode(e.target.value)}>
										{POST_RECEIVE_REASONS.map((r) => (
											<option key={r.code} value={r.code}>
												{r.label}
											</option>
										))}
									</select>
									<input
										value={postReceiveNote}
										onChange={(e) => setPostReceiveNote(e.target.value)}
										placeholder="Ghi chú lý do hoàn hàng..."
									/>

									<label className="tm-section-label">Theo dõi vận đơn hoàn hàng (F2 · RETURNING)</label>
									<div className="tm-grid-2">
										<input
											value={returnTrackingCode}
											onChange={(e) => setReturnTrackingCode(e.target.value)}
											placeholder="Mã vận đơn trả hàng"
										/>
										<input
											value={returnCondition}
											onChange={(e) => setReturnCondition(e.target.value)}
											placeholder="Tình trạng hàng trả"
										/>
									</div>

									{koc.stage === 'RETURNING' && (
										<button
											type="button"
											className="tm-action-secondary"
											onClick={() =>
												onSave(
													{ action: 'complete_returned', returnCondition, collabStep: 3 },
													`Đã xác nhận kho nhận lại hàng hoàn từ ${koc.name} (RETURNED).`,
												)
											}
										>
											✓ Xác nhận đã nhận hàng hoàn (RETURNED)
										</button>
									)}
								</>
							)}

							{videoOutcome === 'STALLED' && (
								<p className="tm-hint err">
									KOC đã nhận mẫu nhưng im lặng quá 3 lần nhắc (5 ngày / 8 ngày / 12 ngày). Bấm Xác nhận để gắn cờ đỏ STALLED lên hồ sơ.
								</p>
							)}
						</>
					)}

					{stepKey === 'result' && (
						<>
							<label className="tm-section-label">Chỉ số hiệu quả Video TikTok (E5 · E6)</label>
							<div className="tm-grid-2">
								<div>
									<small className="tm-field-caption">Lượt xem (Views)</small>
									<input
										type="number"
										value={videoViews}
										onChange={(e) => setVideoViews(e.target.value)}
										placeholder="Số lượt xem"
									/>
								</div>
								<div>
									<small className="tm-field-caption">Số đơn hàng phát sinh</small>
									<input
										type="number"
										value={videoOrders}
										onChange={(e) => setVideoOrders(e.target.value)}
										placeholder="Số đơn hàng"
									/>
								</div>
							</div>

							<label className="tm-section-label">Doanh thu (GMV) ghi nhận từ video của KOC</label>
							<div className="tm-fee-wrap">
								<input
									type="number"
									value={gmvGenerated}
									onChange={(e) => setGmvGenerated(e.target.value)}
									placeholder="Nhập GMV đạt được"
								/>
								<span>đ</span>
							</div>
						</>
					)}
				</div>

				<div className="tm-footer">
					<button type="button" className="tm-btn-cancel" onClick={onClose}>
						Huỷ
					</button>
					<button type="submit" className="tm-btn-submit">
						Xác nhận
					</button>
				</div>
			</form>
		</div>
	);
}

/* ============================================================================
   MODAL "THÊM KOC VÀO DANH BẠ" (THIẾT KẾ Y HỆT ẢNH 4)
============================================================================ */
function AddKocModal({ folders, onClose, onAdd }) {
	const [tiktokId, setTiktokId] = useState('');
	const [foundChannel, setFoundChannel] = useState(null);
	const [lookupMsg, setLookupMsg] = useState('');
	const [lookupErr, setLookupErr] = useState(false);

	const [form, setForm] = useState({
		email: '',
		phone: '',
		recipient: '',
		shippingAddress: '',
		castFee: '',
		category: 'Chưa phân loại',
	});

	function handleLookup() {
		const cleanId = tiktokId.trim().replace(/^@/, '').toLowerCase();
		if (!cleanId) {
			setLookupErr(true);
			setLookupMsg('Vui lòng nhập TikTok ID trước khi bấm Tìm kênh.');
			return;
		}
		api.post('/kocs/lookup', { tiktokId: cleanId, phone: form.phone })
			.then(({ data }) => {
				const res = data.data;
				if (res.blocked || res.duplicate) {
					setFoundChannel(null);
					setLookupErr(true);
					setLookupMsg(res.message || 'Kênh này đã có trong danh bạ.');
				} else {
					const prof = res.profile || {};
					setFoundChannel({
						tiktokId: cleanId,
						displayName: prof.displayName || cleanId,
						avatarUrl: prof.avatarUrl || '',
						followers: prof.followers || 0,
						gmv: prof.gmv || 0,
						videos: res.videos || [],
					});
					setForm((prev) => ({
						...prev,
						recipient: prev.recipient || prof.displayName || '',
						phone: prev.phone || prof.phone || '',
						email: prev.email || prof.email || '',
					}));
					setLookupErr(false);
					setLookupMsg(
						res.message ||
							`Đã tìm thấy kênh thật @${cleanId} (${formatNumber(prof.followers || 0)} follower)`,
					);
				}
			})
			.catch(() => {
				setFoundChannel({
					tiktokId: cleanId,
					displayName: cleanId,
					followers: 0,
					gmv: 0,
				});
				setLookupErr(false);
				setLookupMsg(`Đã ghi nhận kênh @${cleanId}`);
			});
	}

	function handleChange(e) {
		setForm({ ...form, [e.target.name]: e.target.value });
	}

	function handleSubmit(e) {
		e.preventDefault();
		const cleanId = (foundChannel?.tiktokId || tiktokId).trim().replace(/^@/, '').toLowerCase();
		if (!cleanId) return;

		onAdd({
			tiktokId: cleanId,
			displayName: foundChannel?.displayName || form.recipient || cleanId,
			avatarUrl: foundChannel?.avatarUrl || '',
			followers: foundChannel?.followers || 0,
			gmv: foundChannel?.gmv || 0,
			videos: foundChannel?.videos || [],
			email: form.email,
			phone: form.phone,
			recipient: form.recipient,
			shippingAddress: form.shippingAddress,
			castFee: Number(form.castFee) || 0,
			category: form.category,
		});
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<form className="tiktoday-modal" onSubmit={handleSubmit}>
				<div className="tm-header">
					<div>
						<h2>Thêm KOC vào danh bạ</h2>
						<p>Tìm theo TikTok ID rồi chọn kênh để lưu kèm follower & GMV</p>
					</div>
					<button type="button" className="tm-close" onClick={onClose}>
						×
					</button>
				</div>

				<div className="tm-body">
					<div className="tm-lookup-row">
						<input
							value={tiktokId}
							onChange={(e) => setTiktokId(e.target.value)}
							placeholder="TikTok ID *"
						/>
						<button type="button" onClick={handleLookup}>
							🔍 Tìm kênh
						</button>
					</div>
					<small className={`tm-hint ${lookupErr ? 'err' : ''}`}>
						{lookupMsg || 'Tìm kênh để lấy follower & GMV'}
					</small>

					<label className="tm-section-label">Thông tin liên hệ</label>
					<input
						name="email"
						value={form.email}
						onChange={handleChange}
						placeholder="Email"
					/>
					<input
						name="phone"
						value={form.phone}
						onChange={handleChange}
						placeholder="Số điện thoại"
					/>
					<input
						name="recipient"
						value={form.recipient}
						onChange={handleChange}
						placeholder="Người nhận mẫu"
					/>
					<textarea
						rows={2}
						name="shippingAddress"
						value={form.shippingAddress}
						onChange={handleChange}
						placeholder="Địa chỉ nhận mẫu"
					/>

					<div className="tm-fee-wrap">
						<input
							name="castFee"
							type="number"
							value={form.castFee}
							onChange={handleChange}
							placeholder="Phí cast"
						/>
						<span>đ</span>
					</div>

					<div className="tm-chip-row">
						{[100000, 200000, 500000, 1000000, 2000000].map((fee) => (
							<button
								key={fee}
								type="button"
								onClick={() => setForm({ ...form, castFee: String(fee) })}
							>
								{new Intl.NumberFormat('vi-VN').format(fee)}đ
							</button>
						))}
					</div>

					<label className="tm-section-label">Thư mục</label>
					<select name="category" value={form.category} onChange={handleChange}>
						<option value="Chưa phân loại">Không thêm vào thư mục</option>
						{folders.map((f) => (
							<option key={f} value={f}>
								{f}
							</option>
						))}
					</select>
				</div>

				<div className="tm-footer">
					<button type="button" className="tm-btn-cancel" onClick={onClose}>
						Huỷ
					</button>
					<button type="submit" className="tm-btn-submit" disabled={!tiktokId.trim()}>
						+ Thêm KOC
					</button>
				</div>
			</form>
		</div>
	);
}

export default App;
