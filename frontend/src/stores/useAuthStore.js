const TOKEN_KEY = 'tiktoday_access_token';
const USER_KEY = 'tiktoday_user';

export function getToken() { return localStorage.getItem(TOKEN_KEY); }
export function getUser() { const value = localStorage.getItem(USER_KEY); return value ? JSON.parse(value) : null; }
export function saveSession({ token, user }) { localStorage.setItem(TOKEN_KEY, token); localStorage.setItem(USER_KEY, JSON.stringify(user)); }
export function clearSession() { localStorage.removeItem(TOKEN_KEY); localStorage.removeItem(USER_KEY); }
