import type { ReactNode } from 'react';
import { createPortal } from 'react-dom';

/**
 * Renders children straight into <body>. A fixed-position window drawn inside a page is positioned against the
 * nearest ancestor that has a transform / contain / scroll box, not the screen, so it can end up shifted or cut off
 * (the page transitions and scrolling areas of this app do that). Windows and drawers go through here.
 */
export default function Portal({ children }: { children: ReactNode }) {
	return createPortal(children, document.body);
}
