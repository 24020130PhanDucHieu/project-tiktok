import { useCallback, useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { DashboardData, TaskKoc } from '../../types/admin';

const STAGE_LABELS: Record<string, string> = {
	SOURCED: 'Mới tìm',
	QUALIFIED: 'Đạt phễu',
	AWAITING_APPROVAL: 'Chờ duyệt',
	FILTERED_OUT: 'Loại',
	CONTACTED: 'Đã liên hệ',
	NO_REPLY: 'Chưa phản hồi',
	REPLIED: 'Đã phản hồi',
	DORMANT: 'Ngủ đông',
	AGREED: 'Đồng ý',
	DECLINED: 'Từ chối',
	SHIPPING: 'Đang giao',
	DELIVERED: 'Đã nhận mẫu',
	VIDEO_POSTED: 'Đã lên video',
	NOT_FIT: 'Không hợp',
	RETURNING: 'Đang hoàn',
	RETURNED: 'Đã hoàn',
	STALLED: 'Đình trệ',
	COMPLETED: 'Hoàn tất',
};

const STAGE_ORDER = Object.keys(STAGE_LABELS);

const TASKS: { key: string; title: string; hint: string; tone: 'blue' | 'amber' | 'red' | 'violet' }[] = [
	{ key: 'needApproval', title: 'Chờ chủ shop duyệt cast', hint: 'KOC đòi/được đề xuất phí cast', tone: 'amber' },
	{ key: 'needCall', title: 'Cần gọi điện', hint: 'Quá 7 ngày chưa phản hồi', tone: 'red' },
	{ key: 'needShip', title: 'Chờ gửi mẫu', hint: 'Đã có địa chỉ nhận hàng', tone: 'blue' },
	{ key: 'delayedShipments', title: 'Vận đơn chậm > 5 ngày', hint: 'J&T / tự vận chuyển chưa giao', tone: 'red' },
	{ key: 'stalled', title: 'Đình trệ (STALLED)', hint: 'Quá 3 lần nhắc video', tone: 'violet' },
	{ key: 'pendingReturn', title: 'Chờ thu hồi mẫu', hint: 'KOC báo không hợp / đang hoàn', tone: 'amber' },
];

const money = (n: number) => new Intl.NumberFormat('vi-VN').format(Math.round(n));
const pct = (a: number, b: number) => (b > 0 ? Math.round((a / b) * 100) : 0);

function TaskCard({ title, hint, tone, items, onOpenKoc }: { title: string; hint: string; tone: string; items: TaskKoc[]; onOpenKoc: () => void }) {
	return (
		<div className={`dash-task tone-${tone}`}>
			<div className="dash-task-head">
				<div>
					<strong>{title}</strong>
					<small>{hint}</small>
				</div>
				<span className="dash-count">{items.length}</span>
			</div>
			{items.length === 0 ? (
				<p className="dash-empty">Không có việc tồn đọng</p>
			) : (
				<ul>
					{items.slice(0, 5).map((k) => (
						<li key={k.id} onClick={onOpenKoc}>
							{k.avatarUrl ? <img src={k.avatarUrl} alt="" loading="lazy" /> : <span className="dash-avatar">{k.displayName.slice(0, 1)}</span>}
							<span>
								<b>{k.displayName}</b>
								<small>@{k.tiktokId}{k.phone ? ` · ${k.phone}` : ''}</small>
							</span>
						</li>
					))}
					{items.length > 5 && <li className="dash-more">+{items.length - 5} KOC khác</li>}
				</ul>
			)}
		</div>
	);
}

export default function DashboardPage({ onOpenKoc, showToast }: { onOpenKoc: () => void; showToast: (m: string, t?: 'success' | 'error') => void }) {
	const [data, setData] = useState<DashboardData | null>(null);
	const [loading, setLoading] = useState(true);

	const load = useCallback(() => {
		setLoading(true);
		api.get('/kocs/stats')
			.then(({ data: res }) => setData(res.data))
			.catch((e) => showToast(apiMessage(e, 'Không thể tải dữ liệu tổng quan.'), 'error'))
			.finally(() => setLoading(false));
	}, [showToast]);

	useEffect(load, [load]);

	const maxStage = Math.max(1, ...STAGE_ORDER.map((s) => data?.byStage[s] ?? 0));
	const open = TASKS.reduce((sum, t) => sum + (data?.tasks[t.key]?.length ?? 0), 0);

	return (
		<div className="page-shell">
			<header className="page-head">
				<div>
					<h1>Tổng quan</h1>
					<p>Việc cần làm hôm nay, phễu KOC theo giai đoạn và theo từng đợt booking.</p>
				</div>
				<button className="tm-btn-cancel" onClick={load} disabled={loading}>{loading ? 'Đang tải...' : 'Làm mới'}</button>
			</header>

			<section className="kpi-row">
				<div className="kpi-card"><span>Tổng KOC đang quản lý</span><strong>{money(data?.total ?? 0)}</strong></div>
				<div className="kpi-card"><span>Việc cần xử lý hôm nay</span><strong className={open ? 'warn' : ''}>{open}</strong></div>
				<div className="kpi-card"><span>GMV từ video</span><strong>{money(data?.videoGmv ?? 0)}đ</strong></div>
				<div className="kpi-card">
					<span>Đã lên video / Đã liên hệ</span>
					<strong>{(data?.byStage.VIDEO_POSTED ?? 0) + (data?.byStage.COMPLETED ?? 0)}<small> / {Object.entries(data?.byStage ?? {}).filter(([s]) => !['SOURCED', 'QUALIFIED', 'AWAITING_APPROVAL', 'FILTERED_OUT'].includes(s)).reduce((a, [, c]) => a + c, 0)}</small></strong>
				</div>
			</section>

			<h2 className="section-title">Việc hôm nay</h2>
			<section className="dash-tasks">
				{TASKS.map((t) => (
					<TaskCard key={t.key} title={t.title} hint={t.hint} tone={t.tone} items={data?.tasks[t.key] ?? []} onOpenKoc={onOpenKoc} />
				))}
			</section>

			<div className="dash-split">
				<section className="panel-card">
					<h2 className="section-title">Phân bố theo giai đoạn</h2>
					<div className="stage-bars">
						{STAGE_ORDER.filter((s) => (data?.byStage[s] ?? 0) > 0).map((s) => (
							<div key={s} className="stage-bar-row">
								<span>{STAGE_LABELS[s]}</span>
								<div className="stage-bar"><i style={{ width: `${(100 * (data?.byStage[s] ?? 0)) / maxStage}%` }} /></div>
								<b>{data?.byStage[s]}</b>
							</div>
						))}
						{!loading && (data?.total ?? 0) === 0 && <p className="dash-empty">Chưa có KOC nào. Hãy thêm KOC đầu tiên ở mục Quản lý KOC.</p>}
					</div>
				</section>

				<section className="panel-card">
					<h2 className="section-title">Funnel theo đợt booking</h2>
					<div className="table-scroll">
						<table className="adm-table compact">
							<thead>
								<tr><th>Đợt</th><th>Pipeline</th><th>Liên hệ</th><th>Phản hồi</th><th>Đồng ý</th><th>Đã nhận</th><th>Lên video</th><th>GMV</th></tr>
							</thead>
							<tbody>
								{(data?.funnels ?? []).map((f) => (
									<tr key={f.campaign || '_'}>
										<td><b>{f.campaign || 'Chưa gán đợt'}</b></td>
										<td>{f.pipeline}</td>
										<td>{f.contacted}</td>
										<td>{f.replied} <small>{pct(f.replied, f.contacted)}%</small></td>
										<td>{f.agreed}</td>
										<td>{f.delivered}</td>
										<td>{f.posted} <small>{pct(f.posted, f.delivered)}%</small></td>
										<td>{money(f.gmv)}đ</td>
									</tr>
								))}
								{!loading && !(data?.funnels?.length) && <tr><td colSpan={8} className="dash-empty">Chưa có dữ liệu.</td></tr>}
							</tbody>
						</table>
					</div>
				</section>
			</div>
		</div>
	);
}
