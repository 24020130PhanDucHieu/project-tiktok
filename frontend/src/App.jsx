import { useEffect, useMemo, useState } from 'react';
import api from './lib/axios';
import LoginPage from './pages/LoginPage';
import { clearSession, getToken, getUser } from './stores/useAuthStore';

const STAGE_META = {
	SOURCED: { label: 'Mới tìm (Chờ chấm)', tone: 'stage-sourced' },
	AWAITING_APPROVAL: { label: 'Chờ Chủ shop duyệt Cast', tone: 'stage-approval' },
	QUALIFIED: { label: 'Đạt phễu (Sẵn sàng mời)', tone: 'stage-qualified' },
	FILTERED_OUT: { label: 'Đã loại khỏi phễu', tone: 'stage-filtered' },
	CONTACTED: { label: 'Đã gửi lời mời', tone: 'stage-contacted' },
	REPLIED: { label: 'KOC đã phản hồi', tone: 'stage-qualified' },
	AGREED: { label: 'Đồng ý hợp tác', tone: 'stage-qualified' },
	DECLINED: { label: 'KOC từ chối', tone: 'stage-filtered' },
};

const FILTER_REJECT_REASONS = [
	{ code: 'F01', label: 'F01 · Sai tệp người xem (Target Audience)' },
	{ code: 'F02', label: 'F02 · Content pillar không phù hợp sản phẩm' },
	{ code: 'F03', label: 'F03 · Tương tác / GMV TikTok quá thấp' },
	{ code: 'F04', label: 'F04 · Giá Cast vượt ngân sách (Chủ shop từ chối)' },
	{ code: 'F05', label: 'F05 · Kênh vi phạm chính sách / Nội dung nhạy cảm' },
];

const initialCreators = [
	{
		id: 'seed-1',
		name: 'Minh Ái 🎀',
		handle: '@minhaireview',
		tiktokId: 'minhaireview',
		contact: '0327637213',
		phone: '0327637213',
		followers: '12,7k',
		gmv: '1Mđ+',
		category: 'Làm đẹp',
		contentPillar: 'Review skincare & unbox mỹ phẩm',
		targetAudience: 'Nữ 18-28 tuổi',
		source: 'TikToday',
		campaign: 'Đợt T11 - Serum B5',
		productName: 'Serum phục hồi B5',
		commissionRate: 15,
		castType: 'freecast',
		castFee: 0,
		stage: 'QUALIFIED',
		daHopTac: false,
		avatar: 'MA',
		tone: 'pink',
	},
	{
		id: 'seed-2',
		name: 'Gia đình trái cây 🍋🍊',
		handle: '@giadinhtraicay1202',
		tiktokId: 'giadinhtraicay1202',
		contact: '0866073103',
		phone: '0866073103',
		followers: '3,1k',
		gmv: '5Mđ+',
		category: 'Đời sống',
		contentPillar: 'Vlog nấu ăn gia đình',
		targetAudience: 'Mẹ bỉm & nội trợ 24-35',
		source: 'TikTok Liên kết',
		campaign: 'Đợt T11 - Máy ép chậm',
		productName: 'Máy ép chậm mini',
		commissionRate: 10,
		castType: 'cast',
		castFee: 500000,
		stage: 'AWAITING_APPROVAL',
		daHopTac: false,
		avatar: 'GT',
		tone: 'red',
	},
	{
		id: 'seed-3',
		name: 'Cô bé lười đi tập 🧚',
		handle: '@changditap',
		tiktokId: 'changditap',
		contact: '—',
		phone: '',
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
		daHopTac: false,
		avatar: 'C',
		tone: 'green',
	},
];

const previewConversations = [
	{
		_id: 'preview-1',
		contactName: 'Gia đình trái cây 🍋🍊',
		contactHandle: '@giadinhtraicay1202',
		lastMessage: 'Mình đã nhận được thông tin rồi ạ',
		lastMessageAt: new Date().toISOString(),
		unreadCount: 2,
	},
];

const navItems = [
	['▱', 'Hộp thư'],
	['▣', 'Danh bạ KOC'],
];

