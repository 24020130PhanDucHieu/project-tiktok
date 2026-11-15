import { useRef, useState } from 'react';
import type { StepKind } from '../../types/campaign';
import { type DraftStep, MAX_STEPS, MAX_STEP_TEXT, addStep, editStep, moveStep, previewLine, removeStep, toggleStep } from './campaignSteps';

interface Props {
	steps: DraftStep[];
	onChange: (next: DraftStep[]) => void;
	/** only an admin adds or removes steps and uploads pictures; everybody ticks, orders and edits for this send */
	isAdmin: boolean;
	/** the KOC the previews are written for */
	koc: { name: string; handle: string };
	imageUrl: (id?: string) => string | undefined;
	/** uploads a picture and returns its id, or null when it was refused (the window has told the user why) */
	onUpload: (file: File) => Promise<string | null>;
}

const icon = { width: 15, height: 15, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true } as const;

const Grip = () => (
	<svg {...icon} strokeWidth={2.4}>
		<circle cx="9" cy="6" r="0.6" />
		<circle cx="15" cy="6" r="0.6" />
		<circle cx="9" cy="12" r="0.6" />
		<circle cx="15" cy="12" r="0.6" />
		<circle cx="9" cy="18" r="0.6" />
		<circle cx="15" cy="18" r="0.6" />
	</svg>
);
const Pencil = () => (
	<svg {...icon}>
		<path d="M12 20h9" />
		<path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
	</svg>
);
const Up = () => (
	<svg {...icon}>
		<path d="m18 15-6-6-6 6" />
	</svg>
);
const Down = () => (
	<svg {...icon}>
		<path d="m6 9 6 6 6-6" />
	</svg>
);
const Trash = () => (
	<svg {...icon}>
		<path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14" />
	</svg>
);
const ImageIcon = () => (
	<svg {...icon}>
		<rect x="3" y="4" width="18" height="16" rx="2" />
		<circle cx="9" cy="10" r="1.6" />
		<path d="m21 16-5-5-9 9" />
	</svg>
);

/**
 * The steps of the campaign as an editable list: tick the ones to send, drag (or use the arrows) to reorder, and open one
 * to edit it. Nothing here is saved to the campaign; the window sends what the list shows.
 */
