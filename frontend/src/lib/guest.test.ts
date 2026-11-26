import { describe, expect, it } from 'vitest';
import { canEdit, GUEST_TOOLTIP, isGuestUser, isNavVisibleFor, lockProps } from './guest';

describe('guest helpers', () => {
	it('detects guests', () => {
		expect(isGuestUser({ role: 'guest' })).toBe(true);
		expect(isGuestUser({ role: 'member' })).toBe(false);
		expect(isGuestUser(null)).toBe(false);
	});
	it('hides inbox and admin nav from guests only', () => {
		expect(isNavVisibleFor('Hộp thư', 'guest')).toBe(false);
		expect(isNavVisibleFor('Quản lý KOC', 'guest')).toBe(true);
		expect(isNavVisibleFor('Hộp thư', 'member')).toBe(true);
	});
	it('canEdit is false for guests and missing users', () => {
		expect(canEdit({ role: 'guest' })).toBe(false);
		expect(canEdit(null)).toBe(false);
		expect(canEdit(undefined)).toBe(false);
		expect(canEdit({ role: 'member' })).toBe(true);
		expect(canEdit({ role: 'admin' })).toBe(true);
	});
	it('lockProps disables with tooltip only when read-only', () => {
		expect(lockProps(true)).toEqual({ disabled: true, title: GUEST_TOOLTIP });
		expect(lockProps(false)).toEqual({});
	});
});
