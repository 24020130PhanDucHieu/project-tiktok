import type { UserProfile } from '../types/koc';

const TOKEN_KEY = 'tiktoday_access_token';
const USER_KEY = 'tiktoday_user';

export function getToken(): string | null {
	return localStorage.getItem(TOKEN_KEY);
}

export function getUser(): UserProfile | null {
	const value = localStorage.getItem(USER_KEY);
	return value ? (JSON.parse(value) as UserProfile) : null;
}

export function saveSession({ token, user }: { token: string; user: UserProfile }): void {
	localStorage.setItem(TOKEN_KEY, token);
	localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function clearSession(): void {
	localStorage.removeItem(TOKEN_KEY);
	localStorage.removeItem(USER_KEY);
}

