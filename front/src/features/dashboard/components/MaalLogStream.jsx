import { useRef, useEffect } from 'react';
import { useDashboardStore } from '../store/dashboardStore';
import styles from './MaalLogStream.module.css';

const typeColors = {
  SUCCESS: 'var(--color-primary)',
  WARNING: 'var(--color-warning)',
  ERROR: 'var(--color-error)',
  INFO: 'var(--color-info)',
};

export function MaalLogStream() {
  const logs = useDashboardStore((s) => s.logs);
  const autoScroll = useDashboardStore((s) => s.autoScroll);
  const toggleAutoScroll = useDashboardStore((s) => s.toggleAutoScroll);
  const listRef = useRef(null);

  useEffect(() => {
    if (autoScroll && listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <span className={styles.icon}>&#9641;</span>
          <span className={styles.title}>MAAL Log Stream</span>
        </div>
        <a href="/log-viewer" className={styles.extLink}>&#8599;</a>
      </div>

      <div className={styles.list} ref={listRef}>
        {logs.map((log) => (
          <div key={log.id} className={styles.logItem}>
            <span className={styles.time}>[{log.time}]</span>
            <span className={styles.type} style={{ color: typeColors[log.type] }}>
              {log.type}
            </span>
            <span className={styles.agent}>{log.agent}</span>
            <p className={styles.message}>{log.message}</p>
          </div>
        ))}
      </div>

      <div className={styles.footer}>
        <span className={styles.count}>{logs.length} entries</span>
        <button
          className={`${styles.scrollBtn} ${autoScroll ? styles.scrollActive : ''}`}
          onClick={toggleAutoScroll}
        >
          Auto-scroll: {autoScroll ? 'ON' : 'OFF'}
        </button>
      </div>
    </div>
  );
}
