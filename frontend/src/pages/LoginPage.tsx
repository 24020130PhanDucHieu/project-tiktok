import { type ChangeEvent, type FormEvent, useCallback, useEffect, useState } from 'react';
import api from '../lib/axios';
import Turnstile from '../components/Turnstile';
import { HONEYPOT_NEUTRAL_MESSAGE, buildEmailBody, buildRegisterBody, hasSession } from '../lib/loginForm';
import { saveSession } from '../stores/useAuthStore';
import type { UserProfile } from '../types/koc';

interface LoginPageProps {
	onLogin: (user: UserProfile) => void;
	onForgot: () => void;
}

export default function LoginPage({ onLogin, onForgot }: LoginPageProps) {
	const [form, setForm] = useState({ email: '', password: '', name: '', website: '' });
	const [error, setError] = useState('');
	const [loading, setLoading] = useState(false);
	const [mode, setMode] = useState<'login' | 'register'>('login');
	const [notice, setNotice] = useState('');
	const [options, setOptions] = useState<{ allowRegistration: boolean; captchaSiteKey?: string }>({ allowRegistration: false });
	const [captchaToken, setCaptchaToken] = useState('');
	const onCaptcha = useCallback((token: string) => setCaptchaToken(token), []);
	const registering = mode === 'register';

	useEffect(() => {
		api.get('/auth/options')
			.then(({ data }) => setOptions({ allowRegistration: !!data?.data?.allowRegistration, captchaSiteKey: data?.data?.captchaSiteKey || undefined }))
			.catch(() => setOptions({ allowRegistration: false }));
	}, []);

	function switchMode(next: 'login' | 'register') {
		setMode(next);
		setError('');
		setNotice('');
		setCaptchaToken('');
	}

	function update(event: ChangeEvent<HTMLInputElement>) {
		setForm({ ...form, [event.target.name]: event.target.value });
	}

	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setLoading(true);
		setError('');
		setNotice('');
		try {
			if (registering) {
				const { data } = await api.post('/auth/register', buildRegisterBody({ ...form, captchaToken }));
				setNotice(data?.message || 'Đã gửi yêu cầu đăng ký. Quản trị viên sẽ xem xét và kích hoạt tài khoản.');
				setForm({ email: '', password: '', name: '', website: '' });
				return;
			}
			const { data } = await api.post('/auth/email', buildEmailBody(form));
			if (!hasSession(data)) {
				setError(data?.message || HONEYPOT_NEUTRAL_MESSAGE);
				return;
			}
			saveSession(data.data);
			onLogin(data.data.user);
		} catch (requestError: any) {
			setError(requestError.response?.data?.message || (registering ? 'Không thể gửi yêu cầu đăng ký. Hãy kiểm tra lại thông tin.' : 'Không thể đăng nhập. Hãy kiểm tra lại thông tin.'));
		} finally {
			setLoading(false);
		}
	}

	async function enterGuest() {
		setLoading(true);
		setError('');
		try {
			const { data } = await api.post('/auth/guest');
			saveSession(data.data);
			onLogin(data.data.user);
		} catch (requestError: any) {
			setError(requestError.response?.data?.message || 'Không thể vào chế độ Demo lúc này.');
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
							<circle cx="17.5" cy="5.5" r="2.2" fill="#60a5fa" />
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
					<p className="eyebrow">{registering ? 'TẠO TÀI KHOẢN' : 'CHÀO MỪNG QUAY LẠI'}</p>
					<h2>{registering ? 'Đăng ký tài khoản' : 'Đăng nhập bằng Email'}</h2>
					<p className="login-muted">
						{registering ? 'Tài khoản mới cần quản trị viên duyệt trước khi đăng nhập được.' : 'Email chưa có sẽ được tự động tạo tài khoản mới.'}
					</p>
					<label>
						{registering ? 'Tên hiển thị' : 'Tên hiển thị (tuỳ chọn)'}
						<input
							name="name"
							type="text"
							value={form.name}
							onChange={update}
							placeholder={registering ? 'Họ và tên của bạn' : 'Chỉ dùng khi tạo tài khoản mới'}
							minLength={registering ? 2 : undefined}
							required={registering}
							maxLength={80}
						/>
					</label>
					<div className="hp-field" aria-hidden="true">
						<span>Website</span>
						<input name="website" type="text" value={form.website} onChange={update} tabIndex={-1} autoComplete="off" />
					</div>
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
					{registering && options.captchaSiteKey && <Turnstile siteKey={options.captchaSiteKey} onToken={onCaptcha} />}
					{error && <div className="login-error">{error}</div>}
					{notice && <div className="login-notice" role="status">{notice}</div>}
					{!registering && (
						<button type="button" className="auth-link auth-link-right" onClick={onForgot}>
							Quên mật khẩu?
						</button>
					)}
					<button className="primary-button login-submit" disabled={loading || (registering && !!options.captchaSiteKey && !captchaToken)}>
						{loading ? 'Đang kiểm tra...' : registering ? 'Gửi yêu cầu đăng ký' : 'Đăng nhập'}
					</button>
					{options.allowRegistration && (
						<button type="button" className="auth-link auth-link-center" disabled={loading} onClick={() => switchMode(registering ? 'login' : 'register')}>
							{registering ? 'Đã có tài khoản? Đăng nhập' : 'Chưa có tài khoản? Đăng ký'}
						</button>
					)}
					<button type="button" className="login-guest" disabled={loading} onClick={enterGuest}>
						Xem Demo (Chế độ Khách)
					</button>
					<p className="login-footnote">Hệ thống tự động đồng bộ danh bạ KOC, hộp thư Zalo và vận đơn J&T Express.</p>
				</form>
			</section>
		</main>
	);
}

