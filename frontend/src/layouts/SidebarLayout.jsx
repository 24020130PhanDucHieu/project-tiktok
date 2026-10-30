export default function SidebarLayout({
	selectedNav,
	onSelectNav,
	onOpenAddKoc,
	currentUser,
	onLogout,
	toast,
	onDismissToast,
	children,
}) {
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

				<button className="sb-search-btn" onClick={onOpenAddKoc}>
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
						onClick={() => onSelectNav('Hộp thư')}
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
						onClick={() => onSelectNav('Quản lý KOC')}
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
					<button className="sb-account-btn" onClick={onLogout} title="Bấm để đăng xuất">
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
				{toast && (
					<div className={`web-toast-card ${toast.type || 'success'}`}>
						<div className="web-toast-icon">
							{toast.type === 'error' ? (
								<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2">
									<circle cx="12" cy="12" r="9" />
									<line x1="12" y1="8" x2="12" y2="12" />
									<line x1="12" y1="16" x2="12.01" y2="16" />
								</svg>
							) : (
								<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4">
									<path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" />
								</svg>
							)}
						</div>
						<div className="web-toast-content">
							<strong>{toast.title || 'Thông báo hệ thống'}</strong>
							<span>{toast.message || toast}</span>
						</div>
						<button type="button" className="web-toast-close" onClick={onDismissToast}>
							×
						</button>
					</div>
				)}
				{children}
			</div>
		</div>
	);
}

