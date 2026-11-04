/** Small down-arrow used inside dropdown-style buttons (replaces the "⌄" text glyph, which renders as a tiny low "v"). */
export default function Chevron({ size = 13 }: { size?: number }) {
	return (
		<svg className="chev" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
			<polyline points="6 9 12 15 18 9" />
		</svg>
	);
}
