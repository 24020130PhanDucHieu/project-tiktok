export interface EmailLoginInput {
	email: string;
	password: string;
	name: string;
	/** honeypot: a real person never fills it */
	website: string;
}

/** The body of POST /auth/email; `website` is always sent (normally ""). */
export function buildEmailBody(form: EmailLoginInput) {
	const name = form.name.trim();
	return { email: form.email, password: form.password, ...(name ? { name } : {}), website: form.website };
}

/** The backend answers a filled honeypot with 202 {success:true,message} and no token; such a reply is not a login. */
export function hasSession(payload: unknown): boolean {
	const data = (payload as { data?: { token?: unknown } } | null | undefined)?.data;
	return !!data && typeof data.token === 'string' && data.token !== '';
}

export const HONEYPOT_NEUTRAL_MESSAGE = 'Yêu cầu đã được ghi nhận. Vui lòng thử lại sau.';

export interface RegisterInput {
	name: string;
	email: string;
	password: string;
	/** honeypot: a real person never fills it */
	website: string;
	captchaToken: string;
}

/** The body of POST /auth/register: no role and no status, the server decides both (a pending member). */
export function buildRegisterBody(form: RegisterInput) {
	return { name: form.name.trim(), email: form.email, password: form.password, website: form.website, captchaToken: form.captchaToken };
}
