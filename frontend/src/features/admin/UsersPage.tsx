import { type FormEvent, useCallback, useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { AdminUser, Pagination, Role } from '../../types/admin';
import type { UserProfile } from '../../types/koc';
import Select from '../../components/Select';

type Toast = (message: string, type?: 'success' | 'error') => void;

const fmt = (iso?: string) => (iso ? new Date(iso).toLocaleString('vi-VN', { dateStyle: 'short', timeStyle: 'short' }) : '—');

type Confirm = { kind: 'delete' | 'suspend' | 'activate' | 'reset'; user: AdminUser } | null;

function CreateUserModal({ onClose, onCreated, showToast }: { onClose: () => void; onCreated: () => void; showToast: Toast }) {
	const [form, setForm] = useState({ name: '', email: '', role: 'member' as Role, password: '' });
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState('');
	const [temp, setTemp] = useState<string | null>(null);

	async function submit(event: FormEvent) {
		event.preventDefault();
		setBusy(true);
		setError('');
		try {
			const { data } = await api.post('/admin/users', { ...form, password: form.password || undefined });
			onCreated();
			if (data.data.temporaryPassword) {
				setTemp(data.data.temporaryPassword);
			} else {
				showToast(form.password ? 'Đã tạo tài khoản.' : 'Đã tạo tài khoản và gửi email mời đặt mật khẩu.');
				onClose();
			}
		} catch (e) {
			setError(apiMessage(e, 'Không thể tạo tài khoản.'));
		} finally {
			setBusy(false);
		}
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && !temp && onClose()}>
			<form className="tiktoday-modal account-modal" onSubmit={submit}>
				<div className="account-section">
					<h4>Thêm người dùng</h4>
					{temp ? (
						<>
							<div className="auth-success">
								Chưa cấu hình SMTP nên không gửi được email mời. Hãy chuyển mật khẩu tạm thời này cho người dùng (chỉ hiển thị một lần):
								<code className="temp-pass">{temp}</code>
							</div>
							<button type="button" className="tm-btn-submit" onClick={onClose}>Đã sao chép, đóng</button>
						</>
					) : (
						<>
							<label>Họ tên<input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} minLength={2} required autoFocus /></label>
							<label>Email<input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} required /></label>
							<label>
								Vai trò
								<Select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as Role })}>
									<option value="member">Booker (người dùng)</option>
									<option value="admin">Quản trị viên</option>
								</Select>
							</label>
							<label>
								Mật khẩu <small>(bỏ trống = gửi email mời đặt mật khẩu)</small>
								<input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} minLength={8} autoComplete="new-password" />
							</label>
							{error && <div className="login-error">{error}</div>}
						</>
					)}
				</div>
				{!temp && (
					<div className="tm-footer">
						<button type="button" className="tm-btn-cancel" onClick={onClose}>Huỷ</button>
						<button className="tm-btn-submit" disabled={busy}>{busy ? 'Đang tạo...' : 'Tạo tài khoản'}</button>
					</div>
				)}
			</form>
		</div>
	);
}

