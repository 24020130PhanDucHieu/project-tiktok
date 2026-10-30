import { useEffect, useMemo, useState } from 'react';
import api from './lib/axios';
import LoginPage from './pages/LoginPage';
import { clearSession, getToken, getUser } from './stores/useAuthStore';

const STAGE_META = {
	SOURCED: { label: 'Mới tìm', sub: 'Chờ chấm phễu', tone: 'neutral', phase: 'G1' },
	AWAITING_APPROVAL: { label: 'Chờ duyệt Cast', sub: 'Chủ shop duyệt', tone: 'amber', phase: 'G1' },
	QUALIFIED: { label: 'Đạt phễu', sub: 'Chờ gửi lời mời', tone: 'blue', phase: 'G1' },
	FILTERED_OUT: { label: 'Đã loại', sub: 'Không đạt phễu', tone: 'muted', phase: 'CLOSED' },
	CONTACTED: { label: 'Đã mời lần 1', sub: 'Chờ phản hồi 48h', tone: 'indigo', phase: 'G2' },
	NO_REPLY: { label: 'Chưa phản hồi', sub: 'Đang nhắc tự động', tone: 'amber', phase: 'G2' },
	REPLIED: { label: 'Đã phản hồi', sub: 'Đang trao đổi', tone: 'emerald', phase: 'G2' },
	DORMANT: { label: 'Ngủ đông', sub: 'Mở lại sau 30 ngày', tone: 'muted', phase: 'CLOSED' },
	AGREED: { label: 'Đồng ý hợp tác', sub: 'Sẵn sàng gửi mẫu', tone: 'emerald', phase: 'AGREED' },
	DECLINED: { label: 'Từ chối', sub: 'Đã đóng hồ sơ', tone: 'rose', phase: 'CLOSED' },
};

const FILTER_REJECT_REASONS = [
	{ code: 'F01', label: 'F01 — Sai tệp người xem (Target Audience)' },
	{ code: 'F02', label: 'F02 — Tuyến nội dung (Content Pillar) không phù hợp' },
	{ code: 'F03', label: 'F03 — Tương tác hoặc GMV kênh quá thấp' },
	{ code: 'F04', label: 'F04 — Phí Cast vượt ngân sách cho phép' },
	{ code: 'F05', label: 'F05 — Kênh vi phạm chính sách / Nội dung nhạy cảm' },
];

const DECLINE_REASONS = [
	{ code: 'R01', label: 'R01 — Không nhận hình thức Freecast (Yêu cầu có phí)' },
	{ code: 'R02', label: 'R02 — Không thống nhất được mức phí Cast' },
	{ code: 'R03', label: 'R03 — Mức hoa hồng Affiliate chưa hấp dẫn' },
	{ code: 'R04', label: 'R04 — KOC đang kín lịch booking trong tháng' },
	{ code: 'R05', label: 'R05 — Sản phẩm không hợp định hướng kênh hiện tại' },
	{ code: 'R06', label: 'R06 — Đang độc quyền cho nhãn hàng đối thủ' },
	{ code: 'R07', label: 'R07 — Không đồng ý điều khoản thời gian lên video' },
	{ code: 'R08', label: 'R08 — KOC tạm ngưng nhận job cá nhân' },
	{ code: 'R09', label: 'R09 — Lý do khác' },
];

const REMINDER_STEP_LABELS = {
	0: 'Đã gửi Zalo lần 1 (Chờ 48h)',
	1: 'Đã nhắc Zalo sau 2 ngày',
	2: 'Đã nhắc Gmail sau 4 ngày',
	3: 'Cần gọi điện trực tiếp (Sau 7 ngày)',
};

const initialCreators = [
	{
		id: 'seed-1',
		name: 'Minh Ái',
		handle: '@minhaireview',
		tiktokId: 'minhaireview',
		contact: '0327637213',
		phone: '0327637213',
		email: 'minhaireview@gmail.com',
		followers: '12,7k',
		gmv: '1Mđ+',
		category: 'Làm đẹp',
		contentPillar: 'Review skincare & mỹ phẩm',
		targetAudience: 'Nữ 18-28 tuổi',
		source: 'TikToday',
		campaign: 'Đợt T11 - Serum B5',
		productName: 'Serum phục hồi B5',
		commissionRate: 15,
		castType: 'freecast',
		castFee: 0,
		stage: 'QUALIFIED',
		reminderStep: 0,
		callTaskPending: false,
		daHopTac: false,
		avatar: 'MA',
	},
	{
		id: 'seed-2',
		name: 'Gia đình trái cây',
		handle: '@giadinhtraicay1202',
		tiktokId: 'giadinhtraicay1202',
		contact: '0866073103',
		phone: '0866073103',
		email: 'giadinhtraicay@gmail.com',
		followers: '3,1k',
		gmv: '5Mđ+',
		category: 'Đời sống',
		contentPillar: 'Vlog nấu ăn gia đình',
		targetAudience: 'Nội trợ 24-35 tuổi',
		source: 'TikTok Liên kết',
		campaign: 'Đợt T11 - Máy ép chậm',
		productName: 'Máy ép chậm mini',
		commissionRate: 10,
		castType: 'cast',
		castFee: 500000,
		stage: 'AWAITING_APPROVAL',
		reminderStep: 0,
		callTaskPending: false,
		daHopTac: false,
		avatar: 'GT',
	},
	{
		id: 'seed-3',
		name: 'Bếp Nhà Mây',
		handle: '@bepnhamay',
		tiktokId: 'bepnhamay',
		contact: '0987654321',
		phone: '0987654321',
		email: 'bepnhamay@gmail.com',
		followers: '28,4k',
		gmv: '12Mđ+',
		category: 'Đời sống',
		contentPillar: 'Review đồ gia dụng thông minh',
		targetAudience: 'Gia đình trẻ 25-40 tuổi',
		source: 'TikToday',
		campaign: 'Đợt T11 - Nồi chiên',
		productName: 'Nồi chiên hơi nước',
		commissionRate: 12,
		castType: 'freecast',
		castFee: 0,
		stage: 'NO_REPLY',
		reminderStep: 3,
		callTaskPending: true,
		daHopTac: false,
		avatar: 'BM',
	},
	{
		id: 'seed-4',
		name: 'Cô bé lười đi tập',
		handle: '@changditap',
		tiktokId: 'changditap',
		contact: '—',
		phone: '',
		email: '',
		followers: '126',
		gmv: '—',
		category: 'Chưa phân loại',
		contentPillar: '',
		targetAudience: '',
		source: 'TikTok',
		campaign: '',
		productName: '',
		commissionRate: 10,
		castType: 'freecast',
		castFee: 0,
		stage: 'SOURCED',
		reminderStep: 0,
		callTaskPending: false,
		daHopTac: false,
		avatar: 'CB',
	},
];

const previewConversations = [
	{
		_id: 'preview-1',
		contactName: 'Gia đình trái cây',
		contactHandle: '@giadinhtraicay1202',
		lastMessage: 'Mình đã nhận được thông tin rồi ạ',
		lastMessageAt: new Date().toISOString(),
		unreadCount: 2,
	},
];

function mapItemToCreator(item) {
	const rawName = (item.displayName || item.name || 'KOC').replace(/[^\p{L}\p{N}\s._-]/gu, '').trim() || item.displayName || 'KOC';
	return {
		...item,
		id: item._id || item.id,
		name: rawName,
		handle: `@${(item.tiktokId || '').replace(/^@/, '')}`,
		contact: item.phone || '—',
		followers: typeof item.followers === 'number' ? formatNumber(item.followers) : item.followers || '0',
		gmv: typeof item.gmv === 'number' ? (item.gmv ? `${formatNumber(item.gmv)}đ+` : '—') : item.gmv || '—',
		source: item.source || 'TikToday',
		stage: item.stage || 'SOURCED',
		castFee: Number(item.castFee) || 0,
		castType: Number(item.castFee) > 0 ? 'cast' : 'freecast',
		reminderStep: Number(item.reminderStep) || 0,
		callTaskPending: Boolean(item.callTaskPending),
		daHopTac: Boolean(item.daHopTac),
		avatar: rawName.slice(0, 2).toUpperCase(),
	};
}

function buildDefaultBrief(creator) {
	const product = creator.productName || 'sản phẩm mẫu của nhãn hàng';
	const offer =
		creator.castFee > 0
			? `phí booking ${formatNumber(creator.castFee)}đ + hoa hồng ${creator.commissionRate || 10}%`
			: `tặng mẫu miễn phí (Freecast) + hoa hồng Affiliate ${creator.commissionRate || 10}%`;
	return `Chào ${creator.name}, bên mình mời bạn hợp tác chiến dịch ${creator.campaign || 'mới'} cho dòng ${product} với chính sách ${offer}. Bạn xác nhận giúp mình để bên mình gửi mẫu nhé!`;
}

