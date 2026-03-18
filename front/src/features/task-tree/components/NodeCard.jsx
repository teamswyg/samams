import { useTaskTreeStore } from '../store/taskTreeStore';
import styles from './NodeCard.module.css';

const statusColorMap = {
  active: '#00F5A0',
  complete: '#3B82F6',
  pending: '#F59E0B',
  error: '#EF4444',
  reviewing: '#A78BFA',
  paused: '#6B7280',
};

const agentColorMap = (agent) => {
  const lower = agent.toLowerCase();
  if (lower.includes('cursor')) return '#00F5A0';
  if (lower.includes('claude')) return '#F59E0B';
  if (lower.includes('opencode')) return '#3B82F6';
  if (lower === 'system' || lower === 'unassigned') return '#6B7280';
  return '#00F5A0';
};

const priorityColorMap = {
  high: '#EF4444',
  medium: '#F59E0B',
  low: '#3B82F6',
};

const typeConfig = {
  proposal: { label: 'PROPOSAL', color: '#34D399', icon: '◆', width: 320, height: 120 },
  milestone: { label: 'MILESTONE', color: '#FBBF24', icon: '◈', width: 300, height: 115 },
  task: { label: 'TASK', color: '#60A5FA', icon: '▸', width: 280, height: 110 },
};

export function NodeCard({ node, position, index }) {
  const selectedNodeId = useTaskTreeStore((s) => s.selectedNodeId);
  const hoveredNodeId = useTaskTreeStore((s) => s.hoveredNodeId);
  const selectNode = useTaskTreeStore((s) => s.selectNode);
  const setHoveredNode = useTaskTreeStore((s) => s.setHoveredNode);

  const isSelected = selectedNodeId === node.id;
  const isHovered = hoveredNodeId === node.id;
  const nodeType = node.type || 'task';
  const tc = typeConfig[nodeType] || typeConfig.task;
  const color = statusColorMap[node.status] || '#00F5A0';
  const aColor = agentColorMap(node.agent);
  const pColor = priorityColorMap[node.priority] || '#F59E0B';

  let borderColor = `${tc.color}25`;
  let boxShadow = '0 2px 12px rgba(0,0,0,0.4)';
  if (isSelected) {
    borderColor = tc.color;
    boxShadow = `0 0 0 2px ${tc.color}40, 0 0 30px ${tc.color}30, 0 8px 32px rgba(0,0,0,0.6)`;
  } else if (isHovered) {
    borderColor = `${tc.color}60`;
    boxShadow = `0 0 20px ${tc.color}20, 0 4px 16px rgba(0,0,0,0.5)`;
  }

  const bgMap = {
    proposal: 'linear-gradient(135deg, #0d2818 0%, #111827 100%)',
    milestone: 'linear-gradient(135deg, #1a1708 0%, #111827 100%)',
    task: 'linear-gradient(135deg, #0d1a2a 0%, #111827 100%)',
  };

  return (
    <div
      className={styles.card}
      style={{
        position: 'absolute',
        left: position.x,
        top: position.y,
        width: tc.width,
        height: tc.height,
        background: bgMap[nodeType] || bgMap.task,
        borderColor,
        boxShadow,
      }}
      onClick={(e) => { e.stopPropagation(); selectNode(node.id); }}
      onMouseEnter={() => setHoveredNode(node.id)}
      onMouseLeave={() => setHoveredNode(null)}
    >
      <div className={styles.top}>
        <div className={styles.topLeft}>
          <span className={styles.typeBadge} style={{ color: tc.color, background: `${tc.color}1A` }}>
            {tc.icon} {tc.label}
          </span>
          <span className={styles.uid}>{node.uid}</span>
          {node.origin === 'review' && (
            <span style={{ fontSize: 8, fontWeight: 700, color: '#A78BFA', background: 'rgba(167,139,250,0.15)', borderRadius: 3, padding: '1px 5px', marginLeft: 4, letterSpacing: 0.5 }}>
              REVIEW R{node.reviewCycle}
            </span>
          )}
        </div>
        <span
          className={`${styles.statusDot} ${node.status === 'active' ? styles.pulse : ''}`}
          style={{ background: color, boxShadow: `0 0 8px ${color}` }}
        />
      </div>
      <div className={styles.summary}>{node.summary}</div>
      <div className={styles.bottom}>
        <div className={styles.agentInfo}>
          <span className={styles.agentIcon} style={{ color: aColor }}>&#129302;</span>
          <span className={styles.agentName}>{node.agent}</span>
        </div>
        <div className={styles.badges}>
          <span className={styles.priBadge} style={{ color: pColor, background: `${pColor}2E` }}>
            {node.priority.toUpperCase()}
          </span>
          <span className={styles.staBadge} style={{ color, background: `${color}2E` }}>
            {node.status.toUpperCase()}
          </span>
        </div>
      </div>
    </div>
  );
}
