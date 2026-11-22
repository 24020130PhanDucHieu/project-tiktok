import { LoginQRCallbackEventType, Reactions, ThreadType, Zalo } from 'zca-js';

// A friend request carries a short note: Zalo itself caps it at 150 characters.
export const FRIEND_NOTE_MAX = 150;
const REACTION_CODES = new Set(Object.values(Reactions));

/**
 * Holds the single personal Zalo session (login by cookie/IMEI or QR) and forwards inbound messages.
 * State machine: disconnected -> connecting|waiting_qr -> scanned -> connected | expired | error
 */
export class ZaloSession {
	// createClient: where the Zalo client (login, loginQR) comes from; tests give a fake one so no login reaches the network
	constructor({ onMessage, onSession, createClient = () => new Zalo() }) {
		this.onMessage = onMessage;
		this.onSession = onSession;
		this.createClient = createClient;
		this.api = null;
		this.listener = null;
		this.state = 'disconnected';
		this.error = '';
		this.qrImage = '';
		this.qrLoginPromise = null;
		this.qrGeneration = 0;
		this.qrStartedAt = 0;
	}

	status() {
		// Watchdog: Zalo normally answers within a few seconds; never leave the UI spinning forever.
		if (this.state === 'waiting_qr' && !this.qrImage && this.qrStartedAt && Date.now() - this.qrStartedAt > 25000) {
			this.qrGeneration += 1;
			this.qrLoginPromise = null;
			this.state = 'error';
			this.error = 'Không nhận được mã QR từ máy chủ Zalo sau 25 giây. Kiểm tra kết nối mạng của máy chủ rồi thử lại.';
		}
		return { state: this.state, qrImage: this.qrImage, error: this.error };
	}

	isConnected() {
		return Boolean(this.api) && this.state === 'connected';
	}

	/**
	 * What the API must keep to log this account in again later: the cookie jar as plain JSON (login() takes it back as
	 * is), the device id and user agent it was issued for, and the Zalo user id that tells accounts apart.
	 */
	static exportCredentials(api) {
		const ctx = api.getContext();
		return { zaloUid: String(ctx.uid ?? ''), cookie: api.getCookie().toJSON().cookies, imei: ctx.imei, userAgent: ctx.userAgent };
	}

	/**
	 * Are these the same cookies? Only what Zalo cares about counts (name, value, domain, path, expiry): the jar also
	 * records when a cookie was created and last used, which differ on every login and say nothing about a renewal.
	 */
	static sameCookie(a, b) {
		const essence = (cookies) =>
			(Array.isArray(cookies) ? cookies : [])
				.map((c) => [c.key ?? c.name ?? '', c.value ?? '', c.domain ?? '', c.path ?? '', c.expires ?? ''].join('\u0000'))
				.sort()
				.join('\u0001');
		return essence(a) === essence(b);
	}

	static parseCookie(value) {
		if (Array.isArray(value)) return value;
		const parsed = typeof value === 'string' ? JSON.parse(value) : value;
		if (Array.isArray(parsed)) return parsed;
		if (Array.isArray(parsed?.cookies)) return parsed.cookies;
		throw new Error('ZALO_PERSONAL_COOKIE phải là JSON cookie export dạng mảng.');
	}

	#attach(api) {
		this.api = api;
		this.listener = api.listener;
		this.listener.on('message', (message) => this.#handleMessage(message));
		this.listener.on('error', (error) => {
			this.error = error instanceof Error ? error.message : String(error);
			this.state = 'error';
		});
		this.listener.on('disconnected', () => {
			if (this.state === 'connected') this.state = 'disconnected';
		});
		this.listener.start({ retryOnClose: true });
		this.state = 'connected';
	}

	/**
	 * Takes a logged-in api as this session's connection and tells the API which credentials to keep. A QR login
	 * (always) has nothing stored yet; a cookie login reports only when Zalo handed out a different cookie than the
	 * one it was given, so the stored one never goes stale. A failing report never breaks the login.
	 */
	async adopt(api, { always = false, sentCookie } = {}) {
		this.#attach(api);
		if (!this.onSession) return;
		try {
			const creds = ZaloSession.exportCredentials(api);
			if (always || !ZaloSession.sameCookie(sentCookie, creds.cookie)) {
				await this.onSession(creds);
				console.log('zalo-bridge: the credentials of the login were handed to the API');
			}
		} catch (error) {
			this.error = error instanceof Error ? error.message : String(error);
			// kept in the status AND printed: a failure here must show in `docker logs`, not only in memory
			console.error(`zalo-bridge: the credentials of the login were NOT handed to the API: ${this.error}`);
		}
	}

