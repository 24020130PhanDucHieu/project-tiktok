import type { BulkRun, RunTarget } from '../../types/campaign';
import { reasonText, runView, statusLabel, stopText } from './campaignSteps';

interface Props {
	/** the run that is running now, if any */
	current: BulkRun | null;
	/** the latest runs, newest first; null while loading */
	runs: BulkRun[] | null;
	loadFailed: boolean;
	isAdmin: boolean;
	stopping: boolean;
	onStop: () => void;
}

const when = (iso: string) => new Date(iso).toLocaleString('vi-VN', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });

/** KOCs that need a person to look come first: unknown, failed, replied, partial. */
const ATTENTION = ['unknown', 'failed', 'replied', 'partial'];
const byAttention = (a: RunTarget, b: RunTarget) => Number(ATTENTION.includes(b.status)) - Number(ATTENTION.includes(a.status));

const runLabel = (run: BulkRun) => {
	if (run.status === 'running') return 'Đang chạy';
	if (run.status === 'done') return 'Đã xong';
	const why = stopText(run.stopReason);
	return why ? `Đã dừng · ${why}` : 'Đã dừng';
};

function Counts({ counts }: { counts: Record<string, number> }) {
	return (
		<span className="bm-run-counts">
			{Object.entries(counts).map(([status, n]) => (
				<span key={status} className={`bm-pill ${status}`}>
					{statusLabel(status)} {n}
				</span>
			))}
		</span>
	);
}

/** The "Theo dõi" tab: the run that is going out now, live, and what happened to every KOC of the earlier runs. */
export default function BulkRunTracker({ current, runs, loadFailed, isAdmin, stopping, onStop }: Props) {
	const now = new Date();
	const view = current ? runView(current, now) : null;
	const history = (runs ?? []).filter((r) => r._id !== current?._id);

	return (
		<div className="bm-track">
			{current && view && (
				<section className="bm-live" aria-live="polite">
					<div className="bm-live-head">
						<div>
							<strong>Đang gửi: {current.campaignName}</strong>
							<small>
								Bắt đầu {when(current.createdAt)} · {current.by}
							</small>
						</div>
						{isAdmin && (
							<button type="button" className="tm-btn-cancel bm-stop" disabled={stopping} onClick={onStop}>
								{stopping ? 'Đang dừng…' : 'Dừng đợt'}
							</button>
						)}
					</div>
					<div className="bi-bar" role="progressbar" aria-valuemin={0} aria-valuemax={view.total} aria-valuenow={view.done}>
						<i style={{ width: `${view.total ? (view.done / view.total) * 100 : 0}%` }} />
					</div>
					<p className="bm-live-line">
						<b>
							{view.done}/{view.total}
						</b>{' '}
						KOC đã xong
						{view.current && (
							<>
								{' '}
								· đang gửi cho <b>{view.current.name}</b>, tin {view.current.step}/{view.current.steps}
							</>
						)}
						{!view.current && view.nextInMinutes !== undefined && <> · KOC kế tiếp sau khoảng {view.nextInMinutes} phút</>}
					</p>
					<Counts counts={view.counts} />
					<p className="bm-hint">Đóng cửa sổ không dừng đợt gửi. Muốn dừng, bấm "Dừng đợt" hoặc tắt công tắc.</p>
				</section>
			)}

			{runs === null && !loadFailed && <p className="bm-empty">Đang tải…</p>}
			{loadFailed && <p className="bm-alert">Không tải được lịch sử gửi. Bấm Làm mới để thử lại.</p>}
			{runs !== null && !current && history.length === 0 && !loadFailed && <p className="bm-empty">Chưa có đợt gửi nào.</p>}

			{history.map((run) => {
				const v = runView(run, now);
				return (
					<details key={run._id} className="bm-run">
						<summary>
							<span className="bm-run-when">{when(run.createdAt)}</span>
							<span className="bm-run-by">{run.by || '—'}</span>
							<span className="bm-run-text">
								{run.campaignName} · {run.targets.length} KOC · {run.steps.length} tin
							</span>
							<span className={`bm-pill run-${run.status}`}>{runLabel(run)}</span>
						</summary>
						<div className="bm-run-rows">
							<Counts counts={v.counts} />
							{[...run.targets].sort(byAttention).map((t) => (
								<div key={t.kocId} className="bm-run-row">
									<span className="bm-who">
										<strong>{t.kocName || t.kocId}</strong>
										<small>{t.handle}</small>
									</span>
									<span className="bm-run-steps">
										{t.stepsSent}/{run.steps.length} tin
									</span>
									<span className={`bm-pill ${t.status}`}>{statusLabel(t.status)}</span>
									<em>{reasonText(t.reason)}</em>
								</div>
							))}
						</div>
					</details>
				);
			})}
		</div>
	);
}

