import { useEffect, useMemo, useRef, useState } from 'react';
import api from '../../lib/axios';
import type { Creator, SavedTemplate } from '../../types/koc';
import {
	type BatchResponse,
	MSG_MAX_BODY,
	MSG_RECENT_DAYS,
	type MsgStatus,
	type SendRow,
	SUGGESTED_TEMPLATES,
	buildRecipients,
	filterRecipients,
	needsAttention,
	renderPreview,
	runBulkMessage,
	sendBlockReason,
	shouldRefresh,
} from '../kocs/bulkMessage';

type Step = 'compose' | 'running' | 'report';

interface Props {
	/** every KOC the app has loaded */
	creators: Creator[];
	/** the master switch for messages the system sends on its own has been read and is OFF: the server would refuse */
	switchOff: boolean;
	onClose: () => void;
	/** called when the window closes after messages may have gone out, so the lists reload */
	onDone: () => void;
	showToast: (msg: string, type?: 'success' | 'error') => void;
}

const STATUS_TEXT: Record<MsgStatus, string> = {
	queued: 'Chờ gửi',
	sent: 'Đã gửi',
	pending: 'Đã lưu, Zalo chưa nhận',
	skipped: 'Bỏ qua',
	not_sent: 'Chưa gửi',
	failed: 'Lỗi',
	unknown: 'Không rõ kết quả',
};

/**
 * Send one message to many KOCs over Zalo. One screen: the message (suggested or saved templates, editable) and the
 * recipients (untick who should not get it). Sending needs the "Cho phép gửi" switch, which is OFF every time the
 * window opens and goes back OFF after each send.
 */
