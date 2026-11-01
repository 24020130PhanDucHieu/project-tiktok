import { type FormEvent, useState } from 'react';
import api, { apiMessage } from '../lib/axios';
import AuthShell from './AuthShell';

export default function ResetPasswordPage({ token, onDone }: { token: string; onDone: () => void }) {
	const [password, setPassword] = useState('');
	const [confirm, setConfirm] = useState('');
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState('');
	const [done, setDone] = useState(false);

	async function submit(event: FormEvent) {
		event.preventDefault();
		if (password !== confirm) return setError('Mật khẩu nhập lại chưa khớp.');
		setLoading(true);
		setError('');
		try {
			await api.post('/auth/reset-password', { token, password });
			setDone(true);
		} catch (e) {
			setError(apiMessage(e, 'Không thể đặt lại mật khẩu.'));
		} finally {
			setLoading(false);
		}
	}

	return (
		<AuthShell>
			<form onSubmit={submit}>
				<p className="eyebrow">ĐẶT MẬT KHẨU MỚI</p>
				<h2>Tạo mật khẩu mới</h2>
				{done ? (
					<>
						<div className="auth-success">Đã đặt lại mật khẩu. Bạn có thể đăng nhập bằng mật khẩu mới.</div>
						<button type="button" className="primary-button login-submit" onClick={onDone}>
							Đến trang đăng nhập
						</button>
					</>
				) : (
					<>
						<p className="login-muted">Tối thiểu 8 ký tự, gồm cả chữ và số.</p>
						<label>
							Mật khẩu mới
							<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required autoFocus />
						</label>
						<label>
							Nhập lại mật khẩu
							<input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} minLength={8} required />
						</label>
						{error && <div className="login-error">{error}</div>}
						<button className="primary-button login-submit" disabled={loading}>
							{loading ? 'Đang lưu...' : 'Đặt lại mật khẩu'}
						</button>
					</>
				)}
			</form>
		</AuthShell>
	);
}