function App() {
	const [authenticated, setAuthenticated] = useState(Boolean(getToken()));
	const [currentUser, setCurrentUser] = useState(getUser());
	const [creators, setCreators] = useState(initialCreators);
	const [conversations, setConversations] = useState(previewConversations);
	const [apiReady, setApiReady] = useState(false);

	// Modals
	const [isModalOpen, setModalOpen] = useState(false);
	const [isBulkOpen, setBulkOpen] = useState(false);
	const [evaluatingCreator, setEvaluatingCreator] = useState(null);
	const [rejectingCreator, setRejectingCreator] = useState(null);
	const [invitingCreator, setInvitingCreator] = useState(null);
	const [decliningCreator, setDecliningCreator] = useState(null);
	const [agreeingCreator, setAgreeingCreator] = useState(null);

	// Navigation & Filters
	const [selectedNav, setSelectedNav] = useState('Quản lý KOC');
	const [phaseTab, setPhaseTab] = useState('ALL');
	const [stageFilter, setStageFilter] = useState('ALL');
	const [sourceFilter, setSourceFilter] = useState('ALL');
	const [category, setCategory] = useState('Tất cả');
	const [query, setQuery] = useState('');
	const [actionError, setActionError] = useState('');
	const [actionNotice, setActionNotice] = useState('');

	function refreshAllData() {
		if (!authenticated) return;
		Promise.all([api.get('/kocs'), api.get('/inbox/conversations')])
			.then(([kocResponse, inboxResponse]) => {
				setCreators(kocResponse.data.data.items.map(mapItemToCreator));
				setConversations(inboxResponse.data.data);
				setApiReady(true);
			})
			.catch(() => setApiReady(false));
	}

	useEffect(() => {
		refreshAllData();
	}, [authenticated]);

	const metrics = useMemo(() => {
		const active = creators.filter((c) => !c.daHopTac);
		return {
			total: creators.length,
			g1Pending: active.filter((c) => c.stage === 'SOURCED' || c.stage === 'AWAITING_APPROVAL').length,
			awaitingApproval: active.filter((c) => c.stage === 'AWAITING_APPROVAL').length,
			qualified: active.filter((c) => c.stage === 'QUALIFIED').length,
			g2Outreach: active.filter((c) => ['CONTACTED', 'NO_REPLY', 'REPLIED'].includes(c.stage)).length,
			replied: active.filter((c) => c.stage === 'REPLIED').length,
			callTasks: active.filter((c) => c.callTaskPending || (c.stage === 'NO_REPLY' && c.reminderStep >= 3)).length,
			agreed: active.filter((c) => c.stage === 'AGREED').length,
			closed: creators.filter((c) => ['FILTERED_OUT', 'DECLINED', 'DORMANT'].includes(c.stage) || c.daHopTac).length,
		};
	}, [creators]);

	const filteredCreators = useMemo(() => {
		return creators.filter((creator) => {
			const matchesQuery = `${creator.name} ${creator.handle} ${creator.contact} ${creator.campaign || ''} ${creator.productName || ''}`
				.toLowerCase()
				.includes(query.toLowerCase());
			const matchesCategory = category === 'Tất cả' || creator.category === category;
			const matchesSource = sourceFilter === 'ALL' || creator.source === sourceFilter;
			const matchesStage = stageFilter === 'ALL' || creator.stage === stageFilter;

			let matchesPhase = true;
			if (phaseTab === 'G1') {
				matchesPhase = ['SOURCED', 'AWAITING_APPROVAL', 'QUALIFIED'].includes(creator.stage) && !creator.daHopTac;
			} else if (phaseTab === 'G2') {
				matchesPhase = ['CONTACTED', 'NO_REPLY', 'REPLIED'].includes(creator.stage) && !creator.daHopTac;
			} else if (phaseTab === 'TASKS') {
				matchesPhase =
					!creator.daHopTac &&
					(creator.stage === 'AWAITING_APPROVAL' || creator.stage === 'REPLIED' || creator.callTaskPending);
			} else if (phaseTab === 'AGREED') {
				matchesPhase = creator.stage === 'AGREED' && !creator.daHopTac;
			} else if (phaseTab === 'CLOSED') {
				matchesPhase = ['FILTERED_OUT', 'DECLINED', 'DORMANT'].includes(creator.stage) || creator.daHopTac;
			}

			return matchesQuery && matchesCategory && matchesSource && matchesStage && matchesPhase;
		});
	}, [creators, query, category, sourceFilter, stageFilter, phaseTab]);

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

	function notifySuccess(msg) {
		setActionError('');
		setActionNotice(msg);
	}

	function notifyError(err, fallback) {
		setActionNotice('');
		setActionError(err?.response?.data?.message || fallback);
	}

	function addCreator(creatorPayload) {
		api.post('/kocs', creatorPayload)
			.then(({ data }) => {
				const mapped = mapItemToCreator(data.data);
				setCreators((current) => [mapped, ...current]);
				setModalOpen(false);
				notifySuccess(
					mapped.stage === 'AWAITING_APPROVAL'
						? `Đã thêm ${mapped.name}. KOC có phí Cast (${formatNumber(mapped.castFee)}đ) — Đang chờ Chủ shop duyệt.`
						: `Đã thêm ${mapped.name} vào hệ thống (${STAGE_META[mapped.stage]?.label || mapped.stage}).`,
				);
			})
			.catch((error) => notifyError(error, 'Không thể thêm KOC. Vui lòng kiểm tra trùng lặp.'));
	}

	function handleBulkImport(payload) {
		api.post('/kocs/bulk', payload)
			.then(({ data }) => {
				const importedItems = (data.data.imported || []).map(mapItemToCreator);
				setCreators((current) => [...importedItems, ...current]);
				setBulkOpen(false);
				notifySuccess(data.message);
			})
			.catch((error) => notifyError(error, 'Không thể nhập hàng loạt KOC.'));
	}

	function updateStage(creator, payload, successMessage) {
		if (String(creator.id).startsWith('seed-')) {
			setCreators((current) =>
				current.map((item) => {
					if (item.id !== creator.id) return item;
					const nextFee = payload.castFee !== undefined ? Number(payload.castFee) : item.castFee;
					let nextStage = item.stage;
					let nextStep = item.reminderStep || 0;
					let nextCallTask = item.callTaskPending;

					if (payload.action === 'qualify') nextStage = nextFee > 0 ? 'AWAITING_APPROVAL' : 'QUALIFIED';
					if (payload.action === 'approve_cast') nextStage = 'QUALIFIED';
					if (payload.action === 'filter_out') nextStage = 'FILTERED_OUT';
					if (payload.action === 'send_invite') {
						nextStage = 'CONTACTED';
						nextStep = 0;
						nextCallTask = false;
					}
					if (payload.action === 'remind_step') {
						nextStage = 'NO_REPLY';
						nextStep = payload.reminderStep || Math.min(nextStep + 1, 3);
						nextCallTask = nextStep >= 3;
					}
					if (payload.action === 'mark_replied') {
						nextStage = 'REPLIED';
						nextCallTask = false;
					}
					if (payload.action === 'mark_dormant') {
						nextStage = 'DORMANT';
						nextCallTask = false;
					}
					if (payload.action === 'reopen_dormant') {
						nextStage = 'QUALIFIED';
						nextStep = 0;
						nextCallTask = false;
					}
					if (payload.action === 'agree') {
						nextStage = 'AGREED';
						nextCallTask = false;
					}
					if (payload.action === 'decline') {
						nextStage = 'DECLINED';
						nextCallTask = false;
					}

					return {
						...item,
						...payload,
						castFee: nextFee,
						castType: nextFee > 0 ? 'cast' : 'freecast',
						stage: nextStage,
						reminderStep: nextStep,
						callTaskPending: nextCallTask,
						daHopTac: payload.daHopTac !== undefined ? payload.daHopTac : item.daHopTac,
					};
				}),
			);
			setEvaluatingCreator(null);
			setRejectingCreator(null);
			setInvitingCreator(null);
			setDecliningCreator(null);
			setAgreeingCreator(null);
			notifySuccess(successMessage);
			return;
		}

		api.patch(`/kocs/${creator.id}/stage`, payload)
			.then(({ data }) => {
				const updated = mapItemToCreator(data.data);
				setCreators((current) => current.map((item) => (item.id === creator.id ? updated : item)));
				setEvaluatingCreator(null);
				setRejectingCreator(null);
				setInvitingCreator(null);
				setDecliningCreator(null);
				setAgreeingCreator(null);
				notifySuccess(successMessage);
				if (payload.action === 'send_invite' || payload.action === 'remind_step') {
					api.get('/inbox/conversations').then(({ data: convData }) => setConversations(convData.data)).catch(() => {});
				}
			})
			.catch((error) => notifyError(error, 'Không thể cập nhật trạng thái KOC.'));
	}

	function runBatchOutreach(mode, simulateNextStep = false) {
		if (!apiReady) {
			// Chế độ preview offline
			if (mode === 'invite_all_qualified') {
				setCreators((current) =>
					current.map((c) => (c.stage === 'QUALIFIED' && !c.daHopTac ? { ...c, stage: 'CONTACTED', reminderStep: 0 } : c)),
				);
				notifySuccess('Đã gửi lời mời lần 1 cho toàn bộ KOC đạt phễu.');
			} else {
				setCreators((current) =>
					current.map((c) => {
						if (!['CONTACTED', 'NO_REPLY'].includes(c.stage) || c.daHopTac) return c;
						const nextStep = Math.min((c.reminderStep || 0) + 1, 3);
						return { ...c, stage: 'NO_REPLY', reminderStep: nextStep, callTaskPending: nextStep >= 3 };
					}),
				);
				notifySuccess('Đã chạy bước nhắc lịch tiếp theo (2 ngày Zalo / 4 ngày Gmail / 7 ngày Gọi điện).');
			}
			return;
		}

		api.post('/kocs/outreach/run', { mode, simulateNextStep })
			.then(({ data }) => {
				notifySuccess(data.message);
				refreshAllData();
			})
			.catch((error) => notifyError(error, 'Không thể chạy tác vụ tự động Giai đoạn 2.'));
	}

	function deleteCreator(creator) {
		if (!window.confirm(`Lưu trữ và ẩn ${creator.name} khỏi danh sách?`)) return;
		if (String(creator.id).startsWith('seed-')) {
			setCreators((current) => current.filter((item) => item.id !== creator.id));
			notifySuccess(`Đã xóa ${creator.name}.`);
			return;
		}
		api.delete(`/kocs/${creator.id}`)
			.then(() => {
				setCreators((current) => current.filter((item) => item.id !== creator.id));
				notifySuccess(`Đã lưu trữ ${creator.name}.`);
			})
			.catch((error) => notifyError(error, 'Không thể lưu trữ KOC.'));
	}

	return (
		<div className="app-shell">
			<aside className="sidebar">
				<div className="brand">
					<div className="brand-logo">T</div>
					<div>
						<span className="brand-title">Tiktoday</span>
						<span className="brand-badge">CRM</span>
					</div>
				</div>

				<button className="primary-button sidebar-cta" onClick={() => setModalOpen(true)}>
					+ Thêm KOC mới
				</button>

				<div className="nav-section-label">MENU CHÍNH</div>
				<nav>
					<button
						className={selectedNav === 'Quản lý KOC' ? 'nav-item active' : 'nav-item'}
						onClick={() => setSelectedNav('Quản lý KOC')}
					>
						<span>Phễu & Liên hệ KOC</span>
						<small className="nav-counter">{metrics.total}</small>
					</button>
					<button
						className={selectedNav === 'Hộp thư' ? 'nav-item active' : 'nav-item'}
						onClick={() => setSelectedNav('Hộp thư')}
					>
						<span>Hộp thư Zalo / Nội bộ</span>
						<small className="nav-counter">{conversations.length}</small>
					</button>
				</nav>

				<div className="nav-section-label">VIỆC CẦN XỬ LÝ</div>
				<div className="sidebar-tasks">
					<button
						className="task-shortcut"
						onClick={() => {
							setSelectedNav('Quản lý KOC');
							setPhaseTab('G1');
							setStageFilter('AWAITING_APPROVAL');
						}}
					>
						<span>Chờ duyệt giá Cast</span>
						<b className={metrics.awaitingApproval > 0 ? 'badge-warn' : ''}>{metrics.awaitingApproval}</b>
					</button>
					<button
						className="task-shortcut"
						onClick={() => {
							setSelectedNav('Quản lý KOC');
							setPhaseTab('TASKS');
							setStageFilter('ALL');
						}}
					>
						<span>Gọi điện (Quá 7 ngày)</span>
						<b className={metrics.callTasks > 0 ? 'badge-danger' : ''}>{metrics.callTasks}</b>
					</button>
					<button
						className="task-shortcut"
						onClick={() => {
							setSelectedNav('Quản lý KOC');
							setPhaseTab('G2');
							setStageFilter('REPLIED');
						}}
					>
						<span>KOC phản hồi chờ chốt</span>
						<b className={metrics.replied > 0 ? 'badge-ok' : ''}>{metrics.replied}</b>
					</button>
				</div>

				<button
					className="account"
					onClick={() => {
						clearSession();
						setAuthenticated(false);
						setCurrentUser(null);
					}}
				>
					<div className="account-avatar">{currentUser?.name?.slice(0, 1) || 'A'}</div>
					<div className="account-info">
						<strong>{currentUser?.name || 'Quản trị viên'}</strong>
						<small>{currentUser?.email || 'Đăng xuất'}</small>
					</div>
					<span className="logout-text">Thoát</span>
				</button>
			</aside>

			<main className="main-content">
				<header className="topbar">
					<div className="crumb">
						<span>Hệ thống Booking KOC</span>
						<span className="crumb-sep">/</span>
						<b>{selectedNav === 'Hộp thư' ? 'Hộp thư đa kênh' : 'Quản lý Vòng đời KOC (Giai đoạn 1 & 2)'}</b>
					</div>
					<div className="top-actions">
						<span className={`conn-indicator ${apiReady ? 'online' : 'offline'}`}>
							{apiReady ? 'Đã đồng bộ Server' : 'Chế độ xem trước'}
						</span>
					</div>
				</header>

				<section className="content">
					{selectedNav === 'Hộp thư' ? (
						<InboxView conversations={conversations} setConversations={setConversations} apiReady={apiReady} />
					) : (
						<>
							<div className="page-heading">
								<div>
									<p className="eyebrow">QUẢN LÝ QUY TRÌNH BOOKING</p>
									<h1>Danh sách & Phễu KOC</h1>
								</div>
								<div className="heading-actions">
									<button
										className="secondary-button"
										onClick={() => runBatchOutreach('sweep_followups', true)}
										title="Chuyển các KOC chưa phản hồi sang bước nhắc tiếp theo (2 ngày Zalo -> 4 ngày Gmail -> 7 ngày Gọi điện)"
									>
										Chạy quét nhắc lịch (2N / 4N / 7N)
									</button>
									{metrics.qualified > 0 && (
										<button
											className="secondary-button accent-outline"
											onClick={() => runBatchOutreach('invite_all_qualified', false)}
										>
											Gửi mời tất cả Đạt phễu ({metrics.qualified})
										</button>
									)}
									<button className="secondary-button" onClick={() => setBulkOpen(true)}>
										Nhập hàng loạt
									</button>
									<button className="primary-button" onClick={() => setModalOpen(true)}>
										+ Thêm KOC
									</button>
								</div>
							</div>

							{actionError && <div className="alert-banner alert-error">{actionError}</div>}
							{actionNotice && <div className="alert-banner alert-success">{actionNotice}</div>}

							{/* KPI Cards */}
							<div className="kpi-grid">
								<div
									className={`kpi-card ${phaseTab === 'ALL' ? 'active' : ''}`}
									onClick={() => {
										setPhaseTab('ALL');
										setStageFilter('ALL');
									}}
								>
									<span className="kpi-label">Tổng danh bạ</span>
									<strong className="kpi-value">{metrics.total}</strong>
									<small className="kpi-sub">Đã kiểm tra chống trùng</small>
								</div>
								<div
									className={`kpi-card ${phaseTab === 'G1' ? 'active' : ''}`}
									onClick={() => {
										setPhaseTab('G1');
										setStageFilter('ALL');
									}}
								>
									<span className="kpi-label">Giai đoạn 1 · Lọc & Duyệt</span>
									<strong className="kpi-value">{metrics.g1Pending + metrics.qualified}</strong>
									<small className="kpi-sub">
										{metrics.qualified} sẵn sàng mời · {metrics.awaitingApproval} chờ duyệt Cast
									</small>
								</div>
								<div
									className={`kpi-card ${phaseTab === 'G2' ? 'active' : ''}`}
									onClick={() => {
										setPhaseTab('G2');
										setStageFilter('ALL');
									}}
								>
									<span className="kpi-label">Giai đoạn 2 · Đang liên hệ</span>
									<strong className="kpi-value">{metrics.g2Outreach}</strong>
									<small className="kpi-sub">
										{metrics.replied} đã phản hồi · {metrics.callTasks} cần gọi điện
									</small>
								</div>
								<div
									className={`kpi-card ${phaseTab === 'AGREED' ? 'active' : ''}`}
									onClick={() => {
										setPhaseTab('AGREED');
										setStageFilter('ALL');
									}}
								>
									<span className="kpi-label">Đã chốt hợp tác (AGREED)</span>
									<strong className="kpi-value text-emerald">{metrics.agreed}</strong>
									<small className="kpi-sub">Chờ chuyển sang GĐ 3 Giao mẫu</small>
								</div>
							</div>

							{/* Segmented Phase Tabs + Filters */}
							<div className="filter-bar">
								<div className="segmented-tabs">
									{[
										['ALL', `Tất cả (${metrics.total})`],
										['G1', `GĐ 1: Lọc & Duyệt (${metrics.g1Pending + metrics.qualified})`],
										['G2', `GĐ 2: Liên hệ & Nhắc (${metrics.g2Outreach})`],
										['TASKS', `Cần xử lý (${metrics.awaitingApproval + metrics.callTasks + metrics.replied})`],
										['AGREED', `Đồng ý (${metrics.agreed})`],
										['CLOSED', `Đã đóng / Ngủ đông (${metrics.closed})`],
									].map(([key, label]) => (
										<button
											key={key}
											className={phaseTab === key ? 'seg-tab active' : 'seg-tab'}
											onClick={() => {
												setPhaseTab(key);
												setStageFilter('ALL');
											}}
										>
											{label}
										</button>
									))}
								</div>

								<div className="toolbar">
									<div className="table-search">
										<input
											value={query}
											onChange={(event) => setQuery(event.target.value)}
											placeholder="Tìm kiếm theo tên kênh, TikTok ID, số điện thoại, chiến dịch..."
										/>
									</div>
									<select value={stageFilter} onChange={(e) => setStageFilter(e.target.value)}>
										<option value="ALL">Mọi trạng thái</option>
										<optgroup label="Giai đoạn 1 · Tìm & Lọc">
											<option value="SOURCED">Mới tìm (SOURCED)</option>
											<option value="AWAITING_APPROVAL">Chờ duyệt Cast (AWAITING_APPROVAL)</option>
											<option value="QUALIFIED">Đạt phễu (QUALIFIED)</option>
											<option value="FILTERED_OUT">Đã loại (FILTERED_OUT)</option>
										</optgroup>
										<optgroup label="Giai đoạn 2 · Liên hệ & Bám đuổi">
											<option value="CONTACTED">Đã mời lần 1 (CONTACTED)</option>
											<option value="NO_REPLY">Quá 48h chưa rep (NO_REPLY)</option>
											<option value="REPLIED">Đã phản hồi (REPLIED)</option>
											<option value="AGREED">Đồng ý hợp tác (AGREED)</option>
											<option value="DECLINED">Từ chối (DECLINED)</option>
											<option value="DORMANT">Ngủ đông 30 ngày (DORMANT)</option>
										</optgroup>
									</select>
									<select value={sourceFilter} onChange={(e) => setSourceFilter(e.target.value)}>
										<option value="ALL">Mọi nguồn tìm</option>
										<option value="TikToday">TikToday</option>
										<option value="TikTok Liên kết">TikTok Liên kết</option>
										<option value="TikTok">TikTok App</option>
									</select>
									<select value={category} onChange={(event) => setCategory(event.target.value)}>
										<option value="Tất cả">Mọi danh mục</option>
										<option value="Làm đẹp">Làm đẹp</option>
										<option value="Đời sống">Đời sống</option>
										<option value="Thời trang">Thời trang</option>
										<option value="Chưa phân loại">Chưa phân loại</option>
									</select>
								</div>
							</div>

							{/* Data Table */}
							<div className="table-wrap">
								<table>
									<thead>
										<tr>
											<th>KOC / KÊNH</th>
											<th>LIÊN HỆ</th>
											<th>PHỄU & CHIẾN DỊCH</th>
											<th>CHÍNH SÁCH</th>
											<th>TRẠNG THÁI VÒNG ĐỜI</th>
											<th>TIẾN ĐỘ LIÊN HỆ (GĐ 2)</th>
											<th className="th-actions">THAO TÁC NGHIỆP VỤ</th>
										</tr>
									</thead>
									<tbody>
										{filteredCreators.map((creator) => {
											const meta = STAGE_META[creator.stage] || {
												label: creator.stage,
												sub: '',
												tone: 'neutral',
											};
											return (
												<tr key={creator.id} className={creator.daHopTac ? 'row-blocked' : ''}>
													<td>
														<div className="creator">
															<div className="creator-avatar">{creator.avatar}</div>
															<div>
																<strong>{creator.name}</strong>
																<small className="creator-sub">
																	{creator.handle} · <span className="source-tag">{creator.source}</span> · {creator.followers} flw
																</small>
															</div>
														</div>
													</td>
													<td>
														<div className="cell-primary">{creator.contact}</div>
														<small className="cell-secondary">{creator.email || 'Chưa có email'}</small>
													</td>
													<td>
														<div className="cell-primary">
															<span className="cat-tag">{creator.category}</span>
															{creator.campaign ? ` · ${creator.campaign}` : ''}
														</div>
														<small className="cell-secondary">
															{creator.productName
																? `SP: ${creator.productName}`
																: creator.contentPillar || 'Chưa cập nhật tuyến nội dung'}
														</small>
													</td>
													<td>
														{creator.castFee > 0 ? (
															<div className="policy-paid">Cast: {formatNumber(creator.castFee)}đ</div>
														) : (
															<div className="policy-free">Freecast (0đ)</div>
														)}
														<small className="cell-secondary">Hoa hồng: {creator.commissionRate || 10}%</small>
													</td>
													<td>
														{creator.daHopTac ? (
															<span className="status-pill tone-rose">
																<i className="dot" /> Đã hợp tác (Chặn)
															</span>
														) : (
															<span className={`status-pill tone-${meta.tone}`}>
																<i className="dot" /> {meta.label}
															</span>
														)}
														{creator.stage === 'FILTERED_OUT' && creator.rejectCode && (
															<small className="reason-note">
																Mã lọc: {creator.rejectCode} {creator.rejectNote ? `— ${creator.rejectNote}` : ''}
															</small>
														)}
														{creator.stage === 'DECLINED' && creator.declineCode && (
															<small className="reason-note">
																Từ chối: {creator.declineCode} {creator.declineNote ? `— ${creator.declineNote}` : ''}
															</small>
														)}
													</td>
													<td>
														{['CONTACTED', 'NO_REPLY'].includes(creator.stage) && (
															<div className="outreach-progress">
																<span className="step-text">
																	{REMINDER_STEP_LABELS[creator.reminderStep || 0]}
																</span>
																<div className="step-bars">
																	{[0, 1, 2, 3].map((step) => (
																		<span
																			key={step}
																			className={(creator.reminderStep || 0) >= step ? 'bar filled' : 'bar'}
																		/>
																	))}
																</div>
															</div>
														)}
														{creator.stage === 'REPLIED' && (
															<span className="step-highlight text-emerald">KOC đã phản hồi — Chờ chốt</span>
														)}
														{creator.stage === 'DORMANT' && (
															<span className="cell-secondary">Tự mở lại sau 30 ngày</span>
														)}
														{creator.stage === 'AGREED' && (
															<span className="step-highlight text-emerald">
																{creator.shippingAddress ? `Giao tới: ${creator.shippingAddress}` : 'Chờ lấy địa chỉ nhận mẫu'}
															</span>
														)}
														{['SOURCED', 'AWAITING_APPROVAL', 'QUALIFIED', 'FILTERED_OUT', 'DECLINED'].includes(
															creator.stage,
														) && <span className="cell-secondary">{meta.sub}</span>}
													</td>
													<td className="td-actions">
														<div className="row-actions">
															{/* GĐ 1: SOURCED */}
															{!creator.daHopTac && creator.stage === 'SOURCED' && (
																<>
																	<button
																		className="btn-action btn-primary-soft"
																		onClick={() => setEvaluatingCreator(creator)}
																	>
																		Chấm phễu
																	</button>
																	<button
																		className="btn-action btn-danger-soft"
																		onClick={() => setRejectingCreator(creator)}
																	>
																		Loại
																	</button>
																</>
															)}

															{/* GĐ 1: AWAITING_APPROVAL */}
															{!creator.daHopTac && creator.stage === 'AWAITING_APPROVAL' && (
																<>
																	<button
																		className="btn-action btn-warn-soft"
																		onClick={() =>
																			updateStage(
																				creator,
																				{ action: 'approve_cast' },
																				`Đã duyệt phí Cast cho ${creator.name} — Chuyển sang Đạt phễu (QUALIFIED).`,
																			)
																		}
																	>
																		Duyệt Cast
																	</button>
																	<button
																		className="btn-action btn-danger-soft"
																		onClick={() => setRejectingCreator(creator)}
																	>
																		Từ chối
																	</button>
																</>
															)}

															{/* GĐ 2: C1 - Gửi tin mời lần 1 */}
															{!creator.daHopTac && creator.stage === 'QUALIFIED' && (
																<>
																	<button
																		className="btn-action btn-primary-solid"
																		onClick={() => setInvitingCreator(creator)}
																	>
																		Gửi lời mời
																	</button>
																	<button
																		className="btn-action btn-ghost"
																		onClick={() => setEvaluatingCreator(creator)}
																	>
																		Sửa
																	</button>
																</>
															)}

															{/* GĐ 2: CONTACTED hoặc NO_REPLY */}
															{!creator.daHopTac && ['CONTACTED', 'NO_REPLY'].includes(creator.stage) && (
																<>
																	<button
																		className="btn-action btn-success-soft"
																		onClick={() =>
																			updateStage(
																				creator,
																				{ action: 'mark_replied' },
																				`Đã ghi nhận ${creator.name} phản hồi (REPLIED). Hãy chọn Đồng ý hoặc Từ chối.`,
																			)
																		}
																	>
																		KOC đã rep
																	</button>
																	{(creator.reminderStep || 0) < 3 ? (
																		<button
																			className="btn-action btn-ghost"
																			title="Chuyển sang bước nhắc tiếp theo (2 ngày Zalo / 4 ngày Gmail / 7 ngày Gọi điện)"
																			onClick={() => {
																				const nextStep = (creator.reminderStep || 0) + 1;
																				updateStage(
																					creator,
																					{ action: 'remind_step', reminderStep: nextStep },
																					`Đã kích hoạt bước nhắc số ${nextStep} cho ${creator.name}.`,
																				);
																			}}
																		>
																			{creator.reminderStep === 0
																				? 'Nhắc Zalo (2N)'
																				: creator.reminderStep === 1
																					? 'Nhắc Mail (4N)'
																					: 'Tạo Task Gọi (7N)'}
																		</button>
																	) : (
																		<button
																			className="btn-action btn-danger-soft"
																			title="Sau 7 ngày gọi điện vẫn không phản hồi -> Đưa vào Ngủ đông 30 ngày (C7)"
																			onClick={() =>
																				updateStage(
																					creator,
																					{ action: 'mark_dormant' },
																					`Đã chuyển ${creator.name} sang trạng thái Ngủ đông (DORMANT) — Tự mở lại sau 30 ngày.`,
																				)
																			}
																		>
																			Ngủ đông 30N
																		</button>
																	)}
																</>
															)}

															{/* GĐ 2: REPLIED -> C8 (Đồng ý / Từ chối R01-R09) */}
															{!creator.daHopTac && creator.stage === 'REPLIED' && (
																<>
																	<button
																		className="btn-action btn-success-soft"
																		onClick={() => setAgreeingCreator(creator)}
																	>
																		Đồng ý
																	</button>
																	<button
																		className="btn-action btn-danger-soft"
																		onClick={() => setDecliningCreator(creator)}
																	>
																		Từ chối (R01-R09)
																	</button>
																</>
															)}

															{/* DORMANT -> Mở lại */}
															{!creator.daHopTac && creator.stage === 'DORMANT' && (
																<button
																	className="btn-action btn-ghost"
																	onClick={() =>
																		updateStage(
																			creator,
																			{ action: 'reopen_dormant' },
																			`Đã mở lại ${creator.name} về trạng thái QUALIFIED cho đợt mới.`,
																		)
																	}
																>
																	Mở lại đợt mới
																</button>
															)}

															<button
																className={`btn-icon-text ${creator.daHopTac ? 'active-block' : ''}`}
																title={
																	creator.daHopTac
																		? 'Bấm để gỡ chặn đã hợp tác'
																		: 'Đánh dấu đã từng hợp tác (Chặn liên hệ)'
																}
																onClick={() =>
																	updateStage(
																		creator,
																		{ action: 'toggle_collaborated', daHopTac: !creator.daHopTac },
																		!creator.daHopTac
																			? `Đã đánh dấu ${creator.name} đã hợp tác (Chặn liên hệ).`
																			: `Đã gỡ chặn liên hệ cho ${creator.name}.`,
																	)
																}
															>
																{creator.daHopTac ? 'Gỡ chặn' : 'Chặn'}
															</button>
															<button
																className="btn-icon-text danger"
																title="Lưu trữ / Xóa"
																onClick={() => deleteCreator(creator)}
															>
																Xóa
															</button>
														</div>
													</td>
												</tr>
											);
										})}
									</tbody>
								</table>
								{filteredCreators.length === 0 && (
									<div className="empty-state">Không có KOC nào trong danh mục lọc này.</div>
								)}
								<footer className="table-footer">
									<span>
										Hiển thị <b>{filteredCreators.length}</b> trên tổng số <b>{creators.length}</b> KOC
									</span>
								</footer>
							</div>
						</>
					)}
				</section>
			</main>

			{isModalOpen && <AddCreatorModal onClose={() => setModalOpen(false)} onAdd={addCreator} />}
			{isBulkOpen && <BulkImportModal onClose={() => setBulkOpen(false)} onImport={handleBulkImport} />}

			{evaluatingCreator && (
				<QualifyKocModal
					creator={evaluatingCreator}
					onClose={() => setEvaluatingCreator(null)}
					onSubmit={(payload) =>
						updateStage(
							evaluatingCreator,
							{ action: 'qualify', ...payload },
							Number(payload.castFee) > 0
								? `Đã chấm phễu ${evaluatingCreator.name}. Có phí Cast (${formatNumber(payload.castFee)}đ) — Chuyển sang Chờ Chủ shop duyệt.`
								: `Đã chấm đạt ${evaluatingCreator.name} (Freecast) — Sẵn sàng gửi lời mời Giai đoạn 2.`,
						)
					}
				/>
			)}

			{rejectingCreator && (
				<RejectKocModal
					creator={rejectingCreator}
					onClose={() => setRejectingCreator(null)}
					onSubmit={({ rejectCode, rejectNote }) =>
						updateStage(
							rejectingCreator,
							{ action: 'filter_out', rejectCode, rejectNote },
							`Đã loại ${rejectingCreator.name} khỏi phễu (${rejectCode}).`,
						)
					}
				/>
			)}

			{invitingCreator && (
				<SendInviteModal
					creator={invitingCreator}
					onClose={() => setInvitingCreator(null)}
					onSubmit={({ inviteBrief }) =>
						updateStage(
							invitingCreator,
							{ action: 'send_invite', inviteBrief },
							`Đã gửi tin mời lần 1 (Zalo + Brief) cho ${invitingCreator.name} — Bắt đầu đếm 48h phản hồi.`,
						)
					}
				/>
			)}

			{decliningCreator && (
				<DeclineKocModal
					creator={decliningCreator}
					onClose={() => setDecliningCreator(null)}
					onSubmit={({ declineCode, declineNote }) =>
						updateStage(
							decliningCreator,
							{ action: 'decline', declineCode, declineNote },
							`Đã ghi nhận ${decliningCreator.name} từ chối hợp tác (Mã lý do: ${declineCode}).`,
						)
					}
				/>
			)}

			{agreeingCreator && (
				<AgreeKocModal
					creator={agreeingCreator}
					onClose={() => setAgreeingCreator(null)}
					onSubmit={(payload) =>
						updateStage(
							agreeingCreator,
							{ action: 'agree', ...payload },
							`Chốt hợp tác thành công với ${agreeingCreator.name} (AGREED) — Sẵn sàng chuyển sang Giai đoạn 3 (Giao mẫu).`,
						)
					}
				/>
			)}
		</div>
	);
}

