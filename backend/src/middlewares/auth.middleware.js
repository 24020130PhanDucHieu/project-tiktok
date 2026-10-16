import jwt from 'jsonwebtoken';
import env from '../config/env.js';

export function requireAuth(request, response, next) {
	const header = request.get('authorization');
	if (!header?.startsWith('Bearer ')) return response.status(401).json({ success: false, message: 'Bạn cần đăng nhập' });
	try { request.user = jwt.verify(header.slice(7), env.jwtSecret); next(); } catch { return response.status(401).json({ success: false, message: 'Phiên đăng nhập không còn hợp lệ' }); }
}
