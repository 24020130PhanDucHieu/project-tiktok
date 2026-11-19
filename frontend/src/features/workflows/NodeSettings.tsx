import { type ChangeEvent, type DragEvent, useRef, useState } from 'react';
import Select from '../../components/Select';
import type { Campaign } from '../../types/campaign';
import { BULK_MAX_ROWS, type BulkRow, parseBulkSheet, withTimeout } from '../kocs/bulkImport';
import { MAX_WAIT_MINUTES } from './graph';
import { NODE_HINTS, NODE_TITLES, type NodeData } from './WorkflowNodeCard';

const MAX_BYTES = 2 * 1024 * 1024;
const UNITS = { minute: 1, hour: 60, day: 1440 } as const;
type Unit = keyof typeof UNITS;

function unitOf(minutes: number): Unit {
	if (minutes > 0 && minutes % 1440 === 0) return 'day';
	if (minutes > 0 && minutes % 60 === 0) return 'hour';
	return 'minute';
}

/** The Excel file of the import block: what was read from it, kept in the editor (never stored in the browser). */
export interface SheetInfo {
	fileName: string;
	rows: BulkRow[];
}

interface Props {
	nodeId: string;
	data: NodeData;
	campaigns: Campaign[];
	folders: string[];
	sheet: SheetInfo | null;
	onSheet: (sheet: SheetInfo | null) => void;
	onChange: (patch: Partial<NodeData>) => void;
	onDelete: () => void;
}

function WaitSettings({ data, onChange }: Pick<Props, 'data' | 'onChange'>) {
	const minutes = data.waitMinutes ?? 0;
	const [unit, setUnit] = useState<Unit>(unitOf(minutes));
	const amount = minutes ? minutes / UNITS[unit] : '';
	function set(nextAmount: number, nextUnit: Unit) {
		const total = Math.round(nextAmount * UNITS[nextUnit]);
		onChange({ waitMinutes: Number.isFinite(total) && total > 0 ? total : 0 });
	}
	return (
		<div className="wf-field">
			<label>Chờ bao lâu</label>
			<div className="wf-wait-row">
				<input
					type="number" min={1} step={1} value={amount}
					onChange={(e: ChangeEvent<HTMLInputElement>) => set(Number(e.target.value), unit)}
					aria-label="Số lượng"
				/>
				<Select value={unit} onChange={(e) => { setUnit(e.target.value as Unit); set(Number(amount), e.target.value as Unit); }}>
					<option value="minute">phút</option>
					<option value="hour">giờ</option>
					<option value="day">ngày</option>
				</Select>
			</div>
			<small>Từ 1 phút đến {MAX_WAIT_MINUTES / 1440} ngày. Thời gian chờ được giữ cả khi đóng trang hoặc máy chủ khởi động lại.</small>
		</div>
	);
}

function ImportSettings({ folders, sheet, onSheet }: Pick<Props, 'folders' | 'sheet' | 'onSheet'>) {
	const fileInput = useRef<HTMLInputElement>(null);
	const [reading, setReading] = useState(false);
	const [error, setError] = useState('');

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
			else onSheet({ fileName: file.name, rows: result.rows });
		} catch {
			setError('Không đọc được file. Hãy dùng đúng file Excel .xlsx (nên điền vào file mẫu).');
		} finally {
			setReading(false);
		}
	}

	const ready = sheet?.rows.filter((r) => r.status === 'ready').length ?? 0;
	const bad = sheet?.rows.filter((r) => r.status === 'invalid') ?? [];
	return (
		<div className="wf-field">
			<label>File Excel</label>
			<div
				className="bi-drop wf-drop" role="button" tabIndex={0}
				onDragOver={(e: DragEvent<HTMLDivElement>) => e.preventDefault()}
				onDrop={(e: DragEvent<HTMLDivElement>) => { e.preventDefault(); readFile(e.dataTransfer.files?.[0]); }}
				onClick={() => fileInput.current?.click()}
				onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && fileInput.current?.click()}
			>
				<strong>{reading ? 'Đang đọc file…' : sheet ? 'Chọn file khác' : 'Chọn hoặc kéo thả file'}</strong>
				<span>.xlsx, tối đa {BULK_MAX_ROWS} dòng</span>
				<input ref={fileInput} type="file" accept=".xlsx" hidden onChange={(e) => { readFile(e.target.files?.[0]); e.target.value = ''; }} />
			</div>
			{error && <p className="bi-error" role="alert">{error}</p>}
			{sheet && (
				<p className="bi-summary">
					<b>{sheet.fileName}</b>: <b className="ok">{ready}</b> dòng hợp lệ{bad.length > 0 && <>, <b className="bad">{bad.length}</b> dòng lỗi (sẽ bỏ qua)</>}
				</p>
			)}
			{bad.slice(0, 3).map((r) => <small key={r.line} className="wf-bad-row">Dòng {r.line}: {r.reason}</small>)}
			<small>File chỉ ở trong trình duyệt cho đến khi bấm Bắt đầu; tải lại trang thì phải chọn lại. Chưa có file? <a href="/mau-nhap-koc.xlsx" download>Tải file mẫu</a>.</small>
		</div>
	);
}

/** The settings of the block that is selected on the canvas. */
export default function NodeSettings({ data, campaigns, folders, sheet, onSheet, onChange, onDelete }: Props) {
	const campaign = campaigns.find((c) => c._id === data.campaignId);
	return (
		<div className="wf-settings">
			<h3>{NODE_TITLES[data.kind]}</h3>
			<p className="wf-hint">{NODE_HINTS[data.kind]}</p>

			{data.kind === 'import' && <ImportSettings folders={folders} sheet={sheet} onSheet={onSheet} />}
			{data.kind === 'wait' && <WaitSettings data={data} onChange={onChange} />}
			{data.kind === 'send' && (
				<div className="wf-field">
					<label>Chiến dịch</label>
					<Select value={data.campaignId ?? ''} onChange={(e) => {
						const c = campaigns.find((x) => x._id === e.target.value);
						onChange({ campaignId: e.target.value, campaignName: c?.name });
					}}>
						<option value="">— Chọn chiến dịch —</option>
						{campaigns.map((c) => <option key={c._id} value={c._id}>{c.name}</option>)}
					</Select>
					{campaign && <small>{campaign.steps.length} tin. Nội dung được chụp lại lúc bấm Bắt đầu; sửa chiến dịch sau đó không đổi quy trình đang chạy.</small>}
					<small>Chỉ gửi trong khung giờ 08:30 đến 20:30, theo hạn mức KOC mỗi ngày. Hết hạn mức thì gửi tiếp vào sáng hôm sau.</small>
				</div>
			)}
			{data.kind === 'if_replied' && (
				<div className="wf-field">
					<small>Một KOC đã trả lời kể từ lúc nhận tin thì đi cổng "đã trả lời", ngược lại đi cổng "chưa trả lời". KOC không xác định được thì không đi nhánh nào (để không nhắc nhầm người đã trả lời). Cổng nào không nối thì nhánh đó kết thúc.</small>
				</div>
			)}
			<button type="button" className="btn-row-del wf-delete" onClick={onDelete}>Xóa khối này</button>
		</div>
	);
}
