import { BaseEdge, EdgeLabelRenderer, type EdgeProps, getBezierPath, useReactFlow } from '@xyflow/react';

/** A wire with a small × in the middle: one click removes it (the Delete key removes a selected wire too). */
export default function WireEdge({ id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, markerEnd, selected }: EdgeProps) {
	const { deleteElements } = useReactFlow();
	const [path, labelX, labelY] = getBezierPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition });
	return (
		<>
			<BaseEdge id={id} path={path} markerEnd={markerEnd} interactionWidth={28} />
			<EdgeLabelRenderer>
				<button
					type="button" className={`wf-wire-x nodrag nopan${selected ? ' is-selected' : ''}`} aria-label="Xóa đường nối" title="Xóa đường nối"
					style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
					onClick={(e) => { e.stopPropagation(); void deleteElements({ edges: [{ id }] }); }}
				>×</button>
			</EdgeLabelRenderer>
		</>
	);
}
