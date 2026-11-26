export const GUEST_TOOLTIP = 'Đăng nhập để chỉnh sửa';
export const GUEST_BANNER_TEXT = 'Bạn đang ở Chế độ Demo (chỉ xem). Đăng nhập để chỉnh sửa.';

/** Sections a guest cannot open (the API answers 403 for them). */
export const GUEST_HIDDEN_NAV = ['Hộp thư', 'Người dùng', 'Nhật ký', 'Hệ thống'];

export function isGuestUser(user: { role?: string } | null | undefined): boolean {
	return user?.role === 'guest';
}

export function isNavVisibleFor(label: string, role?: string): boolean {
	return !(role === 'guest' && GUEST_HIDDEN_NAV.includes(label));
}

/** False for guests and when there is no user: the single check for "may this user change data". */
export function canEdit(user: { role?: string } | null | undefined): boolean {
	return !!user && !isGuestUser(user);
}

/** Props that lock a control for guests (disabled + tooltip); spread onto a button. */
export function lockProps(readOnly: boolean): { disabled?: boolean; title?: string } {
	return readOnly ? { disabled: true, title: GUEST_TOOLTIP } : {};
}
