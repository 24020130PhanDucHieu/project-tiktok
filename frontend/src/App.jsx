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

const MESSAGE_TEMPLATES = [
	{
		id: 'invite-1',
		label: 'Mẫu 1: Lời mời hợp tác lần đầu (Brief chuẩn)',
		build: (koc) =>
			`Chúng mình có theo dõi kênh ${koc?.handle || '@koc'} và nhận thấy định hướng nội dung, hình ảnh của bạn khá phù hợp với thương hiệu bên mình.\nSau khi nhận sản phẩm ${koc?.productName || 'mẫu'}, bạn có thể lên video review/giới thiệu sản phẩm theo phong cách bạn thường làm. Mức hoa hồng đề xuất là ${koc?.commissionRate || 10}% cho đơn tự nhiên.`,
	},
	{
		id: 'remind-2d',
		label: 'Mẫu 2: Nhắc phản hồi sau 48h (Zalo)',
		build: (koc) =>
			`Chào ${koc?.name || 'bạn'}, bên mình có gửi lời mời hợp tác chiến dịch ${koc?.campaign || 'mới'} cách đây 2 ngày. Bạn xem qua thông tin rồi phản hồi giúp mình nhé!`,
	},
	{
		id: 'ask-address',
		label: 'Mẫu 3: Xin địa chỉ gửi sản phẩm mẫu',
		build: (koc) =>
			`Cảm ơn ${koc?.name || 'bạn'} đã đồng ý hợp tác. Bạn cho mình xin Tên người nhận, Số điện thoại và Địa chỉ chi tiết để bên mình lên đơn gửi mẫu nhé!`,
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
			return 3;
		case 'VIDEO_POSTED':
		case 'COMPLETED':
			return 4;
		default:
			return 0;
	}
}

function stageBadgeText(stage) {
	switch (stage) {
		case 'AGREED':
			return 'Chờ gửi mẫu';
		case 'SHIPPING':
		case 'DELIVERED':
			return 'Chờ đăng bài';
		case 'VIDEO_POSTED':
		case 'COMPLETED':
			return 'Đã lên bài';
		case 'REPLIED':
			return 'Đang trao đổi';
		case 'DECLINED':
			return 'Đã từ chối';
		default:
			return 'Chưa có việc';
	}
}