function SendInviteModal({ creator, onClose, onSubmit }) {
	const [inviteBrief, setInviteBrief] = useState(creator.inviteBrief || buildDefaultBrief(creator));

	function handleSubmit(e) {
		e.preventDefault();
		onSubmit({ inviteBrief });
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<form className="modal" onSubmit={handleSubmit}>
				<div className="modal-header">
					<div>
						<h2>Gửi lời mời hợp tác lần 1 (Zalo + Brief)</h2>
						<p>
							KOC: <b>{creator.name}</b> ({creator.handle}) · SĐT: {creator.contact}
						</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<div className="info-summary">
						<div>
							<span>Chiến dịch:</span> <b>{creator.campaign || 'Chưa đặt tên'}</b>
						</div>
						<div>
							<span>Sản phẩm:</span> <b>{creator.productName || 'Sản phẩm mẫu'}</b>
						</div>
						<div>
							<span>Chính sách:</span>{' '}
							<b>
								{creator.castFee > 0 ? `Cast ${formatNumber(creator.castFee)}đ` : 'Freecast (0đ)'} · Hoa hồng{' '}
								{creator.commissionRate || 10}%
							</b>
						</div>
					</div>
					<label>Nội dung tin nhắn mời & Brief gửi qua Zalo</label>
					<textarea
						className="bulk-textarea"
						rows={6}
						value={inviteBrief}
						onChange={(e) => setInviteBrief(e.target.value)}
						required
					/>
					<small className="hint">
						Sau khi gửi, trạng thái sẽ chuyển sang CONTACTED. Nếu sau 48h KOC chưa trả lời, hệ thống sẽ chuyển sang chuỗi nhắc tự động (2 ngày Zalo, 4 ngày Gmail, 7 ngày gọi điện).
					</small>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button type="submit" className="primary-button">
						Gửi tin mời qua Zalo
					</button>
				</div>
			</form>
		</div>
	);
}

