import { useCallback, useEffect, useMemo, useState } from 'react';
import { BarChart, LineChart } from '../../components/charts';
import Select from '../../components/Select';
import api, { apiMessage } from '../../lib/axios';
import type { CollabGranularity, CollabOverview, CollabRow } from '../../types/koc';

interface Props {
	onOpenKoc: (handle: string) => void;
	showToast: (m: string, t?: 'success' | 'error') => void;
}

interface Payload {
	overview: CollabOverview;
	items: CollabRow[];
	total: number;
}

const GRANS: { key: CollabGranularity; label: string }[] = [
	{ key: 'day', label: 'Ngày' },
	{ key: 'week', label: 'Tuần' },
	{ key: 'month', label: 'Tháng' },
	{ key: 'year', label: 'Năm' },
];

const STEPS = ['Chốt phí', 'Hình thức', 'Mẫu', 'Video', 'Kết quả'];
const OUTCOMES: Record<string, string> = { posted: 'Đã đăng bài', no_post: 'Nhận mẫu, không đăng', no_response: 'Không phản hồi', no_need: 'Hết nhu cầu' };
const MODES: Record<string, string> = { open: 'Cộng tác mở', targeted: 'Mời mục tiêu', product: 'Link sản phẩm', none: 'Chưa chọn hình thức' };

const money = (n: number) => `${new Intl.NumberFormat('vi-VN').format(Math.round(n))}đ`;
const num = (n: number) => new Intl.NumberFormat('vi-VN').format(Math.round(n));
const day = (iso: string) => new Date(iso).toLocaleDateString('vi-VN');

function Kpi({ label, value, sub, tone }: { label: string; value: string; sub?: string; tone?: 'good' | 'bad' }) {
	return (
		<div className="kpi-card">
			<span>{label}</span>
			<strong className={tone === 'bad' ? 'warn' : tone === 'good' ? 'good' : ''}>{value}</strong>
			{sub && <small className="co-kpi-sub">{sub}</small>}
		</div>
	);
}

function Split({ title, data, labels }: { title: string; data: Record<string, number>; labels: Record<string, string> }) {
	const rows = Object.keys(labels).map((k) => ({ key: k, label: labels[k], n: data[k] || 0 }));
	const total = rows.reduce((a, r) => a + r.n, 0);
	return (
		<section className="panel-card">
			<h3 className="section-title">{title}</h3>
			{total === 0 ? (
				<p className="dash-empty">Chưa có dữ liệu</p>
			) : (
				<div className="co-split">
					{rows.map((r) => (
						<div key={r.key} className="co-split-row">
							<span>{r.label}</span>
							<div className="co-split-bar"><i style={{ width: `${(r.n / total) * 100}%` }} /></div>
							<b>{r.n}</b>
							<small>{Math.round((r.n / total) * 100)}%</small>
						</div>
					))}
				</div>
			)}
		</section>
	);
}

