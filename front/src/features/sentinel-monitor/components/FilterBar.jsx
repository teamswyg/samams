import { useSentinelUiStore } from '../store/sentinelUiStore';
import { Button } from '../../../shared/components/ui/Button';
import styles from './FilterBar.module.css';

const levels = ['all', 'info', 'warning', 'critical'];

export function FilterBar() {
  const logLevelFilter = useSentinelUiStore((s) => s.logLevelFilter);
  const setLogLevelFilter = useSentinelUiStore((s) => s.setLogLevelFilter);
  const autoRefresh = useSentinelUiStore((s) => s.autoRefresh);
  const toggleAutoRefresh = useSentinelUiStore((s) => s.toggleAutoRefresh);

  return (
    <div className={styles.bar}>
      <div className={styles.filters}>
        {levels.map((level) => (
          <Button
            key={level}
            variant={logLevelFilter === level ? 'primary' : 'secondary'}
            size="sm"
            onClick={() => setLogLevelFilter(level)}
          >
            {level}
          </Button>
        ))}
      </div>
      <Button variant="ghost" size="sm" onClick={toggleAutoRefresh}>
        Auto-refresh: {autoRefresh ? 'ON' : 'OFF'}
      </Button>
    </div>
  );
}
