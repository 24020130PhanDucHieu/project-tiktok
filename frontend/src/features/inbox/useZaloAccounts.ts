import { useCallback, useEffect, useState } from 'react';
import api from '../../lib/axios';
import type { ZaloAccount } from '../../types/zalo';

/** How often the list (and so every account's live status) is read while the page is in view. */
const REFRESH_MS = 10000;

/**
 * The personal Zalo accounts with their live status. Reading is cheap (the server asks the bridge once for all of
 * them), so the list is refreshed now and then: a lost connection shows up without anybody reloading the page.
 * Against a server that does not know the routes the list stays empty and the screen falls back to the single account.
 */
export function useZaloAccounts(enabled: boolean) {
	const [accounts, setAccounts] = useState<ZaloAccount[]>([]);
	const [loaded, setLoaded] = useState(false);

	const refresh = useCallback(() => {
		return api
			.get('/zalo/accounts')
			.then(({ data }) => setAccounts(Array.isArray(data.data) ? data.data : []))
			.catch(() => {})
			.finally(() => setLoaded(true));
	}, []);

	useEffect(() => {
		if (!enabled) return undefined;
		refresh();
		const timer = window.setInterval(() => {
			if (!document.hidden) refresh();
		}, REFRESH_MS);
		return () => window.clearInterval(timer);
	}, [enabled, refresh]);

	return { accounts, loaded, refresh };
}
