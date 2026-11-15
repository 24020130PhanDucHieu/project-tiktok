import { useCallback, useEffect, useRef, useState } from 'react';
import api from '../../lib/axios';

/**
 * The pictures of the campaigns, loaded once each. An <img src> cannot carry the login token, so each picture is fetched
 * through axios as a blob and shown from an object URL; the URLs are released when the window closes.
 */
export function useCampaignImages() {
	const [urls, setUrls] = useState<Record<string, string>>({});
	const asked = useRef(new Set<string>());
	const made = useRef<string[]>([]);

	const load = useCallback((id?: string) => {
		if (!id || asked.current.has(id)) return;
		asked.current.add(id);
		api.get(`/campaigns/images/${id}`, { responseType: 'blob' })
			.then(({ data }) => {
				const url = URL.createObjectURL(data as Blob);
				made.current.push(url);
				setUrls((u) => ({ ...u, [id]: url }));
			})
			.catch(() => asked.current.delete(id)); // may be asked again when the steps change
	}, []);

	useEffect(() => {
		const list = made.current;
		return () => list.forEach((u) => URL.revokeObjectURL(u));
	}, []);

	return { urlOf: (id?: string) => (id ? urls[id] : undefined), load };
}
