import styles from './CanvasLegend.module.css';

const statusItems = [
  { label: 'Active', color: '#00F5A0' },
  { label: 'Complete', color: '#3B82F6' },
  { label: 'Pending', color: '#F59E0B' },
  { label: 'Error', color: '#EF4444' },
];

const typeItems = [
  { label: 'Proposal', color: '#34D399', icon: '◆' },
  { label: 'Milestone', color: '#FBBF24', icon: '◈' },
  { label: 'Task', color: '#60A5FA', icon: '▸' },
];

export function CanvasLegend() {
  return (
    <div className={styles.legend}>
      <span className={styles.title}>NODE TYPE</span>
      {typeItems.map((item) => (
        <div key={item.label} className={styles.item}>
          <span style={{ color: item.color, fontSize: 10 }}>{item.icon}</span>
          <span className={styles.label}>{item.label}</span>
        </div>
      ))}
      <span className={styles.title} style={{ marginTop: 6 }}>STATUS</span>
      {statusItems.map((item) => (
        <div key={item.label} className={styles.item}>
          <span
            className={styles.dot}
            style={{ background: item.color, boxShadow: `0 0 5px ${item.color}` }}
          />
          <span className={styles.label}>{item.label}</span>
        </div>
      ))}
    </div>
  );
}
