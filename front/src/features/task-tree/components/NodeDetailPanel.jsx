import { useTaskTreeStore } from '../store/taskTreeStore';
import styles from './NodeDetailPanel.module.css';

const statusColorMap = {
  active: '#00F5A0',
  complete: '#3B82F6',
  pending: '#F59E0B',
  error: '#EF4444',
};

const priorityColorMap = {
  high: '#EF4444',
  medium: '#F59E0B',
  low: '#3B82F6',
};

const typeColorMap = {
  proposal: '#34D399',
  milestone: '#FBBF24',
  task: '#60A5FA',
};

export function NodeDetailPanel() {
  const selectedNodeId = useTaskTreeStore((s) => s.selectedNodeId);
  const getSelectedNode = useTaskTreeStore((s) => s.getSelectedNode);
  const closeDetail = useTaskTreeStore((s) => s.closeDetail);

  if (!selectedNodeId) return null;
  const node = getSelectedNode();
  if (!node) return null;

  const sColor = statusColorMap[node.status] || '#00F5A0';
  const pColor = priorityColorMap[node.priority];
  const nodeType = node.type || 'task';
  const tColor = typeColorMap[nodeType] || '#00F5A0';

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <span className={styles.headerTitle}>Node Detail</span>
        <button className={styles.closeBtn} onClick={closeDetail}>&#10005;</button>
      </div>
      <div className={styles.body}>
        <div className={styles.field}>
          <span className={styles.label}>TYPE</span>
          <span className={styles.typeBadge} style={{ color: tColor, background: `${tColor}1A` }}>
            {nodeType.toUpperCase()}
          </span>
        </div>
        <div className={styles.field}>
          <span className={styles.label}>UID</span>
          <span className={styles.uidVal}>{node.uid}</span>
        </div>
        <div className={styles.field}>
          <span className={styles.label}>SUMMARY</span>
          <span className={styles.summaryVal}>{node.summary}</span>
        </div>
        <div className={styles.grid2}>
          <div className={styles.field}>
            <span className={styles.label}>AGENT</span>
            <span className={styles.val}>{node.agent}</span>
          </div>
          <div className={styles.field}>
            <span className={styles.label}>STATUS</span>
            <div className={styles.statusRow}>
              <span className={styles.sDot} style={{ background: sColor, boxShadow: `0 0 8px ${sColor}` }} />
              <span style={{ color: sColor, fontWeight: 700, fontSize: 12, textTransform: 'uppercase' }}>{node.status}</span>
            </div>
          </div>
        </div>
        {node.priority && (
          <div className={styles.field}>
            <span className={styles.label}>PRIORITY</span>
            <span className={styles.priBadge} style={{ color: pColor, background: `${pColor}33` }}>
              {node.priority.toUpperCase()}
            </span>
          </div>
        )}
        <div className={styles.field}>
          <span className={styles.label}>DEPTH LEVEL</span>
          <span className={styles.val}>Level {node.depth}</span>
        </div>
        {node.childCount > 0 && (
          <div className={styles.field}>
            <span className={styles.label}>CHILDREN</span>
            <span className={styles.val}>
              {node.childCount} {nodeType === 'proposal' ? 'milestones' : nodeType === 'milestone' ? 'tasks' : 'subtasks'}
            </span>
          </div>
        )}
      </div>
    </div>
  );
}
