import { type FormEvent, useEffect, useState } from 'react';
import api, { apiMessage } from '../lib/axios';

interface Props {
	/** Pre-filled (and locked) e-mail, e.g. for the signed-in user. */
	email?: string;
	onDone: () => void;
	doneLabel?: string;
}

const RESEND_SECONDS = 60;

/** Two steps: request a 6-digit code by e-mail, then enter the code and a new password. */
export default function ResetCodeForm({ email: fixedEmail, onDone, doneLabel = 'Đến trang đăng nhập' }: Props) {
	const [step, setStep] = useState<'email' | 'code' | 'done'>('email');
	const [email, setEmail] = useState(fixedEmail ?? '');
	const [code, setCode] = useState('');
	const [password, setPassword] = useState('');
	const [confirm, setConfirm] = useState('');
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState('');
	const [notice, setNotice] = useState('');
	const [cooldown, setCooldown] = useState(0);

	useEffect(() => {
		if (cooldown <= 0) return undefined;
		const t = window.setTimeout(() => setCooldown((c) => c - 1), 1000);
		return () => window.clearTimeout(t);
	}, [cooldown]);

	async function requestCode(event?: FormEvent) {
		event?.preventDefault();
		setBusy(true);
		setError('');
		try {
			const { data } = await api.post('/auth/forgot-password', { email });
			setNotice(data.message);
			setStep('code');
			setCooldown(RESEND_SECONDS);
		} catch (e) {
			setError(apiMessage(e, 'Không thể gửi mã. Vui lòng thử lại.'));
		} finally {
			setBusy(false);
		}
	}

	async function submit(event: FormEvent) {
		event.preventDefault();
		if (password !== confirm) return setError('Mật khẩu nhập lại chưa khớp.');
		setBusy(true);
		setError('');
		try {
			await api.post('/auth/reset-password', { email, code, password });
			setStep('done');
		} catch (e) {
			setError(apiMessage(e, 'Không thể đặt lại mật khẩu.'));
		} finally {
			setBusy(false);
		}
	}

	if (step === 'done') {
		return (
			<div className="reset-form">
				<div className="auth-success">Đã đặt lại mật khẩu. Các phiên đăng nhập cũ đã bị thu hồi.</div>
				<button type="button" className="primary-button login-submit" onClick={onDone}>{doneLabel}</button>
			</div>
		);
	}

	if (step === 'email') {
		return (
			<form className="reset-form" onSubmit={requestCode}>
				<label>
					Email tài khoản
					<input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@kocflow.vn"
						required autoFocus={!fixedEmail} readOnly={Boolean(fixedEmail)} />
				</label>
				{error && <div className="login-error">{error}</div>}
				<button className="primary-button login-submit" disabled={busy}>{busy ? 'Đang gửi...' : 'Gửi mã xác nhận'}</button>
			</form>
		);
	}

	return (
		<form className="reset-form" onSubmit={submit}>
			{notice && <div className="auth-info">{notice}</div>}
			<label>
				Mã xác nhận (6 số)
				<input className="otp-input" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code}
					onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))} placeholder="••••••" required autoFocus />
			</label>
			<label>
				Mật khẩu mới <small>(≥ 8 ký tự, có chữ và số)</small>
				<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required autoComplete="new-password" />
			</label>
			<label>
				Nhập lại mật khẩu mới
				<input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} minLength={8} required autoComplete="new-password" />
			</label>
			{error && <div className="login-error">{error}</div>}
			<button className="primary-button login-submit" disabled={busy || code.length !== 6}>{busy ? 'Đang lưu...' : 'Đặt lại mật khẩu'}</button>
			<div className="reset-actions">
				<button type="button" className="auth-link" disabled={cooldown > 0 || busy} onClick={() => requestCode()}>
					{cooldown > 0 ? `Gửi lại mã sau ${cooldown}s` : 'Gửi lại mã'}
				</button>
				{!fixedEmail && (
					<button type="button" className="auth-link" onClick={() => { setStep('email'); setCode(''); setError(''); }}>
						Đổi email
					</button>
				)}
			</div>
		</form>
	);
}
