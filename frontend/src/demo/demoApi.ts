import type { AxiosAdapter, AxiosResponse, InternalAxiosRequestConfig } from 'axios';
import { AxiosError } from 'axios';
import type { ChatMessage, ConversationItem } from '../types/koc';
import { DEMO_CREATORS, DEMO_ID_PREFIX, DEMO_LABELS, DEMO_ZALO_ACCOUNT, buildDemoConversations, buildDemoMessages } from './inboxData';

export const DEMO_DENIED = 'Chế độ Demo: thao tác này chỉ để xem, không được thực hiện thật. Đăng nhập để dùng đầy đủ.';

/** The areas the server refuses to a guest: for a guest they are answered here with made-up data instead. */
const SIMULATED_AREAS = ['/inbox', '/zalo', '/gmail', '/automation'];

// the demo "server" keeps its state in memory only: reloading the page resets it
let conversations: ConversationItem[] | null = null;
const sent = new Map<string, ChatMessage[]>();

export function resetDemoState(): void {
	conversations = null;
	sent.clear();
}

/** The made-up phone and email of one demo KOC, in the shape of the contacts list. */
function demoContacts(kocId: string) {
	const koc = DEMO_CREATORS.find((c) => c.id === kocId);
	if (!koc) return [];
	const at = new Date().toISOString();
	return [
		{ id: `${kocId}-phone`, type: 'phone', value: koc.phone, source: 'demo', unreachable: false, updatedAt: at },
		{ id: `${kocId}-email`, type: 'email', value: koc.email, source: 'demo', unreachable: false, updatedAt: at },
	];
}

function convs(): ConversationItem[] {
	if (!conversations) conversations = buildDemoConversations();
	return conversations;
}

export function pathOf(config: Pick<InternalAxiosRequestConfig, 'url'>): string {
	return (config.url || '').split('?')[0].replace(/^\/+/, '/');
}

/** True when the request belongs to the simulated inbox and so must be answered here and never reach the network. */
export function isDemoRequest(config: Pick<InternalAxiosRequestConfig, 'url'>): boolean {
	const p = pathOf(config);
	return p.includes(`/${DEMO_ID_PREFIX}`) || SIMULATED_AREAS.some((a) => p === a || p.startsWith(`${a}/`));
}

function reply(config: InternalAxiosRequestConfig, status: number, data: unknown): AxiosResponse {
	return { data, status, statusText: String(status), headers: {}, config };
}

function deny(config: InternalAxiosRequestConfig): Promise<never> {
	const response = reply(config, 403, { success: false, message: DEMO_DENIED });
	return Promise.reject(new AxiosError(DEMO_DENIED, 'ERR_BAD_REQUEST', config, null, response));
}

function bodyOf(config: InternalAxiosRequestConfig): Record<string, unknown> {
	if (typeof config.data !== 'string') return (config.data as Record<string, unknown>) || {};
	try {
		return JSON.parse(config.data || '{}');
	} catch {
		return {};
	}
}

/** Answers one request of the simulated inbox. Anything it does not know is refused: it must never fall through to the server. */
export function handleDemo(config: InternalAxiosRequestConfig): Promise<AxiosResponse> {
	const method = (config.method || 'get').toLowerCase();
	const p = pathOf(config);
	const ok = (data: unknown) => Promise.resolve(reply(config, 200, { success: true, data }));
	let m: RegExpMatchArray | null;

	if (method === 'get') {
		if (p === '/inbox/conversations') return ok(config.params?.hidden ? convs().filter((c) => c.hidden) : convs().filter((c) => !c.hidden));
		if ((m = p.match(/^\/inbox\/conversations\/([^/]+)\/messages$/))) return ok([...buildDemoMessages(m[1]), ...(sent.get(m[1]) ?? [])]);
		if (p === '/inbox/labels') return ok(DEMO_LABELS);
		if (p === '/inbox/templates') return ok([]);
		if (p === '/automation') return ok({ enabled: false });
		if (p === '/zalo/status') return ok({ configured: true, connected: true, state: 'connected', bridgeOnline: true });
		if (p === '/zalo/accounts') return ok([DEMO_ZALO_ACCOUNT]);
		if (p === '/gmail/status') return ok({ configured: false, connected: false, broken: false, email: '' });
		if ((m = p.match(/^\/inbox\/conversations\/[^/]+\/zalo\/friend$/))) return ok({ available: true, isFriend: true, requestSent: false, requestReceived: false, noteMax: 120 });
		if ((m = p.match(/^\/kocs\/([^/]+)\/contacts$/))) return ok(demoContacts(m[1]));
		if (p.includes(`/${DEMO_ID_PREFIX}`)) return ok([]); // reads about one demo KOC (collabs…): nothing yet
		return deny(config);
	}
	if (method === 'post' && (m = p.match(/^\/inbox\/conversations\/([^/]+)\/messages$/))) {
		const id = m[1];
		const msg: ChatMessage = {
			_id: `${id}-s${Date.now()}`,
			conversationId: id,
			direction: 'outbound',
			text: String(bodyOf(config).text ?? ''),
			sentAt: new Date().toISOString(),
			status: 'Đã gửi (demo)',
			channel: 'zalo',
		};
		sent.set(id, [...(sent.get(id) ?? []), msg]);
		conversations = convs().map((c) => (c._id === id ? { ...c, lastMessage: msg.text, lastMessageAt: msg.sentAt } : c));
		return ok(msg);
	}
	if (method === 'post' && (m = p.match(/^\/inbox\/conversations\/([^/]+)\/read$/))) {
		const id = m[1];
		conversations = convs().map((c) => (c._id === id ? { ...c, unreadCount: 0 } : c));
		return ok({});
	}
	return deny(config);
}

/** Wraps the real adapter: guests get the simulated inbox, everyone else is untouched. */
export function withDemo(real: AxiosAdapter, isGuest: () => boolean): AxiosAdapter {
	return (config) => (isGuest() && isDemoRequest(config) ? handleDemo(config) : real(config));
}
