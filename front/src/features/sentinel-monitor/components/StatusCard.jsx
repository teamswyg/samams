import { Badge } from '../../../shared/components/data-display/Badge';
import styles from './StatusCard.module.css';

export function StatusCard({ title, value, variant }) {
  return (
    <div className={styles.card}>
      <span className={styles.title}>{title}</span>
      <div className={styles.value}>
        <Badge label={value} variant={variant} />
      </div>
    </div>
  );
}
