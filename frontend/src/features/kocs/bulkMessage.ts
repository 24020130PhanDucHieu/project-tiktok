// Who can get a bulk message: every KOC of the list as a recipient (blocked, unticked or ticked) and the search over
// them (pure, no network). The steps of the message are in features/inbox/campaignSteps.ts; the window is
// features/inbox/BulkMessageModal.tsx.

import type { Creator } from '../../types/koc';

export const MSG_RECENT_DAYS = 7; // a KOC contacted within this many days starts unticked

export interface Recipient {
	creator: Creator;
	checked: boolean;
	/** why this KOC cannot be sent to: shown greyed out, cannot be ticked */
	blocked?: string;
	/** contacted this many days ago (inside MSG_RECENT_DAYS): starts unticked but can be ticked */
	recentDaysAgo?: number;
}

const DAY = 24 * 3600 * 1000;

/** Every KOC of the list as a recipient: blocked ones cannot be ticked, recently contacted ones start unticked. */
export function buildRecipients(creators: Creator[], now: Date = new Date()): Recipient[] {
	return creators.map((creator) => {
		const last = creator.lastContactedAt ? new Date(creator.lastContactedAt).getTime() : NaN;
		const age = Number.isNaN(last) ? Infinity : now.getTime() - last;
		let blocked: string | undefined;
		if (creator.dnc) blocked = 'Không liên hệ';
		else if (creator.daHopTac) blocked = 'Đã hợp tác';
		else if (!creator.phone?.trim()) blocked = 'Chưa có số điện thoại';
		else if (age < DAY) blocked = 'Vừa liên hệ trong 24 giờ';
		if (blocked) return { creator, checked: false, blocked };
		if (age < MSG_RECENT_DAYS * DAY) return { creator, checked: false, recentDaysAgo: Math.floor(age / DAY) };
		return { creator, checked: true };
	});
}

/** lowercase, no accents: "Nguyễn" and "nguyen" are the same text */
const plain = (s: string) => s.normalize('NFD').replace(/[\u0300-\u036f]/g, '').replace(/đ/g, 'd').replace(/Đ/g, 'd').toLowerCase();

/** The recipients whose name, handle or phone contains the query (accents and case do not matter); an empty query keeps everyone. */
export function filterRecipients(recipients: Recipient[], query: string): Recipient[] {
	const q = plain(query.trim());
	if (!q) return recipients;
	return recipients.filter((r) => plain(`${r.creator.name} ${r.creator.handle ?? ''} ${r.creator.phone ?? ''}`).includes(q));
}
