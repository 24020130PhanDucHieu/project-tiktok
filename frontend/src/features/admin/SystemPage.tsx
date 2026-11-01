import { Fragment, useCallback, useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { SystemInfo } from '../../types/admin';

const n1 = (v?: number) => (v ?? 0).toLocaleString('vi-VN', { maximumFractionDigits: 1 });
const n2 = (v?: number) => (v ?? 0).toLocaleString('vi-VN', { maximumFractionDigits: 2 });

function uptime(seconds: number) {
	const d = Math.floor(seconds / 86400);
	const h = Math.floor((seconds % 86400) / 3600);
	const m = Math.floor((seconds % 3600) / 60);
	return `${d ? `${d} ngày ` : ''}${h} giờ ${m} phút`;
}

function Health({ ok, label, detail }: { ok: boolean; label: string; detail?: string }) {
	return (
		<div className={`health-card ${ok ? 'ok' : 'bad'}`}>
			<i />
			<div>
				<strong>{label}</strong>
				<small>{detail || (ok ? 'Hoạt động bình thường' : 'Có sự cố')}</small>
			</div>
		</div>
	);
}

function Meter({ label, used, total, unit, invert }: { label: string; used: number; total: number; unit: string; invert?: boolean }) {
	const pct = total > 0 ? Math.min(100, (used / total) * 100) : 0;
	const level = (invert ? 100 - pct : pct) > 90 ? 'bad' : (invert ? 100 - pct : pct) > 75 ? 'warn' : '';
	return (
		<div className="meter-row">
			<div><span>{label}</span><b>{n1(used)} / {n1(total)} {unit} ({n1(pct)}%)</b></div>
			<div className={`meter ${level}`}><i style={{ width: `${pct}%` }} /></div>
		</div>
	);
}

export default function SystemPage({ showToast }: { showToast: (m: string, t?: 'success' | 'error') => void }) {
	const [info, setInfo] = useState<SystemInfo | null>(null);
	const [auto, setAuto] = useState(true);

	const load = useCallback(() => {
		api.get('/admin/system')
			.then(({ data }) => setInfo(data.data))
			.catch((e) => showToast(apiMessage(e, 'Không thể tải thông tin hệ thống.'), 'error'));
	}, [showToast]);

	useEffect(() => {
		load();
		if (!auto) return undefined;
		const t = window.setInterval(() => !document.hidden && load(), 10000);
		return () => window.clearInterval(t);
	}, [load, auto]);

	if (!info) return <div className="page-shell"><p className="dash-empty">Đang tải...</p></div>;
	const { http, runtime, database: db, services, users, host, jobs } = info;
	const errRate = http.requests ? ((http.serverErrors / http.requests) * 100).toFixed(2) : '0.00';
	const hasHost = host.memTotalMB > 0;
	const cache = db.cache as Record<string, number>;
	const conn = db.connections as Record<string, number>;
	const ops = db.opcounters as Record<string, number>;
	const net = db.network as Record<string, number>;

	return (
		<div className="page-shell">
			<header className="page-head">
				<div>
					<h1>Giám sát hệ thống</h1>
					<p>Phiên bản <b>{info.version}</b> · {info.goVersion} · {host.os} · API chạy liên tục {uptime(http.uptimeSeconds)}</p>
				</div>
				<label className="auto-refresh"><input type="checkbox" checked={auto} onChange={(e) => setAuto(e.target.checked)} /> Tự làm mới 10 giây</label>
			</header>

			<section className="health-row">
				<Health ok label="API" detail={`${http.inFlight} yêu cầu đang xử lý`} />
				<Health ok={db.ok} label="MongoDB" detail={db.ok ? `v${db.serverVersion || '?'} · ping ${n1(db.pingMs)} ms` : db.error} />
				<Health ok={services.zaloPersonal.connected} label="Zalo cá nhân"
					detail={services.zaloPersonal.bridgeOnline === false ? 'Dịch vụ zalo-bridge không chạy' : services.zaloPersonal.connected ? 'Đã kết nối' : `Trạng thái: ${services.zaloPersonal.state}`} />
				<Health ok={services.smtpConfigured} label="Email SMTP" detail={services.smtpConfigured ? 'Đã cấu hình' : 'Chưa cấu hình: không gửi được mã xác nhận / nhắc việc'} />
			</section>

			<section className="kpi-row">
				<div className="kpi-card"><span>Yêu cầu HTTP</span><strong>{http.requests.toLocaleString('vi-VN')}</strong></div>
				<div className="kpi-card"><span>Trung bình / P95</span><strong>{n1(http.avgMs)}<small> / {n1(http.p95Ms)} ms</small></strong></div>
				<div className="kpi-card"><span>Lỗi 5xx</span><strong className={http.serverErrors ? 'warn' : ''}>{http.serverErrors}<small> ({errRate}%)</small></strong></div>
				<div className="kpi-card"><span>Lỗi 4xx</span><strong>{http.clientErrors}</strong></div>
			</section>

			<div className="sys-grid">
				{hasHost && (
					<section className="panel-card">
						<h2 className="section-title">Máy chủ</h2>
						<Meter label="RAM" used={host.memTotalMB - host.memAvailableMB} total={host.memTotalMB} unit="MB" />
						{host.swapTotalMB > 0 && <Meter label="Swap" used={host.swapTotalMB - host.swapFreeMB} total={host.swapTotalMB} unit="MB" />}
						{host.diskTotalGB > 0 && <Meter label="Ổ đĩa" used={host.diskTotalGB - host.diskFreeGB} total={host.diskTotalGB} unit="GB" />}
						<dl className="kv">
							<dt>Tải CPU (1 / 5 / 15 phút)</dt><dd>{n2(host.load1)} / {n2(host.load5)} / {n2(host.load15)} ({host.cpus} lõi)</dd>
							<dt>Máy chủ bật được</dt><dd>{uptime(host.uptimeSeconds)}</dd>
							<dt>Hostname</dt><dd>{host.hostname}</dd>
						</dl>
					</section>
				)}

				<section className="panel-card">
					<h2 className="section-title">Tiến trình API (Go)</h2>
					<dl className="kv">
						<dt>RAM tiến trình (RSS)</dt><dd>{n1(host.processRssMB)} MB</dd>
						<dt>Heap đang dùng / hệ thống cấp</dt><dd>{n1(runtime.heapAllocMB)} / {n1(runtime.sysMB)} MB</dd>
						<dt>Goroutines</dt><dd>{runtime.goroutines}</dd>
						<dt>Lần GC / dừng gần nhất</dt><dd>{runtime.gcRuns} / {n2(runtime.lastGcPauseMs)} ms</dd>
						<dt>Yêu cầu chậm nhất</dt><dd>{n1(http.slowestMs)} ms</dd>
					</dl>
				</section>

				<section className="panel-card">
					<h2 className="section-title">MongoDB</h2>
					{cache.maxMB > 0 && <Meter label="WiredTiger cache" used={cache.usedMB} total={cache.maxMB} unit="MB" />}
					<dl className="kv">
						<dt>Phiên bản / chạy được</dt><dd>{db.serverVersion || '—'} / {uptime(db.uptimeSeconds)}</dd>
						<dt>Kết nối (đang mở / còn lại)</dt><dd>{conn.current ?? 0} / {conn.available ?? 0}</dd>
						<dt>Bộ nhớ resident</dt><dd>{n1((db.memory as Record<string, number>).residentMB)} MB</dd>
						<dt>Dữ liệu / lưu trữ / chỉ mục</dt><dd>{n2(db.dataSizeMB)} / {n2(db.storageSizeMB)} / {n2(db.indexSizeMB)} MB</dd>
						<dt>Mạng vào / ra</dt><dd>{n1(net.inMB)} / {n1(net.outMB)} MB</dd>
						<dt>Thao tác (đọc / ghi / sửa / xóa)</dt><dd>{ops.query ?? 0} / {ops.insert ?? 0} / {ops.update ?? 0} / {ops.delete ?? 0}</dd>
					</dl>
				</section>

				<section className="panel-card">
					<h2 className="section-title">Tài khoản</h2>
					<dl className="kv">
						<dt>Tổng số</dt><dd>{users.total}</dd>
						<dt>Quản trị viên</dt><dd>{users.admins}</dd>
						<dt>Đăng nhập trong 7 ngày</dt><dd>{users.activeLast7d ?? 0}</dd>
						<dt>Đang bị khóa</dt><dd>{users.suspended}</dd>
					</dl>
				</section>
			</div>

			<h2 className="section-title">Collections trong cơ sở dữ liệu "{db.name}"</h2>
			<div className="table-scroll panel-card flush">
				<table className="adm-table compact">
					<thead>
						<tr><th>Collection</th><th>Bản ghi</th><th>Dữ liệu</th><th>Lưu trữ</th><th>TB / bản ghi</th><th>Chỉ mục</th><th>Dung lượng chỉ mục</th></tr>
					</thead>
					<tbody>
						{db.collections.map((c) => (
							<tr key={c.name}>
								<td><b>{c.name}</b></td>
								<td>{c.count.toLocaleString('vi-VN')}</td>
								<td>{n2(c.sizeMB)} MB</td>
								<td>{n2(c.storageMB)} MB</td>
								<td>{n1(c.avgObjBytes)} B</td>
								<td>{c.indexes}</td>
								<td>{n2(c.indexSizeMB)} MB</td>
							</tr>
						))}
					</tbody>
				</table>
			</div>

			<h2 className="section-title">Tác vụ nền</h2>
			<div className="table-scroll panel-card flush">
				<table className="adm-table compact">
					<thead><tr><th>Tác vụ</th><th>Chu kỳ</th><th>Lần chạy gần nhất</th><th>Thời gian</th><th>Kết quả</th><th>Số lần chạy / lỗi</th></tr></thead>
					<tbody>
						{jobs.map((j) => (
							<Fragment key={j.name}>
								<tr>
									<td><b>{j.name}</b></td>
									<td>{j.interval}</td>
									<td>{j.runs ? new Date(j.lastStart).toLocaleString('vi-VN') : 'Chưa chạy'}</td>
									<td>{j.runs ? `${j.durationMs} ms` : '—'}</td>
									<td>{j.runs ? (j.ok ? <span className="job-ok">Thành công</span> : <span className="job-bad" title={j.error}>Lỗi: {j.error}</span>) : '—'}</td>
									<td>{j.runs} / {j.failures}</td>
								</tr>
							</Fragment>
						))}
						{jobs.length === 0 && <tr><td colSpan={6} className="dash-empty">Chưa đăng ký tác vụ nào.</td></tr>}
					</tbody>
				</table>
			</div>
		</div>
	);
}
