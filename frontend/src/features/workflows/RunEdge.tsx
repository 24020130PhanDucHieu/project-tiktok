import { BaseEdge, type EdgeProps, getBezierPath } from '@xyflow/react';
import type { EdgeFlow } from './flow';

/** A wire of a run: when KOCs are on their way, small dots travel along it. */
export default function RunEdge({ id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, markerEnd, data }: EdgeProps) {
	const [path] = getBezierPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition });
	const flow = ((data as { flow?: EdgeFlow } | undefined)?.flow ?? 'idle');
	return (
		<>
			<BaseEdge id={id} path={path} markerEnd={markerEnd} className={`wf-edge-${flow}`} />
			{flow === 'active' && [0, 0.53, 1.06].map((delay) => (
				<circle key={delay} r="4.5" className="wf-edge-dot">
					<animateMotion dur="1.6s" begin={`${delay}s`} repeatCount="indefinite" path={path} />
				</circle>
			))}
		</>
	);
}
