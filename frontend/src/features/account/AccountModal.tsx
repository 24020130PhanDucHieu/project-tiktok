import { type FormEvent, useEffect, useState } from 'react';
import ResetCodeForm from '../../components/ResetCodeForm';
import api, { apiMessage } from '../../lib/axios';
import { getToken, getUser, saveSession } from '../../stores/useAuthStore';
import type { UserProfile } from '../../types/koc';

interface Props {
	user: UserProfile;
	onClose: () => void;
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

type Tab = 'info' | 'profile' | 'password';

const fmt = (iso?: string) => (iso ? new Date(iso).toLocaleString('vi-VN', { dateStyle: 'medium', timeStyle: 'short' }) : '—');

/** Account information, display name and password management for the signed-in user. */
export default function AccountModal({ user, onClose, onUserChange, onLogout, showToast }: Props) {
	const [tab, setTab] = useState<Tab>('info');
	const [detail, setDetail] = useState<AccountDetail | null>(null);
	const [name, setName] = useState(user.name);
	const [mode, setMode] = useState<'current' | 'email'>('current');
	const [current, setCurrent] = useState('');
	const [next, setNext] = useState('');
	const [confirm, setConfirm] = useState('');
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState('');

	useEffect(() => {
		api.get('/auth/me').then(({ data }) => setDetail(data.data)).catch(() => {});
	}, []);

	useEffect(() => {
		const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	}, [onClose]);

	function switchTab(t: Tab) {
		setTab(t);
		setError('');
	}

	async function saveProfile(event: FormEvent) {
		event.preventDefault();
		setBusy(true);
		setError('');
		try {
			const { data } = await api.patch('/auth/me', { name });
			const updated = { ...user, name: data.data.name };
			saveSession({ token: getToken() || '', user: updated });
			onUserChange(updated);
			setDetail((d) => (d ? { ...d, name: data.data.name } : d));
			showToast('Đã cập nhật tên hiển thị.');
		} catch (e) {
			setError(apiMessage(e, 'Không thể cập nhật hồ sơ.'));
		} finally {
			setBusy(false);
		}
	}

	async function changePassword(event: FormEvent) {
		event.preventDefault();
		if (next !== confirm) return setError('Mật khẩu nhập lại chưa khớp.');
		setBusy(true);
		setError('');
		try {
			const { data } = await api.post('/auth/change-password', { currentPassword: current, newPassword: next });
			saveSession(data.data); // fresh token; every older session is revoked server-side
			onUserChange(getUser() ?? user);
			setCurrent('');
			setNext('');
			setConfirm('');
			showToast('Đã đổi mật khẩu. Các thiết bị khác sẽ phải đăng nhập lại.');
		} catch (e) {
			setError(apiMessage(e, 'Không thể đổi mật khẩu.'));
		} finally {
			setBusy(false);
		}
	}

	const role = detail?.role ?? user.role;

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<div className="tiktoday-modal account-modal" role="dialog" aria-label="Tài khoản">
				<div className="account-head">
					<div className="account-avatar">{user.name?.slice(0, 1)?.toUpperCase()}</div>
					<div>
						<strong>{detail?.name ?? user.name}</strong>
						<small>{user.email}</small>
					</div>
					<span className={`role-badge ${role === 'admin' ? 'admin' : 'member'}`}>{role === 'admin' ? 'Quản trị viên' : 'Booker'}</span>
				</div>

				<div className="seg-tabs" role="tablist">
					{([['info', 'Thông tin'], ['profile', 'Hồ sơ'], ['password', 'Mật khẩu']] as [Tab, string][]).map(([key, label]) => (
						<button key={key} type="button" role="tab" aria-selected={tab === key} className={tab === key ? 'active' : ''} onClick={() => switchTab(key)}>
							{label}
						</button>
					))}
				</div>

				<div className="account-body" key={tab}>
					{tab === 'info' && (
						<dl className="info-grid">
							<dt>Họ tên</dt><dd>{detail?.name ?? user.name}</dd>
							<dt>Email đăng nhập</dt><dd>{user.email}</dd>
							<dt>Vai trò</dt><dd>{role === 'admin' ? 'Quản trị viên — toàn quyền quản lý tài khoản & hệ thống' : 'Booker — quản lý KOC, hộp thư, tự động hóa'}</dd>
							<dt>Trạng thái</dt><dd><span className="status-pill on">{detail?.status === 'suspended' ? 'Đã khóa' : 'Đang hoạt động'}</span></dd>
							<dt>Đăng nhập gần nhất</dt><dd>{fmt(detail?.lastLoginAt)}</dd>
							<dt>Ngày tạo tài khoản</dt><dd>{fmt(detail?.createdAt)}</dd>
							<dt>Mã tài khoản</dt><dd className="mono">{detail?.id ?? user.id ?? '—'}</dd>
						</dl>
					)}

					{tab === 'profile' && (
						<form className="account-section" onSubmit={saveProfile}>
							<label>
								Tên hiển thị
								<input value={name} onChange={(e) => setName(e.target.value)} minLength={2} maxLength={120} required />
							</label>
							<label>
								Email <small>(liên hệ quản trị viên để đổi email)</small>
								<input value={user.email} readOnly disabled />
							</label>
							{error && <div className="login-error">{error}</div>}
							<button className="tm-btn-submit" disabled={busy || name.trim() === (detail?.name ?? user.name)}>Lưu thay đổi</button>
						</form>
					)}

					{tab === 'password' && (
						<div className="account-section">
							<div className="seg-tabs small">
								<button type="button" className={mode === 'current' ? 'active' : ''} onClick={() => { setMode('current'); setError(''); }}>Dùng mật khẩu hiện tại</button>
								<button type="button" className={mode === 'email' ? 'active' : ''} onClick={() => { setMode('email'); setError(''); }}>Xác nhận qua email</button>
							</div>
							{mode === 'current' ? (
								<form className="account-section flat" onSubmit={changePassword}>
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
									{error && <div className="login-error">{error}</div>}
									<button className="tm-btn-submit" disabled={busy || !current || !next}>Đổi mật khẩu</button>
								</form>
							) : (
								<>
									<p className="muted-note">Không nhớ mật khẩu hiện tại? Nhận mã 6 số qua email {user.email}. Sau khi đặt lại, bạn cần đăng nhập lại.</p>
									<ResetCodeForm email={user.email} onDone={onLogout} doneLabel="Đăng nhập lại" />
								</>
							)}
						</div>
					)}
				</div>

				<div className="tm-footer account-footer">
					<button type="button" className="tm-btn-danger" onClick={onLogout}>Đăng xuất</button>
					<button type="button" className="tm-btn-cancel" onClick={onClose}>Đóng</button>
				</div>
			</div>
		</div>
	);
}
