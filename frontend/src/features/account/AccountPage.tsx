import { type FormEvent, useEffect, useState } from 'react';
import ResetCodeForm from '../../components/ResetCodeForm';
import api, { apiMessage } from '../../lib/axios';
import { getToken, getUser, saveSession } from '../../stores/useAuthStore';
import type { UserProfile } from '../../types/koc';

interface Props {
	user: UserProfile;
	onUserChange: (user: UserProfile) => void;
	onLogout: () => void;
	showToast: (message: string, type?: 'success' | 'error') => void;
}

interface AccountDetail {
	id: string;
	name: string;
	email: string;
	role: 'admin' | 'member';
	status?: string;
	lastLoginAt?: string;
	createdAt?: string;
}

const fmt = (iso?: string) => (iso ? new Date(iso).toLocaleString('vi-VN', { dateStyle: 'medium', timeStyle: 'short' }) : '—');

/** Account information, display name and password management for the signed-in user (a full page, not a modal). */
export default function AccountPage({ user, onUserChange, onLogout, showToast }: Props) {
	const [detail, setDetail] = useState<AccountDetail | null>(null);
	const [name, setName] = useState(user.name);
	const [mode, setMode] = useState<'current' | 'email'>('current');
	const [current, setCurrent] = useState('');
	const [next, setNext] = useState('');
	const [confirm, setConfirm] = useState('');
	const [busy, setBusy] = useState<'profile' | 'password' | null>(null);
	const [profileError, setProfileError] = useState('');
	const [passwordError, setPasswordError] = useState('');

	useEffect(() => {
		api.get('/auth/me').then(({ data }) => setDetail(data.data)).catch(() => {});
	}, []);

	async function saveProfile(event: FormEvent) {
		event.preventDefault();
		setBusy('profile');
		setProfileError('');
		try {
			const { data } = await api.patch('/auth/me', { name });
			const updated = { ...user, name: data.data.name };
			saveSession({ token: getToken() || '', user: updated });
			onUserChange(updated);
			setDetail((d) => (d ? { ...d, name: data.data.name } : d));
			showToast('Đã cập nhật tên hiển thị.');
		} catch (e) {
			setProfileError(apiMessage(e, 'Không thể cập nhật hồ sơ.'));
		} finally {
			setBusy(null);
		}
	}

	async function changePassword(event: FormEvent) {
		event.preventDefault();
		if (next !== confirm) return setPasswordError('Mật khẩu nhập lại chưa khớp.');
		setBusy('password');
		setPasswordError('');
		try {
			const { data } = await api.post('/auth/change-password', { currentPassword: current, newPassword: next });
			saveSession(data.data); // fresh token; every older session is revoked server-side
			onUserChange(getUser() ?? user);
			setCurrent('');
			setNext('');
			setConfirm('');
			showToast('Đã đổi mật khẩu. Các thiết bị khác sẽ phải đăng nhập lại.');
		} catch (e) {
			setPasswordError(apiMessage(e, 'Không thể đổi mật khẩu.'));
		} finally {
			setBusy(null);
		}
	}

	const role = detail?.role ?? user.role;
	const shownName = detail?.name ?? user.name;

	return (
		<div className="page-shell">
			<header className="page-head">
				<div>
					<h1>Tài khoản của tôi</h1>
					<p>Xem thông tin đăng nhập, đổi tên hiển thị và quản lý mật khẩu.</p>
				</div>
			</header>

			<div className="acct-layout">
				<aside className="panel-card acct-card">
					<div className="account-avatar">{shownName?.slice(0, 1)?.toUpperCase()}</div>
					<strong>{shownName}</strong>
					<small>{user.email}</small>
					<span className={`role-badge ${role === 'admin' ? 'admin' : 'member'}`}>{role === 'admin' ? 'Quản trị viên' : 'Booker'}</span>
					<dl className="info-grid">
						<dt>Trạng thái</dt>
						<dd><span className="status-pill on">{detail?.status === 'suspended' ? 'Đã khóa' : 'Đang hoạt động'}</span></dd>
						<dt>Đăng nhập gần nhất</dt><dd>{fmt(detail?.lastLoginAt)}</dd>
						<dt>Ngày tạo tài khoản</dt><dd>{fmt(detail?.createdAt)}</dd>
						<dt>Quyền hạn</dt>
						<dd>{role === 'admin' ? 'Toàn quyền quản lý tài khoản & hệ thống' : 'Quản lý KOC, hộp thư, tự động hóa'}</dd>
						<dt>Mã tài khoản</dt><dd className="mono">{detail?.id ?? user.id ?? '—'}</dd>
					</dl>
					<button type="button" className="tm-btn-danger" onClick={onLogout}>Đăng xuất</button>
				</aside>

				<div className="acct-stack">
					<section className="panel-card">
						<h2 className="section-title">Hồ sơ</h2>
						<form className="acct-form" onSubmit={saveProfile}>
							<label>
								Tên hiển thị
								<input value={name} onChange={(e) => setName(e.target.value)} minLength={2} maxLength={120} required />
							</label>
							<label>
								Email <small>(liên hệ quản trị viên để đổi email)</small>
								<input value={user.email} readOnly disabled />
							</label>
							{profileError && <div className="login-error">{profileError}</div>}
							<button className="tm-btn-submit" disabled={busy === 'profile' || name.trim() === shownName}>
								{busy === 'profile' ? 'Đang lưu...' : 'Lưu thay đổi'}
							</button>
						</form>
					</section>

					<section className="panel-card">
						<h2 className="section-title">Mật khẩu</h2>
						<div className="seg-tabs small" style={{ margin: '0 0 14px', maxWidth: 460 }}>
							<button type="button" className={mode === 'current' ? 'active' : ''} onClick={() => { setMode('current'); setPasswordError(''); }}>Dùng mật khẩu hiện tại</button>
							<button type="button" className={mode === 'email' ? 'active' : ''} onClick={() => { setMode('email'); setPasswordError(''); }}>Xác nhận qua email</button>
						</div>
						{mode === 'current' ? (
							<form className="acct-form" onSubmit={changePassword}>
								<label>
									Mật khẩu hiện tại
									<input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} required autoComplete="current-password" />
								</label>
								<label>
									Mật khẩu mới <small>(≥ 8 ký tự, có chữ và số)</small>
									<input type="password" value={next} onChange={(e) => setNext(e.target.value)} minLength={8} required autoComplete="new-password" />
								</label>
								<label>
									Nhập lại mật khẩu mới
									<input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} minLength={8} required autoComplete="new-password" />
								</label>
								{passwordError && <div className="login-error">{passwordError}</div>}
								<button className="tm-btn-submit" disabled={busy === 'password' || !current || !next}>
									{busy === 'password' ? 'Đang đổi...' : 'Đổi mật khẩu'}
								</button>
							</form>
						) : (
							<div className="acct-form">
								<p className="muted-note">Không nhớ mật khẩu hiện tại? Nhận mã 6 số qua email {user.email}. Sau khi đặt lại, bạn cần đăng nhập lại.</p>
								<ResetCodeForm email={user.email} onDone={onLogout} doneLabel="Đăng nhập lại" />
							</div>
						)}
					</section>
				</div>
			</div>
		</div>
	);
}
