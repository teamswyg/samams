import { useTaskTreeStore } from '../store/taskTreeStore';

const statusColorMap = {
  active: 'rgba(0,245,160,0.4)',
  complete: 'rgba(59,130,246,0.4)',
  pending: 'rgba(0,245,160,0.2)',
  error: 'rgba(239,68,68,0.4)',
};

export function CanvasEdges() {
  const edges = useTaskTreeStore((s) => s.edges);
  const selectedNodeId = useTaskTreeStore((s) => s.selectedNodeId);
  const hoveredNodeId = useTaskTreeStore((s) => s.hoveredNodeId);

  return (
    <svg
      style={{
        position: 'absolute',
        left: 0,
        top: 0,
        width: 1,
        height: 1,
        overflow: 'visible',
        pointerEvents: 'none',
      }}
    >
      <defs>
        <marker id="arrowhead" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
          <polygon points="0 0, 8 3, 0 6" fill="rgba(0,245,160,0.5)" />
        </marker>
        <marker id="arrowhead-active" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
          <polygon points="0 0, 8 3, 0 6" fill="#00F5A0" />
        </marker>
      </defs>
      {edges.map((edge) => {
        const isSelected = edge.to === selectedNodeId || edge.from === selectedNodeId;
        const isHovered = edge.to === hoveredNodeId || edge.from === hoveredNodeId;
        const midY = (edge.y1 + edge.y2) / 2;
        const d = `M ${edge.x1} ${edge.y1} C ${edge.x1} ${midY}, ${edge.x2} ${midY}, ${edge.x2} ${edge.y2}`;

        let stroke = statusColorMap[edge.status] || 'rgba(0,245,160,0.3)';
        let strokeWidth = 1;
        let dashArray = edge.status === 'pending' ? '6 4' : 'none';
        let marker = 'url(#arrowhead)';

        if (isSelected) {
          stroke = '#00F5A0';
          strokeWidth = 2;
          dashArray = 'none';
          marker = 'url(#arrowhead-active)';
        } else if (isHovered) {
          stroke = 'rgba(0,245,160,0.7)';
          strokeWidth = 1.5;
        }

        return (
          <g key={edge.id}>
            <path
              d={d}
              fill="none"
              stroke={stroke}
              strokeWidth={strokeWidth}
              strokeDasharray={dashArray}
              markerEnd={marker}
            />
            {edge.status === 'active' && !isSelected && (
              <circle r="3" fill="#00F5A0" opacity="0.8">
                <animateMotion dur="2s" repeatCount="indefinite" path={d} />
              </circle>
            )}
          </g>
        );
      })}
    </svg>
  );
}
