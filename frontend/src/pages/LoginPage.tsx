import { type ChangeEvent, type FormEvent, useCallback, useEffect, useState } from 'react';
import api from '../lib/axios';
import Turnstile from '../components/Turnstile';
import { HONEYPOT_NEUTRAL_MESSAGE, buildLoginBody, buildRegisterBody, hasSession } from '../lib/loginForm';
import { saveSession } from '../stores/useAuthStore';
import type { UserProfile } from '../types/koc';
import AuthShell from './AuthShell';
import './login.css';

interface LoginPageProps {
	onLogin: (user: UserProfile) => void;
	onForgot: () => void;
}

type Mode = 'login' | 'register';

const EMPTY_FORM = { email: '', password: '', name: '', website: '' };

function EyeIcon({ off }: { off: boolean }) {
	return (
		<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
			<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z" />
			<circle cx="12" cy="12" r="3" />
			{off && <path d="M4 4l16 16" />}
		</svg>
	);
}

export default function LoginPage({ onLogin, onForgot }: LoginPageProps) {
	const [form, setForm] = useState(EMPTY_FORM);
	const [error, setError] = useState('');
	const [notice, setNotice] = useState('');
	const [loading, setLoading] = useState(false);
	const [mode, setMode] = useState<Mode>('login');
	const [showPassword, setShowPassword] = useState(false);
	const [options, setOptions] = useState<{ allowRegistration: boolean; captchaSiteKey?: string }>({ allowRegistration: false });
	const [optionsLoaded, setOptionsLoaded] = useState(false);
	const [captchaToken, setCaptchaToken] = useState('');
	const [captchaNonce, setCaptchaNonce] = useState(0);
	const onCaptcha = useCallback((token: string) => setCaptchaToken(token), []);
	const registering = mode === 'register';
	const signUpClosed = registering && optionsLoaded && !options.allowRegistration;
	const needsCaptcha = registering && !!options.captchaSiteKey;

	useEffect(() => {
		api.get('/auth/options')
			.then(({ data }) => setOptions({ allowRegistration: !!data?.data?.allowRegistration, captchaSiteKey: data?.data?.captchaSiteKey || undefined }))
			.catch(() => setOptions({ allowRegistration: false }))
			.finally(() => setOptionsLoaded(true));
	}, []);

	function switchMode(next: Mode) {
		if (next === mode) return;
		setMode(next);
		setError('');
		setNotice('');
		setCaptchaToken(''); // a captcha token is single-use; the widget is re-rendered for the new tab
		setShowPassword(false);
		setForm((current) => ({ ...current, password: '' }));
	}

	function update(event: ChangeEvent<HTMLInputElement>) {
		setForm({ ...form, [event.target.name]: event.target.value });
	}

	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (signUpClosed) return;
		setLoading(true);
		setError('');
		setNotice('');
		try {
			if (registering) {
				const { data } = await api.post('/auth/register', buildRegisterBody({ ...form, captchaToken }));
				setNotice(data?.message || 'Đã gửi yêu cầu đăng ký. Quản trị viên sẽ xem xét và kích hoạt tài khoản.');
				setForm(EMPTY_FORM);
				return;
			}
			const { data } = await api.post('/auth/email', buildLoginBody(form));
			if (!hasSession(data)) {
				setError(data?.message || HONEYPOT_NEUTRAL_MESSAGE);
				return;
			}
			saveSession(data.data);
			onLogin(data.data.user);
		} catch (requestError: any) {
			setError(requestError.response?.data?.message || (registering ? 'Không thể gửi yêu cầu đăng ký. Hãy kiểm tra lại thông tin.' : 'Không thể đăng nhập. Hãy kiểm tra lại thông tin.'));
		} finally {
			if (registering) {
				setCaptchaToken(''); // the token was spent: render a fresh widget
				setCaptchaNonce((n) => n + 1);
			}
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
		<AuthShell>
			<div className="auth-card">
				<div className="auth-tabs" role="tablist" aria-label="Đăng nhập hoặc đăng ký">
					<button type="button" role="tab" id="tab-login" aria-selected={!registering} className="auth-tab" disabled={loading} onClick={() => switchMode('login')}>
						Đăng nhập
					</button>
					<button type="button" role="tab" id="tab-register" aria-selected={registering} className="auth-tab" disabled={loading} onClick={() => switchMode('register')}>
						Đăng ký
					</button>
				</div>

				<h2>{registering ? 'Tạo tài khoản mới' : 'Chào mừng quay lại'}</h2>
				<p className="login-muted">
					{registering ? 'Tài khoản mới cần quản trị viên duyệt trước khi đăng nhập được.' : 'Đăng nhập để tiếp tục quản lý KOC của bạn.'}
				</p>

				{signUpClosed ? (
					<div className="auth-closed" role="status">
						Đăng ký công khai đang đóng. Vui lòng liên hệ quản trị viên để được cấp tài khoản.
					</div>
				) : (
					<form onSubmit={submit} role="tabpanel" aria-labelledby={registering ? 'tab-register' : 'tab-login'}>
						<div className="auth-fields">
							{registering && (
								<label>
									Tên hiển thị
									<input name="name" type="text" value={form.name} onChange={update} placeholder="Họ và tên của bạn" autoComplete="name" minLength={2} maxLength={80} required />
								</label>
							)}
							<div className="hp-field" aria-hidden="true">
								<span>Website</span>
								<input name="website" type="text" value={form.website} onChange={update} tabIndex={-1} autoComplete="off" />
							</div>
							<label>
								Email công việc
								<input name="email" type="email" value={form.email} onChange={update} placeholder="you@kocflow.vn" autoComplete="email" required />
							</label>
							<div>
								<div className="auth-label-row">
									<label htmlFor="auth-password">Mật khẩu</label>
									{!registering && (
										<button type="button" className="auth-link" onClick={onForgot}>
											Quên mật khẩu?
										</button>
									)}
								</div>
								<div className="auth-password">
									<input
										id="auth-password"
										name="password"
										type={showPassword ? 'text' : 'password'}
										value={form.password}
										onChange={update}
										placeholder="Tối thiểu 8 ký tự"
										autoComplete={registering ? 'new-password' : 'current-password'}
										minLength={8}
										required
									/>
									<button type="button" className="auth-eye" onClick={() => setShowPassword((v) => !v)} aria-label={showPassword ? 'Ẩn mật khẩu' : 'Hiện mật khẩu'} aria-pressed={showPassword}>
										<EyeIcon off={showPassword} />
									</button>
								</div>
							</div>
							{needsCaptcha && <Turnstile key={captchaNonce} siteKey={options.captchaSiteKey!} onToken={onCaptcha} />}
						</div>

						{error && <div className="login-error" role="alert">{error}</div>}
						{notice && <div className="login-notice" role="status">{notice}</div>}

						<button className="auth-submit" disabled={loading || (needsCaptcha && !captchaToken)}>
							{loading ? 'Đang xử lý...' : registering ? 'Tạo tài khoản' : 'Đăng nhập'}
						</button>
					</form>
				)}

				{signUpClosed && error && <div className="login-error" role="alert">{error}</div>}

				<div className="auth-divider">hoặc</div>
				<button type="button" className="auth-guest" disabled={loading} onClick={enterGuest}>
					<EyeIcon off={false} />
					Xem Demo (Chế độ Khách)
				</button>
				<p className="login-footnote">Hệ thống tự động đồng bộ danh bạ KOC, hộp thư Zalo và vận đơn J&T Express.</p>
			</div>
		</AuthShell>
	);
}