function DeclineKocModal({ creator, onClose, onSubmit }) {
	const [declineCode, setDeclineCode] = useState('R01');
	const [declineNote, setDeclineNote] = useState('');

	function handleSubmit(e) {
		e.preventDefault();
		onSubmit({ declineCode, declineNote });
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<form className="modal" onSubmit={handleSubmit}>
				<div className="modal-header">
					<div>
						<h2>Ghi nhận KOC từ chối hợp tác ({creator.name})</h2>
						<p>Chọn mã lý do từ chối từ R01 đến R09 theo quy chuẩn Giai đoạn 2</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<label>Mã lý do từ chối (R01 – R09)</label>
					<select value={declineCode} onChange={(e) => setDeclineCode(e.target.value)}>
						{DECLINE_REASONS.map((r) => (
							<option key={r.code} value={r.code}>
								{r.label}
							</option>
						))}
					</select>
					<label>Ghi chú trao đổi chi tiết</label>
					<input
						value={declineNote}
						onChange={(e) => setDeclineNote(e.target.value)}
						placeholder="Nhập phản hồi cụ thể của KOC..."
					/>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button type="submit" className="primary-button danger-btn">
						Xác nhận Từ chối (DECLINED)
					</button>
				</div>
			</form>
		</div>
	);
}

