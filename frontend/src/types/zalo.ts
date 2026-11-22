/** One personal Zalo account the app can send from, as GET /zalo/accounts returns it: who it is and whether it is logged in. */
export interface ZaloAccount {
	_id: string;
	label: string;
	zaloUid?: string;
	displayName?: string;
	/** conversations made before there were several accounts belong to it */
	isDefault: boolean;
	/** a person disconnected it on purpose: it stays out until somebody logs it in again */
	disabled: boolean;
	state: string;
	connected: boolean;
	qrImage?: string;
	error?: string;
	bridgeOnline: boolean;
	/** it can log in again by itself (no QR code needed) */
	hasCredentials: boolean;
	needsQr: boolean;
}
