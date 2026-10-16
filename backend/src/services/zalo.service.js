import env from '../config/env.js';

const SEND_MESSAGE_URL = 'https://openapi.zalo.me/v3.0/oa/message/cs';

export function isZaloConfigured() {
	return Boolean(env.zaloOaAccessToken);
}

export async function sendZaloTextMessage({ userId, text }) {
	if (!isZaloConfigured()) return { delivered: false, reason: 'missing_token' };

	const response = await fetch(SEND_MESSAGE_URL, {
		method: 'POST',
		headers: { access_token: env.zaloOaAccessToken, 'Content-Type': 'application/json' },
		body: JSON.stringify({ recipient: { user_id: userId }, message: { text } }),
	});
	const result = await response.json();
	if (!response.ok || result.error !== 0) {
		throw new Error(result.message || `Zalo API returned HTTP ${response.status}`);
	}
	return { delivered: true, result };
}