function AgreeKocModal({ creator, onClose, onSubmit }) {
	const [recipient, setRecipient] = useState(creator.recipient || creator.name);
	const [phone, setPhone] = useState(creator.phone || '');
	const [shippingAddress, setShippingAddress] = useState(creator.shippingAddress || '');

	function handleSubmit(e) {
		e.preventDefault();
		onSubmit({ recipient, phone, shippingAddress });
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<form className="modal" onSubmit={handleSubmit}>
				<div className="modal-header">
					<div>
						<h2>Xác nhận KOC đồng ý hợp tác ({creator.name})</h2>
						<p>Lưu thông tin người nhận để chuyển sang Giai đoạn 3 (Giao sản phẩm mẫu)</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<div className="grid-2">
						<div>
							<label>Người nhận hàng</label>
							<input value={recipient} onChange={(e) => setRecipient(e.target.value)} required />
						</div>
						<div>
							<label>Số điện thoại nhận hàng</label>
							<input value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="09xxxxxxxx" required />
						</div>
					</div>
					<label>Địa chỉ nhận mẫu</label>
					<input
						value={shippingAddress}
						onChange={(e) => setShippingAddress(e.target.value)}
						placeholder="Số nhà, đường, phường/xã, quận/huyện, tỉnh/thành phố..."
					/>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button type="submit" className="primary-button">
						Chốt Đồng ý (AGREED)
					</button>
				</div>
			</form>
		</div>
	);
}

