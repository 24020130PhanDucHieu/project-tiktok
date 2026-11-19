import { type ChangeEvent, type DragEvent, useRef, useState } from 'react';
import Select from '../../components/Select';
import type { WorkflowNodeType } from '../../types/workflow';
import { BULK_MAX_ROWS, parseBulkSheet, withTimeout } from '../kocs/bulkImport';
import { BLOCK_BY_TYPE, STAGE_LABELS, STAGE_TARGETS, formatClock } from './blocks';
import type { NodeData } from './convert';
import { KOC_STAGE_VALUES, MAX_PICK, MAX_PICK_STAGES, MAX_SUBJECT, MAX_TEXT, MAX_WAIT_MINUTES } from './graph';
import KocChooser from './KocChooser';
import NodeIcon from './NodeIcon';
import { useWorkflowUi } from './WorkflowContext';

const MAX_BYTES = 2 * 1024 * 1024;
const PICK_SOURCES = ['TikToday', 'TikTok Liên kết', 'TikTok'];
const UNITS = { minute: 1, hour: 60, day: 1440 } as const;
type Unit = keyof typeof UNITS;

function unitOf(minutes: number): Unit {
	if (minutes > 0 && minutes % 1440 === 0) return 'day';
	if (minutes > 0 && minutes % 60 === 0) return 'hour';
	return 'minute';
}

interface FormProps {
	data: NodeData;
	onChange: (patch: Partial<NodeData>) => void;
}

function Safety({ children }: { children: string }) {
	return <p className="wfp-safety">{children}</p>;
}

function WaitForm({ data, onChange }: FormProps) {
	const minutes = data.waitMinutes ?? 0;
	const [unit, setUnit] = useState<Unit>(unitOf(minutes));
	const amount = minutes ? minutes / UNITS[unit] : '';
	function set(nextAmount: number, nextUnit: Unit) {
		const total = Math.round(nextAmount * UNITS[nextUnit]);
		onChange({ waitMinutes: Number.isFinite(total) && total > 0 ? total : 0 });
	}
	return (
		<div className="wfp-field">
			<label>Chờ bao lâu</label>
			<div className="wfp-row">
				<input type="number" min={1} step={1} value={amount} aria-label="Số lượng" onChange={(e: ChangeEvent<HTMLInputElement>) => set(Number(e.target.value), unit)} />
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

function WaitUntilForm({ data, onChange }: FormProps) {
	const at = data.atMinute;
	return (
		<div className="wfp-field">
			<label>Chờ đến lúc</label>
			<input
				type="time" value={Number.isInteger(at) ? formatClock(at as number) : ''} aria-label="Giờ"
				onChange={(e) => {
					const [h, m] = e.target.value.split(':').map(Number);
					if (Number.isInteger(h) && Number.isInteger(m)) onChange({ atMinute: h * 60 + m });
				}}
			/>
			<small>Giờ Việt Nam. Hôm nay nếu giờ đó chưa tới, ngược lại là ngày mai. Giữ cả khi máy chủ khởi động lại.</small>
		</div>
	);
}

function ImportForm() {
	const ui = useWorkflowUi();
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
			const result = parseBulkSheet(await withTimeout(readSheet(file), 20000), ui.folders);
			if (result.fatal) setError(result.fatal);
			else if (result.tooMany) setError(`File có hơn ${BULK_MAX_ROWS} dòng. Hãy chia thành nhiều file, mỗi file tối đa ${BULK_MAX_ROWS} dòng.`);
			else ui.setSheet({ fileName: file.name, rows: result.rows });
		} catch {
			setError('Không đọc được file. Hãy dùng đúng file Excel .xlsx (nên điền vào file mẫu).');
		} finally {
			setReading(false);
		}
	}

	const ready = ui.sheet?.rows.filter((r) => r.status === 'ready').length ?? 0;
	const bad = ui.sheet?.rows.filter((r) => r.status === 'invalid') ?? [];
	return (
		<div className="wfp-field">
			<label>File Excel</label>
			<div
				className="bi-drop wfp-drop" role="button" tabIndex={0}
				onDragOver={(e: DragEvent<HTMLDivElement>) => e.preventDefault()}
				onDrop={(e: DragEvent<HTMLDivElement>) => { e.preventDefault(); readFile(e.dataTransfer.files?.[0]); }}
				onClick={() => fileInput.current?.click()}
				onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && fileInput.current?.click()}
			>
				<strong>{reading ? 'Đang đọc file…' : ui.sheet ? 'Chọn file khác' : 'Chọn hoặc kéo thả file'}</strong>
				<span>.xlsx, tối đa {BULK_MAX_ROWS} dòng</span>
				<input ref={fileInput} type="file" accept=".xlsx" hidden onChange={(e) => { readFile(e.target.files?.[0]); e.target.value = ''; }} />
			</div>
			{error && <p className="bi-error" role="alert">{error}</p>}
			{ui.sheet && (
				<p className="bi-summary">
					<b>{ui.sheet.fileName}</b>: <b className="ok">{ready}</b> dòng hợp lệ{bad.length > 0 && <>, <b className="bad">{bad.length}</b> dòng lỗi (sẽ bỏ qua)</>}
				</p>
			)}
			{bad.slice(0, 3).map((r) => <small key={r.line} className="wfp-bad">Dòng {r.line}: {r.reason}</small>)}
			<small>File chỉ ở trong trình duyệt cho đến khi bấm Bắt đầu; tải lại trang thì phải chọn lại. Chưa có file? <a href="/mau-nhap-koc.xlsx" download>Tải file mẫu</a>.</small>
		</div>
	);
}

