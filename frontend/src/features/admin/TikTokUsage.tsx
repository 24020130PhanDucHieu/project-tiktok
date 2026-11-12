import { useCallback, useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';

interface UsageHour {
	hour: string; // 2026-11-12T17 (UTC)
	calls: number;
	rejected: number;
}

interface UsageKind {
	kind: string;
	calls: number;
	rejected: number;
}

interface Usage {
	from: string;
	to: string;
	calls: number;
	rejected: number;
	hours: UsageHour[];
	kinds: UsageKind[];
	complete: boolean;
}

const fmt = (n: number) => n.toLocaleString('vi-VN');

/** "2026-11-12T17" (UTC) as the hour on the viewer's clock. */
const hourLabel = (h: string) => `${new Date(`${h}:00:00Z`).getHours()}h`;

/** The route a call went to, without the method noise: "POST /affiliate_seller/{id}/orders/search" -> "orders/search". */
const kindLabel = (k: string) => {
	const [method, path] = k.split(' ');
	const parts = path.split('/').filter((p) => p && p !== '{id}');
	return `${method} ${parts.slice(-2).join('/')}`;
};

/**
 * How many calls this app sent to TikTok Shop, hour by hour and by kind. TikTok's rate limit is shared by the whole app
 * group, so this shows how much of it is ours. Read from the call log of the server (kept across deploys).
 */
export default function TikTokUsage() {
	const [data, setData] = useState<Usage | null>(null);
	const [error, setError] = useState('');

	const load = useCallback(() => {
		api.get<{ data: Usage }>('/ttshop/usage', { params: { hours: 24 } })
			.then(({ data: r }) => {
				setData(r.data);
				setError('');
			})
			.catch((e) => setError(apiMessage(e, 'Không đọc được số lần gọi TikTok.')));
	}, []);

	useEffect(() => {
		load();
		const t = window.setInterval(() => !document.hidden && load(), 60000);
		return () => window.clearInterval(t);
	}, [load]);

	const max = Math.max(1, ...(data?.hours.map((h) => h.calls) ?? [1]));

	return (
		<section className="panel-card tku">
			<h2 className="section-title">Số lần gọi TikTok Shop (24 giờ gần nhất)</h2>
			{error && <p className="tku-note bad">{error}</p>}
			{!data && !error && <p className="dash-empty">Đang tải...</p>}
			{data && (
				<>
					<div className="tku-sum">
						<div><b>{fmt(data.calls)}</b><span>lần gọi</span></div>
						<div className={data.rejected ? 'bad' : ''}><b>{fmt(data.rejected)}</b><span>bị TikTok từ chối (quá giới hạn)</span></div>
						<div><b>{data.calls ? fmt(Math.round(data.calls / Math.max(1, data.hours.length))) : 0}</b><span>lần mỗi giờ, trung bình</span></div>
					</div>
					{!data.complete && data.from && (
						<p className="tku-note">Nhật ký mới có từ {new Date(data.from).toLocaleString('vi-VN')}, nên số liệu chưa đủ 24 giờ.</p>
					)}
					{data.hours.length > 0 && (
						<div className="tku-bars" role="img" aria-label="Số lần gọi theo giờ">
							{data.hours.map((h) => (
								<div className="tku-col" key={h.hour} title={`${hourLabel(h.hour)}: ${h.calls} lần${h.rejected ? `, ${h.rejected} bị từ chối` : ''}`}>
									<i className={h.rejected ? 'bad' : ''} style={{ height: `${Math.max(4, (h.calls / max) * 100)}%` }} />
									<span>{hourLabel(h.hour)}</span>
								</div>
							))}
						</div>
					)}
					<table className="adm-table compact">
						<thead><tr><th>Loại lần gọi</th><th>Số lần</th><th>Bị từ chối</th></tr></thead>
						<tbody>
							{data.kinds.slice(0, 8).map((k) => (
								<tr key={k.kind}>
									<td title={k.kind}>{kindLabel(k.kind)}</td>
									<td>{fmt(k.calls)}</td>
									<td>{k.rejected ? <span className="job-bad">{fmt(k.rejected)}</span> : '0'}</td>
								</tr>
							))}
							{data.kinds.length === 0 && <tr><td colSpan={3} className="dash-empty">Chưa có lần gọi nào trong nhật ký.</td></tr>}
						</tbody>
					</table>
				</>
			)}
		</section>
	);
}
