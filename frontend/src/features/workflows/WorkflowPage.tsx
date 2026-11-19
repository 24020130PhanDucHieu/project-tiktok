import { useCallback, useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { Campaign } from '../../types/campaign';
import type { Workflow } from '../../types/workflow';
import { markWorkflowSeen, seenWorkflowId, shouldShowFinished } from './runText';
import { useLabels } from './useLabels';
import { useWorkflowRun } from './useWorkflowRun';
import WorkflowEditor from './WorkflowEditor';
import WorkflowRunView from './WorkflowRunView';

interface Props {
	isAdmin: boolean;
	userEmail: string;
	folders: string[];
	showToast: (m: string, t?: 'success' | 'error') => void;
}

/** The Quy trình tab: draw a workflow from blocks, start it, follow it. */
export default function WorkflowPage({ isAdmin, userEmail, folders, showToast }: Props) {
	const [campaigns, setCampaigns] = useState<Campaign[]>([]);
	const [starting, setStarting] = useState(false);
	const [drawing, setDrawing] = useState(false); // the person asked for a new drawing after a report
	const { workflow, loaded, setWorkflow, stop } = useWorkflowRun(showToast);
	const labels = useLabels();

	useEffect(() => {
		api.get('/campaigns')
			.then(({ data }) => setCampaigns(data.data || []))
			.catch(() => showToast('Không tải được danh sách chiến dịch.', 'error'));
	}, [showToast]);

	const start = useCallback(async (payload: Record<string, unknown>) => {
		setStarting(true);
		try {
			const { data } = await api.post<{ data: Workflow }>('/workflows', payload, { timeout: 30000 });
			setWorkflow(data.data);
			setDrawing(false);
			showToast('Đã nhận quy trình, máy chủ sẽ tự chạy.');
			return true;
		} catch (e) {
			showToast(apiMessage(e, 'Không bắt đầu được quy trình.'), 'error');
			// another workflow may be running: show it instead of leaving the person with an error and no picture
			api.get('/workflows/latest')
				.then(({ data }) => data.data?.workflow?.status === 'running' && setWorkflow(data.data.workflow))
				.catch(() => undefined);
			return false;
		} finally {
			setStarting(false);
		}
	}, [setWorkflow, showToast]);

	function newDrawing() {
		if (workflow) markWorkflowSeen(workflow._id);
		setDrawing(true);
	}

	// a running workflow, or an ended one whose report has not been seen, takes the screen; otherwise the canvas
	const showRun = !!workflow && !drawing && (workflow.status === 'running' || shouldShowFinished(workflow, seenWorkflowId(), new Date()));

	return (
		<div className="page-shell wf-page">
			<header className="page-head">
				<div>
					<h1>Quy trình</h1>
					<p>Ghép các chức năng thành một quy trình tự chạy: nhập KOC từ Excel, chờ, gửi chiến dịch Zalo, rồi nhắc người chưa trả lời.</p>
				</div>
			</header>
			{!loaded ? (
				<div className="view-loading"><span className="qr-spinner" /></div>
			) : showRun && workflow ? (
				<WorkflowRunView workflow={workflow} isAdmin={isAdmin} onStop={stop} onNew={newDrawing} />
			) : (
				<WorkflowEditor campaigns={campaigns} labels={labels} folders={folders} isAdmin={isAdmin} userEmail={userEmail} starting={starting} onStart={start} notify={showToast} />
			)}
		</div>
	);
}
