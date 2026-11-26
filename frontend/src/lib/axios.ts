import axios from 'axios';
import { clearSession, getToken } from '../stores/useAuthStore';

const api = axios.create({
	baseURL: import.meta.env.VITE_API_URL || 'http://127.0.0.1:5000/api/v1',
	timeout: 20000,
	headers: { 'Content-Type': 'application/json' },
});

api.interceptors.request.use((config) => {
	const token = getToken();
	if (token) config.headers.Authorization = `Bearer ${token}`;
	return config;
});

// An expired / revoked session (password changed, account suspended...) sends the user back to the login page.
api.interceptors.response.use(
	(response) => response,
	(error) => {
		const url: string = error.config?.url || '';
		if (error.response?.status === 401 && getToken() && !url.startsWith('/auth/login')) {
			clearSession();
			window.location.reload();
		}
		return Promise.reject(error);
	},
);

export default api;

export function apiMessage(error: unknown, fallback: string): string {
	const res = (error as any)?.response;
	// 403 (guest mode / forbidden section): show the server's own message, never log out
	if (res?.status === 403) return res?.data?.message || 'Bạn không có quyền thực hiện thao tác này. Đăng nhập để chỉnh sửa.';
	return res?.data?.message || fallback;
}