export default function CampaignStepList({ steps, onChange, isAdmin, koc, imageUrl, onUpload }: Props) {
	const [openKey, setOpenKey] = useState<string | null>(null);
	const [dragKey, setDragKey] = useState<string | null>(null);
	const [overKey, setOverKey] = useState<string | null>(null);
	const area = useRef<HTMLTextAreaElement>(null);
	const file = useRef<HTMLInputElement>(null);
	const uploadFor = useRef<string | null>(null);

	function insertTag(key: string, tag: string) {
		const el = area.current;
		const step = steps.find((s) => s.key === key);
		if (!el || !step) return;
		const text = step.text ?? '';
		const from = el.selectionStart ?? text.length;
		const to = el.selectionEnd ?? from;
		const next = text.slice(0, from) + tag + text.slice(to);
		if (next.length > MAX_STEP_TEXT) return;
		onChange(editStep(steps, key, { text: next }));
		requestAnimationFrame(() => {
			el.focus();
			el.setSelectionRange(from + tag.length, from + tag.length);
		});
	}

	async function picked(files: FileList | null) {
		const key = uploadFor.current;
		const f = files?.[0];
		if (file.current) file.current.value = '';
		if (!key || !f) return;
		const id = await onUpload(f);
		if (!id) return;
		// a picture that has just been given an image is ticked: the person chose it to send it
		onChange(editStep(steps, key, { imageId: id }).map((s) => (s.key === key ? { ...s, on: true } : s)));
	}

	function add(kind: StepKind) {
		const next = addStep(steps, kind);
		onChange(next);
		setOpenKey(next[next.length - 1].key);
	}

	function drop(to: number) {
		const from = steps.findIndex((s) => s.key === dragKey);
		setDragKey(null);
		setOverKey(null);
		if (from >= 0) onChange(moveStep(steps, from, to));
	}

	return (
		<div className="bm-steps">
			<ol className="bm-step-list">
				{steps.map((s, i) => {
					const open = openKey === s.key;
					const noImage = s.kind === 'image' && !s.imageId;
					const url = imageUrl(s.imageId);
					return (
						<li
							key={s.key}
							className={`bm-step${s.on ? '' : ' off'}${open ? ' open' : ''}${overKey === s.key && dragKey !== s.key ? ' over' : ''}`}
							onDragOver={(e) => {
								if (!dragKey) return;
								e.preventDefault();
								setOverKey(s.key);
							}}
							onDrop={(e) => {
								e.preventDefault();
								drop(i);
							}}
						>
							<div className="bm-step-row">
								<span
									className="bm-grip"
									draggable
									title="Kéo để đổi thứ tự"
									onDragStart={(e) => {
										setDragKey(s.key);
										e.dataTransfer.effectAllowed = 'move';
										e.dataTransfer.setData('text/plain', s.key);
										const li = e.currentTarget.closest('li');
										if (li) e.dataTransfer.setDragImage(li, 16, 16);
									}}
									onDragEnd={() => {
										setDragKey(null);
										setOverKey(null);
									}}
								>
									<Grip />
								</span>
								<input
									type="checkbox"
									className="bm-check"
									checked={s.on}
									disabled={noImage}
									aria-label={`Gửi tin số ${i + 1}`}
									title={noImage ? 'Tin ảnh này chưa có ảnh' : undefined}
									onChange={() => onChange(toggleStep(steps, s.key))}
								/>
								<span className="bm-step-no">{i + 1}</span>
								{s.kind === 'image' && (
									<span className="bm-thumb" aria-hidden="true">
										{url ? <img src={url} alt="" /> : <ImageIcon />}
									</span>
								)}
								<button type="button" className="bm-step-text" onClick={() => setOpenKey(open ? null : s.key)} title="Bấm để sửa">
									{noImage ? <em>Chưa có ảnh</em> : previewLine(s, koc) || <em>Tin trống</em>}
								</button>
								<span className="bm-step-actions">
									<button type="button" className="bm-icon-btn" aria-label={`Sửa tin số ${i + 1}`} aria-expanded={open} onClick={() => setOpenKey(open ? null : s.key)}>
										<Pencil />
									</button>
									<button type="button" className="bm-icon-btn" aria-label={`Đưa tin số ${i + 1} lên`} disabled={i === 0} onClick={() => onChange(moveStep(steps, i, i - 1))}>
										<Up />
									</button>
									<button type="button" className="bm-icon-btn" aria-label={`Đưa tin số ${i + 1} xuống`} disabled={i === steps.length - 1} onClick={() => onChange(moveStep(steps, i, i + 1))}>
										<Down />
									</button>
									{isAdmin && (
										<button type="button" className="bm-icon-btn danger" aria-label={`Xóa tin số ${i + 1}`} onClick={() => onChange(removeStep(steps, s.key))}>
											<Trash />
										</button>
									)}
								</span>
							</div>

							{open && s.kind === 'text' && (
								<div className="bm-step-edit">
									<textarea
										ref={area}
										className="bm-text"
										rows={5}
										autoFocus
										value={s.text ?? ''}
										maxLength={MAX_STEP_TEXT}
										placeholder="Nội dung tin. Dùng {ten} để chèn tên KOC, {kenh} để chèn tên kênh."
										onChange={(e) => onChange(editStep(steps, s.key, { text: e.target.value }))}
									/>
									<div className="bm-edit-bar">
										<span className="bm-tags">
											Chèn
											<button type="button" onClick={() => insertTag(s.key, '{ten}')}>
												{'{ten}'}
											</button>
											<button type="button" onClick={() => insertTag(s.key, '{kenh}')}>
												{'{kenh}'}
											</button>
										</span>
										<span>
											{(s.text ?? '').length}/{MAX_STEP_TEXT}
										</span>
									</div>
								</div>
							)}

							{open && s.kind === 'image' && (
								<div className="bm-step-edit">
									<div className="bm-image-edit">
										<span className="bm-image-big">{url ? <img src={url} alt="Ảnh của tin" /> : <ImageIcon />}</span>
										<div>
											<label className="bm-edit-label" htmlFor={`cap-${s.key}`}>
												Chú thích (có thể để trống)
											</label>
											<input
												id={`cap-${s.key}`}
												className="bm-search"
												value={s.caption ?? ''}
												maxLength={MAX_STEP_TEXT}
												placeholder="Dùng {ten}, {kenh} nếu cần"
												onChange={(e) => onChange(editStep(steps, s.key, { caption: e.target.value }))}
											/>
											{isAdmin ? (
												<button
													type="button"
													className="bm-link"
													onClick={() => {
														uploadFor.current = s.key;
														file.current?.click();
													}}
												>
													{s.imageId ? 'Đổi ảnh' : 'Chọn ảnh'} (PNG hoặc JPG, tối đa 2 MB)
												</button>
											) : (
												!s.imageId && <span className="bm-hint">Chưa có ảnh. Nhờ quản trị viên tải ảnh lên.</span>
											)}
										</div>
									</div>
								</div>
							)}
						</li>
					);
				})}
			</ol>

			<input ref={file} type="file" accept="image/png,image/jpeg" hidden onChange={(e) => void picked(e.target.files)} />

			{isAdmin && (
				<div className="bm-add">
					<button type="button" className="bm-link" disabled={steps.length >= MAX_STEPS} onClick={() => add('text')}>
						+ Thêm tin chữ
					</button>
					<button type="button" className="bm-link" disabled={steps.length >= MAX_STEPS} onClick={() => add('image')}>
						+ Thêm ảnh
					</button>
					{steps.length >= MAX_STEPS && <span className="bm-hint">Tối đa {MAX_STEPS} tin</span>}
				</div>
			)}
		</div>
	);
}