function mapItemToCreator(item) {
	const rawName = (item.displayName || item.name || 'KOC').replace(/[^\p{L}\p{N}\s._-]/gu, '').trim() || item.displayName || 'KOC';
	const stage = item.stage || 'SOURCED';
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
		collabStep: item.collabStep !== undefined ? item.collabStep : stageToCollabStep(stage),
		avatar: rawName.slice(0, 2).toUpperCase(),
		videos: item.videos || initialCreators[0].videos,
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
						<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2">
							<path d="M2 12h4l3-9 4 18 3-9h6" strokeLinecap="round" strokeLinejoin="round" />
						</svg>
					</div>
					<div className="sb-brand-text">
						<span className="sb-title">Tiktoday</span>
						<span className="sb-tag">BOOKING</span>
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
							<small>{currentUser?.email || 'ducviet@tiktoday.vn'}</small>
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
						<div className="vertical-stepper">
							{COLLAB_STEPS.map((step, index) => {
								const isCurrent = index === currentStepIndex;
								const isCompleted = index < currentStepIndex;
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
												onClick={() => setActiveStepModal(step.key)}
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
										</div>
									</div>
								);
							})}
						</div>
					</div>
				) : (
					<div className="inspector-body channel-tab-body">
						<div className="channel-sync-note">
							<span className="sync-spinner" />
							<span>Đang cập nhật dữ liệu kênh · dữ liệu sẽ tự làm mới khi xong.</span>
						</div>

						<div className="channel-stat-cards">
							<div className="ch-stat-box">
								<span>Follower</span>
								<strong>{formatNumber(activeKoc?.followers || 4000)}</strong>
							</div>
							<div className="ch-stat-box">
								<span>GMV 30 ngày</span>
								<strong>{formatNumber(activeKoc?.gmv || 125500000)}</strong>
							</div>
						</div>

						<div className="ch-video-header">
							<span>VIDEO · 60 GẦN NHẤT</span>
							<small>Quét 30/10/2026</small>
						</div>

						<select className="ch-select-full" defaultValue="ALL">
							<option value="ALL">Tất cả</option>
							<option value="AFF">Có gắn giỏ hàng</option>
						</select>

						<div className="ch-select-row">
							<select defaultValue="ALL">
								<option value="ALL">Loại video</option>
							</select>
							<select defaultValue="NEWEST">
								<option value="NEWEST">Mới nhất</option>
							</select>
						</div>

						<div className="ch-video-list">
							{(activeKoc?.videos?.length ? activeKoc.videos : initialCreators[0].videos).map((v) => (
								<div key={v.id} className="ch-video-item">
									<div className="ch-video-thumb">
										<span className="play-triangle">▷</span>
										<small>{v.date}</small>
									</div>
									<div className="ch-video-info">
										<strong>{v.title}</strong>
										<small className="ch-video-metrics">
											▷ {v.views} · ♡ {v.likes} · 💬 {v.comments} · ↗ {v.shares}
										</small>
										<small className="ch-video-prod">{v.product}</small>
									</div>
								</div>
							))}
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
					onSave={(payload, notice) => {
						onUpdateKoc(activeKoc, payload, notice);
						setActiveStepModal(null);
					}}
				/>
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
function CollabStepModal({ stepKey, koc, onClose, onSave }) {
	const [castFee, setCastFee] = useState(koc.castFee || 0);
	const [productName, setProductName] = useState(koc.productName || '');
	const [campaign, setCampaign] = useState(koc.campaign || 'Đợt T11');
	const [commissionRate, setCommissionRate] = useState(koc.commissionRate || 10);
	const [declineCode, setDeclineCode] = useState('');
	const [recipient, setRecipient] = useState(koc.recipient || koc.name);
	const [phone, setPhone] = useState(koc.phone || '');
	const [shippingAddress, setShippingAddress] = useState(koc.shippingAddress || '');
	const [videoLink, setVideoLink] = useState('');
	const [gmvGenerated, setGmvGenerated] = useState(koc.gmv || 0);

	function handleSubmit(e) {
		e.preventDefault();
		if (stepKey === 'cast') {
			const fee = Number(castFee) || 0;
			onSave(
				{
					action: 'qualify',
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
					action: 'agree',
					recipient,
					phone,
					shippingAddress,
					collabStep: 3,
				},
				`Đã duyệt & lưu thông tin gửi mẫu cho ${koc.name}.`,
			);
		} else if (stepKey === 'video') {
			onSave(
				{
					action: 'set_stage',
					stage: 'VIDEO_POSTED',
					rejectNote: videoLink,
					collabStep: 4,
				},
				`Đã ghi nhận KOC ${koc.name} đăng bài.`,
			);
		} else if (stepKey === 'result') {
			onSave(
				{
					action: 'set_stage',
					stage: 'COMPLETED',
					gmv: Number(gmvGenerated) || 0,
					collabStep: 5,
				},
				`Đã hoàn tất nghiệm thu hiệu quả hợp tác với ${koc.name}.`,
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
							{stepKey === 'sample' && 'Bước 3 · Duyệt & gửi mẫu'}
							{stepKey === 'video' && 'Bước 4 · KOC đăng bài'}
							{stepKey === 'result' && 'Bước 5 · Đánh giá hiệu quả'}
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
						</>
					)}

					{stepKey === 'sample' && (
						<>
							<label className="tm-section-label">Thông tin nhận sản phẩm mẫu của KOC</label>
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
							<textarea
								rows={3}
								value={shippingAddress}
								onChange={(e) => setShippingAddress(e.target.value)}
								placeholder="Địa chỉ nhận mẫu chi tiết"
								required
							/>
						</>
					)}

					{stepKey === 'video' && (
						<>
							<label className="tm-section-label">Đường dẫn (Link) video TikTok của KOC</label>
							<input
								value={videoLink}
								onChange={(e) => setVideoLink(e.target.value)}
								placeholder="https://www.tiktok.com/@.../video/..."
								required
							/>
						</>
					)}

					{stepKey === 'result' && (
						<>
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
					setFoundChannel({
						tiktokId: cleanId,
						displayName: res.profile?.displayName || cleanId,
						followers: res.profile?.followers || 4200,
						gmv: res.profile?.gmv || 0,
					});
					setLookupErr(false);
					setLookupMsg(`Đã tìm thấy kênh @${cleanId}`);
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
				setLookupMsg(`Đã tìm thấy kênh @${cleanId}`);
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
			followers: foundChannel?.followers || 0,
			gmv: foundChannel?.gmv || 0,
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
