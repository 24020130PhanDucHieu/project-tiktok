import assert from 'node:assert/strict';
import { test } from 'node:test';
import toughCookie from 'tough-cookie';
import { DEFAULT_ACCOUNT, SessionRegistry } from './registry.js';
import { ZaloSession } from './zalo.js';

const fakeSession = (accountId) => ({ accountId, status: () => ({ state: 'connected', qrImage: '', error: '' }), disconnect() { this.gone = true; } });

test('the same account id always gives the same session', () => {
	const reg = new SessionRegistry((id) => fakeSession(id));
	assert.equal(reg.get('A'), reg.get('A'));
	assert.notEqual(reg.get('A'), reg.get('B'));
});

test('a missing account id is the default account', () => {
	const reg = new SessionRegistry((id) => fakeSession(id));
	assert.equal(reg.get(undefined), reg.get(DEFAULT_ACCOUNT));
	assert.equal(reg.get('').accountId, DEFAULT_ACCOUNT);
});

test('peek never creates a session', () => {
	const reg = new SessionRegistry((id) => fakeSession(id));
	assert.equal(reg.peek('A'), undefined);
	assert.equal(reg.size, 0);
});

test('statuses lists every session by account id', () => {
	const reg = new SessionRegistry((id) => fakeSession(id));
	reg.get('A');
	reg.get('B');
	assert.deepEqual(Object.keys(reg.statuses()).sort(), ['A', 'B']);
	assert.equal(reg.statuses().A.state, 'connected');
});

test('remove disconnects the session and forgets it', () => {
	const reg = new SessionRegistry((id) => fakeSession(id));
	const s = reg.get('A');
	reg.remove('A');
	assert.equal(s.gone, true);
	assert.equal(reg.peek('A'), undefined);
});

test('an account that is not in the map reads as disconnected', () => {
	const reg = new SessionRegistry((id) => fakeSession(id));
	assert.deepEqual(reg.statusOf('nobody'), { state: 'disconnected', qrImage: '', error: '' });
	assert.equal(reg.size, 0); // asking for a status must not create a session
});

test('exported credentials log in again: the cookie survives a JSON round trip', () => {
	const jar = new toughCookie.CookieJar();
	jar.setCookieSync(toughCookie.Cookie.fromJSON({ key: 'zpw_sek', value: 'secret', domain: 'chat.zalo.me', path: '/', hostOnly: false }), 'https://chat.zalo.me');
	const api = { getCookie: () => jar, getContext: () => ({ imei: 'imei-1', userAgent: 'UA/1', uid: '123' }) };
	const exported = ZaloSession.exportCredentials(api);
	assert.equal(exported.imei, 'imei-1');
	assert.equal(exported.userAgent, 'UA/1');
	assert.equal(exported.zaloUid, '123');
	const stored = JSON.parse(JSON.stringify(exported.cookie)); // what MongoDB hands back
	assert.ok(Array.isArray(ZaloSession.parseCookie(stored)));
	assert.equal(stored[0].key, 'zpw_sek');
});

test('credentials report whether the cookie changed since login', () => {
	const jar = new toughCookie.CookieJar();
	jar.setCookieSync(toughCookie.Cookie.fromJSON({ key: 'k', value: 'v1', domain: 'chat.zalo.me', path: '/' }), 'https://chat.zalo.me');
	const api = { getCookie: () => jar, getContext: () => ({ imei: 'i', userAgent: 'u', uid: '1' }) };
	const before = ZaloSession.exportCredentials(api);
	assert.equal(ZaloSession.sameCookie(before.cookie, ZaloSession.exportCredentials(api).cookie), true);
	jar.setCookieSync(toughCookie.Cookie.fromJSON({ key: 'k', value: 'v2', domain: 'chat.zalo.me', path: '/' }), 'https://chat.zalo.me');
	assert.equal(ZaloSession.sameCookie(before.cookie, ZaloSession.exportCredentials(api).cookie), false);
});

const jarWith = (value) => {
	const jar = new toughCookie.CookieJar();
	jar.setCookieSync(toughCookie.Cookie.fromJSON({ key: 'k', value, domain: 'chat.zalo.me', path: '/' }), 'https://chat.zalo.me');
	return jar;
};
const fakeApi = (jar) => ({
	getCookie: () => jar,
	getContext: () => ({ imei: 'i', userAgent: 'u', uid: '1' }),
	listener: { on() {}, start() {}, stop() {} },
});

test('a QR login always reports its credentials so the API can store them', async () => {
	const reported = [];
	const s = new ZaloSession({ onMessage() {}, onSession: (c) => reported.push(c) });
	await s.adopt(fakeApi(jarWith('v1')), { always: true });
	assert.equal(reported.length, 1);
	assert.equal(reported[0].zaloUid, '1');
});

test('a cookie login reports only when Zalo changed the cookie', async () => {
	const reported = [];
	const s = new ZaloSession({ onMessage() {}, onSession: (c) => reported.push(c) });
	const sent = ZaloSession.exportCredentials(fakeApi(jarWith('v1'))).cookie;
	await s.adopt(fakeApi(jarWith('v1')), { sentCookie: sent });
	assert.equal(reported.length, 0);
	await s.adopt(fakeApi(jarWith('v2')), { sentCookie: sent });
	assert.equal(reported.length, 1);
});

test('a failing report never breaks the login', async () => {
	const s = new ZaloSession({ onMessage() {}, onSession: () => { throw new Error('api down'); } });
	await s.adopt(fakeApi(jarWith('v1')), { always: true });
	assert.equal(s.status().state, 'connected');
});

test('the same cookie created at another moment is not a changed cookie', () => {
	const at = (when) => [{ key: 'k', value: 'v', domain: 'chat.zalo.me', path: '/', hostOnly: false, creation: when, lastAccessed: when }];
	assert.equal(ZaloSession.sameCookie(at('2026-01-01T00:00:00.000Z'), at('2026-06-01T12:00:00.000Z')), true);
});

test('another value, domain, path or expiry is a changed cookie', () => {
	const base = { key: 'k', value: 'v', domain: 'chat.zalo.me', path: '/', expires: '2027-01-01T00:00:00.000Z' };
	for (const change of [{ value: 'w' }, { domain: 'zalo.me' }, { path: '/x' }, { expires: '2028-01-01T00:00:00.000Z' }]) {
		assert.equal(ZaloSession.sameCookie([base], [{ ...base, ...change }]), false, JSON.stringify(change));
	}
});

test('the order of the cookies does not matter', () => {
	const a = { key: 'a', value: '1', domain: 'd', path: '/' };
	const b = { key: 'b', value: '2', domain: 'd', path: '/' };
	assert.equal(ZaloSession.sameCookie([a, b], [b, a]), true);
});
