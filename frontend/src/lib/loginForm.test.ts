import { describe, expect, it } from 'vitest';
import { buildLoginBody, buildRegisterBody, hasSession } from './loginForm';

describe('login form helpers', () => {
	it('login body carries only email, password and the honeypot (never a name)', () => {
		expect(buildLoginBody({ email: 'a@b.c', password: 'x', website: '' })).toEqual({ email: 'a@b.c', password: 'x', website: '' });
		const body = buildLoginBody({ email: 'a@b.c', password: 'x', website: 'bot', name: 'An' } as never);
		expect(Object.keys(body).sort()).toEqual(['email', 'password', 'website']);
		expect(body.website).toBe('bot');
	});
	it('treats a reply without token as no session', () => {
		expect(hasSession({ success: true, message: 'ok' })).toBe(false);
		expect(hasSession(undefined)).toBe(false);
		expect(hasSession({ data: { token: '', user: {} } })).toBe(false);
		expect(hasSession({ data: { token: 't', user: {} } })).toBe(true);
	});
	it('register body trims the name, always sends website and captcha, never role or status', () => {
		const body = buildRegisterBody({ name: ' Lan Anh ', email: 'a@b.c', password: 'Matkhau123', website: '', captchaToken: 'tok' });
		expect(body).toEqual({ name: 'Lan Anh', email: 'a@b.c', password: 'Matkhau123', website: '', captchaToken: 'tok' });
		expect(Object.keys(body)).not.toContain('role');
		expect(Object.keys(body)).not.toContain('status');
	});
});
