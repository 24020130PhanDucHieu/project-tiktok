import type { ReactNode } from 'react';

/** Two-column layout shared by the login / forgot-password / reset-password screens. */
export default function AuthShell({ children }: { children: ReactNode }) {
	return (
		<main className="login-page">
			<section className="login-intro">
				<div className="sb-brand login-brand">
					<div className="sb-logo-box">
						<svg width="20" height="20" viewBox="0 0 24 24" fill="none">
							<path d="M6.5 5v14M6.5 12l7.5-7M10.5 9.5L17.5 19" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
							<circle cx="17.5" cy="5.5" r="2.2" fill="#60a5fa" />
						</svg>
					</div>
					<span className="sb-title" style={{ color: '#ffffff' }}>KocFlow</span>
					<span className="sb-tag">STUDIO</span>
				</div>
				<div>
					<p className="eyebrow">CREATOR OPERATIONS & BOOKING CRM</p>
					<h1>Nền tảng quản trị & tự động hóa Booking KOC khép kín.</h1>
					<p className="login-copy">
						Tìm kiếm kênh TikTok thật, tự động hóa chuỗi liên hệ Zalo & Gmail, theo dõi vận đơn J&T Express và đối soát doanh thu GMV trên một màn hình duy nhất.
					</p>
				</div>
				<span className="login-version">KocFlow Studio v2.0</span>
			</section>
			<section className="login-panel">{children}</section>
		</main>
	);
}