function AddCreatorModal({ onClose, onAdd }) {
	const [tiktokId, setTiktokId] = useState('');
	const [creator, setCreator] = useState(null);
	const [lookupState, setLookupState] = useState({ type: '', message: '' });
	const [form, setForm] = useState({
		source: 'TikToday',
		category: 'Làm đẹp',
		contentPillar: '',
		targetAudience: '',
		email: '',
		phone: '',
		recipient: '',
		address: '',
		campaign: 'Đợt T11',
		productName: '',
		commissionRate: '10',
		fee: '0',
		qualifyImmediately: true,
	});

	function findTikTok() {
		const normalized = tiktokId.trim().replace(/^@/, '').toLowerCase();
		if (!normalized) {
			setLookupState({ type: 'error', message: 'Vui lòng nhập TikTok ID.' });
			return;
		}
		api.post('/kocs/lookup', { tiktokId: normalized, phone: form.phone })
			.then(({ data }) => {
				const res = data.data;
				if (res.blocked) {
					setCreator(null);
					setLookupState({ type: 'error', message: res.message });
				} else if (res.duplicate) {
					setCreator(null);
					setLookupState({ type: 'error', message: `Trùng dữ liệu: ${res.message}` });
				} else {
					setCreator({
						name: res.profile.displayName,
						handle: `@${res.profile.tiktokId}`,
						followers: res.profile.followers || 0,
						gmv: res.profile.gmv || 0,
						avatar: res.profile.displayName.slice(0, 2).toUpperCase(),
					});
					setLookupState({ type: 'ok', message: 'Kênh hợp lệ, có thể khôi phục vào phễu.' });
				}
			})
			.catch(() => {
				setCreator({
					name: `KOC ${tiktokId.trim()}`,
					handle: `@${normalized}`,
					followers: 0,
					gmv: 0,
					avatar: normalized.slice(0, 2).toUpperCase(),
				});
				setLookupState({
					type: 'ok',
					message: 'Kiểm tra chống trùng hoàn tất: Kênh mới chưa từng hợp tác.',
				});
			});
	}

	function updateField(event) {
		const { name, value, type, checked } = event.target;
		setForm({ ...form, [name]: type === 'checkbox' ? checked : value });
	}

	function submit(event) {
		event.preventDefault();
		if (!creator) {
			setLookupState({ type: 'error', message: 'Hãy bấm "Kiểm tra trùng" trước khi lưu.' });
			return;
		}
		const castFee = Number(form.fee) || 0;
		const desiredStage = form.qualifyImmediately ? (castFee > 0 ? 'AWAITING_APPROVAL' : 'QUALIFIED') : 'SOURCED';
		onAdd({
			tiktokId: creator.handle.replace(/^@/, ''),
			displayName: creator.name,
			followers: Number(creator.followers) || 0,
			gmv: Number(creator.gmv) || 0,
			source: form.source,
			category: form.category,
			contentPillar: form.contentPillar,
			targetAudience: form.targetAudience,
			email: form.email,
			phone: form.phone,
			recipient: form.recipient,
			shippingAddress: form.address,
			campaign: form.campaign,
			productName: form.productName,
			commissionRate: Number(form.commissionRate) || 10,
			castFee,
			stage: desiredStage,
		});
	}

	return (
		<div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
			<form className="modal" onSubmit={submit}>
				<div className="modal-header">
					<div>
						<h2>Thêm & Chấm phễu KOC mới</h2>
						<p>Kiểm tra chống trùng theo ID kênh / SĐT và khởi tạo bản ghi hợp tác</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<label>Nguồn tìm kiếm & Số điện thoại</label>
					<div className="grid-2">
						<select name="source" value={form.source} onChange={updateField}>
							<option value="TikToday">Nguồn: TikToday</option>
							<option value="TikTok Liên kết">Nguồn: TikTok Liên kết</option>
							<option value="TikTok">Nguồn: TikTok App</option>
						</select>
						<input name="phone" value={form.phone} onChange={updateField} placeholder="Số điện thoại (kiểm tra trùng)" />
					</div>
					<div className="lookup">
						<input
							value={tiktokId}
							onChange={(event) => setTiktokId(event.target.value)}
							placeholder="Nhập TikTok ID (ví dụ: @minhaireview)"
						/>
						<button type="button" onClick={findTikTok}>
							Kiểm tra trùng
						</button>
					</div>
					{lookupState.message && (
						<div className={`alert-banner ${lookupState.type === 'error' ? 'alert-error' : 'alert-success'}`}>
							{lookupState.message}
						</div>
					)}
					{creator && (
						<div className="found-creator">
							<div className="creator-avatar">{creator.avatar}</div>
							<div>
								<strong>{creator.name}</strong>
								<small>{creator.handle} · Đủ điều kiện liên hệ</small>
							</div>
							<span>Hợp lệ</span>
						</div>
					)}

					<label>Đánh giá Phễu (Danh mục, Tệp khán giả, Content Pillar)</label>
					<div className="grid-2">
						<select name="category" value={form.category} onChange={updateField}>
							<option value="Làm đẹp">Làm đẹp</option>
							<option value="Đời sống">Đời sống</option>
							<option value="Thời trang">Thời trang</option>
							<option value="Chưa phân loại">Chưa phân loại</option>
						</select>
						<input
							name="targetAudience"
							value={form.targetAudience}
							onChange={updateField}
							placeholder="Tệp người xem (VD: Nữ 18-25)"
						/>
					</div>
					<input
						name="contentPillar"
						value={form.contentPillar}
						onChange={updateField}
						placeholder="Tuyến nội dung chính (Content Pillar)"
					/>

					<label>Thông tin Hợp tác & Chính sách</label>
					<div className="grid-2">
						<input
							name="campaign"
							value={form.campaign}
							onChange={updateField}
							placeholder="Tên đợt chiến dịch"
						/>
						<input
							name="productName"
							value={form.productName}
							onChange={updateField}
							placeholder="Sản phẩm gửi mẫu"
						/>
					</div>
					<div className="grid-2">
						<div className="fee-input">
							<input
								name="commissionRate"
								type="number"
								value={form.commissionRate}
								onChange={updateField}
								placeholder="Hoa hồng (%)"
							/>
							<span>%</span>
						</div>
						<div className="fee-input">
							<input
								name="fee"
								type="number"
								value={form.fee}
								onChange={updateField}
								placeholder="Phí Cast (0 = Freecast)"
							/>
							<span>đ</span>
						</div>
					</div>
					<div className="fee-chips">
						<button type="button" onClick={() => setForm({ ...form, fee: '0' })}>
							Freecast (0đ)
						</button>
						<button type="button" onClick={() => setForm({ ...form, fee: '200000' })}>
							200.000đ
						</button>
						<button type="button" onClick={() => setForm({ ...form, fee: '500000' })}>
							500.000đ
						</button>
						<button type="button" onClick={() => setForm({ ...form, fee: '1000000' })}>
							1.000.000đ
						</button>
					</div>

					<label>Email & Địa chỉ nhận mẫu (nếu có)</label>
					<div className="grid-2">
						<input name="email" value={form.email} onChange={updateField} placeholder="Địa chỉ Email" />
						<input name="address" value={form.address} onChange={updateField} placeholder="Địa chỉ giao hàng" />
					</div>

					<label className="checkbox-row">
						<input
							type="checkbox"
							name="qualifyImmediately"
							checked={form.qualifyImmediately}
							onChange={updateField}
						/>
						<span>
							Xác nhận đạt tiêu chí phễu ngay{' '}
							{Number(form.fee) > 0 ? '(Sẽ chuyển sang Chờ Chủ shop duyệt Cast)' : '(Chuyển sang QUALIFIED)'}
						</span>
					</label>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button className="primary-button" type="submit" disabled={!creator}>
						Lưu KOC
					</button>
				</div>
			</form>
		</div>
	);
}

