import { useCallback, useEffect, useState } from 'react';
import api, { apiMessage } from '../../lib/axios';
import type { TemplateSummary, WorkflowEdge, WorkflowNode, WorkflowTemplate } from '../../types/workflow';

interface Payload {
	name: string;
	nodes: WorkflowNode[];
	edges: WorkflowEdge[];
}

/** The saved workflows of the shop: the list, and saving, opening, changing and deleting one. */
export function useTemplates(notify: (message: string, type?: 'success' | 'error') => void) {
	const [items, setItems] = useState<TemplateSummary[]>([]);
	const [loading, setLoading] = useState(true);

	const refresh = useCallback(async () => {
		try {
			const { data } = await api.get('/workflow-templates');
			setItems(Array.isArray(data.data) ? data.data : []);
		} catch {
			setItems([]);
		} finally {
			setLoading(false);
		}
	}, []);
	useEffect(() => { refresh(); }, [refresh]);

	const open = useCallback(async (id: string): Promise<WorkflowTemplate | null> => {
		try {
			const { data } = await api.get<{ data: WorkflowTemplate }>(`/workflow-templates/${id}`);
			return data.data;
		} catch (e) {
			notify(apiMessage(e, 'Không mở được quy trình này.'), 'error');
			refresh(); // it may have been deleted by somebody else
			return null;
		}
	}, [notify, refresh]);

	const create = useCallback(async (payload: Payload): Promise<WorkflowTemplate | null> => {
		try {
			const { data } = await api.post<{ data: WorkflowTemplate }>('/workflow-templates', payload);
			notify('Đã lưu quy trình.');
			refresh();
			return data.data;
		} catch (e) {
			notify(apiMessage(e, 'Không lưu được quy trình.'), 'error');
			return null;
		}
	}, [notify, refresh]);

	const update = useCallback(async (id: string, payload: Payload): Promise<boolean> => {
		try {
			await api.put(`/workflow-templates/${id}`, payload);
			notify('Đã lưu thay đổi.');
			refresh();
			return true;
		} catch (e) {
			notify(apiMessage(e, 'Không lưu được thay đổi.'), 'error');
			return false;
		}
	}, [notify, refresh]);

	const remove = useCallback(async (id: string): Promise<boolean> => {
		try {
			await api.delete(`/workflow-templates/${id}`);
			notify('Đã xóa quy trình.');
			refresh();
			return true;
		} catch (e) {
			notify(apiMessage(e, 'Không xóa được quy trình.'), 'error');
			refresh();
			return false;
		}
	}, [notify, refresh]);

	return { items, loading, refresh, open, create, update, remove };
}