	async #handleMessage(message) {
		try {
			if (message.type !== ThreadType.User || message.isSelf || typeof message.data.content !== 'string') return;
			const text = message.data.content.trim();
			if (!text) return;
			await this.onMessage({
				externalContactId: String(message.threadId),
				name: message.data.dName || '',
				text,
				ts: Number(message.data.ts) || Date.now(),
			});
		} catch (error) {
			this.error = error instanceof Error ? error.message : String(error);
		}
	}

	#stop() {
		try {
			this.listener?.stop();
		} catch {
			// ignore
		}
		this.api = null;
		this.listener = null;
	}

	/** persist: a login a person typed in is reported back so the API keeps it; the automatic login (persist false) only reports a renewed cookie. */
	async connect({ cookie, imei, userAgent, persist = false }) {
		if (!cookie || !imei || !userAgent) throw new Error('Thiếu cookie, IMEI hoặc user-agent Zalo cá nhân.');
		this.#stop();
		this.state = 'connecting';
		this.error = '';
		try {
			const sentCookie = ZaloSession.parseCookie(cookie);
			const api = await this.createClient().login({ cookie: sentCookie, imei, userAgent });
			await this.adopt(api, { sentCookie, always: Boolean(persist) });
			return this.status();
		} catch (error) {
			this.#stop();
			this.state = 'error';
			this.error = error instanceof Error ? error.message : String(error);
			throw error;
		}
	}

	startQr({ userAgent } = {}) {
		if (this.state === 'connected') return this.status();
		// A QR attempt that is still alive and not expired/failed is reused; otherwise start a fresh one.
		const stale = this.state === 'expired' || this.state === 'error' || this.state === 'disconnected';
		if (this.qrLoginPromise && !stale) return this.status();
		this.#stop();
		const generation = ++this.qrGeneration;
		const current = () => generation === this.qrGeneration;
		this.state = 'waiting_qr';
		this.error = '';
		this.qrImage = '';
		this.qrStartedAt = Date.now();
		this.qrLoginPromise = (async () => {
			try {
				const api = await this.createClient().loginQR({ userAgent: userAgent || undefined }, (event) => {
					if (!current()) return; // superseded by a newer attempt
					if (event.type === LoginQRCallbackEventType.QRCodeGenerated) {
						this.qrImage = `data:image/png;base64,${event.data.image}`;
						this.state = 'waiting_qr';
					} else if (event.type === LoginQRCallbackEventType.QRCodeScanned) {
						this.state = 'scanned';
					} else if (event.type === LoginQRCallbackEventType.QRCodeExpired) {
						this.state = 'expired';
						this.qrImage = '';
					}
				});
				if (!current()) return;
				await this.adopt(api, { always: true }); // a QR login has nothing stored yet: the API must keep its credentials
				this.qrImage = '';
			} catch (error) {
				if (!current()) return;
				this.state = 'error';
				this.error = error instanceof Error ? error.message : String(error);
				this.qrImage = '';
			} finally {
				if (current()) this.qrLoginPromise = null;
			}
		})();
		return this.status();
	}

	disconnect() {
		this.qrGeneration += 1;
		this.#stop();
		this.qrLoginPromise = null;
		this.qrImage = '';
		this.state = 'disconnected';
		this.error = '';
		return this.status();
	}

	async findUser(phone) {
		if (!this.isConnected()) throw new Error('Zalo cá nhân chưa kết nối.');
		const user = await this.api.findUser(phone);
		if (!user?.uid) throw new Error('Không tìm thấy tài khoản Zalo với số điện thoại này.');
		return { userId: String(user.uid), displayName: user.display_name || user.zalo_name || '' };
	}

	async send(userId, text) {
		if (!this.isConnected()) throw Object.assign(new Error('Zalo cá nhân chưa kết nối.'), { statusCode: 409 }); // 4xx: nothing was sent
		return this.api.sendMessage({ msg: text }, String(userId), ThreadType.User);
	}

	async sendImage(userId, { filename, dataBase64, width, height, caption }) {
		if (!this.isConnected()) throw Object.assign(new Error('Zalo cá nhân chưa kết nối.'), { statusCode: 409 }); // 4xx: nothing was sent
		const data = Buffer.from(String(dataBase64 || ''), 'base64');
		if (!data.length) throw Object.assign(new Error('Thiếu dữ liệu ảnh.'), { statusCode: 400 });
		return this.api.sendMessage(
			{
				msg: String(caption || ''),
				attachments: [{ data, filename: String(filename || 'image.png'), metadata: { totalSize: data.length, width: Number(width) || 0, height: Number(height) || 0 } }],
			},
			String(userId),
			ThreadType.User,
		);
	}

	/** Is this person already a friend, and is a request pending in either direction? */
	async friendStatus(userId) {
		if (!this.isConnected()) throw new Error('Zalo cá nhân chưa kết nối.');
		const r = await this.api.getFriendRequestStatus(String(userId));
		return { isFriend: r?.is_friend === 1, requestSent: r?.is_requested === 1, requestReceived: r?.is_requesting === 1 };
	}

	async sendFriendRequest(userId, message) {
		if (!this.isConnected()) throw new Error('Zalo cá nhân chưa kết nối.');
		const note = String(message || '').trim();
		if (!note) throw new Error('Lời mời kết bạn cần có lời nhắn.');
		if ([...note].length > FRIEND_NOTE_MAX) throw new Error(`Lời nhắn kết bạn tối đa ${FRIEND_NOTE_MAX} ký tự.`);
		await this.api.sendFriendRequest(note, String(userId));
	}

	/** Reacts to a message the contact sent. An empty icon takes the reaction back. */
	async react(userId, msgId, cliMsgId, icon) {
		if (!this.isConnected()) throw new Error('Zalo cá nhân chưa kết nối.');
		if (!msgId || !cliMsgId) throw new Error('Tin nhắn này không có mã Zalo nên không thả cảm xúc được.');
		const code = String(icon ?? '');
		if (code !== '' && !REACTION_CODES.has(code)) throw new Error('Cảm xúc này Zalo không hỗ trợ.');
		await this.api.addReaction(code, { data: { msgId: String(msgId), cliMsgId: String(cliMsgId) }, threadId: String(userId), type: ThreadType.User });
	}
}