function BulkImportModal({ onClose, onImport }) {
	const [source, setSource] = useState('TikToday');
	const [rawText, setRawText] = useState(
		'minhchau_beauty, 0912345678, Minh Châu Review\nbepnhamay, 0987654321, Bếp Nhà Mây\nminhaireview, 0327637213, Minh Ái',
	);

	function handleSubmit(event) {
		event.preventDefault();
		const lines = rawText
			.split('\n')
			.map((l) => l.trim())
			.filter(Boolean);
		const items = lines.map((line) => {
			const [tiktokId = '', phone = '', displayName = ''] = line.split(/[,\t|]/).map((s) => s.trim());
			return {
				tiktokId,
				phone,
				displayName: displayName || `KOC @${tiktokId.replace(/^@/, '')}`,
				source,
			};
		});
		onImport({ source, items });
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<form className="modal" onSubmit={handleSubmit}>
				<div className="modal-header">
					<div>
						<h2>Nhập hàng loạt & Tự động chống trùng</h2>
						<p>Hệ thống tự động lọc trùng theo ID kênh, Số điện thoại và chặn các KOC đã từng hợp tác</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<label>Nguồn dữ liệu</label>
					<select value={source} onChange={(e) => setSource(e.target.value)}>
						<option value="TikToday">TikToday</option>
						<option value="TikTok Liên kết">TikTok Liên kết (Seller Center)</option>
						<option value="TikTok">TikTok App</option>
					</select>
					<label>Danh sách KOC (Định dạng mỗi dòng: tiktok_id, số_điện_thoại, tên_kênh)</label>
					<textarea
						className="bulk-textarea"
						rows={7}
						value={rawText}
						onChange={(e) => setRawText(e.target.value)}
						placeholder="tiktok_id, 09xxxxxxxx, Tên hiển thị"
					/>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button type="submit" className="primary-button">
						Kiểm tra trùng & Nhập danh sách
					</button>
				</div>
			</form>
		</div>
	);
}

function QualifyKocModal({ creator, onClose, onSubmit }) {
	const [form, setForm] = useState({
		contentPillar: creator.contentPillar || '',
		targetAudience: creator.targetAudience || '',
		campaign: creator.campaign || 'Đợt T11',
		productName: creator.productName || '',
		commissionRate: creator.commissionRate ?? 10,
		castFee: creator.castFee ?? 0,
	});

	function handleChange(e) {
		setForm({ ...form, [e.target.name]: e.target.value });
	}

	function handleSubmit(e) {
		e.preventDefault();
		onSubmit({
			...form,
			commissionRate: Number(form.commissionRate) || 0,
			castFee: Number(form.castFee) || 0,
		});
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<form className="modal" onSubmit={handleSubmit}>
				<div className="modal-header">
					<div>
						<h2>Chấm phễu & Thiết lập Hợp tác ({creator.name})</h2>
						<p>Đánh giá tiêu chí kênh và cấu hình gói đề xuất hợp tác</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<label>Đánh giá nội dung & Tệp người xem</label>
					<input
						name="contentPillar"
						value={form.contentPillar}
						onChange={handleChange}
						placeholder="Content Pillar chính của kênh"
						required
					/>
					<input
						name="targetAudience"
						value={form.targetAudience}
						onChange={handleChange}
						placeholder="Tệp khán giả mục tiêu"
					/>

					<label>Chiến dịch & Sản phẩm</label>
					<div className="grid-2">
						<input
							name="campaign"
							value={form.campaign}
							onChange={handleChange}
							placeholder="Tên chiến dịch"
							required
						/>
						<input
							name="productName"
							value={form.productName}
							onChange={handleChange}
							placeholder="Tên sản phẩm mẫu"
							required
						/>
					</div>

					<label>Hoa hồng & Phí Cast</label>
					<div className="grid-2">
						<div className="fee-input">
							<input
								name="commissionRate"
								type="number"
								value={form.commissionRate}
								onChange={handleChange}
								placeholder="% Hoa hồng"
							/>
							<span>%</span>
						</div>
						<div className="fee-input">
							<input
								name="castFee"
								type="number"
								value={form.castFee}
								onChange={handleChange}
								placeholder="Phí Cast (0 = Freecast)"
							/>
							<span>đ</span>
						</div>
					</div>
					<div className="alert-banner alert-info">
						{Number(form.castFee) > 0
							? `Có phí Cast (${formatNumber(form.castFee)}đ): Hồ sơ sẽ chuyển sang trạng thái Chờ Chủ shop duyệt (AWAITING_APPROVAL).`
							: 'Freecast (0đ): Hồ sơ sẽ chuyển thẳng sang trạng thái Đạt phễu (QUALIFIED) để gửi lời mời.'}
					</div>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button type="submit" className="primary-button">
						Lưu & Xác nhận đạt phễu
					</button>
				</div>
			</form>
		</div>
	);
}