function mapItemToCreator(item) {
	return {
		...item,
		id: item._id || item.id,
		name: item.displayName || item.name,
		handle: `@${(item.tiktokId || '').replace(/^@/, '')}`,
		contact: item.phone || '—',
		followers: typeof item.followers === 'number' ? formatNumber(item.followers) : item.followers || '0',
		gmv: typeof item.gmv === 'number' ? (item.gmv ? `${formatNumber(item.gmv)}đ+` : '—') : item.gmv || '—',
		source: item.source || 'TikToday',
		stage: item.stage || 'SOURCED',
		castFee: Number(item.castFee) || 0,
		castType: Number(item.castFee) > 0 ? 'cast' : 'freecast',
		daHopTac: Boolean(item.daHopTac),
		avatar: (item.displayName || item.name || 'K').slice(0, 2).toUpperCase(),
		tone: item.tone || 'green',
	};
}

function App() {
	const [authenticated, setAuthenticated] = useState(Boolean(getToken()));
	const [currentUser, setCurrentUser] = useState(getUser());
	const [creators, setCreators] = useState(initialCreators);
	const [conversations, setConversations] = useState(previewConversations);
	const [apiReady, setApiReady] = useState(false);
	const [isModalOpen, setModalOpen] = useState(false);
	const [isBulkOpen, setBulkOpen] = useState(false);
	const [evaluatingCreator, setEvaluatingCreator] = useState(null);
	const [rejectingCreator, setRejectingCreator] = useState(null);
	const [selectedNav, setSelectedNav] = useState('Danh bạ KOC');
	const [query, setQuery] = useState('');
	const [category, setCategory] = useState('Tất cả');
	const [stageFilter, setStageFilter] = useState('ALL');
	const [sourceFilter, setSourceFilter] = useState('ALL');
	const [actionError, setActionError] = useState('');
	const [actionNotice, setActionNotice] = useState('');

	useEffect(() => {
		if (!authenticated) return;
		Promise.all([api.get('/kocs'), api.get('/inbox/conversations')])
			.then(([kocResponse, inboxResponse]) => {
				setCreators(kocResponse.data.data.items.map(mapItemToCreator));
				setConversations(inboxResponse.data.data);
				setApiReady(true);
			})
			.catch(() => setApiReady(false));
	}, [authenticated]);

	const filteredCreators = useMemo(
		() =>
			creators.filter((creator) => {
				const matchesQuery = `${creator.name} ${creator.handle} ${creator.contact} ${creator.campaign || ''} ${creator.productName || ''}`
					.toLowerCase()
					.includes(query.toLowerCase());
				const matchesCategory = category === 'Tất cả' || creator.category === category;
				const matchesStage = stageFilter === 'ALL' || creator.stage === stageFilter;
				const matchesSource = sourceFilter === 'ALL' || creator.source === sourceFilter;
				return matchesQuery && matchesCategory && matchesStage && matchesSource;
			}),
		[category, creators, query, stageFilter, sourceFilter],
	);

	const stageCounts = useMemo(() => {
		return {
			total: creators.length,
			sourced: creators.filter((c) => c.stage === 'SOURCED' && !c.daHopTac).length,
			awaitingApproval: creators.filter((c) => c.stage === 'AWAITING_APPROVAL' && !c.daHopTac).length,
			qualified: creators.filter((c) => c.stage === 'QUALIFIED' && !c.daHopTac).length,
			blockedOrFiltered: creators.filter((c) => c.stage === 'FILTERED_OUT' || c.daHopTac).length,
		};
	}, [creators]);

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
						? `Đã thêm ${mapped.name}. KOC có phí Cast (${formatNumber(mapped.castFee)}đ) -> Chuyển sang Chờ Chủ shop duyệt!`
						: `Đã thêm ${mapped.name} vào phễu (${STAGE_META[mapped.stage]?.label || mapped.stage}).`,
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
			// Cho phép trải nghiệm trực tiếp cả khi dùng dữ liệu mẫu offline
			setCreators((current) =>
				current.map((item) => {
					if (item.id !== creator.id) return item;
					const nextFee = payload.castFee !== undefined ? Number(payload.castFee) : item.castFee;
					let nextStage = item.stage;
					if (payload.action === 'qualify') nextStage = nextFee > 0 ? 'AWAITING_APPROVAL' : 'QUALIFIED';
					if (payload.action === 'approve_cast') nextStage = 'QUALIFIED';
					if (payload.action === 'filter_out') nextStage = 'FILTERED_OUT';
					return {
						...item,
						...payload,
						castFee: nextFee,
						castType: nextFee > 0 ? 'cast' : 'freecast',
						stage: nextStage,
						daHopTac: payload.daHopTac !== undefined ? payload.daHopTac : item.daHopTac,
					};
				}),
			);
			setEvaluatingCreator(null);
			setRejectingCreator(null);
			notifySuccess(successMessage);
			return;
		}

		api.patch(`/kocs/${creator.id}/stage`, payload)
			.then(({ data }) => {
				const updated = mapItemToCreator(data.data);
				setCreators((current) => current.map((item) => (item.id === creator.id ? updated : item)));
				setEvaluatingCreator(null);
				setRejectingCreator(null);
				notifySuccess(successMessage);
			})
			.catch((error) => notifyError(error, 'Không thể cập nhật trạng thái phễu KOC.'));
	}

	function deleteCreator(creator) {
		if (!window.confirm(`Xóa ${creator.name} khỏi danh bạ?`)) return;
		api.delete(`/kocs/${creator.id}`)
			.then(() => {
				setCreators((current) => current.filter((item) => item.id !== creator.id));
				notifySuccess(`Đã lưu trữ ${creator.name}.`);
			})
			.catch((error) => notifyError(error, 'Không thể xóa KOC.'));
	}

	function emailCreator(creator) {
		if (!creator.email) {
			setActionError('KOC này chưa có email.');
			return;
		}
		api.post(`/kocs/${creator.id}/email`, {})
			.then(() => notifySuccess(`Đã gửi email tới ${creator.email}.`))
			.catch((error) => notifyError(error, 'Không thể gửi email.'));
	}

	return (
		<div className="app-shell">
			<aside className="sidebar">
				<div className="brand">
					<span className="brand-mark">N</span>
					<span>Tiktoday</span>
					<small>BOOKING</small>
				</div>
				<button className="search-button" onClick={() => setModalOpen(true)}>
					<span>⌕</span> Tìm & Chấm phễu KOC <b>↗</b>
				</button>
				<nav>
					{navItems.map(([icon, label]) => (
						<button
							className={selectedNav === label ? 'nav-item active' : 'nav-item'}
							key={label}
							onClick={() => setSelectedNav(label)}
						>
							<span>{icon}</span>
							{label}
						</button>
					))}
				</nav>
				<button
					className="account"
					onClick={() => {
						clearSession();
						setAuthenticated(false);
						setCurrentUser(null);
					}}
				>
					<div className="account-avatar">{currentUser?.name?.slice(0, 1) || 'U'}</div>
					<div>
						<strong>{currentUser?.name || 'Workspace user'}</strong>
						<small>{currentUser?.email || ''}</small>
					</div>
					<span>↪</span>
				</button>
			</aside>

			<main className="main-content">
				<header className="topbar">
					<div className="crumb">
						{selectedNav} <span>/</span>{' '}
						<b>{selectedNav === 'Hộp thư' ? 'Tất cả hội thoại' : 'Giai đoạn 1 · Tìm & Lọc KOC'}</b>
					</div>
					<div className="top-actions">
						<button className="ghost-button">⌕</button>
						<button className="ghost-button">?</button>
						<div className="notification">{stageCounts.awaitingApproval}</div>
						<div className="mini-avatar">Đ</div>
					</div>
				</header>
				<section className="content">
					{selectedNav === 'Hộp thư' ? (
						<InboxView conversations={conversations} setConversations={setConversations} apiReady={apiReady} />
					) : (
						<>
							<div className="page-heading">
								<div>
									<p className="eyebrow">GIAI ĐOẠN 1 · SOURCING, DEDUPE & CHẤM PHỄU KOC</p>
									<h1>
										Phễu KOC <span>{creators.length} KOC</span>
									</h1>
								</div>
								<div className="heading-actions">
									<button
										className="secondary-button"
										onClick={() => {
											setActionError('');
											setBulkOpen(true);
										}}
									>
										📥 Nhập hàng loạt (Dedupe)
									</button>
									<button
										className="primary-button"
										onClick={() => {
											setActionError('');
											setModalOpen(true);
										}}
									>
										<span>♙</span> Thêm & Chấm KOC <b>＋</b>
									</button>
								</div>
							</div>

							{actionError && <div className="status error-status">{actionError}</div>}
							{actionNotice && <div className="status">{actionNotice}</div>}

							<div className="stats-row">
								<div className="stat-clickable" onClick={() => setStageFilter('ALL')}>
									<strong>{stageCounts.total}</strong>
									<span>Tổng KOC trong kho</span>
								</div>
								<div className="stat-clickable" onClick={() => setStageFilter('SOURCED')}>
									<strong className="text-blue">{stageCounts.sourced}</strong>
									<span>Mới tìm (Chờ chấm A6)</span>
								</div>
								<div className="stat-clickable" onClick={() => setStageFilter('AWAITING_APPROVAL')}>
									<strong className="text-amber">{stageCounts.awaitingApproval}</strong>
									<span>Chờ Chủ shop duyệt Cast (A11)</span>
								</div>
								<div className="stat-clickable" onClick={() => setStageFilter('QUALIFIED')}>
									<strong className="text-emerald">{stageCounts.qualified}</strong>
									<span>Đạt phễu (Sẵn sàng mời GĐ2)</span>
								</div>
								<div className="stat-clickable" onClick={() => setStageFilter('FILTERED_OUT')}>
									<strong className="text-rose">{stageCounts.blockedOrFiltered}</strong>
									<span>Đã loại / Chặn (da_hop_tac)</span>
								</div>
							</div>

							<div className="toolbar">
								<select value={stageFilter} onChange={(e) => setStageFilter(e.target.value)}>
									<option value="ALL">Tất cả trạng thái phễu</option>
									<option value="SOURCED">1. Mới tìm (SOURCED)</option>
									<option value="AWAITING_APPROVAL">2. Chờ duyệt Cast (AWAITING_APPROVAL)</option>
									<option value="QUALIFIED">3. Đạt phễu (QUALIFIED)</option>
									<option value="FILTERED_OUT">4. Đã loại (FILTERED_OUT)</option>
								</select>
								<select value={sourceFilter} onChange={(e) => setSourceFilter(e.target.value)}>
									<option value="ALL">Tất cả nguồn tìm (A1)</option>
									<option value="TikToday">Nguồn: TikToday</option>
									<option value="TikTok Liên kết">Nguồn: TikTok Liên kết</option>
									<option value="TikTok">Nguồn: TikTok App</option>
								</select>
								<select value={category} onChange={(event) => setCategory(event.target.value)}>
									<option value="Tất cả">Tất cả thư mục</option>
									<option value="Làm đẹp">Làm đẹp</option>
									<option value="Đời sống">Đời sống</option>
									<option value="Thời trang">Thời trang</option>
									<option value="Chưa phân loại">Chưa phân loại</option>
								</select>
								<div className="table-search">
									<span>⌕</span>
									<input
										value={query}
										onChange={(event) => setQuery(event.target.value)}
										placeholder="Tìm tên, ID kênh, SĐT, đợt chiến dịch, sản phẩm..."
									/>
								</div>
							</div>

							<div className="table-wrap">
								<table>
									<thead>
										<tr>
											<th>#</th>
											<th>KOC & NGUỒN (A1)</th>
											<th>SĐT / LIÊN HỆ (A3)</th>
											<th>PHỄU & ĐỢT HỢP TÁC (A6, A9)</th>
											<th>SỐ LIỆU</th>
											<th>LOẠI / PHÍ CAST (A10)</th>
											<th>TRẠNG THÁI (FSM)</th>
											<th>THAO TÁC GIAI ĐOẠN 1</th>
										</tr>
									</thead>
									<tbody>
										{filteredCreators.map((creator, index) => {
											const stageInfo = STAGE_META[creator.stage] || {
												label: creator.stage,
												tone: 'stage-sourced',
											};
											return (
												<tr key={creator.id} className={creator.daHopTac ? 'row-blocked' : ''}>
													<td className="muted">{index + 1}</td>
													<td>
														<div className="creator">
															<div className={`creator-avatar ${creator.tone}`}>{creator.avatar}</div>
															<div>
																<strong>{creator.name}</strong>
																<small>
																	{creator.handle} <span className="source-pill">{creator.source}</span>
																</small>
															</div>
														</div>
													</td>
													<td>
														<div>{creator.contact}</div>
														{creator.email && <small className="muted">{creator.email}</small>}
													</td>
													<td>
														<div className="funnel-cell">
															<span className="category-badge">{creator.category}</span>
															{creator.contentPillar && (
																<small className="pillar-text">🎯 {creator.contentPillar}</small>
															)}
															{creator.campaign && (
																<small className="campaign-text">
																	📦 {creator.campaign} · {creator.productName || 'SP'} ({creator.commissionRate || 10}% HH)
																</small>
															)}
														</div>
													</td>
													<td>
														<div>
															<b>{creator.followers}</b> flw
														</div>
														<small className="muted">GMV: {creator.gmv}</small>
													</td>
													<td>
														{creator.castFee > 0 ? (
															<span className="cast-badge cast-paid">
																💰 Cast: {formatNumber(creator.castFee)}đ
															</span>
														) : (
															<span className="cast-badge cast-free">🎁 Freecast (0đ)</span>
														)}
													</td>
													<td>
														{creator.daHopTac ? (
															<span className="stage-badge stage-blocked">
																⛔ Chặn (da_hop_tac = true)
															</span>
														) : (
															<span className={`stage-badge ${stageInfo.tone}`}>{stageInfo.label}</span>
														)}
														{creator.stage === 'FILTERED_OUT' && creator.rejectCode && (
															<small className="reject-reason">
																Lý do: <b>{creator.rejectCode}</b> {creator.rejectNote ? `(${creator.rejectNote})` : ''}
															</small>
														)}
													</td>
													<td className="row-actions">
														{!creator.daHopTac && creator.stage === 'SOURCED' && (
															<>
																<button
																	className="action-chip chip-qualify"
																	title="Chấm đạt phễu & Tạo bản ghi hợp tác (A6 -> A9)"
																	onClick={() => setEvaluatingCreator(creator)}
																>
																	✓ Chấm đạt
																</button>
																<button
																	className="action-chip chip-reject"
																	title="Không đạt tiêu chí phễu (A8)"
																	onClick={() => setRejectingCreator(creator)}
																>
																	✕ Loại
																</button>
															</>
														)}

														{!creator.daHopTac && creator.stage === 'AWAITING_APPROVAL' && (
															<>
																<button
																	className="action-chip chip-approve"
																	title="Chủ shop duyệt giá Cast (A11 -> QUALIFIED)"
																	onClick={() =>
																		updateStage(
																			creator,
																			{ action: 'approve_cast' },
																			`Chủ shop đã duyệt giá Cast cho ${creator.name} -> Chuyển sang QUALIFIED!`,
																		)
																	}
																>
																	👑 Duyệt Cast
																</button>
																<button
																	className="action-chip chip-reject"
																	title="Chủ shop không duyệt giá Cast (A11 -> A8)"
																	onClick={() => setRejectingCreator(creator)}
																>
																	✕ Từ chối
																</button>
															</>
														)}

														{!creator.daHopTac && creator.stage === 'QUALIFIED' && (
															<button
																className="action-chip chip-edit"
																title="Sửa thông tin hợp tác / Chấm lại"
																onClick={() => setEvaluatingCreator(creator)}
															>
																✎ Sửa hợp tác
															</button>
														)}

														<button
															className={`icon-button ${creator.daHopTac ? 'active-block' : ''}`}
															title={
																creator.daHopTac
																	? 'Đang chặn (da_hop_tac = true). Bấm để gỡ chặn'
																	: 'Đánh dấu Đã từng hợp tác (da_hop_tac = true) để chặn liên hệ (A5)'
															}
															onClick={() =>
																updateStage(
																	creator,
																	{ action: 'toggle_collaborated', daHopTac: !creator.daHopTac },
																	!creator.daHopTac
																		? `Đã bật cờ da_hop_tac = true cho ${creator.name} (Chặn liên hệ).`
																		: `Đã gỡ chặn da_hop_tac cho ${creator.name}.`,
																)
															}
														>
															⊘
														</button>
														<button className="icon-button" title="Gửi email" onClick={() => emailCreator(creator)}>
															✉
														</button>
														<button className="icon-button danger" title="Xóa KOC" onClick={() => deleteCreator(creator)}>
															×
														</button>
													</td>
												</tr>
											);
										})}
									</tbody>
								</table>
								{filteredCreators.length === 0 && (
									<div className="empty-state">Không tìm thấy KOC phù hợp với bộ lọc hiện tại.</div>
								)}
								<footer className="table-footer">
									Hiển thị <b>{filteredCreators.length ? 1 : 0}-{filteredCreators.length}</b> / {filteredCreators.length}{' '}
									<select>
										<option>20 / trang</option>
									</select>
									<div className="pagination">
										<button>‹</button>
										<button className="current">1</button>
										<button>›</button>
									</div>
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
								? `Đã chấm đạt ${evaluatingCreator.name}. Vì có phí Cast (${formatNumber(payload.castFee)}đ) -> Chuyển sang Chờ Chủ shop duyệt (A11)!`
								: `Đã chấm đạt ${evaluatingCreator.name} (Freecast) -> Sẵn sàng gửi lời mời Giai đoạn 2!`,
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
							`Đã loại ${rejectingCreator.name} khỏi phễu (Mã lý do: ${rejectCode}).`,
						)
					}
				/>
			)}
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
					setLookupState({ type: 'error', message: `⛔ ${res.message}` });
				} else if (res.duplicate) {
					setCreator(null);
					setLookupState({ type: 'error', message: `⚠️ Dedupe tự động: ${res.message}` });
				} else {
					setCreator({
						name: res.profile.displayName,
						handle: `@${res.profile.tiktokId}`,
						followers: res.profile.followers || 0,
						gmv: res.profile.gmv || 0,
						avatar: res.profile.displayName.slice(0, 2).toUpperCase(),
						tone: 'green',
					});
					setLookupState({ type: 'ok', message: 'Kênh hợp lệ, có thể khôi phục và đưa vào phễu.' });
				}
			})
			.catch(() => {
				setCreator({
					name: `KOC ${tiktokId.trim()}`,
					handle: `@${normalized}`,
					followers: 0,
					gmv: 0,
					avatar: normalized.slice(0, 2).toUpperCase(),
					tone: 'green',
				});
				setLookupState({
					type: 'ok',
					message: '✓ Dedupe OK: Kênh mới chưa từng hợp tác, đủ điều kiện đưa vào phễu!',
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
			setLookupState({ type: 'error', message: 'Hãy bấm "Kiểm tra Dedupe" trước khi thêm KOC.' });
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
						<h2>A1–A10 · Tìm, Dedupe & Chấm phễu KOC</h2>
						<p>Kiểm tra trùng ID kênh / SĐT, chặn da_hop_tac và tạo bản ghi Hợp tác</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<label>A1 · Nguồn tìm & ID kênh TikTok</label>
					<div className="grid-2">
						<select name="source" value={form.source} onChange={updateField}>
							<option value="TikToday">Nguồn: TikToday</option>
							<option value="TikTok Liên kết">Nguồn: TikTok Liên kết</option>
							<option value="TikTok">Nguồn: TikTok App</option>
						</select>
						<input name="phone" value={form.phone} onChange={updateField} placeholder="SĐT (để Dedupe A3)" />
					</div>
					<div className="lookup">
						<input
							value={tiktokId}
							onChange={(event) => setTiktokId(event.target.value)}
							placeholder="Nhập TikTok ID * (VD: @minhaireview)"
						/>
						<button type="button" onClick={findTikTok}>
							⌕ &nbsp;Kiểm tra Dedupe (A3/A4)
						</button>
					</div>
					{lookupState.message && (
						<div className={`status ${lookupState.type === 'error' ? 'error-status' : ''}`}>
							{lookupState.message}
						</div>
					)}
					{creator && (
						<div className="found-creator">
							<div className={`creator-avatar ${creator.tone}`}>{creator.avatar}</div>
							<div>
								<strong>{creator.name}</strong>
								<small>{creator.handle} · Chưa từng hợp tác (da_hop_tac = false)</small>
							</div>
							<span>✓ Hợp lệ</span>
						</div>
					)}

					<label>A6 · Chấm phễu KOC (Danh mục, Tệp, Content Pillar)</label>
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
							placeholder="Tệp khán giả (VD: Nữ 18-25)"
						/>
					</div>
					<input
						name="contentPillar"
						value={form.contentPillar}
						onChange={updateField}
						placeholder="Content Pillar (VD: Review mỹ phẩm, Vlog gia đình...)"
					/>

					<label>A9 & A10 · Bản ghi Hợp tác & Phí Cast (Freecast / Cast)</label>
					<div className="grid-2">
						<input
							name="campaign"
							value={form.campaign}
							onChange={updateField}
							placeholder="Đợt booking (VD: Đợt T11)"
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
							<span>% HH</span>
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
							🎁 Freecast (0đ)
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

					<label>Thông tin liên hệ bổ sung</label>
					<div className="grid-2">
						<input name="email" value={form.email} onChange={updateField} placeholder="Email" />
						<input name="recipient" value={form.recipient} onChange={updateField} placeholder="Người nhận mẫu" />
					</div>
					<input name="address" value={form.address} onChange={updateField} placeholder="Địa chỉ nhận mẫu" />

					<label className="checkbox-row">
						<input
							type="checkbox"
							name="qualifyImmediately"
							checked={form.qualifyImmediately}
							onChange={updateField}
						/>
						<span>
							Đánh dấu <b>Đạt tiêu chí phễu (A7 = Có)</b> ngay khi thêm{' '}
							{Number(form.fee) > 0 ? '→ Chuyển sang Chờ Chủ shop duyệt Cast (A11)' : '→ Chuyển sang QUALIFIED'}
						</span>
					</label>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button className="primary-button" type="submit" disabled={!creator}>
						♙ &nbsp;Lưu vào Phễu KOC
					</button>
				</div>
			</form>
		</div>
	);
}

function BulkImportModal({ onClose, onImport }) {
	const [source, setSource] = useState('TikToday');
	const [rawText, setRawText] = useState(
		'minhchau_beauty, 0912345678, Minh Châu Review\nbepnhamây, 0987654321, Bếp Nhà Mây\nminhaireview, 0327637213, Trùng Minh Ái',
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
						<h2>A2 & A3 · Nhập hàng loạt & Dedupe tự động</h2>
						<p>Nhập danh sách ID kênh, SĐT, Tên KOC. Hệ thống tự lọc trùng ID kênh/SĐT và chặn da_hop_tac.</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<label>Chọn nguồn tìm (A1)</label>
					<select value={source} onChange={(e) => setSource(e.target.value)}>
						<option value="TikToday">TikToday</option>
						<option value="TikTok Liên kết">TikTok Liên kết (Seller Center)</option>
						<option value="TikTok">TikTok App</option>
					</select>
					<label>Danh sách KOC (Mỗi dòng: tiktok_id, số_điện_thoại, tên_hiển_thị)</label>
					<textarea
						className="bulk-textarea"
						rows={7}
						value={rawText}
						onChange={(e) => setRawText(e.target.value)}
						placeholder="tiktok_id, 09xxxxxxxx, Tên KOC"
					/>
					<small className="hint">
						Hệ thống sẽ tự động Dedupe: bỏ qua kênh trùng ID/SĐT, chặn kênh có da_hop_tac = true, và đưa các kênh mới vào trạng thái SOURCED.
					</small>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button type="submit" className="primary-button">
						📥 Chạy Dedupe & Nhập hàng loạt
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
						<h2>A6–A10 · Chấm phễu & Tạo Hợp tác ({creator.name})</h2>
						<p>Đánh giá tệp, content pillar và thiết lập sản phẩm, hoa hồng, phí cast</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<label>A6 · Đánh giá kênh (Content Pillar & Tệp người xem)</label>
					<input
						name="contentPillar"
						value={form.contentPillar}
						onChange={handleChange}
						placeholder="Content pillar chính của kênh (VD: Review đồ gia dụng)"
						required
					/>
					<input
						name="targetAudience"
						value={form.targetAudience}
						onChange={handleChange}
						placeholder="Tệp người xem (VD: Nữ 22-35 tuổi, quan tâm chăm sóc nhà cửa)"
					/>

					<label>A9 · Bản ghi Hợp tác (Đợt, Sản phẩm, Hoa hồng)</label>
					<div className="grid-2">
						<input
							name="campaign"
							value={form.campaign}
							onChange={handleChange}
							placeholder="Đợt chiến dịch"
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

					<label>A10 · Hoa hồng & Phí Cast (Tự động rẽ nhánh Freecast / Cast)</label>
					<div className="grid-2">
						<div className="fee-input">
							<input
								name="commissionRate"
								type="number"
								value={form.commissionRate}
								onChange={handleChange}
								placeholder="% Hoa hồng"
							/>
							<span>% HH</span>
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
					<div className="status">
						{Number(form.castFee) > 0
							? `⚠️ KOC có phí Cast (${formatNumber(form.castFee)}đ) -> Sẽ chuyển sang trạng thái AWAITING_APPROVAL để Chủ shop duyệt (A11).`
							: '✓ KOC Freecast (0đ) -> Sẽ chuyển thẳng sang trạng thái QUALIFIED (Sẵn sàng gửi lời mời Zalo ở GĐ 2).'}
					</div>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button type="submit" className="primary-button">
						✓ Xác nhận Đạt phễu
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
						<h2>A8 · Loại KOC khỏi phễu ({creator.name})</h2>
						<p>Chọn mã lý do lọc để lưu lịch sử phân tích chất lượng nguồn KOC</p>
					</div>
					<button type="button" className="close-button" onClick={onClose}>
						×
					</button>
				</div>
				<div className="modal-body">
					<label>Mã lý do loại (A8)</label>
					<select value={rejectCode} onChange={(e) => setRejectCode(e.target.value)}>
						{FILTER_REJECT_REASONS.map((r) => (
							<option key={r.code} value={r.code}>
								{r.label}
							</option>
						))}
					</select>
					<label>Ghi chú chi tiết (Tùy chọn)</label>
					<input
						value={rejectNote}
						onChange={(e) => setRejectNote(e.target.value)}
						placeholder="Nhập ghi chú thêm (VD: Giá cast báo 3 triệu quá cao so với GMV)..."
					/>
				</div>
				<div className="modal-footer">
					<button type="button" className="secondary-button" onClick={onClose}>
						Hủy
					</button>
					<button type="submit" className="primary-button danger-btn">
						✕ Xác nhận Loại (FILTERED_OUT)
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
					<p className="eyebrow">TIN NHẮN ĐỐI TÁC</p>
					<h1>
						Hộp thư <span>{conversations.length} hội thoại</span>
					</h1>
				</div>
				<div className="inbox-actions">
					<div className="api-badge">{apiReady ? '● Đã kết nối dữ liệu' : '○ Đang dùng bản xem trước'}</div>
					{!zaloStatus?.connected && (
						<button className="secondary-button" onClick={startQrLogin}>
							Kết nối Zalo bằng QR
						</button>
					)}
					{zaloStatus?.connected && <div className="zalo-connected">● Zalo đã kết nối</div>}
				</div>
			</div>
			{showQr && (
				<div className="qr-panel">
					<div>
						<strong>Quét mã QR bằng ứng dụng Zalo</strong>
						<p>
							{zaloStatus?.state === 'scanned'
								? 'Đã quét, đang hoàn tất đăng nhập...'
								: 'Mở Zalo trên điện thoại và quét mã này.'}
						</p>
						{zaloStatus?.error && <div className="status error-status">{zaloStatus.error}</div>}
					</div>
					{zaloStatus?.qrImage ? (
						<img src={zaloStatus.qrImage} alt="Mã QR đăng nhập Zalo" />
					) : (
						<div className="qr-placeholder">Đang tạo mã QR...</div>
					)}
				</div>
			)}
			<div className="inbox-layout">
				<div className="conversation-list">
					<div className="inbox-search">
						<span>⌕</span>
						<input
							value={search}
							onChange={(event) => setSearch(event.target.value)}
							placeholder="Tìm tên, TikTok ID, số điện thoại..."
						/>
					</div>
					<div className="inbox-tabs">
						<b>Tất cả</b>
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
							<div className="creator-avatar red">{conversation.contactName.slice(0, 2)}</div>
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
								<div className="creator-avatar red">{selectedConversation.contactName.slice(0, 2)}</div>
								<div>
									<strong>{selectedConversation.contactName}</strong>
									<small>
										{selectedConversation.contactHandle}
										{selectedConversation.contactPhone ? ` · ${selectedConversation.contactPhone}` : ''}
									</small>
								</div>
								<span className="channel-tag">
									{selectedConversation.channel === 'zalo' ? 'Zalo' : 'Nội bộ'}
								</span>
							</div>
							<div className="chat-messages">
								{loading && <div className="empty-state">Đang tải tin nhắn...</div>}
								{error && <div className="status error-status">{error}</div>}
								{!loading && !messages.length && <div className="empty-state">Chưa có tin nhắn.</div>}
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
											? 'Dữ liệu xem trước chưa thể nhắn'
											: 'Nhắn tin cho KOC...'
									}
									disabled={sending || selectedConversation._id.startsWith('preview-')}
								/>
								<button
									className="primary-button"
									disabled={sending || !draft.trim() || selectedConversation._id.startsWith('preview-')}
								>
									{sending ? 'Đang gửi...' : 'Gửi'}
								</button>
							</form>
						</>
					) : (
						<div className="empty-state">Chọn một hội thoại để xem tin nhắn.</div>
					)}
				</div>
			</div>
		</div>
	);
}

export default App;
