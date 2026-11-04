import { useId, useMemo, useState } from 'react';

// Small dependency-free SVG charts (grouped bars and lines) with a hover tooltip and a clickable legend.

export interface ChartSeries {
	key: string;
	label: string;
	color: string;
	values: number[];
}

interface Props {
	labels: string[];
	series: ChartSeries[];
	height?: number;
	/** how a value is written in the tooltip */
	format?: (n: number) => string;
	/** how a value is written on the vertical axis */
	axisFormat?: (n: number) => string;
	empty?: string;
}

const W = 720;
const PAD = { l: 58, r: 14, t: 14, b: 30 };

export function fmtCompact(n: number): string {
	const a = Math.abs(n);
	const sign = n < 0 ? '-' : '';
	const trim = (v: number) => String(Math.round(v * 10) / 10).replace('.', ',');
	if (a >= 1e9) return `${sign}${trim(a / 1e9)} tỷ`;
	if (a >= 1e6) return `${sign}${trim(a / 1e6)} tr`;
	if (a >= 1e3) return `${sign}${trim(a / 1e3)}k`;
	return `${sign}${Math.round(a)}`;
}

function niceStep(raw: number) {
	if (raw <= 0) return 1;
	const pow = 10 ** Math.floor(Math.log10(raw));
	const f = raw / pow;
	return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * pow;
}

function useScale(series: ChartSeries[], hidden: Set<string>, height: number) {
	return useMemo(() => {
		const vals = series.filter((s) => !hidden.has(s.key)).flatMap((s) => s.values);
		let min = Math.min(0, ...vals);
		let max = Math.max(0, ...vals);
		if (max === min) max = min + 1;
		const step = niceStep((max - min) / 4);
		min = Math.floor(min / step) * step;
		max = Math.ceil(max / step) * step;
		const ticks: number[] = [];
		for (let v = min; v <= max + step / 2; v += step) ticks.push(v);
		const plotH = height - PAD.t - PAD.b;
		const y = (v: number) => PAD.t + plotH - ((v - min) / (max - min)) * plotH;
		return { min, max, ticks, y, hasData: vals.some((v) => v !== 0) };
	}, [series, hidden, height]);
}

function Frame({ labels, series, height = 240, format = fmtCompact, axisFormat = fmtCompact, empty = 'Chưa có dữ liệu trong khoảng này', kind }: Props & { kind: 'bar' | 'line' }) {
	const gid = useId().replace(/:/g, '');
	const [hidden, setHidden] = useState<Set<string>>(new Set());
	const [hover, setHover] = useState<number | null>(null);
	const { ticks, y, hasData } = useScale(series, hidden, height);
	const n = Math.max(labels.length, 1);
	const plotW = W - PAD.l - PAD.r;
	const band = plotW / n;
	const cx = (i: number) => PAD.l + band * i + band / 2;
	const shown = series.filter((s) => !hidden.has(s.key));
	const every = Math.ceil(n / 10);
	const zeroY = y(0);

	const toggle = (key: string) =>
		setHidden((prev) => {
			const next = new Set(prev);
			if (next.has(key)) next.delete(key);
			else if (next.size < series.length - 1) next.add(key); // always keep one series visible
			return next;
		});

	return (
		<div className="chart">
			<div className="chart-legend">
				{series.map((s) => (
					<button key={s.key} type="button" className={hidden.has(s.key) ? 'off' : ''} onClick={() => toggle(s.key)} title="Bấm để ẩn/hiện">
						<i style={{ background: s.color }} />
						{s.label}
					</button>
				))}
			</div>
			<div className="chart-box" onMouseLeave={() => setHover(null)}>
				<svg viewBox={`0 0 ${W} ${height}`} role="img" aria-label="Biểu đồ">
					<defs>
						{shown.map((s) => (
							<linearGradient key={s.key} id={`${gid}-${s.key}`} x1="0" y1="0" x2="0" y2="1">
								<stop offset="0%" stopColor={s.color} stopOpacity="0.22" />
								<stop offset="100%" stopColor={s.color} stopOpacity="0" />
							</linearGradient>
						))}
					</defs>
					{ticks.map((t) => (
						<g key={t}>
							<line x1={PAD.l} x2={W - PAD.r} y1={y(t)} y2={y(t)} className={t === 0 ? 'chart-zero' : 'chart-grid'} />
							<text x={PAD.l - 8} y={y(t) + 4} textAnchor="end" className="chart-axis">{axisFormat(t)}</text>
						</g>
					))}
					{labels.map((l, i) =>
						i % every === 0 ? (
							<text key={`${l}-${i}`} x={cx(i)} y={height - 9} textAnchor="middle" className="chart-axis">{l}</text>
						) : null,
					)}

					{kind === 'bar' &&
						shown.map((s, si) => {
							const bw = Math.min(26, (band * 0.7) / shown.length);
							return s.values.map((v, i) => {
								const x = cx(i) - (bw * shown.length) / 2 + si * bw;
								const top = Math.min(y(v), zeroY);
								const h = Math.max(Math.abs(y(v) - zeroY), v === 0 ? 0 : 1.5);
								return <rect key={`${s.key}-${i}`} x={x + 1} y={top} width={Math.max(bw - 2, 1)} height={h} rx={3} fill={s.color} opacity={hover === null || hover === i ? 1 : 0.45} />;
							});
						})}

					{kind === 'line' &&
						shown.map((s, si) => {
							const pts = s.values.map((v, i) => `${cx(i)},${y(v)}`);
							return (
								<g key={s.key}>
									{si === 0 && n > 1 && <polygon points={`${cx(0)},${zeroY} ${pts.join(' ')} ${cx(n - 1)},${zeroY}`} fill={`url(#${gid}-${s.key})`} />}
									<polyline points={pts.join(' ')} fill="none" stroke={s.color} strokeWidth={2.4} strokeLinejoin="round" strokeLinecap="round" />
									{s.values.map((v, i) => (
										<circle key={i} cx={cx(i)} cy={y(v)} r={hover === i ? 4.5 : n > 24 ? 0 : 2.6} fill="#fff" stroke={s.color} strokeWidth={2} />
									))}
								</g>
							);
						})}

					{hover !== null && <line x1={cx(hover)} x2={cx(hover)} y1={PAD.t} y2={height - PAD.b} className="chart-cursor" />}
					{labels.map((_, i) => (
						<rect key={i} x={PAD.l + band * i} y={PAD.t} width={band} height={height - PAD.t - PAD.b} fill="transparent" onMouseEnter={() => setHover(i)} onMouseMove={() => setHover(i)} />
					))}
				</svg>

				{hover !== null && (
					<div className="chart-tip" style={{ left: `${(cx(hover) / W) * 100}%` }}>
						<b>{labels[hover]}</b>
						{shown.map((s) => (
							<span key={s.key}>
								<i style={{ background: s.color }} />
								{s.label}: <strong>{format(s.values[hover] ?? 0)}</strong>
							</span>
						))}
					</div>
				)}
				{!hasData && <div className="chart-empty">{empty}</div>}
			</div>
		</div>
	);
}

export function BarChart(props: Props) {
	return <Frame {...props} kind="bar" />;
}

export function LineChart(props: Props) {
	return <Frame {...props} kind="line" />;
}
