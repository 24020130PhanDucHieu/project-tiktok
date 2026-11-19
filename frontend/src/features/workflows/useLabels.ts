import { useEffect, useState } from 'react';
import api from '../../lib/axios';
import type { InboxLabel } from '../../types/workflow';

/** The labels of the inbox, read once when the Quy trình tab opens (a label block chooses one of them). */
export function useLabels(): InboxLabel[] {
	const [labels, setLabels] = useState<InboxLabel[]>([]);
	useEffect(() => {
		api.get('/inbox/labels')
			.then(({ data }) => setLabels(Array.isArray(data.data) ? data.data : []))
			.catch(() => setLabels([])); // without the list a label block just cannot be set up: the editor says so
	}, []);
	return labels;
}
