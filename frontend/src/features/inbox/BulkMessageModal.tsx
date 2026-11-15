import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { BulkRun, Campaign } from '../../types/campaign';
import type { Creator } from '../../types/koc';
import { MSG_RECENT_DAYS, buildRecipients, filterRecipients } from '../kocs/bulkMessage';
import BulkRunTracker from './BulkRunTracker';
import CampaignStepList from './CampaignStepList';
import {
	type DraftStep,
	estimateMinutes,
	formatDuration,
	sameSteps,
	sendBlockReason,
	startedToday,
	stepProblem,
	stepsToSave,
	stepsToSend,
	toDraft,
} from './campaignSteps';
import { useCampaignImages } from './useCampaignImages';

type View = 'compose' | 'track';

interface Props {
	/** every KOC the app has loaded */
	creators: Creator[];
	/** only an admin turns the "Gửi hàng loạt" switch on or off, edits the campaigns and sets the daily limit */
	isAdmin: boolean;
	onClose: () => void;
	/** called when the window closes after a run may have sent messages, so the lists reload */
	onDone: () => void;
	showToast: (msg: string, type?: 'success' | 'error') => void;
}

const LIMITS = [10, 20, 30, 50];
const POLL_MS = 5000;
const SAMPLE_KOC = { name: 'Nguyễn Lan', handle: '@tenkenh' };

/**
 * Send a campaign (a series of texts and pictures) to many KOCs over Zalo. Tab "Soạn tin": pick a campaign, tick, order
 * and edit its messages, tick the KOCs. The "Gửi hàng loạt" switch lives on the server, independent of the chatbot: OFF
 * until an admin turns it on, it stays on while a run goes out and turns itself off when the run ends. Pressing "Gửi"
 * starts a run on the server, which keeps going when this window is closed. Tab "Theo dõi": the live run and the earlier ones.
 */
