import { timingSafeEqual } from 'node:crypto';
import { createServer } from 'node:http';
import { SessionRegistry } from './registry.js';
import { ZaloSession } from './zalo.js';

const PORT = Number(process.env.PORT || 5100);
// Only this machine may reach the bridge unless HOST says otherwise (the Docker image sets 0.0.0.0, because the API
// runs in another container). The bridge can send Zalo messages as the connected account.
const HOST = process.env.HOST || '127.0.0.1';
const DEV_SECRET = 'development-bridge-secret';
const SECRET = process.env.BRIDGE_SECRET || DEV_SECRET;

// Anybody who knows the secret can send Zalo messages as the connected account: production must not run with the
// development default or a short one (same rule as the Go API).
if (process.env.NODE_ENV === 'production' || process.env.APP_ENV === 'production') {
	if (SECRET === DEV_SECRET || SECRET.toLowerCase().startsWith('replace-with') || SECRET.length < 32) {
		console.error('zalo-bridge: BRIDGE_SECRET must be a real secret of at least 32 characters in production (not the default or the .env.example placeholder).');
		process.exit(1);
	}
}
const API_URL = (process.env.API_URL || 'http://127.0.0.1:5000').replace(/\/$/, '');

async function postToApi(path, payload) {
	const response = await fetch(`${API_URL}${path}`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', 'X-Bridge-Secret': SECRET },
		body: JSON.stringify(payload),
		signal: AbortSignal.timeout(15000),
	});
	if (!response.ok) throw new Error(`API rejected ${path} (HTTP ${response.status})`);
}

// One live session per Zalo account, in this process's memory (rebuilt from the API's stored credentials after a restart).
const sessions = new SessionRegistry(
	(accountId) =>
		new ZaloSession({
			// Deliver inbound messages to the Go API (it stores them, runs the chatbot, and replies through /send).
			onMessage: (payload) => postToApi('/internal/zalo/inbound', { ...payload, accountId }),
			// After a QR login, or when Zalo renews the cookie, the API keeps the credentials to log in again later.
			onSession: (credentials) => postToApi('/internal/zalo/session', { ...credentials, accountId }),
		}),
);

function authorized(request) {
	const provided = Buffer.from(String(request.headers['x-bridge-secret'] || ''));
	const expected = Buffer.from(SECRET);
	return provided.length === expected.length && timingSafeEqual(provided, expected);
}

async function readJson(request, limit = 1024 * 1024) {
	const chunks = [];
	let size = 0;
	for await (const chunk of request) {
		size += chunk.length;
		if (size > limit) throw Object.assign(new Error('Payload too large'), { statusCode: 413 });
		chunks.push(chunk);
	}
	if (!chunks.length) return {};
	try {
		return JSON.parse(Buffer.concat(chunks).toString('utf8'));
	} catch {
		throw Object.assign(new Error('Invalid JSON'), { statusCode: 400 });
	}
}

const IMAGE_LIMIT = 4 * 1024 * 1024; // a picture of up to 2 MB, in base64

// Only a login (connect, QR) may create a session; anything else on an account the bridge does not hold is "not connected".
function live(accountId) {
	const session = sessions.peek(accountId);
	if (!session) throw Object.assign(new Error('Zalo cá nhân chưa kết nối.'), { statusCode: 409 }); // 4xx: nothing was sent
	return session;
}

// Every route works on the account named by `accountId` (body, or query string for GET); no id means the default account.
const routes = {
	'GET /status': async (_body, query) => sessions.statusOf(query.get('accountId')),
	'GET /accounts': async () => sessions.statuses(),
	'POST /connect': async (body) => {
		const session = sessions.get(body.accountId);
		await session.connect(body);
		return session.status();
	},
	'POST /qr/start': async (body) => sessions.get(body.accountId).startQr(body),
	'POST /disconnect': async (body) => sessions.get(body.accountId).disconnect(),
	'POST /forget': async (body) => {
		sessions.remove(body.accountId);
		return { forgotten: true };
	},
	'POST /find-user': async (body) => live(body.accountId).findUser(String(body.phone || '')),
	'POST /friend-status': async (body) => live(body.accountId).friendStatus(String(body.userId || '')),
	'POST /friend-request': async (body) => {
		await live(body.accountId).sendFriendRequest(String(body.userId || ''), body.message);
		return { sent: true };
	},
	'POST /react': async (body) => {
		await live(body.accountId).react(String(body.userId || ''), body.msgId, body.cliMsgId, body.icon);
		return { reacted: true };
	},
	'POST /send': async (body) => {
		await live(body.accountId).send(body.userId, String(body.text || ''));
		return { delivered: true };
	},
	'POST /send-image': async (body) => {
		await live(body.accountId).sendImage(body.userId, body);
		return { delivered: true };
	},
};

const server = createServer(async (request, response) => {
	const reply = (status, payload) => {
		response.writeHead(status, { 'Content-Type': 'application/json' });
		response.end(JSON.stringify(payload));
	};
	try {
		const url = new URL(request.url, 'http://bridge');
		const path = url.pathname;
		if (request.method === 'GET' && path === '/health') return reply(200, { ok: true });
		if (!authorized(request)) return reply(401, { message: 'Invalid bridge secret' });
		const handler = routes[`${request.method} ${path}`];
		if (!handler) return reply(404, { message: 'Not found' });
		// The API names the account in a header, so no payload has to change shape; a body or query value still wins.
		const account = String(request.headers['x-zalo-account'] || '');
		const body = request.method === 'POST' ? await readJson(request, path === '/send-image' ? IMAGE_LIMIT : undefined) : {};
		if (account && !body.accountId) body.accountId = account;
		if (account && !url.searchParams.get('accountId')) url.searchParams.set('accountId', account);
		reply(200, await handler(body, url.searchParams));
	} catch (error) {
		reply(error.statusCode || 500, { message: error instanceof Error ? error.message : String(error) });
	}
});

server.listen(PORT, HOST, () => console.log(`zalo-bridge listening on ${HOST}:${PORT}`));

for (const signal of ['SIGINT', 'SIGTERM']) {
	process.on(signal, () => {
		sessions.disconnectAll();
		server.close(() => process.exit(0));
	});
}
