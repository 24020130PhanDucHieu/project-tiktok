import { LoginQRCallbackEventType, ThreadType, Zalo } from 'zca-js';

/**
 * Holds the single personal Zalo session (login by cookie/IMEI or QR) and forwards inbound messages.
 * State machine: disconnected -> connecting|waiting_qr -> scanned -> connected | expired | error
 */
export class ZaloSession {
	constructor({ onMessage }) {
		this.onMessage = onMessage;
		this.api = null;
		this.listener = null;
		this.state = 'disconnected';
		this.error = '';
		this.qrImage = '';
		this.qrLoginPromise = null;
	}

	status() {
		return { state: this.state, qrImage: this.qrImage, error: this.error };
	}

	isConnected() {
		return Boolean(this.api) && this.state === 'connected';
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

	async connect({ cookie, imei, userAgent }) {
		if (!cookie || !imei || !userAgent) throw new Error('Thiếu cookie, IMEI hoặc user-agent Zalo cá nhân.');
		this.#stop();
		this.state = 'connecting';
		this.error = '';
		try {
			const api = await new Zalo().login({ cookie: ZaloSession.parseCookie(cookie), imei, userAgent });
			this.#attach(api);
			return this.status();
		} catch (error) {
			this.#stop();
			this.state = 'error';
			this.error = error instanceof Error ? error.message : String(error);
			throw error;
		}
	}

	startQr({ userAgent } = {}) {
		if (this.state === 'connected' || this.qrLoginPromise) return this.status();
		this.#stop();
		this.state = 'waiting_qr';
		this.error = '';
		this.qrImage = '';
		this.qrLoginPromise = (async () => {
			try {
				const api = await new Zalo().loginQR({ userAgent: userAgent || undefined }, (event) => {
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
				this.#attach(api);
				this.qrImage = '';
			} catch (error) {
				this.state = 'error';
				this.error = error instanceof Error ? error.message : String(error);
				this.qrImage = '';
			} finally {
				this.qrLoginPromise = null;
			}
		})();
		return this.status();
	}

	disconnect() {
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
		if (!this.isConnected()) throw new Error('Zalo cá nhân chưa kết nối.');
		return this.api.sendMessage({ msg: text }, String(userId), ThreadType.User);
	}
}