export default function BulkMessageModal({ creators, switchOff, onClose, onDone, showToast }: Props) {
	const [step, setStep] = useState<Step>('compose');
	const [saved, setSaved] = useState<SavedTemplate[]>([]);
	const [activeTemplate, setActiveTemplate] = useState('');
	const [body, setBody] = useState('');
	const [armed, setArmed] = useState(false); // always starts OFF
	const [query, setQuery] = useState('');
	const [ticked, setTicked] = useState<Record<string, boolean>>(() => {
		const map: Record<string, boolean> = {};
		buildRecipients(creators).forEach((r) => (map[r.creator.id] = r.checked));
		return map;
	});
	const [rows, setRows] = useState<SendRow[]>([]);
	const [progress, setProgress] = useState({ done: 0, total: 0 });
	const stop = useRef(false);
	const started = useRef(false); // a second click on "Gửi" must not start a second run

	const recipients = useMemo(() => buildRecipients(creators), [creators]);
	const visible = useMemo(() => filterRecipients(recipients, query), [recipients, query]);

	useEffect(() => {
		api.get('/inbox/templates')
			.then(({ data }) => setSaved(data.data || []))
			.catch(() => setSaved([]));
	}, []);

	const eligible = recipients.filter((r) => !r.blocked);
	const selected = eligible.filter((r) => ticked[r.creator.id]);
	const bodyText = body.trim();
	const blockReason = sendBlockReason({ armed, switchOff, selected: selected.length, body });
	const previewFor = selected[0]?.creator ?? eligible[0]?.creator;
	const count = (status: MsgStatus) => rows.filter((r) => r.status === status).length;
	const notDone = needsAttention(rows);

	function pick(id: string, text: string) {
		setActiveTemplate(id);
		setBody(text);
	}

	function setVisible(value: boolean) {
		const next = { ...ticked };
		visible.forEach((r) => {
			if (!r.blocked) next[r.creator.id] = value;
		});
		setTicked(next);
	}

	async function start() {
		if (started.current || blockReason) return;
		started.current = true;
		stop.current = false;
		const initial: SendRow[] = selected.map((r) => ({ id: r.creator.id, name: r.creator.name, status: 'queued' }));
		setRows(initial);
		setProgress({ done: 0, total: initial.length });
		setArmed(false); // the next send has to be allowed again
		setStep('running');
		const finalRows = await runBulkMessage(
			initial,
			async (kocIds) => {
				const { data } = await api.post<{ data: BatchResponse }>('/kocs/message-batch', { body: bodyText, kocIds, confirm: true }, { timeout: 30000 });
				return data.data;
			},
			{
				shouldStop: () => stop.current,
				onProgress: (done, total, current) => {
					setProgress({ done, total });
					setRows(current);
				},
			},
		);
		setRows(finalRows);
		started.current = false;
		const sent = finalRows.filter((r) => r.status === 'sent').length;
		showToast(sent > 0 ? `Đã gửi ${sent} tin.` : 'Chưa gửi được tin nào.', sent > 0 ? 'success' : 'error');
		setStep('report');
	}

	function close() {
		if (step === 'running') return; // stop first; closing mid-way would leave the batch unaccounted for
		if (shouldRefresh(rows)) onDone();
		onClose();
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && step === 'report' && close()}>
			<div className="tiktoday-modal bm-modal" role="dialog" aria-modal="true" aria-labelledby="bm-title">
				<div className="tm-header">
					<div>
						<h2 id="bm-title">Gửi tin hàng loạt</h2>
						<p>
							{step === 'compose' && 'Soạn một tin, chọn người nhận rồi bật "Cho phép gửi". Chữ {ten} được thay bằng tên từng KOC.'}
							{step === 'running' && 'Đang gửi, mỗi lô 5 KOC. Vui lòng không đóng cửa sổ.'}
							{step === 'report' && 'Kết quả gửi.'}
						</p>
					</div>
					<button type="button" className="tm-close" onClick={close} disabled={step === 'running'} aria-label="Đóng">
						×
					</button>
				</div>

				<div className="tm-body bm-body">
					{step === 'compose' && (
						<>
							{switchOff && (
								<p className="bm-alert" role="alert">
									Công tắc "Tin tự động" đang tắt nên chưa gửi được. Nhờ quản trị viên bật ở đầu khung chat.
								</p>
							)}
							<div className="bm-grid">
								<section className="bm-col">
									<h3 className="bm-h">Nội dung</h3>
									<div className="bm-chips" role="group" aria-label="Mẫu gợi ý">
										{SUGGESTED_TEMPLATES.map((t) => (
											<button key={t.id} type="button" className={`bm-chip${activeTemplate === t.id ? ' on' : ''}`} onClick={() => pick(t.id, t.body)}>
												{t.name}
											</button>
										))}
									</div>
									{saved.length > 0 && (
										<div className="bm-chips" role="group" aria-label="Mẫu đã lưu">
											<span className="bm-chips-label">Mẫu của bạn</span>
											{saved.map((t) => (
												<button key={t._id} type="button" className={`bm-chip saved${activeTemplate === t._id ? ' on' : ''}`} onClick={() => pick(t._id, t.body)}>
													{t.name}
												</button>
											))}
										</div>
									)}
									<textarea
										className="bm-text"
										rows={6}
										value={body}
										maxLength={MSG_MAX_BODY}
										placeholder="Chọn một mẫu ở trên hoặc tự viết nội dung. Dùng {ten} để chèn tên KOC."
										onChange={(e) => {
											setBody(e.target.value);
											setActiveTemplate('');
										}}
									/>
									<div className="bm-meta">
										<span>Sửa được, không đổi mẫu đã lưu</span>
										<span>
											{bodyText.length}/{MSG_MAX_BODY}
										</span>
									</div>
									{previewFor && bodyText && (
										<div className="bm-preview">
											<small>Xem trước cho {previewFor.name}</small>
											<p>{renderPreview(bodyText, previewFor)}</p>
										</div>
									)}
								</section>

								<section className="bm-col">
									<div className="bm-h-row">
										<h3 className="bm-h">Người nhận</h3>
										<span className="bm-count">
											{selected.length}/{eligible.length} đã chọn
										</span>
									</div>
									<input className="bm-search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Tìm theo tên, kênh, số điện thoại…" />
									<div className="bm-links">
										<button type="button" className="bm-link" onClick={() => setVisible(true)}>
											Chọn tất cả
										</button>
										<span aria-hidden="true">·</span>
										<button type="button" className="bm-link" onClick={() => setVisible(false)}>
											Bỏ chọn tất cả
										</button>
										<span className="bm-hint">Liên hệ trong {MSG_RECENT_DAYS} ngày qua được bỏ tích sẵn</span>
									</div>
									<div className="bm-list">
										{visible.length === 0 && <p className="bm-empty">Không có KOC phù hợp.</p>}
										{visible.map((r) => (
											<label key={r.creator.id} className={`bm-row${r.blocked ? ' blocked' : ''}`}>
												<input
													type="checkbox"
													className="bm-check"
													checked={!r.blocked && !!ticked[r.creator.id]}
													disabled={!!r.blocked}
													onChange={(e) => setTicked({ ...ticked, [r.creator.id]: e.target.checked })}
												/>
												<span className="bm-who">
													<strong>{r.creator.name}</strong>
													<small>{r.creator.handle}</small>
												</span>
												<span className="bm-phone">{r.creator.phone || '—'}</span>
												{(r.blocked || r.recentDaysAgo !== undefined) && (
													<em className={`bm-note${r.blocked ? ' bad' : ''}`}>{r.blocked ?? `${r.recentDaysAgo} ngày trước`}</em>
												)}
											</label>
										))}
									</div>
								</section>
							</div>
						</>
					)}

					{step === 'running' && (
						<div className="bi-progress">
							<div className="bi-bar" role="progressbar" aria-valuemin={0} aria-valuemax={progress.total} aria-valuenow={progress.done}>
								<i style={{ width: `${progress.total ? (progress.done / progress.total) * 100 : 0}%` }} />
							</div>
							<p>
								Đã xử lý <b>{progress.done}</b> / {progress.total} KOC
							</p>
						</div>
					)}

					{step === 'report' && (
						<>
							<p className="bi-summary">
								Đã gửi <b className="ok">{count('sent')}</b>
								{count('pending') > 0 && <>, đã lưu nhưng Zalo chưa nhận <b className="bad">{count('pending')}</b></>}
								{count('skipped') > 0 && <>, bỏ qua <b>{count('skipped')}</b></>}
								{count('not_sent') > 0 && <>, chưa gửi <b className="bad">{count('not_sent')}</b></>}
								{count('failed') > 0 && <>, lỗi <b className="bad">{count('failed')}</b></>}
								{count('unknown') > 0 && <>, không rõ kết quả <b className="bad">{count('unknown')}</b></>}
							</p>
							{notDone.length > 0 && (
								<div className="bi-scroll">
									<table className="bi-table">
										<thead>
											<tr>
												<th>KOC</th>
												<th>Kết quả</th>
											</tr>
										</thead>
										<tbody>
											{notDone.map((r) => (
												<tr key={r.id} className="bad">
													<td>{r.name}</td>
													<td>
														<span className="bi-chip">{STATUS_TEXT[r.status]}</span> {r.reason}
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
							)}
						</>
					)}
				</div>

				<div className="tm-footer bm-footer">
					{step === 'compose' && (
						<>
							<label className={`ti-switch bm-arm${armed ? ' on' : ''}`} title="Bật công tắc này mới gửi được. Mỗi lần mở cửa sổ, và sau mỗi lần gửi, công tắc tự tắt.">
								<input type="checkbox" checked={armed} onChange={(e) => setArmed(e.target.checked)} />
								<span className="ti-track">
									<i />
								</span>
								<b>Cho phép gửi: {armed ? 'Bật' : 'Tắt'}</b>
							</label>
							<span className="bm-why">{blockReason}</span>
							<button type="button" className="tm-btn-cancel" onClick={close}>
								Đóng
							</button>
							<button type="button" className="tm-btn-submit" disabled={!!blockReason} onClick={start}>
								Gửi {selected.length} tin
							</button>
						</>
					)}
					{step === 'running' && (
						<button type="button" className="tm-btn-cancel" onClick={() => { stop.current = true; }}>
							Dừng sau lô này
						</button>
					)}
					{step === 'report' && (
						<button type="button" className="tm-btn-submit" onClick={close}>
							Xong
						</button>
					)}
				</div>
			</div>
		</div>
	);
}
