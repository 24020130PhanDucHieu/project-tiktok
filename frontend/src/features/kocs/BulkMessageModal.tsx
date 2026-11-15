import { useEffect, useMemo, useRef, useState } from 'react';
import api from '../../lib/axios';
import type { Creator, SavedTemplate } from '../../types/koc';
import { type BatchResponse, MSG_MAX_BODY, MSG_RECENT_DAYS, type MsgStatus, type SendRow, buildRecipients, needsAttention, renderPreview, runBulkMessage, shouldRefresh } from './bulkMessage';

type Step = 'template' | 'recipients' | 'running' | 'report';

interface Props {
	/** the KOCs the directory is showing (its filters already applied) */
	creators: Creator[];
	onClose: () => void;
	/** called when the window closes after messages went out, so the directory reloads (the "last contacted" column) */
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

const SWITCH_OFF = 'Công tắc gửi tự động đang tắt, nhờ quản trị viên bật.';

/** Send one template to many KOCs over Zalo: pick the template, untick who should not get it, send in small batches, read the report. */
export default function BulkMessageModal({ creators, onClose, onDone, showToast }: Props) {
	const [step, setStep] = useState<Step>('template');
	const [templates, setTemplates] = useState<SavedTemplate[]>([]);
	const [body, setBody] = useState('');
	const [automation, setAutomation] = useState<{ loaded: boolean; enabled: boolean }>({ loaded: false, enabled: false });
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

	useEffect(() => {
		api.get('/inbox/templates')
			.then(({ data }) => setTemplates(data.data || []))
			.catch(() => setTemplates([]));
		api.get('/automation')
			.then(({ data }) => setAutomation({ loaded: true, enabled: !!data.data?.enabled }))
			.catch(() => setAutomation({ loaded: false, enabled: false })); // unknown: the server still refuses when the switch is off
	}, []);

	const selected = recipients.filter((r) => !r.blocked && ticked[r.creator.id]);
	const switchOff = automation.loaded && !automation.enabled;
	const bodyText = body.trim();
	const canSend = selected.length > 0 && bodyText.length > 0 && bodyText.length <= MSG_MAX_BODY && !switchOff;
	const previewFor = selected[0]?.creator ?? recipients[0]?.creator;

	const count = (status: MsgStatus) => rows.filter((r) => r.status === status).length;
	const notDone = needsAttention(rows);

	function setAll(value: boolean) {
		const next = { ...ticked };
		recipients.forEach((r) => {
			if (!r.blocked) next[r.creator.id] = value;
		});
		setTicked(next);
	}

	async function start() {
		if (started.current || !canSend) return;
		started.current = true;
		stop.current = false;
		const initial: SendRow[] = selected.map((r) => ({ id: r.creator.id, name: r.creator.name, status: 'queued' }));
		setRows(initial);
		setProgress({ done: 0, total: initial.length });
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

	const subtitle: Record<Step, string> = {
		template: 'Chọn một mẫu tin. Chữ {ten} sẽ được thay bằng tên từng KOC.',
		recipients: 'Bỏ tích những KOC không muốn gửi. KOC bị mờ không gửi được.',
		running: `Đang gửi, mỗi lô 5 KOC. Vui lòng không đóng cửa sổ.`,
		report: 'Kết quả gửi.',
	};

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && close()}>
			<div className="tiktoday-modal bi-modal" role="dialog" aria-modal="true" aria-labelledby="bm-title">
				<div className="tm-header">
					<div>
						<h2 id="bm-title">Gửi tin hàng loạt qua Zalo</h2>
						<p>{subtitle[step]}</p>
					</div>
					<button type="button" className="tm-close" onClick={close} disabled={step === 'running'} aria-label="Đóng">
						×
					</button>
				</div>

				<div className="tm-body bi-body">
					{switchOff && (
						<p className="bi-error" role="alert">
							{SWITCH_OFF}
						</p>
					)}

					{step === 'template' && (
						<>
							<label className="bm-label">
								Mẫu tin
								<select
									className="bm-field"
									defaultValue=""
									onChange={(e) => setBody(templates.find((t) => t._id === e.target.value)?.body ?? '')}
								>
									<option value="">{templates.length ? 'Chọn một mẫu…' : 'Chưa có mẫu nào (soạn mẫu ở hộp thư)'}</option>
									{templates.map((t) => (
										<option key={t._id} value={t._id}>
											{t.name}
										</option>
									))}
								</select>
							</label>
							<label className="bm-label">
								Nội dung gửi (sửa được, không đổi mẫu đã lưu)
								<textarea className="bm-field" rows={6} value={body} onChange={(e) => setBody(e.target.value)} maxLength={MSG_MAX_BODY} />
								<span className="bi-hint">
									{bodyText.length}/{MSG_MAX_BODY} ký tự
								</span>
							</label>
							{previewFor && bodyText && (
								<div className="bm-preview">
									<small>Xem trước cho {previewFor.name}:</small>
									<p>{renderPreview(bodyText, previewFor)}</p>
								</div>
							)}
						</>
					)}

					{step === 'recipients' && (
						<>
							<p className="bi-summary">
								Sẽ gửi <b className="ok">{selected.length}</b> tin
								<span className="bi-hint"> · đã liên hệ trong {MSG_RECENT_DAYS} ngày qua thì bỏ tích sẵn</span>
							</p>
							<p className="bi-hint">
								<button type="button" className="btn-text-link" onClick={() => setAll(true)}>
									Chọn tất cả
								</button>
								{' · '}
								<button type="button" className="btn-text-link" onClick={() => setAll(false)}>
									Bỏ chọn tất cả
								</button>
							</p>
							<div className="bi-scroll">
								<table className="bi-table">
									<thead>
										<tr>
											<th></th>
											<th>KOC</th>
											<th>Số điện thoại</th>
											<th>Ghi chú</th>
										</tr>
									</thead>
									<tbody>
										{recipients.map((r) => (
											<tr key={r.creator.id} className={r.blocked ? 'bad' : ''}>
												<td>
													<input
														type="checkbox"
														checked={!r.blocked && !!ticked[r.creator.id]}
														disabled={!!r.blocked}
														onChange={(e) => setTicked({ ...ticked, [r.creator.id]: e.target.checked })}
													/>
												</td>
												<td>
													{r.creator.name} <small>{r.creator.handle}</small>
												</td>
												<td>{r.creator.phone || '—'}</td>
												<td>{r.blocked ?? (r.recentDaysAgo !== undefined ? `Đã liên hệ ${r.recentDaysAgo} ngày trước` : '')}</td>
											</tr>
										))}
									</tbody>
								</table>
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

				<div className="tm-footer">
					{step === 'template' && (
						<>
							<button type="button" className="tm-btn-cancel" onClick={close}>
								Đóng
							</button>
							<button type="button" className="tm-btn-submit" disabled={!bodyText} onClick={() => setStep('recipients')}>
								Tiếp theo
							</button>
						</>
					)}
					{step === 'recipients' && (
						<>
							<button type="button" className="tm-btn-cancel" onClick={() => setStep('template')}>
								Quay lại
							</button>
							<button type="button" className="tm-btn-submit" disabled={!canSend} onClick={start}>
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
