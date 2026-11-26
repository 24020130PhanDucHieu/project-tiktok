import { useEffect, useRef } from 'react';

interface TurnstileApi {
	render: (el: HTMLElement, opts: { sitekey: string; callback: (token: string) => void; 'expired-callback': () => void; 'error-callback': () => void }) => string;
	remove: (id: string) => void;
}
declare global {
	interface Window {
		turnstile?: TurnstileApi;
	}
}

const SCRIPT_SRC = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';

function loadScript(): Promise<void> {
	if (window.turnstile) return Promise.resolve();
	return new Promise((resolve, reject) => {
		const existing = document.querySelector<HTMLScriptElement>(`script[src="${SCRIPT_SRC}"]`);
		const el = existing ?? Object.assign(document.createElement('script'), { src: SCRIPT_SRC, async: true });
		el.addEventListener('load', () => resolve());
		el.addEventListener('error', () => reject(new Error('turnstile')));
		if (!existing) document.head.appendChild(el);
	});
}

/** Cloudflare Turnstile widget: reports the token (or '' when it expires or fails) to onToken. */
export default function Turnstile({ siteKey, onToken }: { siteKey: string; onToken: (token: string) => void }) {
	const box = useRef<HTMLDivElement>(null);
	useEffect(() => {
		let id = '';
		let gone = false;
		loadScript()
			.then(() => {
				if (gone || !box.current || !window.turnstile) return;
				id = window.turnstile.render(box.current, {
					sitekey: siteKey,
					callback: onToken,
					'expired-callback': () => onToken(''),
					'error-callback': () => onToken(''),
				});
			})
			.catch(() => onToken(''));
		return () => {
			gone = true;
			if (id && window.turnstile) window.turnstile.remove(id);
		};
	}, [siteKey, onToken]);
	return <div ref={box} className="login-captcha" />;
}
