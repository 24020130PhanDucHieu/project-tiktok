import { useEffect, useState } from 'react';
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
const fmtShortDay = (iso: string) => new Date(iso).toLocaleDateString('vi-VN', { day: '2-digit', month: '2-digit' });
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
								<small>J&T Express</small>
								<strong>#{code}</strong>
							</div>
							<button type="button" className="cp-link" onClick={onOpenShipping}>Sửa vận đơn</button>
						</div>
						{latest && <p className="cp-ship-latest">{latest}</p>}
						{koc?.lastTrackedAt && <small className="cp-muted">Cập nhật lúc {fmtWhen(koc.lastTrackedAt)}</small>}
						<div className="cp-ship-actions">
							<button type="button" className="cp-btn sm" aria-expanded={open} onClick={toggleTracking}>{open ? 'Ẩn hành trình' : 'Theo dõi đơn'}</button>
							{open && <button type="button" className="cp-btn ghost sm" disabled={busy} onClick={track}>{busy ? 'Đang tải…' : 'Làm mới'}</button>}
							<a className="cp-link cp-ship-out" href={`https://jtexpress.vn/vi/tracking?type=track&billcode=${encodeURIComponent(code)}`} target="_blank" rel="noreferrer">Mở trên J&T</a>
						</div>
						{open && (
							<div className="cp-track" aria-live="polite">
								{busy && !tracking && <small className="cp-muted">Đang lấy hành trình từ J&T…</small>}
								{error && <small className="cp-err">{error}</small>}
								{!busy && !error && tracking && events.length === 0 && (
									<small className="cp-muted">
										{noAnswer ? 'Chưa lấy được hành trình từ J&T lúc này. Bấm "Làm mới" để thử lại.' : tracking.statusText || 'J&T chưa có hành trình cho vận đơn này.'}
									</small>
								)}
								{events.length > 0 && (
									<ol className="cp-track-list">
										{events.map((e, i) => (
											<li key={`${e.time}-${i}`} className={i === 0 ? 'latest' : ''}>
												<time dateTime={e.time}>
													<b>{fmtClock(e.time)}</b>
													<span>{fmtShortDay(e.time)}</span>
												</time>
												<p>{e.text}</p>
											</li>
										))}
									</ol>
								)}
							</div>
						)}
					</div>
				)}
			</section>
		</div>
	);
}