function PickForm({ data, onChange }: FormProps) {
	const ui = useWorkflowUi();
	const stages = data.stages ?? [];
	const toggle = (s: string) => {
		const next = stages.includes(s) ? stages.filter((x) => x !== s) : [...stages, s];
		if (next.length <= MAX_PICK_STAGES) onChange({ stages: next });
	};
	const chosen = data.kocIds ?? [];
	return (
		<>
			<div className="wfp-field">
				<label>Chọn từng KOC theo tên <small>(tùy chọn)</small></label>
				<KocChooser ids={chosen} names={data.kocNames ?? {}} onChange={(kocIds, kocNames) => onChange({ kocIds, kocNames })} />
				{chosen.length > 0 && <small>Đã chọn {chosen.length} KOC: quy trình chỉ lấy đúng những KOC này, các bộ lọc bên dưới không dùng.</small>}
			</div>
			<div className={chosen.length > 0 ? 'wfp-dim' : undefined} aria-disabled={chosen.length > 0}>
			<div className="wfp-field">
				<label>Giai đoạn <small>(chọn tối đa {MAX_PICK_STAGES})</small></label>
				<div className="wfp-chips">
					{KOC_STAGE_VALUES.map((s) => (
						<button key={s} type="button" className={`wfp-chip${stages.includes(s) ? ' on' : ''}`} onClick={() => toggle(s)} aria-pressed={stages.includes(s)}>{STAGE_LABELS[s]}</button>
					))}
				</div>
			</div>
			<div className="wfp-field">
				<label>Thư mục</label>
				<Select value={data.folder ?? ''} onChange={(e) => onChange({ folder: e.target.value })}>
					<option value="">Tất cả thư mục</option>
					{ui.folders.map((f) => <option key={f} value={f}>{f}</option>)}
				</Select>
			</div>
			<div className="wfp-field">
				<label>Nguồn</label>
				<Select value={data.source ?? ''} onChange={(e) => onChange({ source: e.target.value })}>
					<option value="">Tất cả nguồn</option>
					{PICK_SOURCES.map((s) => <option key={s} value={s}>{s}</option>)}
				</Select>
			</div>
			<div className="wfp-field">
				<label>Lấy tối đa bao nhiêu KOC</label>
				<input type="number" min={1} max={MAX_PICK} value={data.limit ?? ''} onChange={(e) => onChange({ limit: Number(e.target.value) || 0 })} aria-label="Số KOC tối đa" />
				<small>Danh sách được lấy <b>một lần</b> lúc bấm Bắt đầu và không đổi nữa. KOC không được liên hệ hoặc đã hợp tác sẽ bị loại. Bấm <b>Xem trước</b> ở thanh dưới để biết trước sẽ lấy những KOC nào.</small>
			</div>
			</div>
		</>
	);
}

