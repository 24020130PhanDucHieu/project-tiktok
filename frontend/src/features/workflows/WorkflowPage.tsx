import { useCallback, useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { Campaign } from '../../types/campaign';
import WorkflowEditor from './WorkflowEditor';

interface Props {
	isAdmin: boolean;
	folders: string[];
	showToast: (m: string, t?: 'success' | 'error') => void;
}

/** The Quy trình tab: draw a workflow from blocks, start it, follow it. */
export default function WorkflowPage({ isAdmin, folders, showToast }: Props) {
	const [campaigns, setCampaigns] = useState<Campaign[]>([]);
	const [starting, setStarting] = useState(false);

	useEffect(() => {
		api.get('/campaigns')
			.then(({ data }) => setCampaigns(data.data || []))
			.catch(() => showToast('Không tải được danh sách chiến dịch.', 'error'));
	}, [showToast]);

	const start = useCallback(async (payload: Record<string, unknown>) => {
		setStarting(true);
		try {
			await api.post('/workflows', payload, { timeout: 30000 });
			showToast('Đã nhận quy trình, máy chủ sẽ tự chạy.');
			return true;
		} catch (e) {
			showToast(apiMessage(e, 'Không bắt đầu được quy trình.'), 'error');
			return false;
		} finally {
			setStarting(false);
		}
	}, [showToast]);

	return (
		<div className="page-shell wf-page">
			<header className="page-head">
				<div>
					<h1>Quy trình</h1>
					<p>Ghép các chức năng thành một quy trình tự chạy: nhập KOC từ Excel, chờ, gửi chiến dịch Zalo, rồi nhắc người chưa trả lời.</p>
				</div>
			</header>
			<WorkflowEditor campaigns={campaigns} folders={folders} isAdmin={isAdmin} starting={starting} onStart={start} notify={showToast} />
		</div>
	);
}
