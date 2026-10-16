import { useState } from 'react';
import api from '../lib/axios';
import { saveSession } from '../stores/useAuthStore';

export default function LoginPage({ onLogin }) {
	const [form, setForm] = useState({ email: '', password: '' });
	const [error, setError] = useState('');
	const [loading, setLoading] = useState(false);
	function update(event) { setForm({ ...form, [event.target.name]: event.target.value }); }
	async function submit(event) {
		event.preventDefault(); setLoading(true); setError('');
		try { const { data } = await api.post('/auth/login', form); saveSession(data.data); onLogin(data.data.user); } catch (requestError) { setError(requestError.response?.data?.message || 'Không thể đăng nhập. Hãy kiểm tra lại thông tin.'); } finally { setLoading(false); }
	}
	return <main className="login-page"><section className="login-intro"><div className="brand login-brand"><span className="brand-mark">N</span><span>Tiktoday</span><small>WORKSPACE</small></div><div><p className="eyebrow">CREATOR OPERATIONS</p><h1>Một nơi để đội ngũ quản lý KOC và hội thoại.</h1><p className="login-copy">Theo dõi danh bạ nhà sáng tạo, lưu thông tin hợp tác và chuẩn bị nền tảng nhắn tin tập trung.</p></div><span className="login-version">Workspace v1.0</span></section><section className="login-panel"><form onSubmit={submit}><p className="eyebrow">CHÀO MỪNG QUAY LẠI</p><h2>Đăng nhập workspace</h2><p className="login-muted">Sử dụng tài khoản công việc của bạn để tiếp tục.</p><label>Email công việc<input name="email" type="email" value={form.email} onChange={update} placeholder="you@company.com" required /></label><label>Mật khẩu<input name="password" type="password" value={form.password} onChange={update} placeholder="Tối thiểu 8 ký tự" required minLength="8" /></label>{error && <div className="login-error">{error}</div>}<button className="primary-button login-submit" disabled={loading}>{loading ? 'Đang kiểm tra...' : 'Đăng nhập'}</button><p className="login-footnote">Tài khoản mới được tạo qua API quản trị hoặc endpoint đăng ký nội bộ.</p></form></section></main>;
}