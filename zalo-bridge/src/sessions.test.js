import assert from 'node:assert/strict';
import { test } from 'node:test';
import toughCookie from 'tough-cookie';
import { SessionRegistry } from './registry.js';
import { createSessions } from './sessions.js';

// ---- the registry can rename a session

const fake = () => ({ gone: false, status: () => ({ state: 'connected', qrImage: '', error: '' }), disconnect() { this.gone = true; } });

test('rekey moves a session to another key', () => {
	const reg = new SessionRegistry(() => fake());
	const s = reg.get('default');
	assert.equal(reg.rekey('default', 'X'), true);
	assert.equal(reg.peek('X'), s);
	assert.equal(reg.peek('default'), undefined);
	assert.deepEqual(Object.keys(reg.statuses()), ['X']);
	assert.equal(s.gone, false, 'the session that was renamed must keep running');
});

test('rekey to the same key, or of a session that does not exist, changes nothing', () => {
	const reg = new SessionRegistry(() => fake());
	reg.get('A');
	assert.equal(reg.rekey('A', 'A'), false);
	assert.equal(reg.rekey('nobody', 'B'), false);
	assert.deepEqual(Object.keys(reg.statuses()), ['A']);
});

test('rekey onto a key that already holds a session replaces it and stops the old one', () => {
	const reg = new SessionRegistry(() => fake());
	const fresh = reg.get('default');
	const stale = reg.get('X');
	assert.equal(reg.rekey('default', 'X'), true);
	assert.equal(reg.peek('X'), fresh, 'the session that just logged in must win');
	assert.equal(stale.gone, true, 'the replaced session was left listening: two listeners for one account');
	assert.equal(reg.size, 1);
});

// ---- a login under a temporary key follows the account id the API answers with

const jarWith = (value) => {
	const jar = new toughCookie.CookieJar();
	jar.setCookieSync(toughCookie.Cookie.fromJSON({ key: 'k', value, domain: 'chat.zalo.me', path: '/' }), 'https://chat.zalo.me');
	return jar;
};
const fakeApi = () => ({
	getCookie: () => jarWith('v1'),
	getContext: () => ({ imei: 'i', userAgent: 'u', uid: '777' }),
	listener: { on() {}, start() {}, stop() {} },
});
const qrClient = () => ({ loginQR: async () => fakeApi(), login: async () => fakeApi() });

function setup(reply) {
	const posts = [];
	const postToApi = async (path, payload) => {
		posts.push({ path, payload });
		if (reply instanceof Error) throw reply;
		return path === '/internal/zalo/session' ? reply : {};
	};
	return { posts, sessions: createSessions({ postToApi, createClient: qrClient }) };
}

test('a QR login that had no account to name is renamed to the id the API created for it', async () => {
	const { sessions, posts } = setup({ success: true, accountId: '6b03aaaa' });
	const session = sessions.get(undefined); // the old single-account flow: no id, the key is "default"
	session.startQr({});
	await session.qrLoginPromise;

	assert.deepEqual(Object.keys(sessions.statuses()), ['6b03aaaa'], 'a session was left under the temporary key');
	assert.equal(sessions.peek('6b03aaaa'), session);
	assert.equal(posts[0].payload.accountId, 'default', 'the first report names the key it was made under');
	// what the session reports from now on carries the real id
	await session.onMessage({ externalContactId: 'u1', text: 'hi' });
	assert.equal(posts[1].path, '/internal/zalo/inbound');
	assert.equal(posts[1].payload.accountId, '6b03aaaa');
});

test('the API folding the login into an account that already exists renames it to that account', async () => {
	const { sessions } = setup({ success: true, accountId: 'existing' });
	const session = sessions.get('placeholder');
	session.startQr({});
	await session.qrLoginPromise;
	assert.deepEqual(Object.keys(sessions.statuses()), ['existing']);
});

test('the session the API logged in under that id in the meantime is replaced, not kept beside the new one', async () => {
	const { sessions } = setup({ success: true, accountId: 'X' });
	const rival = sessions.get('X');
	let rivalStopped = false;
	rival.disconnect = () => { rivalStopped = true; };
	const session = sessions.get('default');
	session.startQr({});
	await session.qrLoginPromise;
	assert.equal(sessions.peek('X'), session);
	assert.equal(rivalStopped, true);
});

test('nothing is renamed when the API names the same id, names none, or fails', async () => {
	for (const reply of [{ success: true, accountId: 'A' }, { success: true }, {}, null, new Error('api down')]) {
		const { sessions } = setup(reply);
		const session = sessions.get('A');
		session.startQr({});
		await session.qrLoginPromise;
		assert.deepEqual(Object.keys(sessions.statuses()), ['A'], `reply ${reply instanceof Error ? 'error' : JSON.stringify(reply)} moved the session`);
		assert.equal(sessions.peek('A'), session);
	}
});
