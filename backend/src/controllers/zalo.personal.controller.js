import { connectZaloPersonal, disconnectZaloPersonal, getZaloPersonalStatus, startZaloQrLogin } from '../services/zalo-personal.service.js';

export function getPersonalZaloStatus(request, response) {
	response.json({ success: true, data: getZaloPersonalStatus() });
}

export async function connectPersonalZalo(request, response) {
	const { cookie, imei, userAgent } = request.body;
	const data = await connectZaloPersonal({ cookie, imei, userAgent });
	response.json({ success: true, data });
}

export function startPersonalZaloQr(request, response) {
	response.json({ success: true, data: startZaloQrLogin() });
}

export function disconnectPersonalZalo(request, response) {
	response.json({ success: true, data: disconnectZaloPersonal() });
}