function CampaignForm({ data, onChange }: FormProps) {
	const ui = useWorkflowUi();
	const campaign = ui.campaigns.find((c) => c._id === data.campaignId);
	return (
		<div className="wfp-field">
			<label>Chiến dịch</label>
			<Select value={data.campaignId ?? ''} onChange={(e) => onChange({ campaignId: e.target.value })}>
				<option value="">— Chọn chiến dịch —</option>
				{ui.campaigns.map((c) => <option key={c._id} value={c._id}>{c.name}</option>)}
			</Select>
			{campaign && <small>{campaign.steps.length} tin. Nội dung được chụp lại lúc bấm Bắt đầu; sửa chiến dịch sau đó không đổi quy trình đang chạy.</small>}
			<Safety>Chỉ gửi trong khung giờ 08:30 đến 20:30, theo hạn mức KOC mỗi ngày (hết hạn mức thì gửi tiếp vào sáng hôm sau). KOC đã trả lời kể từ lần gửi gần nhất sẽ không nhận tin.</Safety>
		</div>
	);
}

function TextBox({ value, max, placeholder, onChange }: { value: string; max: number; placeholder: string; onChange: (v: string) => void }) {
	const length = Array.from(value).length;
	return (
		<>
			<textarea rows={6} value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} maxLength={max * 2} aria-label="Nội dung" />
			<small className={length > max ? 'wfp-bad' : ''}>{length}/{max} ký tự. Dùng <code>{'{ten}'}</code> cho tên KOC và <code>{'{kenh}'}</code> cho kênh TikTok của họ.</small>
		</>
	);
}

function SendTextForm({ data, onChange }: FormProps) {
	return (
		<div className="wfp-field">
			<label>Nội dung tin Zalo</label>
			<TextBox value={data.text ?? ''} max={MAX_TEXT} placeholder="Chào {ten}, mình là ... ạ" onChange={(text) => onChange({ text })} />
			<Safety>Gửi qua đúng đường của chiến dịch: chỉ trong khung giờ 08:30 đến 20:30, theo hạn mức KOC mỗi ngày, cách nhau vài phút. KOC đã trả lời kể từ lần gửi gần nhất sẽ không nhận tin.</Safety>
		</div>
	);
}

function EmailForm({ data, onChange }: FormProps) {
	return (
		<>
			<div className="wfp-field">
				<label>Chủ đề <small>(bỏ trống: dùng chủ đề mặc định)</small></label>
				<input value={data.subject ?? ''} maxLength={MAX_SUBJECT} onChange={(e) => onChange({ subject: e.target.value })} aria-label="Chủ đề email" placeholder="Cơ hội hợp tác TikTok dành cho {ten}" />
			</div>
			<div className="wfp-field">
				<label>Nội dung email</label>
				<TextBox value={data.text ?? ''} max={MAX_TEXT} placeholder="Chào {ten}, ..." onChange={(text) => onChange({ text })} />
				<Safety>Mỗi email cách nhau 3 đến 8 giây, chỉ trong khung giờ 08:30 đến 20:30, tối đa 100 email mỗi khối. KOC không có email, đã được liên hệ trong 24 giờ qua, hoặc đã trả lời thì được bỏ qua.</Safety>
			</div>
		</>
	);
}

