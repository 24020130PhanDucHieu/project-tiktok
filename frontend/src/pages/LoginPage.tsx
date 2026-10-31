import { type ChangeEvent, type FormEvent, useState } from 'react';
import api from '../lib/axios';
import { saveSession } from '../stores/useAuthStore';
import type { UserProfile } from '../types/koc';

interface LoginPageProps {
	onLogin: (user: UserProfile) => void;
}

export default function LoginPage({ onLogin }: LoginPageProps) {
	const [form, setForm] = useState({ email: '', password: '' });
	const [error, setError] = useState('');
	const [loading, setLoading] = useState(false);

	function update(event: ChangeEvent<HTMLInputElement>) {
		setForm({ ...form, [event.target.name]: event.target.value });
	}

	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setLoading(true);
		setError('');
		try {
			const { data } = await api.post('/auth/login', form);
			saveSession(data.data);
			onLogin(data.data.user);
		} catch (requestError: any) {
			setError(requestError.response?.data?.message || 'Không thể đăng nhập. Hãy kiểm tra lại thông tin.');
		} finally {
			setLoading(false);
		}
	}

	return (
		<main className="login-page">
			<section className="login-intro">
				<div className="sb-brand login-brand">
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
					<span className="sb-title" style={{ color: '#ffffff' }}>
						KocFlow
					</span>
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
			<section className="login-panel">
				<form onSubmit={submit}>
					<p className="eyebrow">CHÀO MỪNG QUAY LẠI</p>
					<h2>Đăng nhập KocFlow</h2>
					<p className="login-muted">Sử dụng tài khoản Booker hoặc Quản trị viên để tiếp tục.</p>
					<label>
						Email công việc
						<input
							name="email"
							type="email"
							value={form.email}
							onChange={update}
							placeholder="you@kocflow.vn"
							required
						/>
					</label>
					<label>
						Mật khẩu
						<input
							name="password"
							type="password"
							value={form.password}
							onChange={update}
							placeholder="Tối thiểu 8 ký tự"
							required
							minLength={8}
						/>
					</label>
					{error && <div className="login-error">{error}</div>}
					<button className="primary-button login-submit" disabled={loading}>
						{loading ? 'Đang kiểm tra...' : 'Đăng nhập'}
					</button>
					<p className="login-footnote">Hệ thống tự động đồng bộ danh bạ KOC, hộp thư Zalo và vận đơn J&T Express.</p>
				</form>
			</section>
		</main>
	);
}