export default function CollabAdminPage({ onOpenKoc, showToast }: Props) {
	const [gran, setGran] = useState<CollabGranularity>('day');
	const [from, setFrom] = useState('');
	const [to, setTo] = useState('');
	const [status, setStatus] = useState('');
	const [mode, setMode] = useState('');
	const [q, setQ] = useState('');
	const [data, setData] = useState<Payload | null>(null);
	const [loading, setLoading] = useState(true);

	const load = useCallback(() => {
		setLoading(true);
		api.get<{ data: Payload }>('/collabs/overview', {
			params: { granularity: gran, from: from || undefined, to: to || undefined, status: status || undefined, mode: mode || undefined, q: q.trim() || undefined },
		})
			.then(({ data: res }) => setData(res.data))
			.catch((e) => showToast(apiMessage(e, 'Không tải được dữ liệu hợp tác.'), 'error'))
			.finally(() => setLoading(false));
	}, [gran, from, to, status, mode, q, showToast]);

	useEffect(() => {
		const t = window.setTimeout(load, q ? 300 : 0);
		return () => window.clearTimeout(t);
	}, [load, q]);

	const ov = data?.overview;
	const series = ov?.series ?? [];
	const labels = useMemo(() => series.map((b) => b.label), [series]);
	const t = ov?.totals;
	const postedRate = t && t.closed > 0 ? Math.round((t.posted / t.closed) * 100) : 0;
	const cost = (t?.castFee ?? 0) + (t?.commission ?? 0);
	const roi = cost > 0 ? Math.round(((t?.revenue ?? 0) / cost) * 100) / 100 : 0;

	return (
		<div className="page-shell co-page">
			<header className="page-head">
				<div>
					<h1>Quản trị hợp tác</h1>
					<p>Theo dõi mọi lần hợp tác với KOC: tiến độ, chi phí, doanh thu và lợi nhuận. Số liệu tính theo ngày bắt đầu của lần hợp tác.</p>
				</div>
				<button className="tm-btn-cancel" onClick={load} disabled={loading}>{loading ? 'Đang tải...' : 'Làm mới'}</button>
			</header>

			<div className="co-toolbar">
				<div className="co-tabs" role="tablist" aria-label="Khoảng thời gian">
					{GRANS.map((g) => (
						<button key={g.key} type="button" role="tab" aria-selected={gran === g.key} className={gran === g.key ? 'on' : ''} onClick={() => { setGran(g.key); setFrom(''); setTo(''); }}>{g.label}</button>
					))}
				</div>
				<label className="co-date">Từ<input type="date" value={from || ov?.from || ''} onChange={(e) => setFrom(e.target.value)} /></label>
				<label className="co-date">Đến<input type="date" value={to || ov?.to || ''} onChange={(e) => setTo(e.target.value)} /></label>
				{(from || to) && <button type="button" className="co-reset" onClick={() => { setFrom(''); setTo(''); }}>Mặc định</button>}
				<Select className="adm-filter" value={status} onChange={(e) => setStatus(e.target.value)}>
					<option value="">Mọi trạng thái</option>
					<option value="active">Đang chạy</option>
					<option value="closed">Đã kết thúc</option>
				</Select>
				<Select className="adm-filter" value={mode} onChange={(e) => setMode(e.target.value)}>
					<option value="">Mọi hình thức</option>
					{Object.entries(MODES).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
				</Select>
				<input className="adm-input co-search" placeholder="Tìm KOC, sản phẩm, tên lần…" value={q} onChange={(e) => setQ(e.target.value)} />
			</div>

			<section className="kpi-row co-kpis">
				<Kpi label="Tổng lần hợp tác" value={num(t?.collabs ?? 0)} sub={`${t?.active ?? 0} đang chạy · ${t?.closed ?? 0} đã kết thúc`} />
				<Kpi label="Doanh thu" value={money(t?.revenue ?? 0)} sub={`${num(t?.orders ?? 0)} đơn · ${num(t?.videos ?? 0)} video`} />
				<Kpi label="Chi phí" value={money(cost)} sub={`Cast ${money(t?.castFee ?? 0)} · hoa hồng ${money(t?.commission ?? 0)}`} />
				<Kpi label="Lợi nhuận" value={money(t?.profit ?? 0)} tone={(t?.profit ?? 0) < 0 ? 'bad' : (t?.profit ?? 0) > 0 ? 'good' : undefined} sub={cost > 0 ? `Doanh thu gấp ${roi} lần chi phí` : 'Chưa có chi phí'} />
				<Kpi label="Lượt xem" value={num(t?.views ?? 0)} sub={t && t.views > 0 ? `Cast ${money((t.castFee / t.views) * 1000)} / 1.000 lượt xem` : undefined} />
				<Kpi label="Tỷ lệ đăng bài" value={`${postedRate}%`} sub={`${t?.posted ?? 0}/${t?.closed ?? 0} lần đã kết thúc có video`} />
				<Kpi label="Phí cast còn nợ" value={money(t?.owed ?? 0)} tone={(t?.owed ?? 0) > 0 ? 'bad' : undefined} sub="Đã đăng bài nhưng chưa trả phí" />
			</section>

			<section className="panel-card co-chart-card">
				<h3 className="section-title">Doanh thu, chi phí và lợi nhuận</h3>
				<LineChart
					labels={labels}
					format={money}
					series={[
						{ key: 'revenue', label: 'Doanh thu', color: '#1d4ed8', values: series.map((b) => b.revenue) },
						{ key: 'cost', label: 'Chi phí', color: '#f59e0b', values: series.map((b) => b.castFee + b.commission) },
						{ key: 'profit', label: 'Lợi nhuận', color: '#10b981', values: series.map((b) => b.profit) },
					]}
				/>
			</section>

			<div className="co-grid2">
				<section className="panel-card co-chart-card">
					<h3 className="section-title">Số lần hợp tác</h3>
					<BarChart
						labels={labels}
						height={220}
						axisFormat={(n) => String(Math.round(n))}
						format={(n) => String(Math.round(n))}
						series={[
							{ key: 'collabs', label: 'Bắt đầu', color: '#3b82f6', values: series.map((b) => b.collabs) },
							{ key: 'closed', label: 'Đã kết thúc', color: '#a3b4cf', values: series.map((b) => b.closed) },
						]}
					/>
				</section>
				<section className="panel-card co-chart-card">
					<h3 className="section-title">Lượt xem và đơn hàng</h3>
					<LineChart
						labels={labels}
						height={220}
						format={num}
						series={[
							{ key: 'views', label: 'Lượt xem', color: '#7c3aed', values: series.map((b) => b.views) },
							{ key: 'orders', label: 'Đơn hàng', color: '#0ea5e9', values: series.map((b) => b.orders) },
						]}
					/>
				</section>
			</div>

			<div className="co-grid2">
				<Split title="Kết cục các lần đã kết thúc" data={ov?.outcomes ?? {}} labels={OUTCOMES} />
				<Split title="Hình thức hợp tác" data={ov?.modes ?? {}} labels={MODES} />
			</div>

			<section className="panel-card flush co-table-card">
				<div className="co-table-head">
					<h3 className="section-title">Danh sách lần hợp tác</h3>
					<small>{data ? `${data.items.length}${data.total > data.items.length ? ` / ${data.total}` : ''} lần` : ''}</small>
				</div>
				<div className="table-scroll">
					<table className="adm-table co-table">
						<thead>
							<tr>
								<th>KOC</th><th>Lần</th><th>Sản phẩm</th><th>Hình thức</th><th>Tiến độ</th>
								<th className="r">Phí cast</th><th className="r">Doanh thu</th><th className="r">Lợi nhuận</th><th>Trạng thái</th><th>Bắt đầu</th>
							</tr>
						</thead>
						<tbody>
							{(data?.items ?? []).map((c) => (
								<tr key={c._id} className="co-row" onClick={() => onOpenKoc(c.koc.handle)} title="Mở trong hộp thư">
									<td>
										<div className="co-koc">
											{c.koc.avatarUrl ? <img src={c.koc.avatarUrl} alt="" loading="lazy" /> : <span className="co-avatar">{(c.koc.name || '?').slice(0, 1)}</span>}
											<span><b>{c.koc.name || 'KOC đã xóa'}</b><small>{c.koc.handle}</small></span>
										</div>
									</td>
									<td className="nowrap"><b>#{c.seq}</b>{c.name && <small className="co-sub">{c.name}</small>}</td>
									<td>
										<div className="co-prod">
											{c.productImage ? <img src={c.productImage} alt="" loading="lazy" /> : <span className="co-thumb" />}
											<span>{c.productName || '—'}</span>
										</div>
									</td>
									<td className="nowrap">{MODES[c.mode || 'none']}</td>
									<td>
										<div className="co-steps" title={STEPS[Math.min(c.step, 4)]}>
											{STEPS.map((s, i) => <i key={s} className={i < c.step || c.status === 'closed' ? 'done' : i === c.step ? 'cur' : ''} />)}
											<small>{c.status === 'closed' ? 'Xong' : STEPS[Math.min(c.step, 4)]}</small>
										</div>
									</td>
									<td className="r nowrap">{money(c.castFee)}</td>
									<td className="r nowrap">{money(c.perf.revenue)}</td>
									<td className={`r nowrap ${c.perf.profit < 0 ? 'co-neg' : c.perf.profit > 0 ? 'co-pos' : ''}`}>{money(c.perf.profit)}</td>
									<td>
										<span className={`tone-pill ${c.status === 'closed' ? (c.outcome === 'posted' ? 'ok' : 'warn') : 'info'}`}>
											{c.status === 'closed' ? OUTCOMES[c.outcome] || 'Đã kết thúc' : 'Đang chạy'}
										</span>
									</td>
									<td className="nowrap">{day(c.startedAt)}</td>
								</tr>
							))}
							{!loading && (data?.items.length ?? 0) === 0 && <tr><td colSpan={10} className="dash-empty">Chưa có lần hợp tác nào trong khoảng này.</td></tr>}
						</tbody>
					</table>
				</div>
			</section>
		</div>
	);
}