function RejectKocModal({ creator, onClose, onSubmit }) {
	const [rejectCode, setRejectCode] = useState(
		creator.stage === 'AWAITING_APPROVAL' ? 'F04' : 'F01',
	);
	const [rejectNote, setRejectNote] = useState('');

	function handleSubmit(e) {
		e.preventDefault();
		onSubmit({ rejectCode, rejectNote });
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<form className="modal" onSubmit={handleSubmit}>
				<div className="modal-header">
					<div>
						<h2>Loại KOC khỏi phễu ({creator.name})</h2>
						<p>Ghi nhận mã lý do không đạt tiêu chí để thống kê chất lượng nguồn</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<label>Mã lý do loại</label>
					<select value={rejectCode} onChange={(e) => setRejectCode(e.target.value)}>
						{FILTER_REJECT_REASONS.map((r) => (
							<option key={r.code} value={r.code}>
								{r.label}
							</option>
						))}
					</select>
					<label>Ghi chú bổ sung</label>
					<input
						value={rejectNote}
						onChange={(e) => setRejectNote(e.target.value)}
						placeholder="Nhập ghi chú chi tiết (nếu có)..."
					/>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button type="submit" className="primary-button danger-btn">
						Xác nhận Loại
					</button>
				</div>
			</form>
		</div>
	);
}

function formatNumber(value) {
	return new Intl.NumberFormat('vi-VN', { notation: 'compact', maximumFractionDigits: 1 }).format(value || 0);
}

function InboxView({ conversations, setConversations, apiReady }) {
	const [selectedConversation, setSelectedConversation] = useState(conversations[0]);
	const [messages, setMessages] = useState([]);
	const [draft, setDraft] = useState('');
	const [loading, setLoading] = useState(false);
	const [sending, setSending] = useState(false);
	const [error, setError] = useState('');
	const [search, setSearch] = useState('');
	const [zaloStatus, setZaloStatus] = useState(null);
	const [showQr, setShowQr] = useState(false);
	const visibleConversations = conversations.filter((conversation) =>
		`${conversation.contactName} ${conversation.contactHandle} ${conversation.contactPhone || ''}`
			.toLowerCase()
			.includes(search.toLowerCase()),
	);

	useEffect(() => {
		api.get('/zalo/status')
			.then(({ data }) => setZaloStatus(data.data))
			.catch(() => {});
	}, []);

	useEffect(() => {
		if (!showQr) return undefined;
		const timer = window.setInterval(
			() =>
				api
					.get('/zalo/status')
					.then(({ data }) => {
						setZaloStatus(data.data);
						if (data.data.connected) setShowQr(false);
					})
					.catch(() => {}),
			2000,
		);
		return () => window.clearInterval(timer);
	}, [showQr]);

	useEffect(() => {
		if (!selectedConversation && visibleConversations[0]) setSelectedConversation(visibleConversations[0]);
		if (selectedConversation && !conversations.some((item) => item._id === selectedConversation._id))
			setSelectedConversation(visibleConversations[0]);
	}, [conversations, selectedConversation]);

	useEffect(() => {
		if (!selectedConversation || selectedConversation._id.startsWith('preview-')) return;
		setLoading(true);
		setError('');
		api.get(`/inbox/conversations/${selectedConversation._id}/messages`)
			.then(({ data }) => setMessages(data.data))
			.catch(() => setError('Không thể tải lịch sử tin nhắn.'))
			.finally(() => setLoading(false));
	}, [selectedConversation]);

	function sendMessage(event) {
		event.preventDefault();
		const text = draft.trim();
		if (!text || !selectedConversation || selectedConversation._id.startsWith('preview-')) return;
		setSending(true);
		api.post(`/inbox/conversations/${selectedConversation._id}/messages`, { text })
			.then(({ data }) => {
				setMessages((current) => [...current, data.data]);
				setConversations((current) =>
					current.map((item) =>
						item._id === selectedConversation._id
							? { ...item, lastMessage: text, lastMessageAt: data.data.sentAt }
							: item,
					),
				);
				setSelectedConversation((current) => ({ ...current, lastMessage: text, lastMessageAt: data.data.sentAt }));
				setDraft('');
			})
			.catch(() => setError('Không thể gửi tin nhắn.'))
			.finally(() => setSending(false));
	}

	function startQrLogin() {
		setError('');
		setShowQr(true);
		api.post('/zalo/qr/start')
			.then(({ data }) => setZaloStatus(data.data))
			.catch((requestError) => setError(requestError.response?.data?.message || 'Không thể tạo mã QR.'));
	}

	return (
		<div className="inbox-page">
			<div className="page-heading">
				<div>
					<p className="eyebrow">GIAO TIẾP ĐA KÊNH</p>
					<h1>
						Hộp thư KOC <span>{conversations.length} hội thoại</span>
					</h1>
				</div>
				<div className="inbox-actions">
					{!zaloStatus?.connected && (
						<button className="secondary-button" onClick={startQrLogin}>
							Kết nối Zalo cá nhân (QR)
						</button>
					)}
					{zaloStatus?.connected && <div className="zalo-connected">Zalo đã kết nối</div>}
				</div>
			</div>
			{showQr && (
				<div className="qr-panel">
					<div>
						<strong>Đăng nhập Zalo qua mã QR</strong>
						<p>
							{zaloStatus?.state === 'scanned'
								? 'Đã quét mã, vui lòng xác nhận trên điện thoại...'
								: 'Mở ứng dụng Zalo trên điện thoại và quét mã QR bên cạnh để đồng bộ tin nhắn.'}
						</p>
						{zaloStatus?.error && <div className="alert-banner alert-error">{zaloStatus.error}</div>}
					</div>
					{zaloStatus?.qrImage ? (
						<img src={zaloStatus.qrImage} alt="Mã QR đăng nhập Zalo" />
					) : (
						<div className="qr-placeholder">Đang khởi tạo mã QR...</div>
					)}
				</div>
			)}
			<div className="inbox-layout">
				<div className="conversation-list">
					<div className="inbox-search">
						<input
							value={search}
							onChange={(event) => setSearch(event.target.value)}
							placeholder="Tìm hội thoại theo tên, ID, SĐT..."
						/>
					</div>
					<div className="inbox-tabs">
						<b>Tất cả hội thoại</b>
						<span>Chưa đọc</span>
					</div>
					{visibleConversations.map((conversation) => (
						<button
							className={
								selectedConversation?._id === conversation._id ? 'conversation active' : 'conversation'
							}
							key={conversation._id}
							onClick={() => setSelectedConversation(conversation)}
						>
							<div className="creator-avatar">{conversation.contactName.slice(0, 2).toUpperCase()}</div>
							<div>
								<strong>{conversation.contactName}</strong>
								<small>
									{conversation.contactHandle} · {conversation.lastMessage}
								</small>
							</div>
							<time>
								{new Date(conversation.lastMessageAt).toLocaleTimeString('vi-VN', {
									hour: '2-digit',
									minute: '2-digit',
								})}
							</time>
						</button>
					))}
					{!visibleConversations.length && <div className="empty-state">Không tìm thấy hội thoại.</div>}
				</div>
				<div className="chat-panel">
					{selectedConversation ? (
						<>
							<div className="chat-header">
								<div className="creator-avatar">{selectedConversation.contactName.slice(0, 2).toUpperCase()}</div>
								<div>
									<strong>{selectedConversation.contactName}</strong>
									<small>
										{selectedConversation.contactHandle}
										{selectedConversation.contactPhone ? ` · ${selectedConversation.contactPhone}` : ''}
									</small>
								</div>
								<span className="channel-tag">
									{selectedConversation.channel === 'zalo' ? 'Kênh Zalo' : 'Lưu nội bộ'}
								</span>
							</div>
							<div className="chat-messages">
								{loading && <div className="empty-state">Đang tải lịch sử tin nhắn...</div>}
								{error && <div className="alert-banner alert-error">{error}</div>}
								{!loading && !messages.length && <div className="empty-state">Chưa có tin nhắn trong hội thoại này.</div>}
								{messages.map((message) => (
									<div
										className={`message ${message.direction === 'inbound' ? 'incoming' : 'outgoing'}`}
										key={message._id}
									>
										<span>{message.text}</span>
										<small>
											{new Date(message.sentAt).toLocaleTimeString('vi-VN', {
												hour: '2-digit',
												minute: '2-digit',
											})}{' '}
											· {message.status}
										</small>
									</div>
								))}
							</div>
							<form className="chat-composer" onSubmit={sendMessage}>
								<input
									value={draft}
									onChange={(event) => setDraft(event.target.value)}
									placeholder={
										selectedConversation._id.startsWith('preview-')
											? 'Chế độ xem trước chưa thể gửi tin'
											: 'Nhập nội dung tin nhắn gửi KOC...'
									}
									disabled={sending || selectedConversation._id.startsWith('preview-')}
								/>
								<button
									className="primary-button"
									disabled={sending || !draft.trim() || selectedConversation._id.startsWith('preview-')}
								>
									{sending ? 'Đang gửi...' : 'Gửi tin'}
								</button>
							</form>
						</>
					) : (
						<div className="empty-state">Chọn một hội thoại bên trái để xem lịch sử trao đổi.</div>
					)}
				</div>
			</div>
		</div>
	);
}

export default App;