export default function BulkMessageModal({ creators, isAdmin, onClose, onDone, showToast }: Props) {
	const [view, setView] = useState<View>('compose');
	const [campaigns, setCampaigns] = useState<Campaign[] | null>(null);
	const [campaignFailed, setCampaignFailed] = useState(false);
	const [campaignId, setCampaignId] = useState('');
	const [draft, setDraft] = useState<DraftStep[]>([]);
	const [saving, setSaving] = useState(false);
	const [bulk, setBulk] = useState<{ loaded: boolean; enabled: boolean; busy: boolean }>({ loaded: false, enabled: false, busy: false });
	const [limit, setLimit] = useState(30);
	const [current, setCurrent] = useState<BulkRun | null>(null);
	const [runs, setRuns] = useState<BulkRun[] | null>(null);
	const [runsFailed, setRunsFailed] = useState(false);
	const [stopping, setStopping] = useState(false);
	const [confirming, setConfirming] = useState(false);
	const [sending, setSending] = useState(false);
	const [query, setQuery] = useState('');
	const [ticked, setTicked] = useState<Record<string, boolean>>(() => {
		const map: Record<string, boolean> = {};
		buildRecipients(creators).forEach((r) => (map[r.creator.id] = r.checked));
		return map;
	});
	const bulkOn = useRef(false); // the latest value of the switch, for the cleanup on close
	const running = useRef(false); // is a run going, for the cleanup on close
	const ownSwitch = useRef(false); // this window turned the switch on, so this window turns it off
	const currentKnown = useRef(false); // the server has answered "is a run going" at least once
	const touched = useRef(false); // a run went out while the window was open: the lists reload on close
	const images = useCampaignImages();

	const recipients = useMemo(() => buildRecipients(creators), [creators]);
	const visible = useMemo(() => filterRecipients(recipients, query), [recipients, query]);
	const eligible = recipients.filter((r) => !r.blocked);
	const selected = eligible.filter((r) => ticked[r.creator.id]);
	const campaign = campaigns?.find((c) => c._id === campaignId);
	const sendSteps = useMemo(() => stepsToSend(draft), [draft]);
	const problem = stepProblem(draft);
	const usedToday = startedToday(runs ?? [], new Date());
	const limitLeft = Math.max(limit - usedToday, 0);
	const blockReason = sendBlockReason({ bulkOn: bulk.enabled, isAdmin, selected: selected.length, problem, running: !!current, limitLeft });
	const preview = selected[0]?.creator ?? eligible[0]?.creator;
	const previewKoc = preview ? { name: preview.name, handle: preview.handle } : SAMPLE_KOC;
	const minutes = estimateMinutes(selected.length, sendSteps.length);
	const edited = !!campaign && !sameSteps(stepsToSave(draft), campaign.steps);

	// first load: campaigns, the switch, the daily limit, the run that may be going, and the earlier runs (they count today's KOCs)
	useEffect(() => {
		api.get('/campaigns')
			.then(({ data }) => {
				const list: Campaign[] = data.data || [];
				setCampaigns(list);
				if (list[0]) {
					setCampaignId(list[0]._id);
					setDraft(toDraft(list[0].steps));
				}
			})
			.catch(() => {
				setCampaigns([]);
				setCampaignFailed(true);
			});
		api.get('/bulk-switch')
			.then(({ data }) => {
				bulkOn.current = !!data.data?.enabled;
				setBulk({ loaded: true, enabled: bulkOn.current, busy: false });
			})
			.catch(() => setBulk({ loaded: false, enabled: false, busy: false }));
		api.get('/bulk-limit')
			.then(({ data }) => setLimit(Number(data.data?.value) || 30))
			.catch(() => undefined);
	}, []);

	const loadRuns = useCallback(() => {
		setRunsFailed(false);
		api.get('/bulk-runs', { params: { limit: 100 } })
			.then(({ data }) => setRuns(data.data || []))
			.catch(() => {
				setRuns((r) => r ?? []);
				setRunsFailed(true);
			});
	}, []);
	useEffect(loadRuns, [loadRuns]);

	const loadCurrent = useCallback(() => {
		api.get('/bulk-runs/current')
			.then(({ data }) => {
				currentKnown.current = true;
				setCurrent(data.data?.run ?? null);
			})
			.catch(() => undefined);
	}, []);
	useEffect(loadCurrent, [loadCurrent]);

	// while a run goes out, ask the server how far it is; when it ends, reload what it changed
	useEffect(() => {
		running.current = !!current;
		if (!current) return;
		touched.current = true;
		const timer = setInterval(loadCurrent, POLL_MS);
		return () => clearInterval(timer);
	}, [current?._id, !!current, loadCurrent]); // eslint-disable-line react-hooks/exhaustive-deps
	const wasRunning = useRef(false);
	useEffect(() => {
		if (wasRunning.current && !current) {
			setStopping(false);
			loadRuns();
			api.get('/bulk-switch')
				.then(({ data }) => {
					bulkOn.current = !!data.data?.enabled;
					setBulk((b) => ({ ...b, enabled: bulkOn.current }));
				})
				.catch(() => undefined);
		}
		wasRunning.current = !!current;
	}, [current, loadRuns]);

	// the pictures of the steps being shown
	useEffect(() => {
		draft.forEach((s) => images.load(s.imageId));
	}, [draft]); // eslint-disable-line react-hooks/exhaustive-deps

	async function changeSwitch(next: boolean, quiet = false): Promise<boolean> {
		setBulk((b) => ({ ...b, busy: true }));
		try {
			await api.put('/bulk-switch', { enabled: next, confirm: next });
			bulkOn.current = next;
			ownSwitch.current = next;
			setBulk({ loaded: true, enabled: next, busy: false });
			if (!quiet) showToast(next ? 'Đã BẬT gửi hàng loạt. Công tắc tự tắt khi đợt gửi kết thúc.' : 'Đã tắt gửi hàng loạt.');
			return true;
		} catch (e) {
			setBulk((b) => ({ ...b, busy: false }));
			showToast(apiMessage(e, 'Không đổi được công tắc gửi hàng loạt.'), 'error');
			return false;
		}
	}

	function chooseCampaign(id: string) {
		const next = campaigns?.find((c) => c._id === id);
		if (!next) return;
		if (edited && !window.confirm('Các chỉnh sửa chưa lưu của chiến dịch này sẽ mất. Đổi chiến dịch?')) return;
		setCampaignId(id);
		setDraft(toDraft(next.steps));
	}

	async function saveDefault() {
		if (!campaign || saving) return;
		setSaving(true);
		try {
			const steps = stepsToSave(draft);
			await api.put(`/campaigns/${campaign._id}`, { steps });
			setCampaigns((list) => (list ?? []).map((c) => (c._id === campaign._id ? { ...c, steps } : c)));
			showToast('Đã lưu làm mặc định cho chiến dịch này.');
		} catch (e) {
			showToast(apiMessage(e, 'Không lưu được chiến dịch.'), 'error');
		} finally {
			setSaving(false);
		}
	}

	async function uploadImage(file: File): Promise<string | null> {
		if (file.type !== 'image/png' && file.type !== 'image/jpeg') {
			showToast('Chỉ nhận ảnh PNG hoặc JPG.', 'error');
			return null;
		}
		if (file.size > 2 * 1024 * 1024) {
			showToast('Ảnh lớn quá, tối đa 2 MB.', 'error');
			return null;
		}
		const form = new FormData();
		form.append('file', file);
		try {
			const { data } = await api.post('/campaigns/images', form, { timeout: 60000 });
			return String(data.data?._id || '') || null;
		} catch (e) {
			showToast(apiMessage(e, 'Không tải được ảnh lên.'), 'error');
			return null;
		}
	}

	async function changeLimit(value: number) {
		try {
			await api.put('/bulk-limit', { value });
			setLimit(value);
		} catch (e) {
			showToast(apiMessage(e, 'Không đổi được hạn mức.'), 'error');
		}
	}

	function setVisible(value: boolean) {
		const next = { ...ticked };
		visible.forEach((r) => {
			if (!r.blocked) next[r.creator.id] = value;
		});
		setTicked(next);
	}

	async function start() {
		if (blockReason || sending || !campaign) return;
		setSending(true);
		try {
			const { data } = await api.post(
				'/bulk-runs',
				{ campaignName: campaign.name, steps: sendSteps, kocIds: selected.map((r) => r.creator.id), confirm: true },
				{ timeout: 30000 },
			);
			setCurrent(data.data?.run ?? null);
			setConfirming(false);
			setView('track');
			showToast(`Đã bắt đầu gửi cho ${selected.length} KOC. Có thể đóng cửa sổ, máy chủ vẫn tiếp tục gửi.`);
		} catch (e) {
			setConfirming(false);
			showToast(apiMessage(e, 'Không bắt đầu được đợt gửi.'), 'error');
		} finally {
			setSending(false);
		}
	}

	async function stopRun() {
		if (!window.confirm('Dừng đợt gửi? KOC đang nhận dở sẽ dừng sau tin hiện tại, các KOC chưa tới lượt sẽ không được gửi.')) return;
		setStopping(true);
		if (!(await changeSwitch(false))) setStopping(false);
	}

	function close() {
		// A run that is going (or may be: not asked yet, or being started) needs the switch on: leave it. The switch is turned
		// off here only when this window turned it on and the server has said that nothing is running.
		if (isAdmin && bulkOn.current && ownSwitch.current && currentKnown.current && !running.current && !sending) void changeSwitch(false, true);
		if (touched.current || sending) onDone();
		onClose();
	}

	const subtitle =
		view === 'compose'
			? 'Chọn chiến dịch, tích những tin cần gửi, sắp xếp và sửa nội dung, rồi chọn KOC. {ten} và {kenh} được thay theo từng KOC.'
			: 'Tiến độ đợt đang gửi và kết quả các đợt trước, từng KOC một.';

	return (
		<div className="modal-backdrop">
			<div className="tiktoday-modal bm-modal" role="dialog" aria-modal="true" aria-labelledby="bm-title">
				<div className="tm-header">
					<div>
						<h2 id="bm-title">Gửi tin hàng loạt</h2>
						<p>{subtitle}</p>
					</div>
					<button type="button" className="tm-close" onClick={close} aria-label="Đóng">
						×
					</button>
				</div>

				<div className="bm-tabs" role="tablist">
					<button type="button" role="tab" aria-selected={view === 'compose'} className={view === 'compose' ? 'on' : ''} onClick={() => setView('compose')}>
						Soạn tin
					</button>
					<button type="button" role="tab" aria-selected={view === 'track'} className={view === 'track' ? 'on' : ''} onClick={() => setView('track')}>
						Theo dõi{current ? <span className="bm-dot" aria-label="đang gửi" /> : null}
					</button>
				</div>

				<div className="tm-body bm-body">
					{view === 'compose' && (
						<>
							<div className="bm-grid">
								<section className="bm-col">
									<div className="bm-h-row">
										<label className="bm-h" htmlFor="bm-campaign">
											Chiến dịch
										</label>
										<span className="bm-count">
											{sendSteps.length}/{draft.length} tin được gửi
										</span>
									</div>
									{campaigns === null && <p className="bm-empty">Đang tải…</p>}
									{campaignFailed && <p className="bm-alert">Không tải được các chiến dịch. Hãy đóng và mở lại cửa sổ.</p>}
									{campaigns && campaigns.length > 0 && (
										<select id="bm-campaign" className="bm-select" value={campaignId} onChange={(e) => chooseCampaign(e.target.value)}>
											{campaigns.map((c) => (
												<option key={c._id} value={c._id}>
													{c.name}
												</option>
											))}
										</select>
									)}
									{campaign && <CampaignStepList steps={draft} onChange={setDraft} isAdmin={isAdmin} koc={previewKoc} imageUrl={images.urlOf} onUpload={uploadImage} />}
									<div className="bm-meta">
										<span>Sửa ở đây chỉ áp dụng cho lần gửi này</span>
										{isAdmin && (
											<button type="button" className="bm-link" disabled={!edited || saving || !!problem} onClick={saveDefault} title={problem ?? undefined}>
												{saving ? 'Đang lưu…' : 'Lưu làm mặc định'}
											</button>
										)}
									</div>
								</section>

								<section className="bm-col">
									<div className="bm-h-row">
										<h3 className="bm-h">Người nhận</h3>
										<span className="bm-count">
											{selected.length}/{eligible.length} đã chọn
										</span>
									</div>
									<input className="bm-search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Tìm theo tên, kênh, số điện thoại…" aria-label="Tìm KOC" />
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
												{(r.blocked || r.recentDaysAgo !== undefined) && <em className={`bm-note${r.blocked ? ' bad' : ''}`}>{r.blocked ?? `${r.recentDaysAgo} ngày trước`}</em>}
											</label>
										))}
									</div>
								</section>
							</div>

							<div className="bm-summary">
								<span>
									<b>{selected.length}</b> KOC · <b>{sendSteps.length}</b> tin mỗi KOC · khoảng <b>{formatDuration(minutes)}</b>
								</span>
								<span className="bm-limit">
									Hôm nay còn <b>{limitLeft}</b>/{limit} KOC
									{isAdmin && (
										<select aria-label="Hạn mức KOC mỗi ngày" value={limit} onChange={(e) => void changeLimit(Number(e.target.value))}>
											{LIMITS.map((n) => (
												<option key={n} value={n}>
													{n}/ngày
												</option>
											))}
										</select>
									)}
								</span>
							</div>
						</>
					)}

					{view === 'track' && <BulkRunTracker current={current} runs={runs} loadFailed={runsFailed} isAdmin={isAdmin} stopping={stopping} onStop={() => void stopRun()} />}
				</div>

				<div className="tm-footer bm-footer">
					{view === 'compose' && !confirming && (
						<>
							<label
								className={`ti-switch bm-arm${bulk.enabled ? ' on' : ''}${!isAdmin || !bulk.loaded || bulk.busy || !!current ? ' disabled' : ''}`}
								title={isAdmin ? 'Bật để gửi hàng loạt. Công tắc này riêng, không liên quan chatbot. Tự tắt khi đợt gửi kết thúc.' : 'Chỉ quản trị viên bật được'}
							>
								<input type="checkbox" checked={bulk.enabled} disabled={!isAdmin || !bulk.loaded || bulk.busy || !!current} onChange={(e) => void changeSwitch(e.target.checked)} />
								<span className="ti-track">
									<i />
								</span>
								<b>Gửi hàng loạt: {bulk.enabled ? 'Bật' : 'Tắt'}</b>
							</label>
							<span className="bm-why">{blockReason}</span>
							<button type="button" className="tm-btn-cancel" onClick={close}>
								Đóng
							</button>
							<button type="button" className="tm-btn-submit" disabled={!!blockReason} onClick={() => setConfirming(true)}>
								Gửi cho {selected.length} KOC
							</button>
						</>
					)}
					{view === 'compose' && confirming && (
						<>
							<span className="bm-confirm">
								Gửi <b>{sendSteps.length} tin</b> cho <b>{selected.length} KOC</b>, khoảng <b>{formatDuration(minutes)}</b>. Máy chủ gửi chậm từng KOC; đóng cửa sổ không dừng đợt. Muốn dừng, tắt công tắc.
							</span>
							<button type="button" className="tm-btn-cancel" disabled={sending} onClick={() => setConfirming(false)}>
								Quay lại
							</button>
							<button type="button" className="tm-btn-submit" disabled={sending} onClick={() => void start()}>
								{sending ? 'Đang bắt đầu…' : 'Xác nhận gửi'}
							</button>
						</>
					)}
					{view === 'track' && (
						<>
							<span className="bm-why" />
							<button type="button" className="tm-btn-cancel" onClick={() => { loadRuns(); loadCurrent(); }}>
								Làm mới
							</button>
							<button type="button" className="tm-btn-submit" onClick={close}>
								Đóng
							</button>
						</>
					)}
				</div>
			</div>
		</div>
	);
}
