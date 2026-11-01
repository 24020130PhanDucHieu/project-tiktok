import { type FormEvent, useState } from 'react';
import api, { apiMessage } from '../lib/axios';
import AuthShell from './AuthShell';

export default function ForgotPasswordPage({ onBack }: { onBack: () => void }) {
	const [email, setEmail] = useState('');
	const [loading, setLoading] = useState(false);
	const [done, setDone] = useState('');
	const [error, setError] = useState('');

	async function submit(event: FormEvent) {
		event.preventDefault();
		setLoading(true);
		setError('');
		try {
			const { data } = await api.post('/auth/forgot-password', { email });
			setDone(data.message);
		} catch (e) {
			setError(apiMessage(e, 'Không thể gửi yêu cầu. Vui lòng thử lại.'));
		} finally {
			setLoading(false);
		}
	}

	return (
		<AuthShell>
			<form onSubmit={submit}>
				<p className="eyebrow">KHÔI PHỤC TÀI KHOẢN</p>
				<h2>Quên mật khẩu?</h2>
				<p className="login-muted">Nhập email đăng nhập, chúng tôi sẽ gửi liên kết đặt lại mật khẩu (hiệu lực 30 phút).</p>
				{done ? (
					<div className="auth-success">{done}</div>
				) : (
					<>
						<label>
							Email công việc
							<input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@kocflow.vn" required autoFocus />
						</label>
						{error && <div className="login-error">{error}</div>}
						<button className="primary-button login-submit" disabled={loading}>
							{loading ? 'Đang gửi...' : 'Gửi liên kết đặt lại'}
						</button>
					</>
				)}
				<button type="button" className="auth-link" onClick={onBack}>
					← Quay lại đăng nhập
				</button>
			</form>
		</AuthShell>
	);
}
