import { type ChangeEvent, type DragEvent, useRef, useState } from 'react';
import api from '../../lib/axios';
import { BULK_MAX_ROWS, NO_FOLDER, type BulkReport, type BulkRow, parseBulkSheet, runBulkImport, withTimeout } from './bulkImport';

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

/** Import KOCs from an Excel file: choose, check, import in small batches, read the report. */
export default function BulkImportModal({ folders, onClose, onDone, showToast }: Props) {
	const [step, setStep] = useState<Step>('pick');
	const [rows, setRows] = useState<BulkRow[]>([]);
	const [error, setError] = useState('');
	const [reading, setReading] = useState(false);
	const [progress, setProgress] = useState({ done: 0, total: 0 });
	const stop = useRef(false);
	const started = useRef(false); // a second click on "Nhập" must not start a second run
	const fileInput = useRef<HTMLInputElement>(null);

	const ready = rows.filter((r) => r.status === 'ready').length;
	const added = rows.filter((r) => r.status === 'added').length;
	const notAdded = rows.filter((r) => r.status !== 'added');

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
		stop.current = false;
		setStep('running');
		const finalRows = await runBulkImport(
			rows,
			async (items) => {
				const { data } = await api.post<{ data: BulkReport }>('/kocs/bulk', { items }, { timeout: 60000 });
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
		const n = finalRows.filter((r) => r.status === 'added').length;
		showToast(n > 0 ? `Đã thêm ${n} KOC vào danh bạ.` : 'Chưa thêm được KOC nào.', n > 0 ? 'success' : 'error');
		setStep('report');
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
		if (step === 'running') return; // stop first; closing mid-way would leave the batch unaccounted for
		if (added > 0) onDone();
		onClose();
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && close()}>
			<div className="tiktoday-modal bi-modal" role="dialog" aria-modal="true" aria-labelledby="bi-title">
				<div className="tm-header">
					<div>
						<h2 id="bi-title">Nhập KOC từ Excel</h2>
						<p>
							{step === 'pick' && 'Chọn file Excel đã điền theo file mẫu. Mỗi KOC phải có trong Nhà sáng tạo của TikTok Shop.'}
							{step === 'preview' && 'Kiểm tra các dòng trước khi nhập. Dòng lỗi sẽ không được nhập.'}
							{step === 'running' && 'Đang nhập, mỗi lô 10 KOC. Vui lòng không đóng cửa sổ.'}
							{step === 'report' && 'Kết quả nhập.'}
						</p>
					</div>
					<button type="button" className="tm-close" onClick={close} disabled={step === 'running'} aria-label="Đóng">
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
								Đã xử lý <b>{progress.done}</b> / {progress.total} KOC
							</p>
						</div>
					)}

					{step === 'report' && (
						<>
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
							<button type="button" className="tm-btn-cancel" onClick={() => { setRows([]); setError(''); setStep('pick'); }}>
								Chọn file khác
							</button>
							<button type="button" className="tm-btn-submit" disabled={ready === 0} onClick={start}>
								Nhập {ready} KOC
							</button>
						</>
					)}
					{step === 'running' && (
						<button type="button" className="tm-btn-cancel" onClick={() => { stop.current = true; }}>
							Dừng sau lô này
						</button>
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
