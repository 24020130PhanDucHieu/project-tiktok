import { useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { WorkflowEdge, WorkflowNode } from '../../types/workflow';
import { previewEffects } from './preview';

interface PreviewKoc {
	id: string;
	name: string;
}

interface Props {
	nodes: WorkflowNode[];
	edges: WorkflowEdge[];
	/** how many rows of the Excel file will be imported, and their names (a workflow that starts from a file) */
	sheetNames: string[] | null;
	/** the window of "Bắt đầu chạy": when given, the preview is part of the confirmation */
	onConfirm?: () => void;
	confirming?: boolean;
	onClose: () => void;
	children?: React.ReactNode;
}

const SHOWN = 30;

/** Who the workflow is about and what it will do to them, before it starts. Nothing is written or sent by looking. */
export default function PreviewDialog({ nodes, edges, sheetNames, onConfirm, confirming, onClose, children }: Props) {
	const [list, setList] = useState<PreviewKoc[] | null>(sheetNames ? sheetNames.map((name, i) => ({ id: String(i), name })) : null);
	const [error, setError] = useState('');

	useEffect(() => {
		if (sheetNames) return;
		let alive = true;
		api.post<{ data: { total: number; kocs: PreviewKoc[] } }>('/workflows/preview', { nodes, edges })
			.then(({ data }) => alive && setList(data.data.kocs))
			.catch((e) => alive && setError(apiMessage(e, 'Không xem trước được.')));
		return () => { alive = false; };
	}, [nodes, edges, sheetNames]);

	const count = list?.length ?? 0;
	const fx = list ? previewEffects(nodes, edges, count) : { lines: [], sends: false };
	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && !confirming && onClose()}>
			<div className="tiktoday-modal wf-confirm wf-preview" role="dialog" aria-modal="true" aria-labelledby="wf-preview-title">
				<div className="tm-header"><div><h2 id="wf-preview-title">{onConfirm ? 'Bắt đầu quy trình?' : 'Xem trước'}</h2></div></div>
				<div className="tm-body">
					{error ? <p className="wf-warn">{error}</p> : list === null ? <div className="view-loading"><span className="qr-spinner" /></div> : (
						<>
							<p>Quy trình sẽ chạy với <b>{count} KOC</b>{count === 0 && ' (không có KOC nào khớp)'}:</p>
							{count > 0 && (
								<p className="wf-preview-names">
									{list.slice(0, SHOWN).map((k) => <span key={k.id} className="wf-report-name">{k.name}</span>)}
									{count > SHOWN && <span className="wf-report-more">và {count - SHOWN} KOC khác</span>}
								</p>
							)}
							{fx.lines.length > 0 && (
								<>
									<p className="wf-preview-title">Sau đó:</p>
									<ol className="wf-preview-steps">{fx.lines.map((l, i) => <li key={i}>{l}</li>)}</ol>
								</>
							)}
						</>
					)}
					{children}
				</div>
				<div className="tm-footer">
					<button type="button" className="tm-btn-cancel" disabled={confirming} onClick={onClose}>{onConfirm ? 'Hủy' : 'Đóng'}</button>
					{onConfirm && <button type="button" className="tm-btn-submit" disabled={confirming || !!error || list === null || count === 0} onClick={onConfirm}>{confirming ? 'Đang bắt đầu…' : 'Bắt đầu chạy'}</button>}
				</div>
			</div>
		</div>
	);
}
