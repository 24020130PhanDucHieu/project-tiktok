import { useCallback, useEffect, useRef, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { Workflow } from '../../types/workflow';

export const WORKFLOW_POLL_MS = 5000; // how often the screen asks the server how far the workflow is

/**
 * Follows the latest workflow. It asks for it when the tab opens (so a workflow started before the page was closed or
 * reloaded shows up again) and, while it runs, again every few seconds. The screen only reads: the server does the work.
 */
export function useWorkflowRun(showToast: (m: string, t?: 'success' | 'error') => void) {
	const [workflow, setWorkflow] = useState<Workflow | null>(null);
	const [loaded, setLoaded] = useState(false);
	const toast = useRef(showToast);
	toast.current = showToast;

	useEffect(() => {
		api.get('/workflows/latest')
			.then(({ data }) => setWorkflow(data.data?.workflow ?? null))
			.catch(() => toast.current('Không tải được quy trình gần nhất.', 'error'))
			.finally(() => setLoaded(true));
	}, []);

	const id = workflow?._id;
	const running = workflow?.status === 'running';
	useEffect(() => {
		if (!id || !running) return;
		const timer = setInterval(() => {
			api.get<{ data: Workflow }>(`/workflows/${id}`)
				.then(({ data }) => setWorkflow(data.data))
				.catch(() => undefined); // a failed question is asked again at the next tick
		}, WORKFLOW_POLL_MS);
		return () => clearInterval(timer);
	}, [id, running]);

	const stop = useCallback(async () => {
		if (!id) return;
		try {
			await api.put(`/workflows/${id}/stop`);
			toast.current('Đã yêu cầu dừng. Vài giây nữa quy trình sẽ dừng.');
		} catch (e) {
			toast.current(apiMessage(e, 'Không dừng được quy trình.'), 'error');
		}
	}, [id]);

	return { workflow, loaded, setWorkflow, stop };
}
