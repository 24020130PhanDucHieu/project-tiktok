/** The account every request meant before there were several: a caller that sends no account id still works. */
export const DEFAULT_ACCOUNT = 'default';

const DISCONNECTED = { state: 'disconnected', qrImage: '', error: '' };

/**
 * accountId -> live Zalo session, kept in this process's memory. It is rebuilt from the credentials the API stores
 * (POST /connect for each account) whenever the bridge restarts: nothing here survives a restart.
 */
export class SessionRegistry {
	/** @param {(accountId: string) => object} create builds a session for an account that has none yet */
	constructor(create) {
		this.create = create;
		this.sessions = new Map();
	}

	static id(accountId) {
		const id = String(accountId ?? '').trim();
		return id || DEFAULT_ACCOUNT;
	}

	get size() {
		return this.sessions.size;
	}

	/** The session of an account, created on first use (a login needs one to start from). */
	get(accountId) {
		const id = SessionRegistry.id(accountId);
		let session = this.sessions.get(id);
		if (!session) {
			session = this.create(id);
			this.sessions.set(id, session);
		}
		return session;
	}

	/** The session of an account, or undefined: asking never creates one. */
	peek(accountId) {
		return this.sessions.get(SessionRegistry.id(accountId));
	}

	/** Status of one account; an account the bridge has no session for is simply disconnected. */
	statusOf(accountId) {
		return this.peek(accountId)?.status() ?? { ...DISCONNECTED };
	}

	/** Status of every session the bridge holds, by account id. */
	statuses() {
		return Object.fromEntries([...this.sessions].map(([id, session]) => [id, session.status()]));
	}

	/**
	 * Moves a session to another key. The session that is moved wins: one already living under the new key is stopped and
	 * replaced, because two sessions for one Zalo account make Zalo close one of the two connections.
	 * Returns false when there was nothing to move.
	 */
	rekey(oldId, newId) {
		const from = SessionRegistry.id(oldId);
		const to = SessionRegistry.id(newId);
		if (from === to) return false;
		const session = this.sessions.get(from);
		if (!session) return false;
		const previous = this.sessions.get(to);
		if (previous && previous !== session) previous.disconnect();
		this.sessions.delete(from);
		this.sessions.set(to, session);
		return true;
	}

	/** Closes the session of an account and forgets it. */
	remove(accountId) {
		const id = SessionRegistry.id(accountId);
		this.sessions.get(id)?.disconnect();
		this.sessions.delete(id);
	}

	disconnectAll() {
		for (const session of this.sessions.values()) session.disconnect();
	}
}
