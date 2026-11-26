import type { AxiosAdapter, AxiosResponse, InternalAxiosRequestConfig } from 'axios';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DEMO_DENIED, isDemoRequest, resetDemoState, withDemo } from './demoApi';
import { DEMO_CREATORS, buildDemoConversations, buildDemoMessages } from './inboxData';

const cfg = (method: string, url: string, extra: Partial<InternalAxiosRequestConfig> = {}) => ({ method, url, headers: {}, ...extra }) as InternalAxiosRequestConfig;
const realOk = (config: InternalAxiosRequestConfig) => Promise.resolve({ data: 'REAL', status: 200, statusText: 'OK', headers: {}, config } as AxiosResponse);

describe('demo inbox adapter', () => {
	let real: ReturnType<typeof vi.fn>;
	let adapter: AxiosAdapter;
	beforeEach(() => {
		resetDemoState();
		real = vi.fn(realOk);
		adapter = withDemo(real as unknown as AxiosAdapter, () => true);
	});

	it('answers the areas the server refuses to a guest and never calls the real adapter', async () => {
		for (const url of ['/inbox/conversations', '/inbox/labels', '/zalo/accounts', '/zalo/status', '/gmail/status', '/automation']) {
			const res = await adapter(cfg('get', url));
			expect(res.status).toBe(200);
			expect(res.data.success).toBe(true);
		}
		expect(real).not.toHaveBeenCalled();
	});

	it('lists the demo conversations and their messages', async () => {
		const list = (await adapter(cfg('get', '/inbox/conversations'))).data.data;
		expect(list).toHaveLength(buildDemoConversations().length);
		const msgs = (await adapter(cfg('get', `/inbox/conversations/${list[0]._id}/messages`))).data.data;
		expect(msgs.length).toBeGreaterThan(0);
	});

	it('answers the friend check and the contacts of a demo KOC so the composer is usable', async () => {
		const friend = (await adapter(cfg('get', '/inbox/conversations/demo-c1/zalo/friend'))).data.data;
		expect(friend.isFriend).toBe(true);
		const contacts = (await adapter(cfg('get', '/kocs/demo-koc-1/contacts'))).data.data;
		expect(contacts.map((c: { type: string }) => c.type)).toEqual(['phone', 'email']);
		expect(real).not.toHaveBeenCalled();
	});

	it('a send is only appended to memory and a reset forgets it', async () => {
		const id = buildDemoConversations()[0]._id;
		await adapter(cfg('post', `/inbox/conversations/${id}/messages`, { data: JSON.stringify({ text: 'xin chào demo' }) }));
		const after = (await adapter(cfg('get', `/inbox/conversations/${id}/messages`))).data.data;
		expect(after.at(-1).text).toBe('xin chào demo');
		resetDemoState();
		const fresh = (await adapter(cfg('get', `/inbox/conversations/${id}/messages`))).data.data;
		expect(fresh).toHaveLength(buildDemoMessages(id).length);
		expect(real).not.toHaveBeenCalled();
	});

	it('refuses every other write with a 403 and the demo message, without touching the network', async () => {
		for (const [method, url] of [
			['put', '/inbox/conversations/demo-c1/labels'],
			['post', '/zalo/disconnect'],
			['delete', '/inbox/conversations/demo-c1'],
			['put', '/automation'],
			['post', '/zalo/qr/start'],
		]) {
			await expect(adapter(cfg(method, url))).rejects.toMatchObject({ response: { status: 403, data: { message: DEMO_DENIED } } });
		}
		expect(real).not.toHaveBeenCalled();
	});

	it('an unknown read of a blocked area is refused, not forwarded', async () => {
		await expect(adapter(cfg('get', '/inbox/something-new'))).rejects.toMatchObject({ response: { status: 403 } });
		expect(real).not.toHaveBeenCalled();
	});

	it('requests about a demo KOC never leave the browser, other requests pass through', async () => {
		expect(isDemoRequest({ url: '/kocs/demo-koc-1/collabs' })).toBe(true);
		expect(isDemoRequest({ url: '/kocs', })).toBe(false);
		await adapter(cfg('get', '/kocs/demo-koc-1/collabs'));
		expect(real).not.toHaveBeenCalled();
		const res = await adapter(cfg('get', '/kocs'));
		expect(res.data).toBe('REAL');
	});

	it('a non-guest is never answered by the demo', async () => {
		const asMember = withDemo(real as unknown as AxiosAdapter, () => false);
		const res = await asMember(cfg('get', '/inbox/conversations'));
		expect(res.data).toBe('REAL');
	});
});

describe('demo data', () => {
	it('uses only fictional contacts: reserved email domain and an unassigned phone range', () => {
		for (const c of DEMO_CREATORS) {
			expect(c.email).toMatch(/@demo\.invalid$/);
			expect(c.phone).toMatch(/^090000000\d$/);
			expect(c.id.startsWith('demo-')).toBe(true);
		}
		for (const c of buildDemoConversations()) expect(c._id.startsWith('demo-')).toBe(true);
	});
});
