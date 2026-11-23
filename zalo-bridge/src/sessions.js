import { SessionRegistry } from './registry.js';
import { ZaloSession } from './zalo.js';

/**
 * The sessions of the bridge, with their calls back to the API.
 *
 * A session lives under an account id, and tells the API with every report which id that is. Most of the time the id is
 * known before the login: the API created the account first. But a login that had no account to name (the old
 * single-account flow, with no account stored yet) lives under the temporary key "default", and the API then creates the
 * account and answers with its real id. The session is renamed to that id at once: otherwise the API, looking for the
 * session of its new account, finds none and logs in a second time, and Zalo sees two connections for one account (it
 * allows one, and closes the other: "Another connection is opened"). The same answer covers a login that the API folded
 * into an account that already existed.
 *
 * @param {{ postToApi: (path: string, payload: object) => Promise<object>, createClient?: () => object }} deps
 */
export function createSessions({ postToApi, createClient }) {
	const sessions = new SessionRegistry((accountId) => {
		const ref = { id: accountId }; // the key this session lives under now; the API may rename it
		return new ZaloSession({
			createClient,
			// Deliver inbound messages to the Go API (it stores them, runs the chatbot, and replies through /send).
			onMessage: (payload) => postToApi('/internal/zalo/inbound', { ...payload, accountId: ref.id }),
			// After a QR login, or when Zalo renews the cookie, the API keeps the credentials to log in again later.
			onSession: async (credentials) => {
				const reply = await postToApi('/internal/zalo/session', { ...credentials, accountId: ref.id });
				const id = typeof reply?.accountId === 'string' ? reply.accountId : '';
				if (id && id !== ref.id && sessions.rekey(ref.id, id)) ref.id = id;
			},
		});
	});
	return sessions;
}
