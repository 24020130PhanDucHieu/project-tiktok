import { useCallback, useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { AuditEntry, Pagination } from '../../types/admin';

const ACTIONS: Record<string, { label: string; tone: 'ok' | 'warn' | 'bad' | 'info' }> = {
	'auth.login': { label: 'Đăng nhập', tone: 'ok' },
	'auth.login_failed': { label: 'Đăng nhập thất bại', tone: 'bad' },
	'auth.password_changed': { label: 'Đổi mật khẩu', tone: 'info' },
	'auth.reset_requested': { label: 'Yêu cầu reset mật khẩu', tone: 'warn' },
	'auth.password_reset': { label: 'Đặt lại mật khẩu', tone: 'info' },
	'admin.user_created': { label: 'Tạo tài khoản', tone: 'info' },
	'admin.user_updated': { label: 'Sửa tài khoản', tone: 'warn' },
	'admin.user_deleted': { label: 'Xóa tài khoản', tone: 'bad' },
	'admin.reset_sent': { label: 'Gửi email reset MK', tone: 'info' },
	'koc.archived': { label: 'Xóa KOC', tone: 'warn' },
	'outreach.run': { label: 'Chạy tự động hóa', tone: 'info' },
	'zalo.connect': { label: 'Kết nối Zalo', tone: 'info' },
	'zalo.qr_login': { label: 'Đăng nhập Zalo QR', tone: 'info' },
	'zalo.disconnect': { label: 'Ngắt Zalo', tone: 'warn' },
};

const GROUPS = [
	{ value: '', label: 'Tất cả hành động' },
	{ value: 'auth', label: 'Xác thực' },
	{ value: 'admin', label: 'Quản trị người dùng' },
	{ value: 'koc', label: 'KOC' },
	{ value: 'outreach', label: 'Tự động hóa' },
	{ value: 'zalo', label: 'Zalo' },
];

export default function AuditPage({ showToast }: { showToast: (m: string, t?: 'success' | 'error') => void }) {
	const [items, setItems] = useState<AuditEntry[]>([]);
	const [pagination, setPagination] = useState<Pagination>({ page: 1, limit: 30, total: 0, pages: 1 });
	const [action, setAction] = useState('');
	const [q, setQ] = useState('');
	const [page, setPage] = useState(1);
	const [loading, setLoading] = useState(true);

	const load = useCallback(() => {
		setLoading(true);
		api.get('/admin/audit', { params: { action, q, page, limit: 30 } })
			.then(({ data }) => {
				setItems(data.data.items);
				setPagination(data.data.pagination);
			})
			.catch((e) => showToast(apiMessage(e, 'Không thể tải nhật ký.'), 'error'))
			.finally(() => setLoading(false));
	}, [action, q, page, showToast]);

	useEffect(() => {
		const t = window.setTimeout(load, q ? 300 : 0);
		return () => window.clearTimeout(t);
	}, [load, q]);

	return (
		<div className="page-shell">
			<header className="page-head">
				<div>
					<h1>Nhật ký hoạt động</h1>
					<p>Ghi lại đăng nhập, thay đổi tài khoản và thao tác nhạy cảm. Lưu 180 ngày. {pagination.total} bản ghi.</p>
				</div>
				<button className="tm-btn-cancel" onClick={load} disabled={loading}>{loading ? 'Đang tải...' : 'Làm mới'}</button>
			</header>
			<div className="adm-toolbar">
				<input className="adm-input" placeholder="Lọc theo email người thực hiện..." value={q} onChange={(e) => { setQ(e.target.value); setPage(1); }} />
				<select className="adm-input" value={action} onChange={(e) => { setAction(e.target.value); setPage(1); }}>
					{GROUPS.map((g) => <option key={g.value} value={g.value}>{g.label}</option>)}
				</select>
			</div>
			<div className="table-scroll panel-card flush">
				<table className="adm-table">
					<thead><tr><th>Thời gian</th><th>Người thực hiện</th><th>Hành động</th><th>Chi tiết</th><th>IP</th></tr></thead>
					<tbody>
						{items.map((a) => {
							const meta = ACTIONS[a.action] ?? { label: a.action, tone: 'info' as const };
							return (
								<tr key={a.id}>
									<td className="nowrap">{new Date(a.createdAt).toLocaleString('vi-VN')}</td>
									<td>{a.actorEmail || '—'}</td>
									<td><span className={`tone-pill ${meta.tone}`}>{meta.label}</span></td>
									<td>{a.detail || '—'}</td>
									<td className="nowrap">{a.ip || '—'}</td>
								</tr>
							);
						})}
						{!loading && items.length === 0 && <tr><td colSpan={5} className="dash-empty">Chưa có bản ghi nào.</td></tr>}
					</tbody>
				</table>
			</div>
			<div className="pager">
				<button className="tm-btn-cancel" disabled={page <= 1} onClick={() => setPage(page - 1)}>← Trước</button>
				<span>Trang {pagination.page} / {Math.max(pagination.pages, 1)}</span>
				<button className="tm-btn-cancel" disabled={page >= pagination.pages} onClick={() => setPage(page + 1)}>Sau →</button>
			</div>
		</div>
	);
}
