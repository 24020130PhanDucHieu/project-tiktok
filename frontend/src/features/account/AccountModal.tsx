import { type FormEvent, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import { getUser, saveSession } from '../../stores/useAuthStore';
import type { UserProfile } from '../../types/koc';

interface Props {
	user: UserProfile;
	onClose: () => void;
	onUserChange: (user: UserProfile) => void;
	onLogout: () => void;
	showToast: (message: string, type?: 'success' | 'error') => void;
}

/** Profile (display name) and password change for the signed-in user. */
export default function AccountModal({ user, onClose, onUserChange, onLogout, showToast }: Props) {
	const [name, setName] = useState(user.name);
	const [current, setCurrent] = useState('');
	const [next, setNext] = useState('');
	const [confirm, setConfirm] = useState('');
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState('');

	async function saveProfile(event: FormEvent) {
		event.preventDefault();
		setBusy(true);
		setError('');
		try {
			const { data } = await api.patch('/auth/me', { name });
			const updated = { ...user, name: data.data.name };
			const token = localStorage.getItem('tiktoday_access_token') || '';
			saveSession({ token, user: updated });
			onUserChange(updated);
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

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<div className="tiktoday-modal account-modal">
				<div className="account-head">
					<div className="sb-user-avatar account-avatar">{user.name?.slice(0, 1)?.toUpperCase()}</div>
					<div>
						<strong>{user.name}</strong>
						<small>{user.email}</small>
					</div>
					<span className={`role-badge ${user.role === 'admin' ? 'admin' : 'member'}`}>
						{user.role === 'admin' ? 'Quản trị viên' : 'Booker'}
					</span>
				</div>

				<form className="account-section" onSubmit={saveProfile}>
					<h4>Hồ sơ</h4>
					<label>
						Tên hiển thị
						<input value={name} onChange={(e) => setName(e.target.value)} minLength={2} maxLength={120} required />
					</label>
					<button className="tm-btn-submit" disabled={busy || name.trim() === user.name}>Lưu hồ sơ</button>
				</form>

				<form className="account-section" onSubmit={changePassword}>
					<h4>Đổi mật khẩu</h4>
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
					<button className="tm-btn-submit" disabled={busy || !current || !next}>Đổi mật khẩu</button>
				</form>

				{error && <div className="login-error account-error">{error}</div>}

				<div className="tm-footer account-footer">
					<button type="button" className="tm-btn-danger" onClick={onLogout}>Đăng xuất</button>
					<button type="button" className="tm-btn-cancel" onClick={onClose}>Đóng</button>
				</div>
			</div>
		</div>
	);
}
