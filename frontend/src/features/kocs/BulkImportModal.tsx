import { type ChangeEvent, type DragEvent, useEffect, useRef, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import {
	BULK_MAX_ROWS,
	type BulkRow,
	type ImportJob,
	NO_FOLDER,
	POLL_MS,
	RATE_MAX_WAITS,
	finishedRows,
	jobProgress,
	jobRows,
	parseBulkSheet,
	startRequest,
	stopText,
	waitSecondsLeft,
	withTimeout,
} from './bulkImport';

type Step = 'pick' | 'preview' | 'running' | 'report';

interface Props {
	folders: string[];
	onClose: () => void;
	/** called when the window closes after KOCs were added, so the directory reloads */
	onDone: () => void;
	showToast: (msg: string, type?: 'success' | 'error') => void;
}

const MAX_BYTES = 2 * 1024 * 1024;

const STATUS_TEXT: Record<BulkRow['status'], string> = {
	ready: 'Hợp lệ',
	invalid: 'Lỗi trong file',
	added: 'Đã thêm',
	duplicate: 'Đã có trong danh bạ',
	collaborated: 'Bị chặn',
	notListed: 'Không có trong TikTok Shop',
	failed: 'Chưa nhập (lỗi)',
	skipped: 'Chưa nhập (đã dừng)',
};

/**
 * Import KOCs from an Excel file: choose, check, send. The server then imports the rows one after the other in the
 * background and this window only shows what the job says, so closing or reloading the page does not stop it.
 */
export default function BulkImportModal({ folders, onClose, onDone, showToast }: Props) {
	const [step, setStep] = useState<Step>('pick');
	const [rows, setRows] = useState<BulkRow[]>([]);
	const [job, setJob] = useState<ImportJob | null>(null);
	const [error, setError] = useState('');
	const [reading, setReading] = useState(false);
	const [sending, setSending] = useState(false);
	const [stopping, setStopping] = useState(false);
	const [now, setNow] = useState(() => new Date()); // ticks every second while the job waits for TikTok
	const liveBox = useRef<HTMLDivElement>(null);
	const started = useRef(false); // a second click on "Nhập" must not start a second job
	const fileInput = useRef<HTMLInputElement>(null);

	const ready = rows.filter((r) => r.status === 'ready').length;
	const added = rows.filter((r) => r.status === 'added').length;
	const notAdded = rows.filter((r) => r.status !== 'added');

	function show(next: ImportJob) {
		setJob(next);
		setRows(jobRows(next));
		setStep(next.status === 'running' ? 'running' : 'report');
	}

	// opening the window brings back a job that is already running (started before the page was reloaded or by someone else)
	useEffect(() => {
		api.get('/import-jobs/current')
			.then(({ data }) => {
				const current: ImportJob | null = data.data?.job ?? null;
				if (current) show(current);
			})
			.catch(() => undefined);
	}, []);

	// while the job runs, ask the server how far it is; when it ends, show the report
	const jobId = job?._id;
	const running = job?.status === 'running';
	useEffect(() => {
		if (!jobId || !running) return;
		const timer = setInterval(() => {
			api.get(`/import-jobs/${jobId}`)
				.then(({ data }) => {
					const next: ImportJob = data.data;
					show(next);
					if (next.status !== 'running') {
						setStopping(false);
						const n = next.rows.filter((r) => r.status === 'added').length;
						showToast(n > 0 ? `Đã thêm ${n} KOC vào danh bạ.` : 'Chưa thêm được KOC nào.', n > 0 ? 'success' : 'error');
					}
				})
				.catch(() => undefined); // a failed question is asked again at the next tick
		}, POLL_MS);
		return () => clearInterval(timer);
	}, [jobId, running]); // eslint-disable-line react-hooks/exhaustive-deps

	const waiting = !!job?.waitUntil;
	useEffect(() => {
		if (!waiting) return;
		const timer = setInterval(() => setNow(new Date()), 1000);
		return () => clearInterval(timer);
	}, [waiting]);

	// the live list follows the newest row
	useEffect(() => {
		const el = liveBox.current;
		if (el) el.scrollTop = el.scrollHeight;
	}, [rows]);

	async function readFile(file: File | undefined) {
		if (!file) return;
		setError('');
		if (!/\.xlsx$/i.test(file.name)) return setError('Chỉ nhận file Excel .xlsx. Hãy tải file mẫu để điền.');
		if (file.size > MAX_BYTES) return setError('File quá lớn (tối đa 2 MB).');
		setReading(true);
		try {
			const { readSheet } = await import('read-excel-file/browser'); // loaded only when a file is chosen
			const result = parseBulkSheet(await withTimeout(readSheet(file), 20000), folders);
			if (result.fatal) setError(result.fatal);
			else if (result.tooMany) setError(`File có hơn ${BULK_MAX_ROWS} dòng. Hãy chia thành nhiều file, mỗi file tối đa ${BULK_MAX_ROWS} dòng.`);
			else {
				setRows(result.rows);
				setStep('preview');
			}
		} catch {
			setError('Không đọc được file. Hãy dùng đúng file Excel .xlsx (nên điền vào file mẫu).');
		} finally {
			setReading(false);
		}
	}

	function onPick(e: ChangeEvent<HTMLInputElement>) {
		readFile(e.target.files?.[0]);
		e.target.value = ''; // choosing the same file again must still fire
	}

	function onDrop(e: DragEvent<HTMLDivElement>) {
		e.preventDefault();
		readFile(e.dataTransfer.files?.[0]);
	}

	async function start() {
		if (started.current || ready === 0) return;
		started.current = true;
		setSending(true);
		try {
			const { data } = await api.post<{ data: ImportJob }>('/import-jobs', startRequest(rows), { timeout: 30000 });
			show(data.data);
		} catch (e) {
			showToast(apiMessage(e, 'Không bắt đầu được việc nhập.'), 'error');
			// another job may be running: show it instead of leaving the person with an error and no picture
			api.get('/import-jobs/current')
				.then(({ data: cur }) => cur.data?.job && show(cur.data.job))
				.catch(() => undefined);
		} finally {
			started.current = false;
			setSending(false);
		}
	}

	async function stop() {
		if (!job || stopping) return;
		setStopping(true);
		try {
			await api.put(`/import-jobs/${job._id}/stop`);
		} catch (e) {
			setStopping(false);
			showToast(apiMessage(e, 'Không dừng được việc nhập.'), 'error');
		}
	}

	async function downloadNotAdded() {
		const { default: writeExcelFile } = await import('write-excel-file/browser'); // loaded only when asked for
		const text = (value: string) => ({ value, type: String, format: '@' });
		const head = ['TikTok ID', 'Email', 'Số điện thoại', 'Người nhận mẫu', 'Địa chỉ nhận mẫu', 'Phí cast', 'Thư mục', 'Lý do'].map((value) => ({ value, fontWeight: 'bold' as const }));
		const body = notAdded.map((r) => [
			text(r.tiktokId), r.email || null, text(r.phone), r.recipient || null, r.shippingAddress || null, r.castFee || null,
			r.category === NO_FOLDER ? null : r.category, r.reason || STATUS_TEXT[r.status],
		]);
		await writeExcelFile([head, ...body], { columns: [{ width: 22 }, { width: 28 }, { width: 16 }, { width: 22 }, { width: 38 }, { width: 12 }, { width: 18 }, { width: 50 }] }).toFile('koc-chua-nhap.xlsx');
	}

	function close() {
		// closing does not stop a job that is running: it goes on in the server and this window shows it again when opened
		if (added > 0) onDone();
		onClose();
	}

	const progress = job ? jobProgress(job) : { done: 0, total: 0, added: 0, current: undefined, waitAttempt: undefined };
	const secondsLeft = job ? waitSecondsLeft(job, now) : 0;

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && step !== 'running' && close()}>
			<div className="tiktoday-modal bi-modal" role="dialog" aria-modal="true" aria-labelledby="bi-title">
				<div className="tm-header">
					<div>
						<h2 id="bi-title">Nhập KOC từ Excel</h2>
						<p>
							{step === 'pick' && 'Chọn file Excel đã điền theo file mẫu. Mỗi KOC phải có trong Nhà sáng tạo của TikTok Shop.'}
							{step === 'preview' && 'Kiểm tra các dòng trước khi nhập. Dòng lỗi sẽ không được nhập.'}
							{step === 'running' && 'Máy chủ đang nhập từng KOC, mỗi tài khoản cách nhau vài giây để TikTok không giới hạn. Bạn có thể đóng cửa sổ, việc nhập vẫn tiếp tục.'}
							{step === 'report' && 'Kết quả nhập.'}
						</p>
					</div>
					<button type="button" className="tm-close" onClick={close} aria-label="Đóng">
						×
					</button>
				</div>

				<div className="tm-body bi-body">
					{step === 'pick' && (
						<>
							<div
								className="bi-drop"
								onDragOver={(e) => e.preventDefault()}
								onDrop={onDrop}
								onClick={() => fileInput.current?.click()}
								role="button"
								tabIndex={0}
								onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && fileInput.current?.click()}
							>
								<strong>{reading ? 'Đang đọc file…' : 'Bấm để chọn file hoặc kéo thả vào đây'}</strong>
								<span>Chỉ nhận .xlsx, tối đa {BULK_MAX_ROWS} dòng</span>
								<input ref={fileInput} type="file" accept=".xlsx" onChange={onPick} hidden />
							</div>
							{error && <p className="bi-error" role="alert">{error}</p>}
							<p className="bi-hint">
								Chưa có file? <a href="/mau-nhap-koc.xlsx" download>Tải file mẫu</a> rồi điền theo hướng dẫn trong file.
							</p>
						</>
					)}

					{step === 'preview' && (
						<>
							<p className="bi-summary">
								<b>{ready}</b> dòng hợp lệ{rows.length - ready > 0 && <>, <b className="bad">{rows.length - ready}</b> dòng lỗi</>}
							</p>
							<div className="bi-scroll">
								<table className="bi-table">
									<thead>
										<tr><th>Dòng</th><th>TikTok ID</th><th>Số điện thoại</th><th>Thư mục</th><th>Trạng thái</th></tr>
									</thead>
									<tbody>
										{rows.map((r) => (
											<tr key={r.line} className={r.status === 'ready' ? '' : 'bad'}>
												<td>{r.line}</td>
												<td>{r.tiktokId || '—'}</td>
												<td>{r.phone || '—'}</td>
												<td>{r.category}</td>
												<td>{r.status === 'ready' ? <span className="bi-ok">Hợp lệ</span> : r.reason}</td>
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
								Đã xử lý <b>{progress.done}</b> / {progress.total} KOC · đã thêm <b className="ok">{progress.added}</b>
							</p>
							{progress.current && !waiting && (
								<p className="bi-now" aria-live="polite">
									Đang xác minh với TikTok: <b>@{progress.current}</b>
								</p>
							)}
							{waiting && (
								<p className="bi-wait" role="status">
									TikTok đang giới hạn tốc độ. Tự thử lại sau <b>{secondsLeft}</b> giây (lần {progress.waitAttempt}/{RATE_MAX_WAITS}), bạn không cần làm gì.
								</p>
							)}
							{finishedRows(rows).length > 0 && (
								<div className="bi-live" ref={liveBox} aria-label="Kết quả từng KOC">
									{finishedRows(rows).map((r) => (
										<div key={r.line} className={`bi-live-row ${r.status}`}>
											<span className="bi-live-id">@{r.tiktokId}</span>
											<span className="bi-live-status">{STATUS_TEXT[r.status]}</span>
										</div>
									))}
								</div>
							)}
						</div>
					)}

					{step === 'report' && (
						<>
							{job?.status === 'stopped' && stopText(job.stopReason) && (
								<p className="bi-wait" role="status">
									{stopText(job.stopReason)}
								</p>
							)}
							<p className="bi-summary">
								Đã thêm <b className="ok">{added}</b> KOC{notAdded.length > 0 && <>, <b className="bad">{notAdded.length}</b> dòng chưa thêm</>}
							</p>
							{notAdded.length > 0 && (
								<div className="bi-scroll">
									<table className="bi-table">
										<thead>
											<tr><th>Dòng</th><th>TikTok ID</th><th>Kết quả</th></tr>
										</thead>
										<tbody>
											{notAdded.map((r) => (
												<tr key={r.line} className="bad">
													<td>{r.line}</td>
													<td>{r.tiktokId || '—'}</td>
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
					{step === 'pick' && (
						<button type="button" className="tm-btn-cancel" onClick={close}>
							Đóng
						</button>
					)}
					{step === 'preview' && (
						<>
							<button type="button" className="tm-btn-cancel" disabled={sending} onClick={() => { setRows([]); setError(''); setStep('pick'); }}>
								Chọn file khác
							</button>
							<button type="button" className="tm-btn-submit" disabled={ready === 0 || sending} onClick={start}>
								{sending ? 'Đang gửi…' : `Nhập ${ready} KOC`}
							</button>
						</>
					)}
					{step === 'running' && (
						<>
							<button type="button" className="tm-btn-cancel" onClick={close}>
								Đóng, việc nhập vẫn chạy
							</button>
							<button type="button" className="tm-btn-submit" disabled={stopping} onClick={stop}>
								{stopping ? 'Đang dừng…' : 'Dừng việc nhập'}
							</button>
						</>
					)}
					{step === 'report' && (
						<>
							{notAdded.length > 0 && (
								<button type="button" className="tm-btn-cancel" onClick={downloadNotAdded}>
									Tải danh sách dòng chưa nhập
								</button>
							)}
							<button type="button" className="tm-btn-submit" onClick={close}>
								Xong
							</button>
						</>
					)}
				</div>
			</div>
		</div>
	);
}
