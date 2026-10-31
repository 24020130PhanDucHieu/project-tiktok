import { timingSafeEqual } from 'node:crypto';
import { createServer } from 'node:http';
import { ZaloSession } from './zalo.js';

const PORT = Number(process.env.PORT || 5100);
const SECRET = process.env.BRIDGE_SECRET || 'development-bridge-secret';
const API_URL = (process.env.API_URL || 'http://127.0.0.1:5000').replace(/\/$/, '');

const session = new ZaloSession({
	// Deliver inbound messages to the Go API (it stores them, runs the chatbot, and replies through /send).
	onMessage: async (payload) => {
		const response = await fetch(`${API_URL}/internal/zalo/inbound`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', 'X-Bridge-Secret': SECRET },
			body: JSON.stringify(payload),
			signal: AbortSignal.timeout(15000),
		});
		if (!response.ok) throw new Error(`API rejected inbound message (HTTP ${response.status})`);
	},
});

function authorized(request) {
	const provided = Buffer.from(String(request.headers['x-bridge-secret'] || ''));
	const expected = Buffer.from(SECRET);
	return provided.length === expected.length && timingSafeEqual(provided, expected);
}

async function readJson(request) {
	const chunks = [];
	let size = 0;
	for await (const chunk of request) {
		size += chunk.length;
		if (size > 1024 * 1024) throw Object.assign(new Error('Payload too large'), { statusCode: 413 });
		chunks.push(chunk);
	}
	if (!chunks.length) return {};
	try {
		return JSON.parse(Buffer.concat(chunks).toString('utf8'));
	} catch {
		throw Object.assign(new Error('Invalid JSON'), { statusCode: 400 });
	}
}

const routes = {
	'GET /status': async () => session.status(),
	'POST /connect': async (body) => {
		await session.connect(body);
		return session.status();
	},
	'POST /qr/start': async (body) => session.startQr(body),
	'POST /disconnect': async () => session.disconnect(),
	'POST /find-user': async (body) => session.findUser(String(body.phone || '')),
	'POST /send': async (body) => {
		await session.send(body.userId, String(body.text || ''));
		return { delivered: true };
	},
};

const server = createServer(async (request, response) => {
	const reply = (status, payload) => {
		response.writeHead(status, { 'Content-Type': 'application/json' });
		response.end(JSON.stringify(payload));
	};
	try {
		const path = new URL(request.url, 'http://bridge').pathname;
		if (request.method === 'GET' && path === '/health') return reply(200, { ok: true });
		if (!authorized(request)) return reply(401, { message: 'Invalid bridge secret' });
		const handler = routes[`${request.method} ${path}`];
		if (!handler) return reply(404, { message: 'Not found' });
		reply(200, await handler(request.method === 'POST' ? await readJson(request) : {}));
	} catch (error) {
		reply(error.statusCode || 500, { message: error instanceof Error ? error.message : String(error) });
	}
});

server.listen(PORT, () => console.log(`zalo-bridge listening on :${PORT}`));

for (const signal of ['SIGINT', 'SIGTERM']) {
	process.on(signal, () => {
		session.disconnect();
		server.close(() => process.exit(0));
	});
}