export default function UsersPage({ currentUser, showToast }: { currentUser: UserProfile | null; showToast: Toast }) {
	const [items, setItems] = useState<AdminUser[]>([]);
	const [pagination, setPagination] = useState<Pagination>({ page: 1, limit: 20, total: 0, pages: 1 });
	const [q, setQ] = useState('');
	const [role, setRole] = useState('');
	const [status, setStatus] = useState('');
	const [page, setPage] = useState(1);
	const [loading, setLoading] = useState(true);
	const [creating, setCreating] = useState(false);
	const [confirm, setConfirm] = useState<Confirm>(null);

	const load = useCallback(() => {
		setLoading(true);
		api.get('/admin/users', { params: { q, role, status, page, limit: 20 } })
			.then(({ data }) => {
				setItems(data.data.items);
				setPagination(data.data.pagination);
			})
			.catch((e) => showToast(apiMessage(e, 'Không thể tải danh sách người dùng.'), 'error'))
			.finally(() => setLoading(false));
	}, [q, role, status, page, showToast]);

	useEffect(() => {
		const t = window.setTimeout(load, q ? 300 : 0); // debounce search
		return () => window.clearTimeout(t);
	}, [load, q]);

	async function patch(user: AdminUser, body: Partial<Pick<AdminUser, 'role' | 'status' | 'name'>>, okMsg: string) {
		try {
			await api.patch(`/admin/users/${user.id}`, body);
			showToast(okMsg);
			load();
		} catch (e) {
			showToast(apiMessage(e, 'Không thể cập nhật tài khoản.'), 'error');
		}
	}

	async function runConfirm() {
		if (!confirm) return;
		const { kind, user } = confirm;
		setConfirm(null);
		try {
			if (kind === 'delete') await api.delete(`/admin/users/${user.id}`);
			if (kind === 'reset') await api.post(`/admin/users/${user.id}/reset-password`);
			if (kind === 'suspend') await api.patch(`/admin/users/${user.id}`, { status: 'suspended' });
			if (kind === 'activate') await api.patch(`/admin/users/${user.id}`, { status: 'active' });
			showToast(
				{ delete: 'Đã xóa tài khoản.', reset: 'Đã gửi email đặt lại mật khẩu.', suspend: 'Đã khóa tài khoản và thu hồi phiên đăng nhập.', activate: 'Đã mở khóa tài khoản.' }[kind],
			);
			load();
		} catch (e) {
			showToast(apiMessage(e, 'Thao tác không thành công.'), 'error');
		}
	}

	const confirmText = confirm && {
		delete: { title: 'Xóa tài khoản?', body: `Tài khoản ${confirm.user.email} sẽ bị xóa vĩnh viễn.`, ok: 'Xóa', danger: true },
		suspend: { title: 'Khóa tài khoản?', body: `${confirm.user.email} sẽ bị đăng xuất ngay và không thể đăng nhập cho đến khi mở khóa.`, ok: 'Khóa', danger: true },
		activate: { title: 'Mở khóa tài khoản?', body: `${confirm.user.email} có thể đăng nhập trở lại.`, ok: 'Mở khóa', danger: false },
		reset: { title: 'Gửi email đặt lại mật khẩu?', body: `Liên kết đặt lại sẽ được gửi tới ${confirm.user.email}.`, ok: 'Gửi email', danger: false },
	}[confirm.kind];

	return (
		<div className="page-shell">
			<header className="page-head">
				<div>
					<h1>Người dùng & phân quyền</h1>
					<p>Tạo tài khoản, phân vai trò, khóa/mở khóa và gửi email đặt lại mật khẩu. {pagination.total} tài khoản.</p>
				</div>
				<button className="primary-button" onClick={() => setCreating(true)}>+ Thêm người dùng</button>
			</header>

			<div className="adm-toolbar">
				<input className="adm-input" placeholder="Tìm theo tên hoặc email..." value={q} onChange={(e) => { setQ(e.target.value); setPage(1); }} />
				<Select className="adm-input" value={role} onChange={(e) => { setRole(e.target.value); setPage(1); }}>
					<option value="">Mọi vai trò</option>
					<option value="admin">Quản trị viên</option>
					<option value="member">Booker</option>
				</Select>
				<Select className="adm-input" value={status} onChange={(e) => { setStatus(e.target.value); setPage(1); }}>
					<option value="">Mọi trạng thái</option>
					<option value="active">Hoạt động</option>
					<option value="suspended">Đã khóa</option>
				</Select>
			</div>

			<div className="table-scroll panel-card flush">
				<table className="adm-table">
					<thead>
						<tr><th>Người dùng</th><th>Vai trò</th><th>Trạng thái</th><th>Đăng nhập gần nhất</th><th>Ngày tạo</th><th className="right">Thao tác</th></tr>
					</thead>
					<tbody>
						{items.map((u) => {
							const self = u.id === (currentUser?.id ?? currentUser?._id);
							const locked = u.lockedUntil && new Date(u.lockedUntil) > new Date();
							return (
								<tr key={u.id}>
									<td>
										<div className="adm-user">
											<span className="sb-user-avatar">{u.name.slice(0, 1).toUpperCase()}</span>
											<span><b>{u.name}{self && <em> (bạn)</em>}</b><small>{u.email}</small></span>
										</div>
									</td>
									<td>
										<Select
											className="adm-select"
											value={u.role}
											disabled={self}
											onChange={(e) => patch(u, { role: e.target.value as Role }, 'Đã đổi vai trò.')}
										>
											<option value="member">Booker</option>
											<option value="admin">Quản trị viên</option>
										</Select>
									</td>
									<td>
										<span className={`status-pill ${u.status === 'suspended' ? 'off' : 'on'}`}>{u.status === 'suspended' ? 'Đã khóa' : 'Hoạt động'}</span>
										{locked && <span className="status-pill warn" title="Nhập sai mật khẩu nhiều lần">Tạm khóa</span>}
									</td>
									<td>{fmt(u.lastLoginAt)}</td>
									<td>{fmt(u.createdAt)}</td>
									<td className="right">
										<div className="adm-actions">
											<button className="link-btn" onClick={() => setConfirm({ kind: 'reset', user: u })}>Gửi reset MK</button>
											{!self && (u.status === 'suspended'
												? <button className="link-btn" onClick={() => setConfirm({ kind: 'activate', user: u })}>Mở khóa</button>
												: <button className="link-btn warn" onClick={() => setConfirm({ kind: 'suspend', user: u })}>Khóa</button>)}
											{!self && <button className="link-btn danger" onClick={() => setConfirm({ kind: 'delete', user: u })}>Xóa</button>}
										</div>
									</td>
								</tr>
							);
						})}
						{!loading && items.length === 0 && <tr><td colSpan={6} className="dash-empty">Không có người dùng phù hợp.</td></tr>}
					</tbody>
				</table>
			</div>

			<div className="pager">
				<button className="tm-btn-cancel" disabled={page <= 1} onClick={() => setPage(page - 1)}>Trước</button>
				<span>Trang {pagination.page} / {Math.max(pagination.pages, 1)}</span>
				<button className="tm-btn-cancel" disabled={page >= pagination.pages} onClick={() => setPage(page + 1)}>Sau</button>
			</div>

			{creating && <CreateUserModal onClose={() => setCreating(false)} onCreated={load} showToast={showToast} />}

			{confirm && confirmText && (
				<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && setConfirm(null)}>
					<div className="tiktoday-modal confirm-web-modal">
						<div className="confirm-web-body">
							<div className="confirm-web-text"><h3>{confirmText.title}</h3><p>{confirmText.body}</p></div>
						</div>
						<div className="tm-footer">
							<button className="tm-btn-cancel" onClick={() => setConfirm(null)}>Huỷ bỏ</button>
							<button className={confirmText.danger ? 'tm-btn-danger' : 'tm-btn-submit'} onClick={runConfirm}>{confirmText.ok}</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
}
