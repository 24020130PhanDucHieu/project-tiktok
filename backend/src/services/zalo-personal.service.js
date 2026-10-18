import { ThreadType, Zalo } from 'zca-js';
import Conversation from '../models/conversation.model.js';
import Message from '../models/message.model.js';
import env from '../config/env.js';

let api = null;
let listener = null;
let connectionState = 'disconnected';
let lastError = '';

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
	return { configured: Boolean(credentialsFromEnv()), connected: connectionState === 'connected', state: connectionState, error: lastError };
}

export async function connectZaloPersonal(credentials = credentialsFromEnv()) {
	if (!credentials) throw new Error('Thiếu cookie, IMEI hoặc user-agent Zalo cá nhân.');
	if (listener) listener.stop();
	connectionState = 'connecting';
	lastError = '';
	try {
		const zalo = new Zalo();
		api = await zalo.login({ ...credentials, cookie: parseCookie(credentials.cookie) });
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
		return getZaloPersonalStatus();
	} catch (error) {
		api = null;
		listener = null;
		connectionState = 'error';
		lastError = error instanceof Error ? error.message : String(error);
		throw error;
	}
}

export function disconnectZaloPersonal() {
	listener?.stop();
	api = null;
	listener = null;
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
