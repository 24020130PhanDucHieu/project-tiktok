import { useCallback, useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { SystemInfo } from '../../types/admin';

function uptime(seconds: number) {
	const d = Math.floor(seconds / 86400);
	const h = Math.floor((seconds % 86400) / 3600);
	const m = Math.floor((seconds % 3600) / 60);
	return `${d ? `${d}n ` : ''}${h}g ${m}p`;
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
		if (!auto) return;
		const t = window.setInterval(load, 10000);
		return () => window.clearInterval(t);
	}, [load, auto]);

	if (!info) return <div className="page-shell"><p className="dash-empty">Đang tải...</p></div>;
	const { http, runtime, database, services, users } = info;
	const errRate = http.requests ? ((http.serverErrors / http.requests) * 100).toFixed(2) : '0.00';

	return (
		<div className="page-shell">
			<header className="page-head">
				<div>
					<h1>Giám sát hệ thống</h1>
					<p>Phiên bản <b>{info.version}</b> · {info.goVersion} · chạy liên tục {uptime(http.uptimeSeconds)}</p>
				</div>
				<label className="auto-refresh"><input type="checkbox" checked={auto} onChange={(e) => setAuto(e.target.checked)} /> Tự làm mới 10s</label>
			</header>

			<section className="health-row">
				<Health ok label="API" detail={`${http.inFlight} yêu cầu đang xử lý`} />
				<Health ok={database.ok} label="MongoDB" detail={database.ok ? `Ping ${database.pingMs.toFixed(1)} ms` : database.error} />
				<Health ok={services.zaloPersonal.connected} label="Zalo cá nhân" detail={services.zaloPersonal.connected ? 'Đã kết nối' : `Trạng thái: ${services.zaloPersonal.state}`} />
				<Health ok={services.smtpConfigured} label="Email SMTP" detail={services.smtpConfigured ? 'Đã cấu hình' : 'Chưa cấu hình — không gửi được email reset/nhắc'} />
			</section>

			<section className="kpi-row">
				<div className="kpi-card"><span>Yêu cầu HTTP</span><strong>{http.requests.toLocaleString('vi-VN')}</strong></div>
				<div className="kpi-card"><span>Trung bình / P95</span><strong>{http.avgMs.toFixed(1)}<small> / {http.p95Ms.toFixed(1)} ms</small></strong></div>
				<div className="kpi-card"><span>Lỗi 5xx</span><strong className={http.serverErrors ? 'warn' : ''}>{http.serverErrors}<small> ({errRate}%)</small></strong></div>
				<div className="kpi-card"><span>Lỗi 4xx</span><strong>{http.clientErrors}</strong></div>
			</section>

			<div className="dash-split">
				<section className="panel-card">
					<h2 className="section-title">Tiến trình máy chủ</h2>
					<dl className="kv">
						<dt>Goroutines</dt><dd>{runtime.goroutines}</dd>
						<dt>CPU</dt><dd>{runtime.cpus} lõi</dd>
						<dt>Bộ nhớ heap</dt><dd>{runtime.heapAllocMB.toFixed(1)} MB</dd>
						<dt>Bộ nhớ hệ thống</dt><dd>{runtime.sysMB.toFixed(1)} MB</dd>
						<dt>Lần GC</dt><dd>{runtime.gcRuns}</dd>
						<dt>Yêu cầu chậm nhất</dt><dd>{http.slowestMs.toFixed(0)} ms</dd>
					</dl>
				</section>
				<section className="panel-card">
					<h2 className="section-title">Cơ sở dữ liệu</h2>
					<dl className="kv">
						<dt>Dữ liệu</dt><dd>{(database.dataSizeMB ?? 0).toFixed(2)} MB</dd>
						<dt>Lưu trữ</dt><dd>{(database.storageSizeMB ?? 0).toFixed(2)} MB</dd>
						<dt>Chỉ mục</dt><dd>{(database.indexSizeMB ?? 0).toFixed(2)} MB</dd>
						{Object.entries(database.collections).map(([k, v]) => (<><dt key={`k${k}`}>{k}</dt><dd key={`v${k}`}>{v.toLocaleString('vi-VN')} bản ghi</dd></>))}
					</dl>
				</section>
				<section className="panel-card">
					<h2 className="section-title">Tài khoản</h2>
					<dl className="kv">
						<dt>Tổng số</dt><dd>{users.total}</dd>
						<dt>Quản trị viên</dt><dd>{users.admins}</dd>
						<dt>Đang bị khóa</dt><dd>{users.suspended}</dd>
					</dl>
				</section>
			</div>
		</div>
	);
}
