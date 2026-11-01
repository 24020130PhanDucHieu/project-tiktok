import type { ReactNode } from 'react';
import type { ToastNotice, UserProfile } from '../types/koc';

interface SidebarLayoutProps {
	selectedNav: string;
	onSelectNav: (nav: string) => void;
	onOpenAddKoc: () => void;
	currentUser: UserProfile | null;
	onOpenAccount: () => void;
	toast: ToastNotice | null;
	onDismissToast: () => void;
	children: ReactNode;
}

const NAV: { label: string; adminOnly?: boolean; icon: ReactNode }[] = [
	{
		label: 'Tổng quan',
		icon: (<><rect x="3" y="3" width="7" height="9" rx="1" /><rect x="14" y="3" width="7" height="5" rx="1" /><rect x="14" y="12" width="7" height="9" rx="1" /><rect x="3" y="16" width="7" height="5" rx="1" /></>),
	},
	{
		label: 'Hộp thư',
		icon: (<><path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" /><polyline points="22,6 12,13 2,6" /></>),
	},
	{
		label: 'Quản lý KOC',
		icon: (<><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" /><path d="M23 21v-2a4 4 0 0 0-3-3.87" /><path d="M16 3.13a4 4 0 0 1 0 7.75" /></>),
	},
	{
		label: 'Người dùng',
		adminOnly: true,
		icon: (<><path d="M12 2l8 3v6c0 5-3.5 9.4-8 11-4.5-1.6-8-6-8-11V5l8-3z" /><path d="M9 12l2 2 4-4" /></>),
	},
	{
		label: 'Nhật ký',
		adminOnly: true,
		icon: (<><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" /><line x1="8" y1="13" x2="16" y2="13" /><line x1="8" y1="17" x2="16" y2="17" /></>),
	},
	{
		label: 'Hệ thống',
		adminOnly: true,
		icon: (<><polyline points="22 12 18 12 15 21 9 3 6 12 2 12" /></>),
	},
];

export default function SidebarLayout({
	selectedNav,
	onSelectNav,
	onOpenAddKoc,
	currentUser,
	onOpenAccount,
	toast,
	onDismissToast,
	children,
}: SidebarLayoutProps) {
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
							<circle cx="17.5" cy="5.5" r="2.2" fill="#4ade80" />
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
				</button>

				<nav className="sb-nav">
					{NAV.filter((item) => !item.adminOnly || currentUser?.role === 'admin').map((item, index, all) => (
						<div key={item.label} className="sb-nav-group">
							{item.adminOnly && !all[index - 1]?.adminOnly && <span className="sb-section">Quản trị</span>}
							<button
								className={selectedNav === item.label ? 'sb-nav-item active' : 'sb-nav-item'}
								onClick={() => onSelectNav(item.label)}
								title={item.label}
							>
								<span className="sb-icon">
									<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
										{item.icon}
									</svg>
								</span>
								<span className="sb-label">{item.label}</span>
							</button>
						</div>
					))}
				</nav>

				<div className="sb-footer">
					<button className="sb-account-btn" onClick={onOpenAccount} title="Tài khoản & đổi mật khẩu">
						<div className="sb-user-avatar">{currentUser?.name?.slice(0, 1)?.toUpperCase() || '?'}</div>
						<div className="sb-user-info">
							<strong>{currentUser?.name}</strong>
							<small>{currentUser?.role === 'admin' ? 'Quản trị viên' : currentUser?.email}</small>
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
							<span>{toast.message}</span>
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

