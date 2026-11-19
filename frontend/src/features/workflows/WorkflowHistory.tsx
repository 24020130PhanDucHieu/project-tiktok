import { useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { Workflow } from '../../types/workflow';
import { durationText, finishText } from './runText';

const STATUS_TEXT = { running: 'Đang chạy', done: 'Đã xong', stopped: 'Đã dừng' } as const;

interface Props {
	/** shows the report of one past run */
	onOpen: (id: string) => void;
	/** back to drawing */
	onBack: () => void;
	showToast: (m: string, t?: 'success' | 'error') => void;
}

/** The past runs, newest first: when each started, how long it took, what it did. Open one to see its report. */
export default function WorkflowHistory({ onOpen, onBack, showToast }: Props) {
	const [list, setList] = useState<Workflow[] | null>(null);

	useEffect(() => {
		let alive = true;
		api.get<{ data: { workflows: Workflow[] } }>('/workflows')
			.then(({ data }) => alive && setList(data.data?.workflows ?? []))
			.catch((e) => {
				if (!alive) return;
				showToast(apiMessage(e, 'Không tải được lịch sử chạy.'), 'error');
				setList([]);
			});
		return () => { alive = false; };
	}, [showToast]);

	return (
		<section className="wf-history" aria-label="Lịch sử chạy">
			<div className="wf-history-head">
				<h2>Lịch sử chạy</h2>
				<span className="wf-run-spacer" />
				<button type="button" className="tm-btn-submit" onClick={onBack}>Vẽ quy trình mới</button>
			</div>
			{list === null ? (
				<div className="view-loading"><span className="qr-spinner" /></div>
			) : list.length === 0 ? (
				<p className="wf-report-empty">Chưa có lần chạy nào. Quy trình đã chạy được giữ lại 30 ngày.</p>
			) : (
				<ul className="wf-history-list">
					{list.map((w) => {
						const result = w.status === 'running' ? 'Đang chạy, bấm để theo dõi.' : finishText(w).message;
						return (
							<li key={w._id}>
								<button type="button" className="wf-history-item" onClick={() => onOpen(w._id)}>
									<span className={`wf-run-status ${w.status}`}>{STATUS_TEXT[w.status]}</span>
									<span className="wf-history-main">
										<b>{w.name}</b>
										<small>
											{new Date(w.createdAt).toLocaleString('vi-VN')} · {w.by}
											{w.status !== 'running' && w.finishedAt && <> · chạy {durationText(w.createdAt, w.finishedAt)}</>}
										</small>
										<small className="wf-history-result">{result}</small>
									</span>
									<span className="wf-history-go" aria-hidden="true">Xem ›</span>
								</button>
							</li>
						);
					})}
				</ul>
			)}
		</section>
	);
}
