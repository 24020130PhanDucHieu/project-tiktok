import { LoginQRCallbackEventType, ThreadType, Zalo } from 'zca-js';
import Conversation from '../models/conversation.model.js';
import Message from '../models/message.model.js';
import env from '../config/env.js';

let api = null;
let listener = null;
let connectionState = 'disconnected';
let lastError = '';
let qrImage = '';
let qrLoginPromise = null;

function parseCookie(value) {
	if (Array.isArray(value)) return value;
	const parsed = JSON.parse(value);
	if (Array.isArray(parsed)) return parsed;
	if (Array.isArray(parsed.cookies)) return parsed.cookies;
	throw new Error('ZALO_PERSONAL_COOKIE phải là JSON cookie export dạng mảng.');
}

function credentialsFromEnv() {
	if (!env.zaloPersonalCookie || !env.zaloPersonalImei || !env.zaloPersonalUserAgent) return null;
	return {
		cookie: parseCookie(env.zaloPersonalCookie),
		imei: env.zaloPersonalImei,
		userAgent: env.zaloPersonalUserAgent,
	};
}

async function saveIncomingMessage(message) {
	if (message.type !== ThreadType.User || message.isSelf || typeof message.data.content !== 'string') return;
	const externalContactId = String(message.threadId);
	const text = message.data.content.trim();
	if (!text) return;

	const conversation = await Conversation.findOneAndUpdate(
		{ channel: 'zalo', externalContactId },
		{
			$set: {
				contactName: message.data.dName || `Zalo ${externalContactId}`,
				contactHandle: `zalo:${externalContactId}`,
				lastMessage: text,
				lastMessageAt: new Date(Number(message.data.ts) || Date.now()),
				status: 'open',
			},
			$inc: { unreadCount: 1 },
		},
		{ new: true, upsert: true, setDefaultsOnInsert: true },
	);
	await Message.create({ conversationId: conversation._id, direction: 'inbound', text, status: 'delivered' });
}

export function getZaloPersonalStatus() {
	return { configured: Boolean(credentialsFromEnv()), connected: connectionState === 'connected', state: connectionState, qrImage, error: lastError };
}

function attachListener(nextApi) {
	api = nextApi;
	listener = api.listener;
	listener.on('message', (message) => saveIncomingMessage(message).catch((error) => {
		lastError = error.message;
	}));
	listener.on('error', (error) => {
		lastError = error instanceof Error ? error.message : String(error);
		connectionState = 'error';
	});
	listener.on('disconnected', () => {
		if (connectionState === 'connected') connectionState = 'disconnected';
	});
	listener.start({ retryOnClose: true });
	connectionState = 'connected';
}

export async function connectZaloPersonal(credentials = credentialsFromEnv()) {
	if (!credentials) throw new Error('Thiếu cookie, IMEI hoặc user-agent Zalo cá nhân.');
	if (listener) listener.stop();
	connectionState = 'connecting';
	lastError = '';
	try {
		const zalo = new Zalo();
		attachListener(await zalo.login({ ...credentials, cookie: parseCookie(credentials.cookie) }));
		return getZaloPersonalStatus();
	} catch (error) {
		api = null;
		listener = null;
		connectionState = 'error';
		lastError = error instanceof Error ? error.message : String(error);
		throw error;
	}
}

export function startZaloQrLogin() {
	if (connectionState === 'connected') return getZaloPersonalStatus();
	if (qrLoginPromise) return getZaloPersonalStatus();
	listener?.stop();
	api = null;
	listener = null;
	connectionState = 'waiting_qr';
	lastError = '';
	qrImage = '';
	qrLoginPromise = (async () => {
		try {
			const zalo = new Zalo();
			const nextApi = await zalo.loginQR({ userAgent: env.zaloPersonalUserAgent || undefined }, (event) => {
				if (event.type === LoginQRCallbackEventType.QRCodeGenerated) {
					qrImage = `data:image/png;base64,${event.data.image}`;
					connectionState = 'waiting_qr';
				} else if (event.type === LoginQRCallbackEventType.QRCodeScanned) {
					connectionState = 'scanned';
				} else if (event.type === LoginQRCallbackEventType.QRCodeExpired) {
					connectionState = 'expired';
					qrImage = '';
				}
			});
			attachListener(nextApi);
			qrImage = '';
			return getZaloPersonalStatus();
		} catch (error) {
			connectionState = 'error';
			lastError = error instanceof Error ? error.message : String(error);
			qrImage = '';
			throw error;
		} finally {
			qrLoginPromise = null;
		}
	})();
	qrLoginPromise.catch(() => {});
	return getZaloPersonalStatus();
}

export function disconnectZaloPersonal() {
	listener?.stop();
	api = null;
	listener = null;
	qrLoginPromise = null;
	qrImage = '';
	connectionState = 'disconnected';
	lastError = '';
	return getZaloPersonalStatus();
}

export function isZaloPersonalConnected() {
	return Boolean(api) && connectionState === 'connected';
}

export async function resolveZaloUserByPhone(phoneNumber) {
	if (!isZaloPersonalConnected()) throw new Error('Zalo cá nhân chưa kết nối.');
	const normalizedPhone = String(phoneNumber || '').replace(/[\s()-]/g, '').replace(/^\+/, '');
	if (!normalizedPhone) throw new Error('Thiếu số điện thoại Zalo.');
	const user = await api.findUser(normalizedPhone);
	if (!user?.uid) throw new Error('Không tìm thấy tài khoản Zalo với số điện thoại này.');
	return { userId: String(user.uid), displayName: user.display_name || user.zalo_name || '' };
}

export async function sendZaloPersonalMessage({ userId, text }) {
	if (!isZaloPersonalConnected()) throw new Error('Zalo cá nhân chưa kết nối.');
	return api.sendMessage({ msg: text }, String(userId), ThreadType.User);
}

export async function initializeZaloPersonal() {
	if (!credentialsFromEnv()) return getZaloPersonalStatus();
	try {
		return await connectZaloPersonal();
	} catch (error) {
		console.error('Unable to connect personal Zalo:', error.message);
		return getZaloPersonalStatus();
	}
}
