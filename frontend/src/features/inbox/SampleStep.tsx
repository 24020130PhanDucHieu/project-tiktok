import { type ReactNode, useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { Collab, Creator } from '../../types/koc';

/** one scan of a J&T parcel, as the API returns it (newest first) */
interface TrackEvent {
	time: string;
	text: string;
}

interface Tracking {
	statusText: string;
	delivered: boolean;
	events: TrackEvent[];
	source: string;
}

interface Props {
	collab: Collab;
	koc: Creator | null;
	locked: boolean;
	/** false in the demo (no server): nothing is fetched */
	live: boolean;
	run: (req: () => Promise<{ data: { data: Collab } }>, ok?: string) => Promise<Collab | null | undefined>;
	onOpenShipping: () => void;
	onRejectSample: (sampleId: string) => void;
	/** the KOC changed on the server (status, delivered date): the parent reloads it */
	onKocChanged: () => void;
}

const SAMPLE_STATUS = {
	pending: { label: 'Chờ duyệt', tone: 'wait' },
	approved: { label: 'Đã duyệt', tone: 'ok' },
	rejected: { label: 'Từ chối', tone: 'bad' },
} as const;

const fmtDay = (iso?: string | null) => (iso ? new Date(iso).toLocaleDateString('vi-VN') : '');
const fmtClock = (iso: string) => new Date(iso).toLocaleTimeString('vi-VN', { hour: '2-digit', minute: '2-digit' });
const dayKey = (iso: string) => new Date(iso).toLocaleDateString('vi-VN', { day: '2-digit', month: '2-digit', year: 'numeric' });
const fmtDayHead = (iso: string) => new Date(iso).toLocaleDateString('vi-VN', { weekday: 'long', day: '2-digit', month: '2-digit' });

/** J&T writes names as 【name】: shown in bold without the brackets, which is easier to read in a narrow column. */
function scanText(text: string): ReactNode[] {
	return text.split(/【\s*([^】]*?)\s*】/).map((part, i) =>
		i % 2 === 1 ? <b key={i}> {part} </b> : <span key={i}>{part}</span>,
	);
}

/** the scans grouped by day, newest day first (the API already sorts newest first) */
function byDay(events: TrackEvent[]): { day: string; head: string; items: TrackEvent[] }[] {
	const groups: { day: string; head: string; items: TrackEvent[] }[] = [];
	for (const e of events) {
		const day = dayKey(e.time);
		const last = groups[groups.length - 1];
		if (last && last.day === day) last.items.push(e);
		else groups.push({ day, head: fmtDayHead(e.time), items: [e] });
	}
	return groups;
}

const Icon = ({ d }: { d: string }) => (
	<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d={d} /></svg>
);
const ICONS = {
	route: 'M6 19a2 2 0 1 0 0-4 2 2 0 0 0 0 4Zm12-10a2 2 0 1 0 0-4 2 2 0 0 0 0 4ZM8 17h6a4 4 0 0 0 0-8h-4a4 4 0 0 1 0-8',
	refresh: 'M21 12a9 9 0 1 1-2.64-6.36M21 4v5h-5',
	out: 'M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5',
	copy: 'M9 9h10a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1V10a1 1 0 0 1 1-1ZM5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1',
	edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z',
	chevron: 'm6 9 6 6 6-6',
};
const fmtWhen = (iso?: string | null) =>
	iso ? new Date(iso).toLocaleString('vi-VN', { hour: '2-digit', minute: '2-digit', day: '2-digit', month: '2-digit' }) : '';

const phoneLast4Of = (koc: Creator) => (koc.trackingPhoneLast4 || (koc.phone || '').replace(/\D/g, '').slice(-4)).trim();

/** The "Duyệt & gửi mẫu" step: the sample requests, and the parcel with its real J&T journey. */
export default function SampleStep({ collab: c, koc, locked, live, run, onOpenShipping, onRejectSample, onKocChanged }: Props) {
	const [adding, setAdding] = useState(false);
	const [product, setProduct] = useState('');
	const [open, setOpen] = useState(false);
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState('');
	const [tracking, setTracking] = useState<Tracking | null>(null);
	const [copied, setCopied] = useState(false);

	const code = koc?.trackingCode || '';
	// another KOC or another bill: what was fetched belongs to the previous one
	useEffect(() => {
		setOpen(false);
		setTracking(null);
		setError('');
	}, [koc?.id, code]);

	async function addSample() {
		const name = product.trim();
		if (!name) return;
		if (await run(() => api.post(`/collabs/${c._id}/samples`, { product: name }))) {
			setProduct('');
			setAdding(false);
		}
	}

	async function track() {
		if (!koc || !code || busy) return;
		const phoneLast4 = phoneLast4Of(koc);
		if (!/^\d{4}$/.test(phoneLast4)) {
			setError('J&T cần 4 số cuối SĐT người nhận. Bấm "Sửa vận đơn" để nhập.');
			return;
		}
		setBusy(true);
		setError('');
		try {
			const demo = !live || String(koc.id).startsWith('seed-') || String(koc.id).startsWith('local-');
			const { data } = demo
				? await api.post('/kocs/track-shipment/lookup', { trackingCode: code, phoneLast4 })
				: await api.post(`/kocs/${koc.id}/track-shipment`, { trackingCode: code, phoneLast4 });
			const t = demo ? data.data : data.tracking;
			setTracking(t ? { statusText: t.statusText || '', delivered: !!t.delivered, events: t.events || [], source: t.source || '' } : null);
			if (!demo) onKocChanged();
		} catch (err) {
			setError(apiMessage(err, 'Không tra cứu được vận đơn J&T. Thử lại sau ít phút.'));
		} finally {
			setBusy(false);
		}
	}

	function copyCode() {
		navigator.clipboard.writeText(code).then(() => {
			setCopied(true);
			setTimeout(() => setCopied(false), 1500);
		}, () => undefined);
	}

	function toggleTracking() {
		const next = !open;
		setOpen(next);
		if (next && !tracking) track();
	}

	const delivered = tracking ? tracking.delivered : koc?.stage === 'DELIVERED' || !!koc?.deliveredAt;
	const state = !code
		? { label: 'Chưa gửi', tone: 'idle' }
		: delivered
			? { label: 'Đã giao', tone: 'ok' }
			: koc?.shippingDelayedAlert
				? { label: 'Giao chậm', tone: 'bad' }
				: { label: 'Đang vận chuyển', tone: 'go' };
	const latest = tracking?.statusText || koc?.trackingStatusText || '';
	const events = tracking?.events || [];
	// the server could not reach J&T: it answers with a placeholder and no journey
	const noAnswer = !!tracking && tracking.source === 'jt-tracker';

	return (
		<div className="cp-body">
			<section className="cp-sec">
				<header className="cp-sec-head">
					<span>Yêu cầu mẫu</span>
					{c.samples.length > 0 && <small>{c.samples.length}</small>}
				</header>
				{c.samples.length === 0 && <small className="cp-muted">Chưa có yêu cầu mẫu từ KOC.</small>}
				{c.samples.map((s) => {
					const st = SAMPLE_STATUS[s.status];
					return (
						<div key={s.id} className="cp-sample">
							<div className="cp-sample-main">
								<strong title={s.product}>{s.product}</strong>
								<small>{fmtDay(s.at)}{s.status === 'rejected' && s.rejectReason ? ` · ${s.rejectReason}` : ''}</small>
							</div>
							<span className={`cp-tag ${st.tone}`}>{st.label}</span>
							{s.status === 'pending' && !locked && (
								<div className="cp-sample-actions">
									<button type="button" className="cp-btn sm" onClick={() => run(() => api.post(`/collabs/${c._id}/samples/${s.id}/approve`, {}), 'Đã duyệt mẫu. Hãy gửi hàng trước hạn.')}>Duyệt</button>
									<button type="button" className="cp-btn ghost sm" onClick={() => onRejectSample(s.id)}>Từ chối</button>
								</div>
							)}
						</div>
					);
				})}
				{!locked && (adding ? (
					<div className="cp-row cp-sample-add">
						<input autoFocus placeholder="Tên sản phẩm mẫu" value={product} onChange={(e) => setProduct(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') addSample(); if (e.key === 'Escape') setAdding(false); }} />
						<button type="button" className="cp-btn sm" disabled={!product.trim()} onClick={addSample}>Thêm</button>
						<button type="button" className="cp-link" onClick={() => { setAdding(false); setProduct(''); }}>Huỷ</button>
					</div>
				) : (
					<button type="button" className="cp-link cp-add" onClick={() => setAdding(true)}>+ Thêm yêu cầu mẫu</button>
				))}
			</section>

			<section className="cp-sec">
				<header className="cp-sec-head">
					<span>Vận chuyển mẫu</span>
					<span className={`cp-tag ${state.tone}`}>{state.label}</span>
				</header>
				{!code ? (
					<div className="cp-ship-empty">
						<small className="cp-muted">Chưa có mã vận đơn. Nhập mã J&T sau khi gửi mẫu để theo dõi hành trình.</small>
						<button type="button" className="cp-btn ghost sm" onClick={onOpenShipping}>Nhập mã vận đơn</button>
					</div>
				) : (
					<div className="cp-ship">
						<div className="cp-ship-top">
							<div className="cp-ship-code">
								<small>Mã vận đơn J&T Express</small>
								<button type="button" className="cp-ship-copy" title="Sao chép mã vận đơn" onClick={copyCode}>
									<strong>{code}</strong>
									<Icon d={ICONS.copy} />
									{copied && <em>Đã sao chép</em>}
								</button>
							</div>
							<button type="button" className="cp-btn ghost sm cp-icon-btn" onClick={onOpenShipping}><Icon d={ICONS.edit} />Sửa</button>
						</div>
						{latest && (
							<div className={`cp-ship-now ${state.tone}`}>
								<small>Trạng thái mới nhất{koc?.lastTrackedAt ? ` · cập nhật ${fmtWhen(koc.lastTrackedAt)}` : ''}</small>
								<p>{scanText(latest)}</p>
							</div>
						)}
						<button type="button" className={`cp-btn wide cp-icon-btn cp-track-toggle${open ? ' open' : ''}`} aria-expanded={open} onClick={toggleTracking}>
							<Icon d={ICONS.route} />
							{open ? 'Ẩn hành trình' : 'Xem hành trình đơn hàng'}
							<span className="cp-chevron"><Icon d={ICONS.chevron} /></span>
						</button>
						{open && (
							<div className="cp-track" aria-live="polite">
								<div className="cp-track-bar">
									<span>{events.length > 0 ? `${events.length} mốc hành trình` : 'Hành trình'}</span>
									<div>
										<button type="button" className="cp-btn ghost sm cp-icon-btn" disabled={busy} onClick={track}><Icon d={ICONS.refresh} />{busy ? 'Đang tải…' : 'Làm mới'}</button>
										<a className="cp-btn ghost sm cp-icon-btn" href={`https://jtexpress.vn/vi/tracking?type=track&billcode=${encodeURIComponent(code)}`} target="_blank" rel="noreferrer"><Icon d={ICONS.out} />Trang J&T</a>
									</div>
								</div>
								{busy && !tracking && <small className="cp-muted">Đang lấy hành trình từ J&T…</small>}
								{error && <small className="cp-err">{error}</small>}
								{!busy && !error && tracking && events.length === 0 && (
									<small className="cp-muted">
										{noAnswer ? 'Chưa lấy được hành trình từ J&T lúc này. Bấm "Làm mới" để thử lại.' : tracking.statusText || 'J&T chưa có hành trình cho vận đơn này.'}
									</small>
								)}
								{byDay(events).map((g, gi) => (
									<div key={g.day} className="cp-track-day">
										<h5>{g.head}</h5>
										<ol className="cp-track-list">
											{g.items.map((e, i) => (
												<li key={`${e.time}-${i}`} className={gi === 0 && i === 0 ? `latest ${delivered ? 'ok' : ''}` : ''}>
													<time dateTime={e.time}>{fmtClock(e.time)}</time>
													<p>{scanText(e.text)}</p>
												</li>
											))}
										</ol>
									</div>
								))}
							</div>
						)}
					</div>
				)}
			</section>
		</div>
	);
}