function LabelForm({ data, onChange }: FormProps) {
	const ui = useWorkflowUi();
	const label = ui.labels.find((l) => l._id === data.labelId);
	return (
		<div className="wfp-field">
			<label>Nhãn</label>
			{ui.labels.length === 0 ? (
				<small className="wfp-bad">Chưa có nhãn nào. Hãy tạo nhãn ở Hộp thư rồi quay lại.</small>
			) : (
				<Select value={data.labelId ?? ''} onChange={(e) => onChange({ labelId: e.target.value })}>
					<option value="">— Chọn nhãn —</option>
					{ui.labels.map((l) => <option key={l._id} value={l._id}>{l.name}</option>)}
				</Select>
			)}
			{label && <small><i className="wfp-swatch" style={{ background: label.color }} /> Nhãn này được thêm vào hội thoại của từng KOC. KOC chưa có hội thoại thì được bỏ qua.</small>}
		</div>
	);
}

function StageForm({ data, onChange }: FormProps) {
	return (
		<div className="wfp-field">
			<label>Chuyển KOC sang giai đoạn</label>
			<Select value={data.stage ?? ''} onChange={(e) => onChange({ stage: e.target.value })}>
				<option value="">— Chọn giai đoạn —</option>
				{STAGE_TARGETS.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
			</Select>
			<small>Chỉ các giai đoạn không cần lý do và không kéo theo việc khác. KOC đang bị chặn liên hệ được bỏ qua.</small>
		</div>
	);
}

function FormFor({ type, data, onChange }: { type: WorkflowNodeType } & FormProps) {
	switch (type) {
		case 'import': return <ImportForm />;
		case 'pick': return <PickForm data={data} onChange={onChange} />;
		case 'wait': return <WaitForm data={data} onChange={onChange} />;
		case 'wait_until': return <WaitUntilForm data={data} onChange={onChange} />;
		case 'send': return <CampaignForm data={data} onChange={onChange} />;
		case 'send_text': return <SendTextForm data={data} onChange={onChange} />;
		case 'email': return <EmailForm data={data} onChange={onChange} />;
		case 'label': return <LabelForm data={data} onChange={onChange} />;
		case 'stage': return <StageForm data={data} onChange={onChange} />;
		default:
			return <div className="wfp-field"><small>Một KOC đã trả lời kể từ lúc nhận tin thì đi cổng "đã trả lời", ngược lại đi cổng "chưa trả lời". KOC không xác định được thì không đi nhánh nào (để không nhắc nhầm người đã trả lời). Cổng nào không nối thì nhánh đó kết thúc.</small></div>;
	}
}

/** The settings of a block, in a panel next to it: what comes in, what goes out, and the form of its type. */
export default function NodeSettingsPanel({ id, data }: { id: string; data: NodeData }) {
	const ui = useWorkflowUi();
	const meta = BLOCK_BY_TYPE[data.kind];
	const isStart = data.kind === 'import' || data.kind === 'pick';
	return (
		<div className="wfp nodrag nopan nowheel" role="dialog" aria-label={`Cài đặt khối ${meta.title}`} onKeyDown={(e) => e.key === 'Escape' && ui.closePanel()}>
			<header className="wfp-head" style={{ '--accent': meta.color } as React.CSSProperties}>
				<span className="wfp-icon"><NodeIcon kind={data.kind} size={20} /></span>
				<strong>{meta.title}</strong>
				<button type="button" className="wfp-close" onClick={ui.closePanel} aria-label="Đóng">×</button>
			</header>
			<p className="wfp-hint">{meta.hint}</p>
			<div className="wfp-io">
				<div><span>VÀO</span><p>{meta.input}</p></div>
				<div><span>RA</span><p>{meta.output}</p></div>
			</div>
			<div className="wfp-form">
				<FormFor type={data.kind} data={data} onChange={(patch) => ui.update(id, patch)} />
			</div>
		</div>
	);
}
