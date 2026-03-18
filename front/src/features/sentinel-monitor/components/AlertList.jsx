import { Badge } from '../../../shared/components/data-display/Badge';
import { timeAgo } from '../../../shared/lib/formatDate';
import styles from './AlertList.module.css';

export function AlertList({ alerts = [], onSelect }) {
  if (alerts.length === 0) {
    return <div className={styles.empty}>No alerts</div>;
  }

  return (
    <div className={styles.list}>
      {alerts.map((alert) => (
        <div key={alert.id} className={styles.item} onClick={() => onSelect?.(alert)}>
          <div className={styles.header}>
            <Badge label={alert.severity} variant={alert.severity} />
            <span className={styles.time}>{timeAgo(alert.occurredAt)}</span>
          </div>
          <p className={styles.message}>{alert.message}</p>
          <span className={styles.event}>{alert.eventName}</span>
        </div>
      ))}
    </div>
  );
}
